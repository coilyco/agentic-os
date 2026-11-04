package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// fakeAccounts signs tokens the way the contract says, with its own minimal ES256, and
// serves the key set. It shares nothing with the verifier it tests.
type fakeAccounts struct {
	key    *ecdsa.PrivateKey
	kid    string
	server *httptest.Server
	down   bool
	mu     sync.Mutex
}

func newFakeAccounts(t *testing.T) *fakeAccounts {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	a := &fakeAccounts{key: key, kid: "k1"}
	a.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		down := a.down
		a.mu.Unlock()
		if down || r.URL.Path != "/.well-known/jwks.json" {
			http.Error(w, "down", http.StatusServiceUnavailable)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kty": "EC", "crv": "P-256", "kid": a.kid, "alg": "ES256", "use": "sig",
			"x": rawURL.EncodeToString(key.X.FillBytes(make([]byte, 32))), "y": rawURL.EncodeToString(key.Y.FillBytes(make([]byte, 32))),
		}}})
	}))
	t.Cleanup(a.server.Close)
	return a
}

func (a *fakeAccounts) setDown(down bool) {
	a.mu.Lock()
	a.down = down
	a.mu.Unlock()
}

func signWith(t *testing.T, key *ecdsa.PrivateKey, header, claims map[string]any) string {
	t.Helper()
	h, _ := json.Marshal(header)
	c, _ := json.Marshal(claims)
	signing := rawURL.EncodeToString(h) + "." + rawURL.EncodeToString(c)
	digest := sha256.Sum256([]byte(signing))
	r, s, err := ecdsa.Sign(rand.Reader, key, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return signing + "." + rawURL.EncodeToString(append(r.FillBytes(make([]byte, 32)), s.FillBytes(make([]byte, 32))...))
}

func (a *fakeAccounts) mint(t *testing.T, typ string, claims map[string]any) string {
	t.Helper()
	return signWith(t, a.key, map[string]any{"alg": "ES256", "typ": typ, "kid": a.kid}, claims)
}

var jtiCounter int

func (a *fakeAccounts) unlockClaims(daemonID, owner, nonce string) map[string]any {
	jtiCounter++
	now := time.Now().Unix()
	return map[string]any{
		"iss": a.server.URL, "use": "unlock", "sub": owner, "aud": daemonID, "nonce": nonce, "how": "passkey", "acr": "uv",
		"jti": fmt.Sprintf("jti-%08d", jtiCounter), "iat": now, "exp": now + 120,
	}
}

// fakeBroker is the other end of a daemon's link, as the contract describes it.
type fakeBroker struct {
	t        *testing.T
	accounts *fakeAccounts
	identity daemonIdentity
	server   *httptest.Server
	up       chan *websocket.Conn
	mu       sync.Mutex
	inbox    map[string]chan brokerMsg
	rejected chan string
	writeMu  sync.Mutex
}

func newFakeBroker(t *testing.T, a *fakeAccounts, identity daemonIdentity) *fakeBroker {
	b := &fakeBroker{t: t, accounts: a, identity: identity, up: make(chan *websocket.Conn, 4), inbox: map[string]chan brokerMsg{}, rejected: make(chan string, 8)}
	b.server = httptest.NewServer(http.HandlerFunc(b.handle))
	t.Cleanup(b.server.Close)
	return b
}

func (b *fakeBroker) address() string { return "ws" + strings.TrimPrefix(b.server.URL, "http") }

func (b *fakeBroker) channel(id string) chan brokerMsg {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.inbox[id] == nil {
		b.inbox[id] = make(chan brokerMsg, 64)
	}
	return b.inbox[id]
}

func (b *fakeBroker) write(ws *websocket.Conn, m brokerMsg) error {
	raw, _ := json.Marshal(m)
	b.writeMu.Lock()
	defer b.writeMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return ws.Write(ctx, websocket.MessageText, raw)
}

func (b *fakeBroker) reject(ws *websocket.Conn, why string) {
	b.rejected <- why
	_ = ws.Close(websocket.StatusCode(4403), why)
}

// handle runs the broker's side of registration: challenge, then a proof it checks the
// way the contract says, then ready.
func (b *fakeBroker) handle(w http.ResponseWriter, r *http.Request) {
	ws, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	ctx := r.Context()
	nonce := rawURL.EncodeToString(bytes.Repeat([]byte{7}, 16))
	if b.write(ws, brokerMsg{Type: "treehouse.reg.challenge.v1", Nonce: nonce}) != nil {
		return
	}
	_, data, err := ws.Read(ctx)
	if err != nil {
		return
	}
	var hello brokerMsg
	if json.Unmarshal(data, &hello) != nil || hello.Type != "treehouse.reg.hello.v1" || hello.Protocol != daemonFormat {
		b.reject(ws, "bad hello")
		return
	}
	if why := b.checkProof(hello, nonce); why != "" {
		b.reject(ws, why)
		return
	}
	if b.write(ws, brokerMsg{Type: "treehouse.reg.ready.v1", DaemonID: b.identity.ID, HeartbeatS: 5}) != nil {
		return
	}
	b.up <- ws
	for {
		_, data, err := ws.Read(ctx)
		if err != nil {
			return
		}
		var m brokerMsg
		if json.Unmarshal(data, &m) == nil && m.Channel != "" {
			b.channel(m.Channel) <- m
		}
	}
}

// checkProof is the broker's reading of reg.hello: the key comes from the proof's
// header, the signature must verify, and aud, nonce and iat must hold.
func (b *fakeBroker) checkProof(hello brokerMsg, nonce string) string {
	parts := strings.Split(hello.Proof, ".")
	if len(parts) != 3 {
		return "proof is not compact"
	}
	headRaw, _ := rawURL.DecodeString(parts[0])
	var head struct {
		Alg, Typ string
		JWK      struct{ X, Y string }
	}
	if json.Unmarshal(headRaw, &head) != nil || head.Alg != "ES256" || head.Typ != proofTokenType {
		return "proof header"
	}
	pub, err := parseP256(head.JWK.X, head.JWK.Y)
	if err != nil {
		return "proof key"
	}
	sig, _ := rawURL.DecodeString(parts[2])
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if len(sig) != 64 || !ecdsa.Verify(pub, digest[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])) {
		return "proof signature"
	}
	claimsRaw, _ := rawURL.DecodeString(parts[1])
	var claims struct {
		Aud, Nonce, Jti string
		Iat             int64
	}
	if json.Unmarshal(claimsRaw, &claims) != nil || claims.Aud != "treehouse-broker" || claims.Nonce != nonce || claims.Jti == "" {
		return "proof claims"
	}
	if d := time.Since(time.Unix(claims.Iat, 0)); d > time.Minute || d < -time.Minute {
		return "proof iat"
	}
	if hello.Cert == "" {
		return "no cert"
	}
	return ""
}

