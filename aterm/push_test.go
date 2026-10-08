package main

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func unb64(t *testing.T, s string) []byte {
	t.Helper()
	raw, err := decodeBase64URL(s)
	if err != nil {
		t.Fatalf("decode %q: %v", s, err)
	}
	return raw
}

// RFC 8291 section 5 and appendix A are the one answer key for the payload format.
func TestEncryptPushMatchesTheRFC8291Example(t *testing.T) {
	uaPrivate, err := ecdh.P256().NewPrivateKey(unb64(t, "q1dXpw3UpT5VOmu_cf_v6ih07Aems3njxI-JWgLcM94"))
	if err != nil {
		t.Fatal(err)
	}
	asPrivate, err := ecdh.P256().NewPrivateKey(unb64(t, "yfWPiYE-n46HLnH0KqZOF1fJJU3MYrct3AELtAQ-oRw"))
	if err != nil {
		t.Fatal(err)
	}
	to := &pushRecipient{public: uaPrivate.PublicKey(), auth: unb64(t, "BTBZMqHH6r4Tts7J_aSIgg")}
	body, err := encryptPush(to, []byte("When I grow up, I want to be a watermelon"), unb64(t, "DGv6ra1nlYgDCS1FRnbzlw"), asPrivate)
	if err != nil {
		t.Fatal(err)
	}
	want := "DGv6ra1nlYgDCS1FRnbzlwAAEABBBP4z9KsN6nGRTbVYI_c7VJSPQTBtkgcy27ml" +
		"mlMoZIIgDll6e3vCYLocInmYWAmS6TlzAC8wEqKK6PBru3jl7A_yl95bQpu6cVPT" +
		"pK4Mqgkf1CXztLVBSt2Ks3oZwbuwXPXLWyouBWLVWGNWQexSgSxsj_Qulcy4a-fN"
	if got := base64.RawURLEncoding.EncodeToString(body); got != want {
		t.Fatalf("the body differs from RFC 8291 section 5:\n got %s\nwant %s", got, want)
	}
}

func TestEncryptPushRefusesAPayloadOneRecordCannotHold(t *testing.T) {
	ua, _ := ecdh.P256().GenerateKey(rand.Reader)
	as, _ := ecdh.P256().GenerateKey(rand.Reader)
	to := &pushRecipient{public: ua.PublicKey(), auth: make([]byte, 16)}
	if _, err := encryptPush(to, make([]byte, pushMaxPlain+1), make([]byte, 16), as); err == nil {
		t.Fatal("a payload over one record should be refused, since a push service drops the message")
	}
	if _, err := encryptPush(to, make([]byte, pushMaxPlain), make([]byte, 16), as); err != nil {
		t.Fatalf("a payload that fits should encrypt: %v", err)
	}
}

func TestVAPIDAuthorizationVerifiesAgainstThePublicKeyItAdvertises(t *testing.T) {
	key, encoded, err := newVAPIDKey()
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := parseVAPIDKey(encoded)
	if err != nil || reloaded.publicKey() != key.publicKey() {
		t.Fatalf("a key should survive its own encoding: %v", err)
	}
	endpoint, _ := url.Parse("https://fcm.googleapis.com/fcm/send/abc")
	now := time.Unix(1_800_000_000, 0)
	header, err := reloaded.authorization(endpoint, now)
	if err != nil {
		t.Fatal(err)
	}
	token, advertised, ok := strings.Cut(strings.TrimPrefix(header, "vapid t="), ", k=")
	if !ok || advertised != key.publicKey() {
		t.Fatalf("the header should carry the public key: %q", header)
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("a JWT has three parts: %q", token)
	}
	var claims struct {
		Aud string `json:"aud"`
		Exp int64  `json:"exp"`
		Sub string `json:"sub"`
	}
	if err := json.Unmarshal(unb64(t, parts[1]), &claims); err != nil {
		t.Fatal(err)
	}
	if claims.Aud != "https://fcm.googleapis.com" || claims.Sub != pushSubject || claims.Exp != now.Add(vapidLife).Unix() {
		t.Fatalf("claims differ from RFC 8292: %+v", claims)
	}
	if claims.Exp-now.Unix() > 24*3600 {
		t.Fatal("a push service refuses a token valid for more than a day")
	}
	signature := unb64(t, parts[2])
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	public, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), key.public)
	if err != nil {
		t.Fatal(err)
	}
	if len(signature) != 64 || !ecdsa.Verify(public, digest[:], new(big.Int).SetBytes(signature[:32]), new(big.Int).SetBytes(signature[32:])) {
		t.Fatal("the ES256 signature should verify as r||s against the advertised key")
	}
}

