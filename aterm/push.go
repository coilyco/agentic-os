package main

import (
	"bytes"
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
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Web Push to a closed browser: RFC 8030 delivery, RFC 8291 encryption, RFC 8292
// VAPID, on the standard library on purpose. docs/aterm-daemon.md

const (
	// vapidKeyEnv holds the VAPID private key, base64url of the 32-byte scalar.
	// SSM /coilysiren/aterm/vapid-key, fed through daemon.env like the Sentry DSN.
	vapidKeyEnv = "ATERM_VAPID_KEY"
	// pushFeature is how a client knows the daemon answers push_key, push_subscribe
	// and push_unsubscribe. A daemon without a VAPID key does not advertise it.
	pushFeature = "web-push"
	// pushSubject is the VAPID contact a push service may reach.
	pushSubject = "https://coilyco.dev"
	// pushRecord is the RFC 8188 record size, and the most a push service takes.
	pushRecord = 4096
	// pushHeader is salt, record size, key length and the 65-byte ephemeral key.
	pushHeader = 16 + 4 + 1 + 65
	// pushMaxPlain is the largest plaintext one record holds, after the header,
	// the AEAD tag and the delimiter byte.
	pushMaxPlain = pushRecord - pushHeader - 16 - 1
	// vapidLife is one token's validity. A push service refuses more than a day.
	vapidLife = 12 * time.Hour
)

// pushServices are the hosts a push endpoint can sit on. A subscription names a URL
// the daemon posts to, so anything else is refused.
var pushServices = []string{
	"fcm.googleapis.com",
	".push.services.mozilla.com",
	".push.apple.com",
	".notify.windows.com",
}

// vapidKey is the application server's identity to every push service.
type vapidKey struct {
	private *ecdsa.PrivateKey
	// public is the uncompressed point, which is what a browser subscribes against.
	public []byte
}

// parseVAPIDKey reads the base64url private scalar the web-push tools print.
func parseVAPIDKey(encoded string) (*vapidKey, error) {
	raw, err := decodeBase64URL(strings.TrimSpace(encoded))
	if err != nil {
		return nil, errors.New("the VAPID key is not base64url")
	}
	private, err := ecdsa.ParseRawPrivateKey(elliptic.P256(), raw)
	if err != nil {
		return nil, errors.New("the VAPID key is not a P-256 private scalar")
	}
	public, err := private.PublicKey.Bytes()
	if err != nil {
		return nil, err
	}
	return &vapidKey{private: private, public: public}, nil
}

// newVAPIDKey makes a fresh key, returning it and its encoded private scalar.
func newVAPIDKey() (*vapidKey, string, error) {
	private, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, "", err
	}
	scalar, err := private.Bytes()
	if err != nil {
		return nil, "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(scalar)
	key, err := parseVAPIDKey(encoded)
	return key, encoded, err
}

// publicKey is the applicationServerKey a browser passes to subscribe().
func (k *vapidKey) publicKey() string { return base64.RawURLEncoding.EncodeToString(k.public) }

// authorization is the RFC 8292 header for one push service.
func (k *vapidKey) authorization(endpoint *url.URL, now time.Time) (string, error) {
	claims, err := json.Marshal(map[string]any{
		"aud": endpoint.Scheme + "://" + endpoint.Host,
		"exp": now.Add(vapidLife).Unix(),
		"sub": pushSubject,
	})
	if err != nil {
		return "", err
	}
	signing := base64.RawURLEncoding.EncodeToString([]byte(`{"typ":"JWT","alg":"ES256"}`)) + "." + base64.RawURLEncoding.EncodeToString(claims)
	digest := sha256.Sum256([]byte(signing))
	r, s, err := ecdsa.Sign(rand.Reader, k.private, digest[:])
	if err != nil {
		return "", err
	}
	signature := make([]byte, 64)
	r.FillBytes(signature[:32])
	s.FillBytes(signature[32:])
	return "vapid t=" + signing + "." + base64.RawURLEncoding.EncodeToString(signature) + ", k=" + k.publicKey(), nil
}

// pushKeys are a subscription's two keys as the browser's toJSON() writes them.
type pushKeys struct {
	P256dh string `json:"p256dh"`
	Auth   string `json:"auth"`
}

// pushSubscription is one browser's PushSubscription.toJSON(), so a client sends
// it as it is.
type pushSubscription struct {
	Endpoint string    `json:"endpoint"`
	Keys     pushKeys  `json:"keys"`
	Added    time.Time `json:"added,omitempty"`
}

// validate refuses a subscription the daemon would not send to.
func (s pushSubscription) validate() error {
	endpoint, err := url.Parse(s.Endpoint)
	if err != nil || endpoint.Scheme != "https" || endpoint.User != nil || endpoint.Hostname() == "" {
		return errors.New("a push endpoint is an https URL")
	}
	if port := endpoint.Port(); port != "" && port != "443" {
		return errors.New("a push endpoint is on port 443")
	}
	host := strings.ToLower(endpoint.Hostname())
	allowed := false
	for _, service := range pushServices {
		if host == service || (strings.HasPrefix(service, ".") && strings.HasSuffix(host, service)) {
			allowed = true
		}
	}
	if !allowed {
		return fmt.Errorf("%s is not a browser push service", host)
	}
	if _, err := s.recipient(); err != nil {
		return err
	}
	return nil
}

// recipient decodes the browser's public key and auth secret.
func (s pushSubscription) recipient() (*pushRecipient, error) {
	point, err := decodeBase64URL(s.Keys.P256dh)
	if err != nil {
		return nil, errors.New("the p256dh key is not base64url")
	}
	public, err := ecdh.P256().NewPublicKey(point)
	if err != nil {
		return nil, errors.New("the p256dh key is not a P-256 point")
	}
	auth, err := decodeBase64URL(s.Keys.Auth)
	if err != nil || len(auth) != 16 {
		return nil, errors.New("the auth secret is 16 bytes of base64url")
	}
	return &pushRecipient{public: public, auth: auth}, nil
}

type pushRecipient struct {
	public *ecdh.PublicKey
	auth   []byte
}

// encryptPush is RFC 8291 section 3: an aes128gcm body for one browser. The
// ephemeral key and salt are arguments so the RFC's own example can be replayed.
func encryptPush(to *pushRecipient, plaintext, salt []byte, ephemeral *ecdh.PrivateKey) ([]byte, error) {
	if len(plaintext) > pushMaxPlain {
		return nil, fmt.Errorf("a push payload is at most %d bytes, this is %d", pushMaxPlain, len(plaintext))
	}
	secret, err := ephemeral.ECDH(to.public)
	if err != nil {
		return nil, err
	}
	sender := ephemeral.PublicKey().Bytes()
	keyInfo := append(append([]byte("WebPush: info\x00"), to.public.Bytes()...), sender...)
	ikm, err := hkdf.Key(sha256.New, secret, to.auth, string(keyInfo), 32)
	if err != nil {
		return nil, err
	}
	key, err := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: aes128gcm\x00", 16)
	if err != nil {
		return nil, err
	}
	nonce, err := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: nonce\x00", 12)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	// The one record is also the last, so its delimiter is 0x02.
	record := append(append([]byte{}, plaintext...), 0x02)
	body := make([]byte, 0, pushHeader+len(record)+aead.Overhead())
	body = append(body, salt...)
	body = binary.BigEndian.AppendUint32(body, pushRecord)
	body = append(body, byte(len(sender)))
	body = append(body, sender...)
	return aead.Seal(body, nonce, record, nil), nil
}

// pushSender posts one encrypted message. A field, so a test needs no network.
type pushSender func(ctx context.Context, key *vapidKey, sub pushSubscription, payload []byte) (int, error)

// pushClient never follows a redirect, since an endpoint is checked once at subscribe.
var pushClient = &http.Client{
	Timeout:       15 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

// sendPush encrypts payload for sub and posts it. The status is the push
// service's answer, where 404 and 410 mean the browser dropped the subscription.
func sendPush(ctx context.Context, key *vapidKey, sub pushSubscription, payload []byte) (int, error) {
	to, err := sub.recipient()
	if err != nil {
		return 0, err
	}
	ephemeral, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return 0, err
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return 0, err
	}
	body, err := encryptPush(to, payload, salt, ephemeral)
	if err != nil {
		return 0, err
	}
	endpoint, err := url.Parse(sub.Endpoint)
	if err != nil {
		return 0, err
	}
	authorization, err := key.authorization(endpoint, time.Now())
	if err != nil {
		return 0, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, sub.Endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	request.Header.Set("Content-Encoding", "aes128gcm")
	request.Header.Set("Content-Type", "application/octet-stream")
	request.Header.Set("Authorization", authorization)
	// A seat waiting is stale in minutes, and a high urgency wakes a dozing phone.
	request.Header.Set("TTL", "600")
	request.Header.Set("Urgency", "high")
	response, err := pushClient.Do(request)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4<<10))
	return response.StatusCode, nil
}

// decodeBase64URL takes the padded and unpadded forms a browser may send.
func decodeBase64URL(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(strings.TrimRight(s, "="))
}