// chanClient is a client the fake broker attached.
type chanClient struct {
	t  *testing.T
	b  *fakeBroker
	ws *websocket.Conn
	id string
}

// attach opens a channel for account once the daemon has registered.
func (b *fakeBroker) attach(t *testing.T, id, account string) *chanClient {
	t.Helper()
	var ws *websocket.Conn
	select {
	case ws = <-b.up:
	case <-time.After(10 * time.Second):
		t.Fatal("the daemon never registered with the broker")
	}
	b.up <- ws
	if err := b.write(ws, brokerMsg{Type: "treehouse.chan.open.v1", Channel: id, Account: account, SID: "sess_12345678"}); err != nil {
		t.Fatal(err)
	}
	return &chanClient{t: t, b: b, ws: ws, id: id}
}

// close ends the channel from the broker's side.
func (c *chanClient) close() {
	_ = c.b.write(c.ws, brokerMsg{Type: "treehouse.chan.close.v1", Channel: c.id, Reason: "client_closed"})
}

func (c *chanClient) send(f frame) {
	c.t.Helper()
	raw, _ := json.Marshal(f)
	if err := c.b.write(c.ws, brokerMsg{Type: "treehouse.chan.frame.v1", Channel: c.id, Frame: raw}); err != nil {
		c.t.Fatal(err)
	}
}