func TestParseVAPIDKeyRefusesWhatIsNotAKey(t *testing.T) {
	for name, encoded := range map[string]string{"empty": "", "not base64": "!!!", "short": "AAAA", "zero scalar": strings.Repeat("A", 43)} {
		if _, err := parseVAPIDKey(encoded); err == nil {
			t.Errorf("%s should not parse as a VAPID key", name)
		}
	}
}

func testSubscription(t *testing.T, endpoint string) (pushSubscription, *ecdh.PrivateKey, []byte) {
	t.Helper()
	ua, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	auth := []byte("0123456789abcdef")
	return pushSubscription{Endpoint: endpoint, Keys: pushKeys{
		P256dh: base64.RawURLEncoding.EncodeToString(ua.PublicKey().Bytes()),
		Auth:   base64.RawURLEncoding.EncodeToString(auth),
	}}, ua, auth
}

func TestValidateSubscriptionOnlyAcceptsABrowserPushService(t *testing.T) {
	for endpoint, ok := range map[string]bool{
		"https://fcm.googleapis.com/fcm/send/abc":                           true,
		"https://updates.push.services.mozilla.com/wpush/v2/abc":            true,
		"https://web.push.apple.com/abc":                                    true,
		"https://wns2-par02p.notify.windows.com/w/?token=abc":               true,
		"https://fcm.googleapis.com:443/fcm/send/abc":                       true,
		"http://fcm.googleapis.com/fcm/send/abc":                            false,
		"https://fcm.googleapis.com:8443/fcm/send/abc":                      false,
		"https://fcm.googleapis.com.evil.example/x":                         false,
		"https://evilfcm.googleapis.com/x":                                  false,
		"https://user@fcm.googleapis.com/x":                                 false,
		"https://127.0.0.1/x":                                               false,
		"https://localhost/x":                                               false,
		"https://169.254.169.254/latest/meta-data":                          false,
		"https://push.apple.com.evil.example/x":                             false,
		"":                                                                  false,
		"ftp://fcm.googleapis.com/x":                                        false,
		"https://evil.example/fcm.googleapis.com":                           false,
		"https://xpush.apple.com/x":                                         false,
		"https://updates.push.services.mozilla.com.evil.example/wpush/v2/a": false,
	} {
		sub, _, _ := testSubscription(t, endpoint)
		if err := sub.validate(); (err == nil) != ok {
			t.Errorf("endpoint %q: accepted=%v, want %v (%v)", endpoint, err == nil, ok, err)
		}
	}
}

