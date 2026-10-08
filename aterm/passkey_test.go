package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fxamacker/cbor/v2"
)

// softKey is a software authenticator: ES256, no attestation, a counter it advances.
type softKey struct {
	t       *testing.T
	key     *ecdsa.PrivateKey
	id      []byte
	counter uint32
	// flags decide user presence and verification, so a test can omit the latter.
	verified bool
	origin   string
}

func newSoftKey(t *testing.T) *softKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	id := make([]byte, 16)
	_, _ = rand.Read(id)
	return &softKey{t: t, key: key, id: id, verified: true, origin: passkeyOrigin}
}

var b64 = base64.RawURLEncoding

// challengeOf reads the challenge out of the options JSON a daemon sent.
func challengeOf(t *testing.T, options json.RawMessage) string {
	t.Helper()
	var parsed struct {
		PublicKey struct {
			Challenge string `json:"challenge"`
		} `json:"publicKey"`
	}
	if err := json.Unmarshal(options, &parsed); err != nil || parsed.PublicKey.Challenge == "" {
		t.Fatalf("options carry no challenge: %v %s", err, options)
	}
	return parsed.PublicKey.Challenge
}

func (k *softKey) flags(attested bool) byte {
	flags := byte(0x01)
	if k.verified {
		flags |= 0x04
	}
	if attested {
		flags |= 0x40
	}
	return flags
}

func (k *softKey) clientData(kind, challenge string) []byte {
	raw, _ := json.Marshal(map[string]any{"type": kind, "challenge": challenge, "origin": k.origin})
	return raw
}

func (k *softKey) rpHash() []byte {
	sum := sha256.Sum256([]byte(passkeyRPID))
	return sum[:]
}

// register answers a creation options with a credential the daemon should accept.
func (k *softKey) register(options json.RawMessage) json.RawMessage {
	x, y := k.key.PublicKey.X.FillBytes(make([]byte, 32)), k.key.PublicKey.Y.FillBytes(make([]byte, 32))
	publicKey, err := cbor.Marshal(map[int]any{1: 2, 3: -7, -1: 1, -2: x, -3: y})
	if err != nil {
		k.t.Fatal(err)
	}
	authData := append(k.rpHash(), k.flags(true))
	authData = binary.BigEndian.AppendUint32(authData, k.counter)
	authData = append(authData, make([]byte, 16)...)
	authData = binary.BigEndian.AppendUint16(authData, uint16(len(k.id)))
	authData = append(append(authData, k.id...), publicKey...)
	attestation, err := cbor.Marshal(map[string]any{"fmt": "none", "attStmt": map[string]any{}, "authData": authData})
	if err != nil {
		k.t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{
		"id": b64.EncodeToString(k.id), "rawId": b64.EncodeToString(k.id), "type": "public-key",
		"response": map[string]any{
			"clientDataJSON":    b64.EncodeToString(k.clientData("webauthn.create", challengeOf(k.t, options))),
			"attestationObject": b64.EncodeToString(attestation),
		},
	})
	return raw
}

// assert signs the daemon's challenge, advancing the counter first.
func (k *softKey) assert(options json.RawMessage) json.RawMessage {
	k.counter++
	authData := append(k.rpHash(), k.flags(false))
	authData = binary.BigEndian.AppendUint32(authData, k.counter)
	client := k.clientData("webauthn.get", challengeOf(k.t, options))
	clientHash := sha256.Sum256(client)
	digest := sha256.Sum256(append(append([]byte{}, authData...), clientHash[:]...))
	signature, err := ecdsa.SignASN1(rand.Reader, k.key, digest[:])
	if err != nil {
		k.t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{
		"id": b64.EncodeToString(k.id), "rawId": b64.EncodeToString(k.id), "type": "public-key",
		"response": map[string]any{
			"clientDataJSON":    b64.EncodeToString(client),
			"authenticatorData": b64.EncodeToString(authData),
			"signature":         b64.EncodeToString(signature),
		},
	})
	return raw
}

func TestEnrollmentCodeIsSingleUseExpiresAndBurnsOnGuesses(t *testing.T) {
	store, err := newPasskeyStore("", passkeyRPID, []string{passkeyOrigin})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	store.now = func() time.Time { return now }
	code, _, _ := store.mintCode()
	if err := store.spendCode(code[:4] + "-" + strings.ToUpper(code[4:])); err != nil {
		t.Fatalf("the code with a dash and any case should spend once: %v", err)
	}
	if store.spendCode(code) == nil {
		t.Fatal("a spent code must not spend twice")
	}
	code, _, _ = store.mintCode()
	for range enrollAttempts {
		if store.spendCode("aaaaaaaa") == nil {
			t.Fatal("a wrong guess must fail")
		}
	}
	if store.spendCode(code) == nil {
		t.Fatalf("%d wrong guesses must burn the code", enrollAttempts)
	}
	code, _, _ = store.mintCode()
	now = now.Add(enrollCodeTTL + time.Second)
	if store.spendCode(code) == nil {
		t.Fatal("an expired code must not spend")
	}
}