// read returns the next message for this channel, frames decoded.
func (c *chanClient) read() (brokerMsg, frame, error) {
	select {
	case m := <-c.b.channel(c.id):
		var f frame
		if m.Type == "treehouse.chan.frame.v1" {
			_ = json.Unmarshal(m.Frame, &f)
		}
		return m, f, nil
	case <-time.After(5 * time.Second):
		return brokerMsg{}, frame{}, errors.New("timed out")
	}
}

func (c *chanClient) next(want string) frame {
	c.t.Helper()
	for {
		m, f, err := c.read()
		if err != nil {
			c.t.Fatalf("waiting for %s: %v", want, err)
		}
		if m.Type == "treehouse.chan.close.v1" {
			c.t.Fatalf("waiting for %s: the channel closed (%s)", want, m.Reason)
		}
		if f.Type == want {
			return f
		}
	}
}

// request sends f and returns the reply with its id, and an error for an error frame.
func (c *chanClient) request(f frame) (frame, error) {
	c.t.Helper()
	f.ID = fmt.Sprintf("r%d", time.Now().UnixNano())
	c.send(f)
	for {
		_, got, err := c.read()
		if err != nil {
			c.t.Fatalf("waiting for the reply to %s: %v", f.Type, err)
		}
		if got.ID == f.ID {
			if got.Type == "error" {
				return got, errors.New(got.Error)
			}
			return got, nil
		}
	}
}

// brokeredRig is a daemon with a seat, dialed to a fake broker, with accounts keys.
type brokeredRig struct {
	d        *daemon
	b        *fakeBroker
	a        *fakeAccounts
	identity daemonIdentity
	logs     *lockedBuffer
	cancel   context.CancelFunc
}

const (
	testOwner    = "acct_01HZX3K9Q7M2"
	testDaemonID = "dmn_01HZX3KA4B8N"
)

