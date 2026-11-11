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
	// codes are the enrollment codes it honors, renewFail the renewals it fails first,
	// renewals the ones it granted, and wrongKey makes it bind a certificate to another key.
	codes     map[string]bool
	renewFail int
	renewals  int
	wrongKey  bool
	// provisionSecret is the secret the claim endpoint honors. provisionRefuse is how many
	// claims it answers 401 first (an owner not signed up), and claims is every key it saw.
	provisionSecret string
	provisionRefuse int
	claims          []string
	boundKey        string
}

func newFakeAccounts(t *testing.T) *fakeAccounts {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	a := &fakeAccounts{key: key, kid: "k1", codes: map[string]bool{}}
	a.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		down := a.down
		a.mu.Unlock()
		switch r.URL.Path {
		case "/v1/daemons":
			a.handleEnroll(w, r)
			return
		case "/v1/daemons/renew":
			a.handleRenew(w, r)
			return
		case "/v1/daemons/provision":
			a.handleProvision(w, r)
			return
		}
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

func decodeStrict(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func (a *fakeAccounts) handleEnroll(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code      string                          `json:"code"`
		DaemonPub struct{ Kty, Crv, X, Y string } `json:"daemon_pub"`
		Name      string                          `json:"name"`
		Substrate string                          `json:"substrate"`
	}
	if err := decodeStrict(r, &req); err != nil || req.DaemonPub.Kty != "EC" || req.Name == "" {
		http.Error(w, `{"error":"bad_request"}`, http.StatusBadRequest)
		return
	}
	a.mu.Lock()
	ok := a.codes[req.Code]
	delete(a.codes, req.Code)
	wrong := a.wrongKey
	a.mu.Unlock()
	if !ok {
		http.Error(w, `{"error":"code_invalid","message":"that code is not valid"}`, http.StatusUnauthorized)
		return
	}
	pub, err := parseP256(req.DaemonPub.X, req.DaemonPub.Y)
	if err != nil {
		http.Error(w, `{"error":"bad_request"}`, http.StatusBadRequest)
		return
	}
	jkt := jwkThumbprint(pub)
	if wrong {
		jkt = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	}
	_ = json.NewEncoder(w).Encode(certReply{DaemonID: testDaemonID, Cert: a.daemonCert(jkt, time.Hour), ExpiresIn: 3600})
}

// handleProvision is accounts' claim endpoint. A known secret binds to the first key that
// presents it, the same key retries to the same daemon, and another key is 403.
func (a *fakeAccounts) handleProvision(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Secret    string                          `json:"secret"`
		DaemonPub struct{ Kty, Crv, X, Y string } `json:"daemon_pub"`
		Name      string                          `json:"name"`
		Substrate string                          `json:"substrate"`
	}
	if err := decodeStrict(r, &req); err != nil || req.DaemonPub.Kty != "EC" || req.Name == "" || req.Substrate == "" {
		http.Error(w, `{"error":"bad_request"}`, http.StatusBadRequest)
		return
	}
	pub, err := parseP256(req.DaemonPub.X, req.DaemonPub.Y)
	if err != nil {
		http.Error(w, `{"error":"bad_request"}`, http.StatusBadRequest)
		return
	}
	jkt := jwkThumbprint(pub)
	a.mu.Lock()
	a.claims = append(a.claims, jkt)
	refuse := a.provisionRefuse > 0
	if refuse {
		a.provisionRefuse--
	}
	known := a.provisionSecret != "" && req.Secret == a.provisionSecret
	conflict := known && a.boundKey != "" && a.boundKey != jkt
	if known && !refuse && !conflict {
		a.boundKey = jkt
	}
	a.mu.Unlock()
	switch {
	case !known || refuse:
		http.Error(w, `{"error":"unauthorized","message":"the enrollment secret is not accepted"}`, http.StatusUnauthorized)
	case conflict:
		http.Error(w, `{"error":"forbidden","message":"another key"}`, http.StatusForbidden)
	default:
		_ = json.NewEncoder(w).Encode(certReply{DaemonID: testDaemonID, Cert: a.daemonCert(jkt, time.Hour), ExpiresIn: 3600})
	}
}