func TestPasskeyEnrollsAssertsAndSurvivesAReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "passkeys.json")
	store, err := newPasskeyStore(path, passkeyRPID, []string{passkeyOrigin})
	if err != nil {
		t.Fatal(err)
	}
	key := newSoftKey(t)
	creation, session, err := store.beginEnroll()
	if err != nil {
		t.Fatal(err)
	}
	options, _ := json.Marshal(creation)
	if err := store.finishEnroll(*session, key.register(options)); err != nil {
		t.Fatalf("a valid enrollment: %v", err)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("credentials should be on disk, 0600: %v %v", info, err)
	}
	reloaded, err := newPasskeyStore(path, passkeyRPID, []string{passkeyOrigin})
	if err != nil || !reloaded.enrolled() {
		t.Fatalf("a reload should keep the passkey: %v", err)
	}
	for name, mutate := range map[string]func(*softKey){
		"user verification missing": func(k *softKey) { k.verified = false },
		"another origin":            func(k *softKey) { k.origin = "https://evil.example" },
		"the tailnet name page":     func(k *softKey) { k.origin = "https://kais-mac.tail1234.ts.net:7419" },
	} {
		bad := *key
		mutate(&bad)
		assertion, session, err := reloaded.beginAssert()
		if err != nil {
			t.Fatal(err)
		}
		options, _ := json.Marshal(assertion)
		if reloaded.finishAssert(*session, bad.assert(options)) == nil {
			t.Fatalf("%s must be refused", name)
		}
	}
	assertion, session, _ := reloaded.beginAssert()
	options, _ = json.Marshal(assertion)
	if err := reloaded.finishAssert(*session, key.assert(options)); err != nil {
		t.Fatalf("a valid assertion with user verification: %v", err)
	}
	if err := reloaded.revoke(); err != nil || reloaded.enrolled() {
		t.Fatalf("revoke should forget every passkey: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("revoke should remove the file: %v", err)
	}
}

// pipeClient serves one connection with the peer standing given. It uses a
// loopback socket, since a pipe has no buffer and a session's output would block.
func pipeClient(t *testing.T, d *daemon, peer peerStanding) *conn {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	client, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	server, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	go d.serveConn(newConn(server), peer)
	c := newConn(client)
	t.Cleanup(func() { _ = c.Close() })
	if err := c.write(frame{Type: "hello", Format: daemonFormat}); err != nil {
		t.Fatalf("hello: %v", err)
	}
	return c
}

func mintCode(t *testing.T, d *daemon) string {
	t.Helper()
	terminal := pipeClient(t, d, peerStanding{pids: []int{os.Getpid()}})
	nextFrame(t, terminal, "welcome")
	reply, err := terminal.request(frame{Type: "passkey_mint"})
	if err != nil || reply.Type != "passkey_minted" || reply.ExpiresIn != int(enrollCodeTTL.Seconds()) {
		t.Fatalf("a terminal outside every session should mint: %+v, %v", reply, err)
	}
	return reply.EnrollCode
}