func newBrokeredRig(t *testing.T) *brokeredRig {
	t.Helper()
	a := newFakeAccounts(t)
	identity := daemonIdentity{ID: testDaemonID, Owner: testOwner}
	b := newFakeBroker(t, a, identity)
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "daemon.key"), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	cert := a.mint(t, "treehouse.token.daemon.v1", map[string]any{
		"iss": a.server.URL, "use": "daemon", "sub": testDaemonID, "aud": "treehouse-broker", "acct": testOwner,
		"cnf": map[string]string{"jkt": "x"}, "name": "test", "substrate": "other", "iat": now, "exp": now + 3600, "jti": "cert-0001",
	})
	if err := os.WriteFile(filepath.Join(dir, "daemon.cert"), []byte(cert+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	logs := &lockedBuffer{}
	d := newDaemon(func(format string, args ...any) { fmt.Fprintf(logs, format+"\n", args...) })
	d.holdDir = testHoldDir(t)
	wsSession(t, d, false)
	prev := brokerBackoff
	brokerBackoff = struct{ first, max time.Duration }{20 * time.Millisecond, 100 * time.Millisecond}
	t.Cleanup(func() { brokerBackoff = prev })
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	d.startTreehouse(ctx, treehouseOptions{Broker: b.address(), Accounts: a.server.URL, Dir: dir})
	return &brokeredRig{d: d, b: b, a: a, identity: identity, logs: logs, cancel: cancel}
}

// openLocked attaches a channel, reads its welcome and returns both.
func (r *brokeredRig) openLocked(t *testing.T, id string) (*chanClient, frame) {
	t.Helper()
	c := r.b.attach(t, id, testOwner)
	t.Cleanup(c.close)
	c.send(frame{Type: "hello", Format: daemonFormat})
	return c, c.next("welcome")
}

// typed reports whether input on c reaches the seat's screen.
func (r *brokeredRig) typed(t *testing.T, c *chanClient, marker string) bool {
	t.Helper()
	if _, err := c.request(frame{Type: "attach", Session: "scientist-evie", Rows: 24, Cols: 80}); err != nil {
		t.Fatalf("attach: %v", err)
	}
	c.send(frame{Type: "input", Session: "scientist-evie", Data: []byte(marker + "\r")})
	return screenHolds(r.d, marker)
}

// refusedInput asserts a locked channel's typing frames get passkey_required.
func refusedInput(t *testing.T, c *chanClient) {
	t.Helper()
	if _, err := c.request(frame{Type: "attach", Session: "scientist-evie", Rows: 24, Cols: 80}); err != nil {
		t.Fatalf("attach: %v", err)
	}
	for _, f := range []frame{{Type: "input", Session: "scientist-evie", Data: []byte("x")}, {Type: "launch", Role: "scientist"}} {
		reply, err := c.request(f)
		if err == nil || reply.Reason != reasonPasskeyRequired {
			t.Fatalf("%s should be refused as %s: %+v, %v", f.Type, reasonPasskeyRequired, reply, err)
		}
	}
}

func TestBrokeredChannelStartsLockedAndTypesOnlyAfterAnAccountsToken(t *testing.T) {
	r := newBrokeredRig(t)
	c, welcome := r.openLocked(t, "c1")
	if welcome.Unlock == nil || welcome.Unlock.DaemonID != testDaemonID || len(welcome.Unlock.Nonce) < 22 {
		t.Fatalf("a brokered welcome offers a nonce: %+v", welcome.Unlock)
	}
	if !contains(welcome.Features, unlockTokenFeature) {
		t.Fatalf("a brokered welcome names %s: %v", unlockTokenFeature, welcome.Features)
	}
	if welcome.Typing == nil || welcome.Typing.Allowed || welcome.Typing.Reason != reasonPasskeyRequired || welcome.Typing.Passkey != "" || welcome.Typing.DeviceKey != "" {
		t.Fatalf("a brokered channel starts locked and knows no enrollment: %+v", welcome.Typing)
	}
	refusedInput(t, c)

	token := r.a.mint(t, unlockTokenType, r.a.unlockClaims(testDaemonID, testOwner, welcome.Unlock.Nonce))
	reply, err := c.request(frame{Type: "unlock", UnlockToken: token})
	if err != nil || reply.Type != "typing" || reply.Typing == nil || !reply.Typing.Allowed {
		t.Fatalf("a valid token unlocks the channel: %+v, %v", reply, err)
	}
	if !r.typed(t, c, "typed-after-token") {
		t.Fatal("an unlocked channel's input never reached the seat")
	}
	if again, err := c.request(frame{Type: "unlock", UnlockToken: token}); err == nil {
		t.Fatalf("an unlocked channel takes no second unlock: %+v", again)
	}
}

func TestBrokeredUnlockRefusesEveryTokenThatIsNotForThisDaemonOwnerChannelAndMoment(t *testing.T) {
	r := newBrokeredRig(t)
	other, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		make func(nonce string) string
	}{
		{"another daemon", func(n string) string {
			c := r.a.unlockClaims("dmn_OTHEROTHEROTHER", testOwner, n)
			return r.a.mint(t, unlockTokenType, c)
		}},
		{"another account", func(n string) string {
			return r.a.mint(t, unlockTokenType, r.a.unlockClaims(testDaemonID, "acct_SOMEONEELSE00", n))
		}},
		{"another channel's nonce", func(string) string {
			return r.a.mint(t, unlockTokenType, r.a.unlockClaims(testDaemonID, testOwner, rawURL.EncodeToString(bytes.Repeat([]byte{9}, 24))))
		}},
		{"expired", func(n string) string {
			c := r.a.unlockClaims(testDaemonID, testOwner, n)
			c["iat"], c["exp"] = time.Now().Unix()-300, time.Now().Unix()-60
			return r.a.mint(t, unlockTokenType, c)
		}},
		{"issued in the future", func(n string) string {
			c := r.a.unlockClaims(testDaemonID, testOwner, n)
			c["iat"], c["exp"] = time.Now().Unix()+600, time.Now().Unix()+700
			return r.a.mint(t, unlockTokenType, c)
		}},
		{"lives past two minutes", func(n string) string {
			c := r.a.unlockClaims(testDaemonID, testOwner, n)
			c["exp"] = time.Now().Unix() + 3600
			return r.a.mint(t, unlockTokenType, c)
		}},
		{"no user verification", func(n string) string {
			c := r.a.unlockClaims(testDaemonID, testOwner, n)
			c["acr"] = "basic"
			return r.a.mint(t, unlockTokenType, c)
		}},
		{"an access token", func(n string) string {
			c := r.a.unlockClaims(testDaemonID, testOwner, n)
			c["use"] = "access"
			return r.a.mint(t, "treehouse.token.access.v1", c)
		}},
		{"an unlock claim under the wrong typ", func(n string) string {
			return r.a.mint(t, "treehouse.token.access.v1", r.a.unlockClaims(testDaemonID, testOwner, n))
		}},
		{"another issuer", func(n string) string {
			c := r.a.unlockClaims(testDaemonID, testOwner, n)
			c["iss"] = "https://accounts.evil.example"
			return r.a.mint(t, unlockTokenType, c)
		}},
		{"a key accounts never published", func(n string) string {
			return signWith(t, other, map[string]any{"alg": "ES256", "typ": unlockTokenType, "kid": r.a.kid}, r.a.unlockClaims(testDaemonID, testOwner, n))
		}},
		{"an unknown kid", func(n string) string {
			return signWith(t, r.a.key, map[string]any{"alg": "ES256", "typ": unlockTokenType, "kid": "nope"}, r.a.unlockClaims(testDaemonID, testOwner, n))
		}},
		{"alg none", func(n string) string {
			h, _ := json.Marshal(map[string]any{"alg": "none", "typ": unlockTokenType, "kid": r.a.kid})
			c, _ := json.Marshal(r.a.unlockClaims(testDaemonID, testOwner, n))
			return rawURL.EncodeToString(h) + "." + rawURL.EncodeToString(c) + "."
		}},
		{"a tampered claim", func(n string) string {
			good := r.a.mint(t, unlockTokenType, r.a.unlockClaims(testDaemonID, "acct_SOMEONEELSE00", n))
			parts := strings.Split(good, ".")
			claims := r.a.unlockClaims(testDaemonID, testOwner, n)
			c, _ := json.Marshal(claims)
			return parts[0] + "." + rawURL.EncodeToString(c) + "." + parts[2]
		}},
		{"not a token", func(string) string { return "garbage" }},
		{"empty", func(string) string { return "" }},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, welcome := r.openLocked(t, fmt.Sprintf("refuse%d", i))
			reply, err := c.request(frame{Type: "unlock", UnlockToken: tc.make(welcome.Unlock.Nonce)})
			if err == nil || reply.Reason != reasonUnlockRefused {
				t.Fatalf("the token should be refused as %s: %+v, %v", reasonUnlockRefused, reply, err)
			}
			refusedInput(t, c)
		})
	}
}

