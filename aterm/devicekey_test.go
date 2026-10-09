package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// attest is what a test phone's leaf says about its key. The zero value is a
// good key held in the trusted environment with a biometric bound to every use.
type attest struct {
	challenge   []byte
	level, mint asn1.Enumerated
	noAuth      bool
	noUserAuth  bool
	curve       int
	algorithm   int
	purposes    []int
	otherKey    bool
	software    bool
}

func entry(tag int, inner []byte) []byte {
	raw, _ := asn1.Marshal(asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: tag, IsCompound: true, Bytes: inner})
	return raw
}

func sequence(entries ...[]byte) asn1.RawValue {
	var all []byte
	for _, each := range entries {
		all = append(all, each...)
	}
	raw, _ := asn1.Marshal(asn1.RawValue{Class: asn1.ClassUniversal, Tag: asn1.TagSequence, IsCompound: true, Bytes: all})
	return asn1.RawValue{FullBytes: raw}
}

func integer(v int) []byte { raw, _ := asn1.Marshal(v); return raw }

// phone makes a key and the attestation chain a root, an intermediate and a leaf
// vouch for it, with the root pooled so a daemon under test trusts it.
type phone struct {
	key   *ecdsa.PrivateKey
	spki  []byte
	chain []string
	roots *x509.CertPool
}

func newPhone(t *testing.T, a attest) *phone {
	t.Helper()
	if !a.software && a.level == 0 && a.mint == 0 {
		a.level, a.mint = levelTrustedEnvironment, levelTrustedEnvironment
	}
	if a.curve == 0 {
		a.curve = curveP256
	}
	if a.algorithm == 0 {
		a.algorithm = algorithmEC
	}
	if a.purposes == nil {
		a.purposes = []int{purposeSign}
	}
	gen := func() *ecdsa.PrivateKey {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		return key
	}
	rootKey, midKey, deviceKey := gen(), gen(), gen()
	sign := func(template, parent *x509.Certificate, pub *ecdsa.PublicKey, signer *ecdsa.PrivateKey) *x509.Certificate {
		der, err := x509.CreateCertificate(rand.Reader, template, parent, pub, signer)
		if err != nil {
			t.Fatal(err)
		}
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			t.Fatal(err)
		}
		return cert
	}
	authority := func(serial int64, name string) *x509.Certificate {
		return &x509.Certificate{
			SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: name},
			NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour),
			IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
		}
	}
	root := sign(authority(1, "test root"), authority(1, "test root"), &rootKey.PublicKey, rootKey)
	mid := sign(authority(2, "test intermediate"), root, &midKey.PublicKey, rootKey)

	hardware := [][]byte{entry(tagPurpose, mustSet(a.purposes)), entry(tagAlgorithm, integer(a.algorithm)), entry(tagECCurve, integer(a.curve))}
	if !a.noUserAuth {
		hardware = append(hardware, entry(tagUserAuthType, integer(2)))
	}
	software := [][]byte{}
	if a.noAuth {
		software = append(software, entry(tagNoAuthRequired, asn1.NullBytes))
	}
	extension, err := asn1.Marshal(keyDescription{
		AttestationVersion: 300, AttestationSecurityLevel: a.level, KeyMintVersion: 300, KeyMintSecurityLevel: a.mint,
		AttestationChallenge: a.challenge, UniqueID: []byte{}, SoftwareEnforced: sequence(software...), HardwareEnforced: sequence(hardware...),
	})
	if err != nil {
		t.Fatal(err)
	}
	certified := &deviceKey.PublicKey
	if a.otherKey {
		certified = &gen().PublicKey
	}
	leafTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(3), Subject: pkix.Name{CommonName: "Android Keystore Key"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour), KeyUsage: x509.KeyUsageDigitalSignature,
		ExtraExtensions: []pkix.Extension{{Id: attestationOID, Value: extension}},
	}
	leaf := sign(leafTemplate, mid, certified, midKey)
	spki, err := x509.MarshalPKIXPublicKey(&deviceKey.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(root)
	return &phone{key: deviceKey, spki: spki, roots: pool, chain: []string{
		rawURL.EncodeToString(leaf.Raw), rawURL.EncodeToString(mid.Raw), rawURL.EncodeToString(root.Raw),
	}}
}