func TestRemoteDeviceTypesOnlyAfterAPasskeyAssertion(t *testing.T) {
	d, _ := wsDaemon(t)
	d.passkeys, _ = newPasskeyStore(filepath.Join(t.TempDir(), "passkeys.json"), passkeyRPID, []string{passkeyOrigin})
	wsSession(t, d, false)
	remote := peerStanding{web: true, remote: true}
	key := newSoftKey(t)

	device := pipeClient(t, d, remote)
	welcome := nextFrame(t, device, "welcome")
	if welcome.Typing == nil || welcome.Typing.Allowed || welcome.Typing.Reason != reasonPasskeyRequired || welcome.Typing.Passkey != "unenrolled" {
		t.Fatalf("a remote device starts locked and unenrolled: %+v", welcome.Typing)
	}
	attach := frame{Type: "attach", Session: "scientist-evie", Rows: 24, Cols: 80}
	if _, err := device.request(attach); err != nil {
		t.Fatal(err)
	}
	for _, refused := range []frame{
		{Type: "input", Session: "scientist-evie", Data: []byte("x")},
		{Type: "launch", Role: "scientist"},
		{Type: "clear", Target: "scientist-evie"},
		{Type: "answer", AskID: "none", Picks: []int{0}},
	} {
		reply, err := device.request(refused)
		if err == nil || reply.Reason != reasonPasskeyRequired {
			t.Fatalf("%s should be refused as %s: %+v, %v", refused.Type, reasonPasskeyRequired, reply, err)
		}
	}
	if _, err := device.request(frame{Type: "passkey_assert_begin"}); err == nil {
		t.Fatal("asserting with nothing enrolled must fail")
	}
	if reply, err := device.request(frame{Type: "passkey_mint"}); err == nil || reply.Type == "passkey_minted" {
		t.Fatal("a websocket must not mint an enrollment code")
	}
	if _, err := device.request(frame{Type: "passkey_enroll_begin", EnrollCode: "aaaa-aaaa"}); err == nil {
		t.Fatal("a wrong code must not begin an enrollment")
	}

	// Enrollment: the code from a terminal, then a verified registration.
	code := mintCode(t, d)
	begin, err := device.request(frame{Type: "passkey_enroll_begin", EnrollCode: code})
	if err != nil || begin.Type != "passkey_enroll_options" {
		t.Fatalf("enroll begin: %+v, %v", begin, err)
	}
	if _, err := device.request(frame{Type: "passkey_enroll_begin", EnrollCode: code}); err == nil {
		t.Fatal("the code is single use")
	}
	done, err := device.request(frame{Type: "passkey_enroll_finish", Credential: key.register(begin.Options)})
	if err != nil || done.Type != "passkey_enrolled" {
		t.Fatalf("enroll finish: %+v, %v", done, err)
	}
	if standing := nextFrame(t, device, "typing"); standing.Typing == nil || !standing.Typing.Allowed {
		t.Fatalf("enrolling should unlock the connection: %+v", standing.Typing)
	}
	// A successful input has no reply, so the screen below is the evidence.
	if err := device.write(frame{Type: "input", Session: "scientist-evie", Data: []byte("typed-after-enroll\r")}); err != nil {
		t.Fatalf("an unlocked device types: %v", err)
	}

	// A new connection must assert again, and a bad signature leaves it locked.
	second := pipeClient(t, d, remote)
	if welcome := nextFrame(t, second, "welcome"); welcome.Typing.Passkey != "enrolled" || welcome.Typing.Allowed {
		t.Fatalf("a new connection is locked even when a passkey is enrolled: %+v", welcome.Typing)
	}
	_, _ = second.request(attach)
	options, err := second.request(frame{Type: "passkey_assert_begin"})
	if err != nil {
		t.Fatal(err)
	}
	forged := newSoftKey(t)
	forged.id = key.id
	if _, err := second.request(frame{Type: "passkey_assert_finish", Credential: forged.assert(options.Options)}); err == nil {
		t.Fatal("a signature from another key must be refused")
	}
	if reply, err := second.request(frame{Type: "input", Session: "scientist-evie", Data: []byte("x")}); err == nil || reply.Reason != reasonPasskeyRequired {
		t.Fatalf("a failed assertion leaves the connection locked: %+v, %v", reply, err)
	}
	if _, err := second.request(frame{Type: "passkey_assert_finish", Credential: key.assert(options.Options)}); err == nil {
		t.Fatal("a challenge is spent by the first finish, right or wrong")
	}
	options, _ = second.request(frame{Type: "passkey_assert_begin"})
	if reply, err := second.request(frame{Type: "passkey_assert_finish", Credential: key.assert(options.Options)}); err != nil || reply.Type != "passkey_asserted" {
		t.Fatalf("a valid assertion: %+v, %v", reply, err)
	}
	if standing := nextFrame(t, second, "typing"); !standing.Typing.Allowed {
		t.Fatalf("asserting should unlock the connection: %+v", standing.Typing)
	}
	if err := second.write(frame{Type: "input", Session: "scientist-evie", Data: []byte("typed-after-assert\r")}); err != nil {
		t.Fatalf("an asserted device types: %v", err)
	}
	var screen string
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		screen = strings.Join(d.session("scientist-evie").status(24).Screen, "\n")
		if strings.Contains(screen, "typed-after-enroll") && strings.Contains(screen, "typed-after-assert") {
			return
		}
	}
	t.Fatalf("both unlocked inputs should reach the session: %q", screen)
}

func TestMintAndRevokeRefuseFromInsideASession(t *testing.T) {
	d, _ := wsDaemon(t)
	wsSession(t, d, true)
	agent := pipeClient(t, d, peerStanding{pids: []int{os.Getpid()}})
	nextFrame(t, agent, "welcome")
	for _, kind := range []string{"passkey_mint", "passkey_revoke"} {
		reply, err := agent.request(frame{Type: kind})
		if err == nil || reply.Reason != reasonSessionDescendant {
			t.Fatalf("%s from a session's process should be refused: %+v, %v", kind, reply, err)
		}
	}
}