func TestBrokeredUnlockTokenIsSingleUseAcrossChannels(t *testing.T) {
	r := newBrokeredRig(t)
	first, w1 := r.openLocked(t, "a")
	token := r.a.mint(t, unlockTokenType, r.a.unlockClaims(testDaemonID, testOwner, w1.Unlock.Nonce))
	if _, err := first.request(frame{Type: "unlock", UnlockToken: token}); err != nil {
		t.Fatalf("the first use unlocks: %v", err)
	}
	second, w2 := r.openLocked(t, "b")
	if w1.Unlock.Nonce == w2.Unlock.Nonce {
		t.Fatal("two channels were offered one nonce")
	}
	// A broker replaying the captured token on another channel fails on the nonce, and
	// even a token rewritten for the new nonce under the same jti fails as spent.
	if reply, err := second.request(frame{Type: "unlock", UnlockToken: token}); err == nil || reply.Reason != reasonUnlockRefused {
		t.Fatalf("a replayed token should be refused: %+v, %v", reply, err)
	}
	claims := r.a.unlockClaims(testDaemonID, testOwner, w2.Unlock.Nonce)
	spent := r.a.mint(t, unlockTokenType, claims)
	if _, err := second.request(frame{Type: "unlock", UnlockToken: spent}); err != nil {
		t.Fatalf("a fresh token unlocks the second channel: %v", err)
	}
	third, w3 := r.openLocked(t, "c")
	claims["nonce"] = w3.Unlock.Nonce
	if reply, err := third.request(frame{Type: "unlock", UnlockToken: r.a.mint(t, unlockTokenType, claims)}); err == nil || reply.Reason != reasonUnlockRefused {
		t.Fatalf("a spent jti should be refused on a new nonce: %+v, %v", reply, err)
	}
	refusedInput(t, third)
}