// handleRenew checks the proof as accounts does: signed by the key whose thumbprint the
// certificate names, aimed at accounts, fresh, with a jti.
func (a *fakeAccounts) handleRenew(w http.ResponseWriter, r *http.Request) {
	var req struct{ Cert, Proof string }
	if err := decodeStrict(r, &req); err != nil {
		http.Error(w, `{"error":"bad_request"}`, http.StatusBadRequest)
		return
	}
	a.mu.Lock()
	if a.renewFail > 0 {
		a.renewFail--
		a.mu.Unlock()
		http.Error(w, `{"error":"stale"}`, http.StatusServiceUnavailable)
		return
	}
	a.mu.Unlock()
	certParts, proofParts := strings.Split(req.Cert, "."), strings.Split(req.Proof, ".")
	if len(certParts) != 3 || len(proofParts) != 3 {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	var cert struct{ Cnf struct{ Jkt string } }
	raw, _ := rawURL.DecodeString(certParts[1])
	_ = json.Unmarshal(raw, &cert)
	var head struct{ JWK struct{ X, Y string } }
	raw, _ = rawURL.DecodeString(proofParts[0])
	_ = json.Unmarshal(raw, &head)
	pub, err := parseP256(head.JWK.X, head.JWK.Y)
	sig, _ := rawURL.DecodeString(proofParts[2])
	digest := sha256.Sum256([]byte(proofParts[0] + "." + proofParts[1]))
	var claims struct {
		Aud, Jti string
		Iat      int64
	}
	raw, _ = rawURL.DecodeString(proofParts[1])
	_ = json.Unmarshal(raw, &claims)
	if err != nil || jwkThumbprint(pub) != cert.Cnf.Jkt || len(sig) != 64 ||
		!ecdsa.Verify(pub, digest[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])) ||
		claims.Aud != "treehouse-accounts" || claims.Jti == "" || time.Since(time.Unix(claims.Iat, 0)) > time.Minute {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	a.mu.Lock()
	a.renewals++
	a.mu.Unlock()
	_ = json.NewEncoder(w).Encode(certReply{DaemonID: testDaemonID, Cert: a.daemonCert(cert.Cnf.Jkt, time.Hour), ExpiresIn: 3600})
}

func (a *fakeAccounts) setDown(down bool) {
	a.mu.Lock()
	a.down = down
	a.mu.Unlock()
}

func signRaw(key *ecdsa.PrivateKey, header, claims map[string]any) string {
	h, _ := json.Marshal(header)
	c, _ := json.Marshal(claims)
	signing := rawURL.EncodeToString(h) + "." + rawURL.EncodeToString(c)
	digest := sha256.Sum256([]byte(signing))
	r, s, err := ecdsa.Sign(rand.Reader, key, digest[:])
	if err != nil {
		panic(err)
	}
	return signing + "." + rawURL.EncodeToString(append(r.FillBytes(make([]byte, 32)), s.FillBytes(make([]byte, 32))...))
}

func signWith(t *testing.T, key *ecdsa.PrivateKey, header, claims map[string]any) string {
	t.Helper()
	return signRaw(key, header, claims)
}

// daemonCert is a daemon certificate accounts would issue, bound to jkt, living for life.
func (a *fakeAccounts) daemonCert(jkt string, life time.Duration) string {
	now := time.Now()
	return signRaw(a.key, map[string]any{"alg": "ES256", "typ": "treehouse.token.daemon.v1", "kid": a.kid}, map[string]any{
		"iss": a.server.URL, "use": "daemon", "sub": testDaemonID, "aud": "treehouse-broker", "acct": testOwner,
		"cnf": map[string]string{"jkt": jkt}, "name": "test", "substrate": "other",
		"iat": now.Add(-time.Minute).Unix(), "exp": now.Add(life).Unix(), "jti": fmt.Sprintf("cert-%d", now.UnixNano()),
	})
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
	events   chan brokerMsg
	certs    chan brokerMsg
	// rawEvents are the bytes of each reg.event, for a test that asserts the exact payload.
	rawEvents chan []byte
}

func newFakeBroker(t *testing.T, a *fakeAccounts, identity daemonIdentity) *fakeBroker {
	b := &fakeBroker{t: t, accounts: a, identity: identity, up: make(chan *websocket.Conn, 4), inbox: map[string]chan brokerMsg{}, rejected: make(chan string, 8),
		events: make(chan brokerMsg, 16), certs: make(chan brokerMsg, 16), rawEvents: make(chan []byte, 16)}
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
	if b.write(ws, brokerMsg{Type: "treehouse.reg.ready.v1", DaemonID: b.identity.ID, HeartbeatS: 5, CertExpires: time.Now().Add(time.Hour).Unix()}) != nil {
		return
	}
	b.up <- ws
	for {
		_, data, err := ws.Read(ctx)
		if err != nil {
			return
		}
		var m brokerMsg
		if json.Unmarshal(data, &m) != nil {
			continue
		}
		switch {
		case m.Channel != "":
			b.channel(m.Channel) <- m
		case m.Type == "treehouse.reg.event.v1":
			b.events <- m
			b.rawEvents <- append([]byte(nil), data...)
		case m.Type == "treehouse.reg.cert.v1":
			b.certs <- m
			_ = b.write(ws, brokerMsg{Type: "treehouse.reg.ready.v1", DaemonID: b.identity.ID, HeartbeatS: 5, CertExpires: time.Now().Add(time.Hour).Unix()})
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
	dir      string
	ctx      context.Context
	// configDir is the snapshot directory the daemon reads, empty until a test publishes.
	configDir string
}

const (
	testOwner    = "acct_01HZX3K9Q7M2"
	testDaemonID = "dmn_01HZX3KA4B8N"
)

func newBrokeredRig(t *testing.T) *brokeredRig { return newBrokeredRigLife(t, time.Hour) }

// newBrokeredRigLife starts a rig whose certificate lives for life, lapsed when negative.
func newBrokeredRigLife(t *testing.T, life time.Duration) *brokeredRig {
	t.Helper()
	r := prepareBrokeredRig(t, life)
	r.start()
	return r
}

// start dials the broker. A test sets up its fakes between prepare and start.
func (r *brokeredRig) start() {
	r.d.startTreehouse(r.ctx, treehouseOptions{Broker: r.b.address(), Accounts: r.a.server.URL, Dir: r.dir, ConfigDir: r.configDir})
}

func prepareBrokeredRig(t *testing.T, life time.Duration) *brokeredRig {
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
	cert := a.daemonCert(jwkThumbprint(&key.PublicKey), life)
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
	return &brokeredRig{d: d, b: b, a: a, identity: identity, logs: logs, cancel: cancel, dir: dir, ctx: ctx, configDir: t.TempDir()}
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

func fileMode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode().Perm()
}

func TestEnrollRedeemsACodeChecksTheCertificateAndWritesPrivateFiles(t *testing.T) {
	a := newFakeAccounts(t)
	a.codes["ABCD-1234"] = true
	dir := filepath.Join(t.TempDir(), "treehouse")
	identity, err := enrollDaemon(context.Background(), enrollOptions{Accounts: a.server.URL, Dir: dir, Code: " abcd-1234 ", Name: "kai-mac", Substrate: "mac"})
	if err != nil || identity.ID != testDaemonID || identity.Owner != testOwner {
		t.Fatalf("enroll: %+v, %v", identity, err)
	}
	key, err := loadDaemonKey(filepath.Join(dir, "daemon.key"))
	if err != nil {
		t.Fatalf("the key is private and loads: %v", err)
	}
	cert, _ := os.ReadFile(filepath.Join(dir, "daemon.cert"))
	if !certBindsKey(strings.TrimSpace(string(cert)), key) || fileMode(t, filepath.Join(dir, "daemon.cert")) != 0o600 || fileMode(t, dir) != 0o700 {
		t.Fatal("the certificate binds the key, and both files are private")
	}
	if _, err := enrollDaemon(context.Background(), enrollOptions{Accounts: a.server.URL, Dir: dir, Code: "ABCD-1234"}); err == nil || !strings.Contains(err.Error(), "--replace") {
		t.Fatalf("an enrolled directory is not overwritten by accident: %v", err)
	}
}

func TestEnrollKeepsNothingWhenTheCodeOrTheCertificateIsWrong(t *testing.T) {
	for name, setup := range map[string]func(a *fakeAccounts){
		"a code accounts does not know": func(a *fakeAccounts) {},
		"a certificate for another key": func(a *fakeAccounts) { a.codes["ABCD-1234"], a.wrongKey = true, true },
	} {
		t.Run(name, func(t *testing.T) {
			a := newFakeAccounts(t)
			setup(a)
			dir := filepath.Join(t.TempDir(), "treehouse")
			if _, err := enrollDaemon(context.Background(), enrollOptions{Accounts: a.server.URL, Dir: dir, Code: "ABCD-1234"}); err == nil {
				t.Fatal("enrollment should fail")
			}
			if entries, _ := os.ReadDir(dir); len(entries) != 0 {
				t.Fatalf("a failed enrollment left %d files behind", len(entries))
			}
		})
	}
	if _, err := enrollDaemon(context.Background(), enrollOptions{Accounts: "http://example.com", Dir: t.TempDir(), Code: "ABCD-1234"}); err == nil {
		t.Fatal("accounts must be https")
	}
	if _, err := enrollDaemon(context.Background(), enrollOptions{Accounts: "https://accounts.example", Dir: t.TempDir(), Code: "short"}); err == nil {
		t.Fatal("a malformed code is refused before any request")
	}
}

func TestALapsedCertificateIsRenewedBeforeTheDialAndTheFileIsReplaced(t *testing.T) {
	r := prepareBrokeredRig(t, -time.Hour)
	before, _ := os.ReadFile(filepath.Join(r.dir, "daemon.cert"))
	r.start()
	r.b.attach(t, "c1", testOwner)
	r.a.mu.Lock()
	renewals := r.a.renewals
	r.a.mu.Unlock()
	after, _ := os.ReadFile(filepath.Join(r.dir, "daemon.cert"))
	if renewals != 1 || bytes.Equal(before, after) || fileMode(t, filepath.Join(r.dir, "daemon.cert")) != 0o600 {
		t.Fatalf("a lapsed certificate is renewed once before dialing: %d renewals, replaced %v", renewals, !bytes.Equal(before, after))
	}
	if !certExpiry(strings.TrimSpace(string(after))).After(time.Now().Add(50 * time.Minute)) {
		t.Fatal("the new certificate lives about an hour")
	}
}

func TestACertificateNearExpiryIsRenewedInsideTheLinkAndTheBrokerIsTold(t *testing.T) {
	prevRetry := renewRetry
	renewRetry = 20 * time.Millisecond
	t.Cleanup(func() { renewRetry = prevRetry })
	r := prepareBrokeredRig(t, 5*time.Minute)
	r.a.renewFail = 1 // the renewal before the dial fails, the one inside the link succeeds
	r.start()
	select {
	case m := <-r.b.certs:
		if m.Cert == "" || certExpiry(m.Cert).Before(time.Now().Add(50*time.Minute)) {
			t.Fatalf("the broker is sent the renewed certificate: %+v", m)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the broker never heard of a renewed certificate")
	}
	if !strings.Contains(r.logs.String(), "could not renew the Treehouse daemon certificate") {
		t.Fatalf("the failed renewal is logged:\n%s", r.logs.String())
	}
}

func TestAnAskReachesTheBrokerWithoutItsQuestion(t *testing.T) {
	notifyRoster(t, "scientist")
	r := newBrokeredRig(t)
	r.b.attach(t, "c1", testOwner)
	// An event before the link is registered is dropped by design, so ask until it lands.
	deadline := time.After(10 * time.Second)
	for {
		r.d.users.NotifyAsk(choiceAsk{Question: "SECRET-QUESTION-TEXT", Session: "scientist-evie", From: "scientist Evie"})
		select {
		case m := <-r.b.events:
			raw, _ := json.Marshal(m)
			if m.Type != "treehouse.reg.event.v1" || m.Kind != "ask" || m.SessionName != "scientist-evie" || m.Identity != "Evie" || strings.Contains(string(raw), "SECRET-QUESTION-TEXT") {
				t.Fatalf("an ask event names the seat and not the question: %s", raw)
			}
			return
		case <-time.After(100 * time.Millisecond):
		case <-deadline:
			t.Fatal("the broker never heard of the ask")
		}
	}
}

func TestWaitingSeatsBecomeBrokerEventsOnTheTrackersTerms(t *testing.T) {
	notifyRoster(t, "scientist")
	var mu sync.Mutex
	state := stateBusy
	events := make(chan brokerMsg, 4)
	d := newDaemon(func(string, ...any) {})
	link := &brokerLink{d: d, settle: 10 * time.Millisecond, sendEvent: func(m brokerMsg) { events <- m }, views: func() []sessionView {
		mu.Lock()
		defer mu.Unlock()
		return []sessionView{{Name: "scientist-evie", Role: "scientist", Identity: "Evie", State: state}}
	}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go link.watchSeats(ctx, 5*time.Millisecond)
	time.Sleep(30 * time.Millisecond)
	mu.Lock()
	state = stateIdle
	mu.Unlock()
	select {
	case m := <-events:
		if m.Kind != "done" || m.SessionName != "scientist-evie" || m.Role != "scientist" || m.Identity != "Evie" {
			t.Fatalf("a finished turn is a done event: %+v", m)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no event for a seat that finished")
	}
}

// provisionRig is a daemon with no certificate, a secret file and fast retries.
func provisionRig(t *testing.T, secret string) (*brokeredRig, string) {
	t.Helper()
	r := prepareBrokeredRig(t, time.Hour)
	for _, f := range []string{"daemon.key", "daemon.cert"} {
		if err := os.Remove(filepath.Join(r.dir, f)); err != nil {
			t.Fatal(err)
		}
	}
	r.a.provisionSecret = secret
	prev := provisionBackoff
	provisionBackoff = struct{ first, max time.Duration }{20 * time.Millisecond, 60 * time.Millisecond}
	t.Cleanup(func() { provisionBackoff = prev })
	file := filepath.Join(t.TempDir(), "secret")
	return r, file
}

func (r *brokeredRig) startProvisioned(file string) {
	r.d.startTreehouse(r.ctx, treehouseOptions{Broker: r.b.address(), Accounts: r.a.server.URL, Dir: r.dir, ConfigDir: r.configDir,
		ProvisionSecretFile: file, Name: "kai-server", Substrate: "other"})
}

func (r *brokeredRig) haveCert() bool {
	_, err := os.Stat(filepath.Join(r.dir, "daemon.cert"))
	return err == nil
}

func TestProvisionedDaemonClaimsItsCertificateWithNoCodeThenRegistersWithTheBroker(t *testing.T) {
	const secret = "q3VnT1uXk0yq8mJf2Zb9Wc7LrE4aHdS6pNtG5oYvIiM"
	r, file := provisionRig(t, secret)
	if err := os.WriteFile(file, []byte(secret+"\n"), 0o440); err != nil {
		t.Fatal(err)
	}
	r.startProvisioned(file)
	r.b.attach(t, "c1", testOwner) // fails unless the daemon claimed, then dialed and registered
	key, err := loadDaemonKey(filepath.Join(r.dir, "daemon.key"))
	if err != nil {
		t.Fatalf("the key is private and loads: %v", err)
	}
	cert, _ := os.ReadFile(filepath.Join(r.dir, "daemon.cert"))
	if !certBindsKey(strings.TrimSpace(string(cert)), key) || fileMode(t, filepath.Join(r.dir, "daemon.cert")) != 0o600 {
		t.Fatal("the certificate binds the key and is private")
	}
	if strings.Contains(r.logs.String(), secret) {
		t.Fatalf("the secret reached the log: %s", r.logs.String())
	}
}

func TestProvisionedClaimKeepsAskingWithOneKeyUntilAccountsAccepts(t *testing.T) {
	const secret = "q3VnT1uXk0yq8mJf2Zb9Wc7LrE4aHdS6pNtG5oYvIiM"
	r, file := provisionRig(t, secret)
	r.a.provisionRefuse = 3 // the owner has not signed up yet
	// The secret store delivers the file after the daemon starts.
	r.startProvisioned(file)
	time.Sleep(100 * time.Millisecond)
	if r.haveCert() || len(r.a.claims) != 0 {
		t.Fatal("with no secret file there is nothing to claim with")
	}
	if err := os.WriteFile(file, []byte(secret), 0o440); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the certificate after three refusals", 5*time.Second, r.haveCert)
	r.a.mu.Lock()
	claims := append([]string(nil), r.a.claims...)
	r.a.mu.Unlock()
	if len(claims) != 4 {
		t.Fatalf("expected three refusals and one success, saw %d claims", len(claims))
	}
	for _, jkt := range claims {
		if jkt != claims[0] {
			t.Fatalf("a retry used another key: %v", claims)
		}
	}
	if strings.Contains(r.logs.String(), secret) {
		t.Fatal("the secret reached the log")
	}
}

func TestADaemonWithACertificateNeverClaimsAgain(t *testing.T) {
	r := prepareBrokeredRig(t, time.Hour)
	file := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(file, []byte("q3VnT1uXk0yq8mJf2Zb9Wc7LrE4aHdS6pNtG5oYvIiM"), 0o440); err != nil {
		t.Fatal(err)
	}
	r.a.provisionSecret = "q3VnT1uXk0yq8mJf2Zb9Wc7LrE4aHdS6pNtG5oYvIiM"
	r.startProvisioned(file)
	r.b.attach(t, "c1", testOwner)
	if len(r.a.claims) != 0 {
		t.Fatalf("an enrolled daemon claimed %d times", len(r.a.claims))
	}
}