func TestValidateSubscriptionRefusesBadKeys(t *testing.T) {
	good, _, _ := testSubscription(t, "https://fcm.googleapis.com/fcm/send/abc")
	for name, edit := range map[string]func(*pushSubscription){
		"p256dh not a point":   func(s *pushSubscription) { s.Keys.P256dh = base64.RawURLEncoding.EncodeToString([]byte("nope")) },
		"auth wrong length":    func(s *pushSubscription) { s.Keys.Auth = base64.RawURLEncoding.EncodeToString([]byte("short")) },
		"auth not base64url":   func(s *pushSubscription) { s.Keys.Auth = "!!!" },
		"p256dh not base64":    func(s *pushSubscription) { s.Keys.P256dh = "!!!" },
		"padded keys are fine": func(s *pushSubscription) {},
	} {
		sub := good
		edit(&sub)
		if err := sub.validate(); (err == nil) != (name == "padded keys are fine") {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// decryptPush is the browser's half, so the sender is checked by something that
// did not share its code.
func decryptPush(t *testing.T, ua *ecdh.PrivateKey, auth, body []byte) []byte {
	t.Helper()
	salt, rest := body[:16], body[16:]
	if size := uint32(rest[0])<<24 | uint32(rest[1])<<16 | uint32(rest[2])<<8 | uint32(rest[3]); size != pushRecord {
		t.Fatalf("record size %d", size)
	}
	idLen := int(rest[4])
	senderBytes, ciphertext := rest[5:5+idLen], rest[5+idLen:]
	sender, err := ecdh.P256().NewPublicKey(senderBytes)
	if err != nil {
		t.Fatal(err)
	}
	secret, err := ua.ECDH(sender)
	if err != nil {
		t.Fatal(err)
	}
	info := append(append([]byte("WebPush: info\x00"), ua.PublicKey().Bytes()...), senderBytes...)
	ikm, _ := hkdf.Key(sha256.New, secret, auth, string(info), 32)
	key, _ := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: aes128gcm\x00", 16)
	nonce, _ := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: nonce\x00", 12)
	block, _ := aes.NewCipher(key)
	aead, _ := cipher.NewGCM(block)
	plain, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		t.Fatalf("the browser could not open the record: %v", err)
	}
	if plain[len(plain)-1] != 0x02 {
		t.Fatalf("the last record ends in 0x02, this ends in %#x", plain[len(plain)-1])
	}
	return plain[:len(plain)-1]
}

func TestSendPushPostsAnEncryptedMessageTheBrowserCanOpen(t *testing.T) {
	key, _, _ := newVAPIDKey()
	var got struct {
		header http.Header
		body   []byte
	}
	service := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.header = r.Header.Clone()
		got.body, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusCreated)
	}))
	t.Cleanup(service.Close)
	previous := pushClient.Transport
	pushClient.Transport = service.Client().Transport
	t.Cleanup(func() { pushClient.Transport = previous })

	sub, ua, auth := testSubscription(t, service.URL+"/push/abc")
	status, err := sendPush(context.Background(), key, sub, []byte(`{"kind":"done"}`))
	if err != nil || status != http.StatusCreated {
		t.Fatalf("send: status %d, %v", status, err)
	}
	if got.header.Get("Content-Encoding") != "aes128gcm" || got.header.Get("TTL") == "" || got.header.Get("Urgency") != "high" {
		t.Fatalf("headers differ from RFC 8030 and 8188: %v", got.header)
	}
	if !strings.HasPrefix(got.header.Get("Authorization"), "vapid t=") || !strings.HasSuffix(got.header.Get("Authorization"), ", k="+key.publicKey()) {
		t.Fatalf("authorization: %q", got.header.Get("Authorization"))
	}
	if plain := decryptPush(t, ua, auth, got.body); string(plain) != `{"kind":"done"}` {
		t.Fatalf("the browser should read what was sent, read %q", plain)
	}
}

func TestSendPushDoesNotFollowARedirect(t *testing.T) {
	key, _, _ := newVAPIDKey()
	hits := 0
	target := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits++ }))
	t.Cleanup(target.Close)
	service := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(service.Close)
	previous := pushClient.Transport
	pushClient.Transport = service.Client().Transport
	t.Cleanup(func() { pushClient.Transport = previous })

	sub, _, _ := testSubscription(t, service.URL+"/push/abc")
	status, err := sendPush(context.Background(), key, sub, []byte("x"))
	if err != nil || status != http.StatusTemporaryRedirect || hits != 0 {
		t.Fatalf("a redirect should be reported, not followed: status %d, hits %d, %v", status, hits, err)
	}
}