func TestBrokeredChannelEndsItsNonceAfterFiveRefusals(t *testing.T) {
	r := newBrokeredRig(t)
	c, welcome := r.openLocked(t, "c1")
	for i := 0; i < unlockFailLimit; i++ {
		if _, err := c.request(frame{Type: "unlock", UnlockToken: "garbage"}); err == nil {
			t.Fatal("garbage unlocked a channel")
		}
	}
	good := r.a.mint(t, unlockTokenType, r.a.unlockClaims(testDaemonID, testOwner, welcome.Unlock.Nonce))
	if reply, err := c.request(frame{Type: "unlock", UnlockToken: good}); err == nil || reply.Reason != reasonUnlockRefused {
		t.Fatalf("a channel past its refusals stays locked: %+v, %v", reply, err)
	}
	refusedInput(t, c)
}

func TestBrokeredChannelRefusesPasskeyFramesAndAnotherAccount(t *testing.T) {
	r := newBrokeredRig(t)
	c, _ := r.openLocked(t, "c1")
	for _, typ := range []string{"passkey_assert_begin", "device_assert_begin", "resume", "push_subscribe", "passkey_mint"} {
		if reply, err := c.request(frame{Type: typ}); err == nil {
			t.Fatalf("%s should not work on a brokered channel: %+v", typ, reply)
		}
	}
	stranger := r.b.attach(t, "s1", "acct_SOMEONEELSE00")
	m, _, err := stranger.read()
	if err != nil || m.Type != "treehouse.chan.close.v1" || m.Reason != "unauthorized" {
		t.Fatalf("a channel for another account is turned away: %+v, %v", m, err)
	}
}

func TestUnlockFrameIsRefusedOnAConnectionTheBrokerDidNotOpen(t *testing.T) {
	r := newBrokeredRig(t)
	// A phone on the local path cannot use the accounts path, whatever token it holds.
	device := pipeClient(t, r.d, peerStanding{web: true, remote: true})
	nextFrame(t, device, "welcome")
	token := r.a.mint(t, unlockTokenType, r.a.unlockClaims(testDaemonID, testOwner, "anything-at-all-anything"))
	if reply, err := device.request(frame{Type: "unlock", UnlockToken: token}); err == nil {
		t.Fatalf("a local connection must not unlock with a token: %+v", reply)
	}
	stillLocked(t, device)
}