func mustSet(values []int) []byte {
	raw, err := asn1.MarshalWithParams(values, "set")
	if err != nil {
		panic(err)
	}
	return raw
}

// signAssertion is what the plugin does: the prefix, a zero byte, the challenge.
func (p *phone) signAssertion(t *testing.T, challenge []byte) string {
	t.Helper()
	digest := sha256.Sum256(append(append([]byte(assertPrefix), 0), challenge...))
	signature, err := ecdsa.SignASN1(rand.Reader, p.key, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return rawURL.EncodeToString(signature)
}

func TestPinnedGoogleRootsAreSelfSignedAuthoritiesInDate(t *testing.T) {
	if len(googleAttestationRoots) != 2 {
		t.Fatalf("%d roots pinned, want the two Google publishes", len(googleAttestationRoots))
	}
	pinnedRoots()
	for _, text := range googleAttestationRoots {
		block, _ := pem.Decode([]byte(text))
		if block == nil {
			t.Fatal("a pinned root is not PEM")
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil || !cert.IsCA || cert.CheckSignatureFrom(cert) != nil || !cert.NotAfter.After(time.Now().AddDate(1, 0, 0)) {
			t.Fatalf("a pinned root is not a self-signed authority with a year left: %v", err)
		}
	}
}

func TestAttestationPolicyAcceptsAGoodKeyAndRefusesEachWeakness(t *testing.T) {
	challenge := make([]byte, 32)
	_, _ = rand.Read(challenge)
	for name, test := range map[string]struct {
		attest    attest
		challenge []byte
		want      string
		level     string
	}{
		"good, trusted environment": {attest: attest{}, level: "tee"},
		"good, strongbox":           {attest: attest{level: levelStrongBox, mint: levelStrongBox}, level: "strongbox"},
		"software attestation":      {attest: attest{software: true}, want: "secure hardware"},
		"software key, tee attest":  {attest: attest{level: levelTrustedEnvironment, mint: 0}, want: "secure hardware"},
		"no user authentication":    {attest: attest{noAuth: true}, want: "does not require user authentication"},
		"auth type missing":         {attest: attest{noUserAuth: true}, want: "not bound to a biometric"},
		"wrong curve":               {attest: attest{curve: 4}, want: "EC P-256"},
		"wrong algorithm":           {attest: attest{algorithm: 1}, want: "EC P-256"},
		"cannot sign":               {attest: attest{purposes: []int{1}}, want: "cannot sign"},
		"another challenge":         {attest: attest{}, challenge: []byte("not the one that was issued....."), want: "another challenge"},
		"certifies another key":     {attest: attest{otherKey: true}, want: "different key"},
	} {
		a := test.attest
		a.challenge = challenge
		if test.challenge != nil {
			a.challenge = test.challenge
		}
		p := newPhone(t, a)
		pub, _ := parsePublicKey(p.spki)
		chain, err := parseAttestation(jsonOf(p.chain))
		if err != nil {
			t.Fatal(err)
		}
		level, err := verifyAttestation(chain, p.roots, challenge, pub, time.Now())
		switch {
		case test.want == "" && (err != nil || level != test.level):
			t.Fatalf("%s: got %q, %v, want level %q", name, level, err, test.level)
		case test.want != "" && (err == nil || !strings.Contains(err.Error(), test.want)):
			t.Fatalf("%s: got %v, want an error containing %q", name, err, test.want)
		}
	}

	// A chain to an unpinned root is refused, which is what stops a self-made one.
	p := newPhone(t, attest{challenge: challenge})
	pub, _ := parsePublicKey(p.spki)
	chain, _ := parseAttestation(jsonOf(p.chain))
	if _, err := verifyAttestation(chain, pinnedRoots(), challenge, pub, time.Now()); err == nil || !strings.Contains(err.Error(), "Google attestation root") {
		t.Fatalf("a chain to an unpinned root read as %v", err)
	}
}

func jsonOf(v any) json.RawMessage {
	raw, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return raw
}

func TestAttestationArrivesAsAnArrayOrOneCommaJoinedString(t *testing.T) {
	p := newPhone(t, attest{challenge: []byte("x")})
	for name, raw := range map[string]json.RawMessage{"array": jsonOf(p.chain), "joined": jsonOf(strings.Join(p.chain, ","))} {
		chain, err := parseAttestation(raw)
		if err != nil || len(chain) != 3 {
			t.Fatalf("%s: %d certificates, %v", name, len(chain), err)
		}
	}
	for name, raw := range map[string]json.RawMessage{"object": json.RawMessage(`{"a":1}`), "empty": json.RawMessage(`[]`), "not der": jsonOf([]string{"AAAA"}), "not base64url": jsonOf([]string{"%%%"})} {
		if _, err := parseAttestation(raw); err == nil {
			t.Fatalf("%s was accepted", name)
		}
	}
}

// deviceDaemon serves a store whose roots are the test phone's.
func deviceDaemon(t *testing.T, p *phone) (*daemon, string) {
	t.Helper()
	d, _ := wsDaemon(t)
	path := filepath.Join(t.TempDir(), "passkeys.json")
	var err error
	if d.passkeys, err = newPasskeyStore(path, passkeyRPID, []string{passkeyOrigin}); err != nil {
		t.Fatal(err)
	}
	d.passkeys.roots = p.roots
	return d, path
}

// enroll runs the whole enrollment on device and returns the key id.
func enroll(t *testing.T, d *daemon, device *conn, a attest) (*phone, string) {
	t.Helper()
	code := mintCode(t, d)
	begin, err := device.request(frame{Type: "device_enroll_begin", EnrollCode: code})
	if err != nil || begin.Type != "device_enroll_challenge" {
		t.Fatalf("enroll begin: %+v, %v", begin, err)
	}
	challenge, err := rawURL.DecodeString(begin.Challenge)
	if err != nil || len(challenge) != 32 {
		t.Fatalf("the challenge is %d bytes, %v, want 32", len(challenge), err)
	}
	a.challenge = challenge
	p := newPhone(t, a)
	d.passkeys.roots = p.roots
	done, err := device.request(frame{Type: "device_enroll_finish", PublicKey: rawURL.EncodeToString(p.spki), Attestation: jsonOf(p.chain)})
	if err != nil {
		t.Fatalf("enroll finish: %v", err)
	}
	sum := sha256.Sum256(p.spki)
	if done.Type != "device_enrolled" || done.KeyID != rawURL.EncodeToString(sum[:]) || len(done.KeyID) != 43 {
		t.Fatalf("enrolled as %+v", done)
	}
	return p, done.KeyID
}

func TestDeviceKeyEnrollsAssertsAndUnlocksTypingOnlyOnAssertion(t *testing.T) {
	d, path := deviceDaemon(t, newPhone(t, attest{}))
	device := pipeClient(t, d, peerStanding{web: true, remote: true})
	welcome := nextFrame(t, device, "welcome")
	if !slices.Contains(welcome.Features, deviceKeyFeature) || welcome.Typing == nil || welcome.Typing.DeviceKey != "unenrolled" || welcome.Typing.Allowed {
		t.Fatalf("a fresh remote device: features %v typing %+v", welcome.Features, welcome.Typing)
	}
	p, keyID := enroll(t, d, device, attest{})
	if standing := nextFrame(t, device, "typing").Typing; standing.DeviceKey != "enrolled" || standing.Allowed {
		t.Fatalf("enrolling must report the key and unlock nothing: %+v", standing)
	}

	begin, err := device.request(frame{Type: "device_assert_begin", KeyID: keyID})
	if err != nil || begin.Type != "device_assert_challenge" {
		t.Fatalf("assert begin: %+v, %v", begin, err)
	}
	challenge, _ := rawURL.DecodeString(begin.Challenge)
	if _, err = device.request(frame{Type: "device_assert_finish", KeyID: keyID, Signature: p.signAssertion(t, challenge)}); err != nil {
		t.Fatalf("assert finish: %v", err)
	}
	if standing := nextFrame(t, device, "typing").Typing; !standing.Allowed || standing.DeviceKey != "enrolled" {
		t.Fatalf("an assertion must unlock typing: %+v", standing)
	}

	// The key survives a restart, and a revoke forgets it.
	reloaded, err := newPasskeyStore(path, passkeyRPID, []string{passkeyOrigin})
	if err != nil || !reloaded.deviceKeyEnrolled() {
		t.Fatalf("the device key did not survive a reload: %v", err)
	}
	terminal := pipeClient(t, d, peerStanding{pids: []int{os.Getpid()}})
	nextFrame(t, terminal, "welcome")
	if reply, err := terminal.request(frame{Type: "passkey_revoke"}); err != nil || reply.Type != "passkey_revoked" {
		t.Fatalf("revoke: %+v, %v", reply, err)
	}
	if d.passkeys.deviceKeyEnrolled() {
		t.Fatal("revoke left a device key")
	}
	again := pipeClient(t, d, peerStanding{web: true, remote: true})
	nextFrame(t, again, "welcome")
	reply, err := again.request(frame{Type: "device_assert_begin", KeyID: keyID})
	if err == nil || reply.Reason != reasonDeviceKeyGone {
		t.Fatalf("a revoked key read as %+v, %v, want %s", reply, err, reasonDeviceKeyGone)
	}
}

func TestDeviceKeyChallengesAreSingleUseBoundAndExpire(t *testing.T) {
	d, _ := deviceDaemon(t, newPhone(t, attest{}))
	device := pipeClient(t, d, peerStanding{web: true, remote: true})
	nextFrame(t, device, "welcome")
	p, keyID := enroll(t, d, device, attest{})

	assertWith := func(sign func(challenge []byte) string) error {
		begin, err := device.request(frame{Type: "device_assert_begin", KeyID: keyID})
		if err != nil {
			t.Fatal(err)
		}
		challenge, _ := rawURL.DecodeString(begin.Challenge)
		_, err = device.request(frame{Type: "device_assert_finish", KeyID: keyID, Signature: sign(challenge)})
		return err
	}
	// A wrong finish spends the challenge, so a right signature for it afterwards fails.
	begin, _ := device.request(frame{Type: "device_assert_begin", KeyID: keyID})
	challenge, _ := rawURL.DecodeString(begin.Challenge)
	if _, err := device.request(frame{Type: "device_assert_finish", KeyID: keyID, Signature: "AAAA"}); err == nil {
		t.Fatal("a wrong signature was accepted")
	}
	if _, err := device.request(frame{Type: "device_assert_finish", KeyID: keyID, Signature: p.signAssertion(t, challenge)}); err == nil {
		t.Fatal("a spent challenge was accepted a second time")
	}
	// A signature over the challenge without the aterm prefix is no assertion.
	if err := assertWith(func(c []byte) string {
		digest := sha256.Sum256(c)
		signature, _ := ecdsa.SignASN1(rand.Reader, p.key, digest[:])
		return rawURL.EncodeToString(signature)
	}); err == nil {
		t.Fatal("a signature without the prefix was accepted")
	}
	// A key signing for another key id is refused.
	other := pipeClient(t, d, peerStanding{web: true, remote: true})
	nextFrame(t, other, "welcome")
	if _, err := other.request(frame{Type: "device_assert_finish", KeyID: keyID, Signature: "AAAA"}); err == nil {
		t.Fatal("a finish with no challenge open was accepted")
	}
	// An expired challenge is refused.
	begin, _ = device.request(frame{Type: "device_assert_begin", KeyID: keyID})
	challenge, _ = rawURL.DecodeString(begin.Challenge)
	later := time.Now().Add(deviceChallengeTTL + time.Second)
	d.passkeys.now = func() time.Time { return later }
	if _, err := device.request(frame{Type: "device_assert_finish", KeyID: keyID, Signature: p.signAssertion(t, challenge)}); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("an expired challenge read as %v", err)
	}
	d.passkeys.now = time.Now
	if err := assertWith(p.signAssertion2(t)); err != nil {
		t.Fatalf("a good assertion after all that failed: %v", err)
	}
}

func (p *phone) signAssertion2(t *testing.T) func([]byte) string {
	return func(c []byte) string { return p.signAssertion(t, c) }
}

func TestEnrollmentNeedsTheTerminalCodeAndAGoodAttestation(t *testing.T) {
	d, _ := deviceDaemon(t, newPhone(t, attest{}))
	device := pipeClient(t, d, peerStanding{web: true, remote: true})
	nextFrame(t, device, "welcome")
	if _, err := device.request(frame{Type: "device_enroll_begin", EnrollCode: "abcd-efgh"}); err == nil {
		t.Fatal("enrollment began without a minted code")
	}
	// A software-only attestation is refused and the key is not stored.
	code := mintCode(t, d)
	begin, err := device.request(frame{Type: "device_enroll_begin", EnrollCode: code})
	if err != nil {
		t.Fatal(err)
	}
	challenge, _ := rawURL.DecodeString(begin.Challenge)
	soft := newPhone(t, attest{challenge: challenge, software: true})
	d.passkeys.roots = soft.roots
	if _, err = device.request(frame{Type: "device_enroll_finish", PublicKey: rawURL.EncodeToString(soft.spki), Attestation: jsonOf(soft.chain)}); err == nil || !strings.Contains(err.Error(), "secure hardware") {
		t.Fatalf("a software attestation read as %v", err)
	}
	if d.passkeys.deviceKeyEnrolled() {
		t.Fatal("a refused enrollment stored a key")
	}
	// The code was spent by the begin, so it cannot start a second enrollment.
	if _, err = device.request(frame{Type: "device_enroll_begin", EnrollCode: code}); err == nil {
		t.Fatal("a spent code started another enrollment")
	}
	// A frame from a terminal, which is not a web connection, is refused.
	terminal := pipeClient(t, d, peerStanding{pids: []int{os.Getpid()}})
	nextFrame(t, terminal, "welcome")
	if _, err = terminal.request(frame{Type: "device_enroll_begin", EnrollCode: mintCode(t, d)}); err == nil {
		t.Fatal("a terminal enrolled a device key")
	}
}

func TestTheAppOriginMayOpenASocketBesideTheConfiguredPages(t *testing.T) {
	base, client, _ := tailnetServer(t, func(string) (tailnetPeer, error) { return peerWith("kai@example.com"), nil })
	address := "wss" + strings.TrimPrefix(base, "https") + "/"
	dial := func(origin string) (*websocket.Conn, *http.Response, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return websocket.Dial(ctx, address, &websocket.DialOptions{HTTPClient: client, HTTPHeader: http.Header{"Origin": {origin}}})
	}
	for _, origin := range []string{appOrigin, "https://coilyco.dev"} {
		ws, _, err := dial(origin)
		if err != nil {
			t.Fatalf("origin %q must open a socket: %v", origin, err)
		}
		_ = ws.CloseNow()
	}
	for _, origin := range []string{"http://tauri.localhost", "https://tauri.localhost.evil.example", "https://evil.tauri.localhost", "https://localhost"} {
		if ws, _, err := dial(origin); err == nil {
			_ = ws.CloseNow()
			t.Fatalf("origin %q must be refused", origin)
		}
	}
}