func TestBrokeredUnlockTokenNeverReachesTheLog(t *testing.T) {
	r := newBrokeredRig(t)
	c, welcome := r.openLocked(t, "c1")
	bad := r.a.mint(t, unlockTokenType, r.a.unlockClaims("dmn_OTHEROTHEROTHER", testOwner, welcome.Unlock.Nonce))
	_, _ = c.request(frame{Type: "unlock", UnlockToken: bad})
	good := r.a.mint(t, unlockTokenType, r.a.unlockClaims(testDaemonID, testOwner, welcome.Unlock.Nonce))
	if _, err := c.request(frame{Type: "unlock", UnlockToken: good}); err != nil {
		t.Fatal(err)
	}
	log := r.logs.String()
	for _, secret := range []string{bad, good, strings.Split(good, ".")[1]} {
		if strings.Contains(log, secret) {
			t.Fatalf("a token reached the log:\n%s", log)
		}
	}
	if !strings.Contains(log, "a brokered unlock was refused") || !strings.Contains(log, "unlocked with an accounts token") {
		t.Fatalf("the log should say what happened:\n%s", log)
	}
}

func TestAccountsKeysFailClosedAndServeStaleOnlyInsideADay(t *testing.T) {
	a := newFakeAccounts(t)
	keys := newAccountsKeys(a.server.URL+"/.well-known/jwks.json", fetchJWKS)
	clock := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	keys.now = func() time.Time { return clock }
	ctx := context.Background()
	if _, err := keys.get(ctx, a.kid); err != nil {
		t.Fatalf("a reachable key set serves: %v", err)
	}
	a.setDown(true)
	clock = clock.Add(2 * time.Hour)
	if _, err := keys.get(ctx, a.kid); err != nil {
		t.Fatalf("a stale key set serves inside a day: %v", err)
	}
	clock = clock.Add(23 * time.Hour)
	if _, err := keys.get(ctx, a.kid); err == nil {
		t.Fatal("a key set older than a day must not serve")
	}
	empty := newAccountsKeys(a.server.URL+"/.well-known/jwks.json", fetchJWKS)
	if _, err := empty.get(ctx, a.kid); err == nil {
		t.Fatal("no key set at all fails closed")
	}
}

func TestBrokerLinkRedialsAfterItDropsAndEndsItsChannels(t *testing.T) {
	r := newBrokeredRig(t)
	c, _ := r.openLocked(t, "c1")
	ws := c.ws
	_ = ws.Close(websocket.StatusGoingAway, "restart")
	// The old link's slot in `up` is consumed by attach, so a redial puts a new one there.
	deadline := time.After(10 * time.Second)
	for {
		select {
		case next := <-r.b.up:
			if next != ws {
				r.b.up <- next
				again := r.b.attach(t, "c2", testOwner)
				again.send(frame{Type: "hello", Format: daemonFormat})
				if welcome := again.next("welcome"); welcome.Unlock == nil {
					t.Fatal("a channel after a redial is still brokered")
				}
				return
			}
		case <-deadline:
			t.Fatal("the daemon never redialed")
		}
	}
}

func TestBrokerLinkRefusesATooManyChannelsAndAConfigThatIsNotWhole(t *testing.T) {
	r := newBrokeredRig(t)
	for i := 0; i < brokerMaxChan; i++ {
		c, _ := r.openLocked(t, fmt.Sprintf("m%d", i))
		_ = c
	}
	extra := r.b.attach(t, "extra", testOwner)
	m, _, err := extra.read()
	if err != nil || m.Type != "treehouse.chan.close.v1" || m.Reason != "daemon_closed" {
		t.Fatalf("a ninth channel is turned away: %+v, %v", m, err)
	}

	half := newDaemon(func(string, ...any) {})
	half.startTreehouse(context.Background(), treehouseOptions{Broker: "wss://broker.example"})
	if half.users.(*localUsers).brokered != nil {
		t.Fatal("a half config must leave the brokered path off")
	}
}
