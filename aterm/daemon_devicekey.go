package main

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	deviceKeyFeature    = "device-key"
	reasonDeviceKeyGone = "device_key_unknown"
	// assertPrefix and a zero byte lead the challenge in what the phone signs, so a
	// page script cannot obtain a signature over anything else.
	assertPrefix       = "aterm-device-assert-v1"
	deviceChallengeTTL = 60 * time.Second
	maxDeviceKeys      = 16
)

var rawURL = base64.RawURLEncoding

// deviceKey is a phone's enrolled key. The private half never leaves its Keystore.
type deviceKey struct {
	KeyID     string    `json:"key_id"`
	PublicKey []byte    `json:"public_key"`
	Level     string    `json:"level"`
	Enrolled  time.Time `json:"enrolled"`
}

// deviceCeremony is the one challenge a connection has open, spent by its first finish.
type deviceCeremony struct {
	kind      string
	keyID     string
	challenge []byte
	expires   time.Time
}

// pinnedRoots is the pool an attestation chain must lead to.
func pinnedRoots() *x509.CertPool {
	pool := x509.NewCertPool()
	for _, pem := range googleAttestationRoots {
		if !pool.AppendCertsFromPEM([]byte(pem)) {
			panic("aterm: a pinned Google attestation root failed to parse")
		}
	}
	return pool
}

func (s *passkeyStore) deviceKeyEnrolled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.user.DeviceKeys) > 0
}

func (s *passkeyStore) findDeviceKey(keyID string) (deviceKey, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, held := range s.user.DeviceKeys {
		if held.KeyID == keyID {
			return held, true
		}
	}
	return deviceKey{}, false
}

// addDeviceKey stores key, replacing one with the same id.
func (s *passkeyStore) addDeviceKey(key deviceKey) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, held := range s.user.DeviceKeys {
		if held.KeyID == key.KeyID {
			s.user.DeviceKeys[i] = key
			return s.save()
		}
	}
	if len(s.user.DeviceKeys) >= maxDeviceKeys {
		return fmt.Errorf("%d device keys are enrolled, run `aterm passkey revoke` to forget them", maxDeviceKeys)
	}
	s.user.DeviceKeys = append(s.user.DeviceKeys, key)
	return s.save()
}

// deviceKeyFrames answers the device key ceremonies. Enrollment spends the same
// one-time code a passkey does, minted from Kai's terminal.
func (d *daemon) deviceKeyFrames(cl *client, message frame) error {
	if !cl.web {
		return withExit(exitUsage, errors.New("a device key is enrolled and asserted from the aterm app"))
	}
	store := d.passkeys
	switch message.Type {
	case "device_enroll_begin":
		if err := store.spendCode(message.EnrollCode); err != nil {
			return withExit(exitUsage, err)
		}
		return d.openDeviceCeremony(cl, message, "enroll", "", "device_enroll_challenge")
	case "device_assert_begin":
		if _, ok := store.findDeviceKey(message.KeyID); !ok {
			return withReason(reasonDeviceKeyGone, errors.New("this phone's key is not enrolled here, run `aterm passkey enroll` for a new code"))
		}
		return d.openDeviceCeremony(cl, message, "assert", message.KeyID, "device_assert_challenge")
	}
	ceremony := cl.deviceCeremony
	cl.deviceCeremony = nil
	want := "enroll"
	if message.Type == "device_assert_finish" {
		want = "assert"
	}
	switch {
	case ceremony == nil || ceremony.kind != want:
		return withExit(exitUsage, errors.New("no "+want+" ceremony is open on this connection"))
	case !store.now().Before(ceremony.expires):
		return withExit(exitUsage, errors.New("the challenge expired, start again"))
	}
	if want == "enroll" {
		keyID, err := d.finishDeviceEnroll(store, ceremony, message)
		if err != nil {
			return withExit(exitUsage, err)
		}
		if err := cl.c.write(frame{Type: "device_enrolled", ID: message.ID, KeyID: keyID}); err != nil {
			return err
		}
		// Enrolling proves no fingerprint, so the client asserts next.
		return cl.c.write(frame{Type: "typing", Typing: d.typingStanding(cl)})
	}
	if err := finishDeviceAssert(store, ceremony, message); err != nil {
		return withExit(exitUsage, err)
	}
	cl.asserted = true
	d.logf("a remote device asserted its device key")
	if err := cl.c.write(frame{Type: "device_asserted", ID: message.ID}); err != nil {
		return err
	}
	return cl.c.write(frame{Type: "typing", Typing: d.typingStanding(cl)})
}

func (d *daemon) openDeviceCeremony(cl *client, message frame, kind, keyID, reply string) error {
	challenge := make([]byte, 32)
	if _, err := rand.Read(challenge); err != nil {
		return err
	}
	cl.deviceCeremony = &deviceCeremony{kind: kind, keyID: keyID, challenge: challenge, expires: d.passkeys.now().Add(deviceChallengeTTL)}
	return cl.c.write(frame{Type: reply, ID: message.ID, Challenge: rawURL.EncodeToString(challenge)})
}

// finishDeviceEnroll verifies the attestation and stores the key.
func (d *daemon) finishDeviceEnroll(store *passkeyStore, ceremony *deviceCeremony, message frame) (string, error) {
	spki, err := rawURL.DecodeString(message.PublicKey)
	if err != nil {
		return "", errors.New("public_key is not base64url")
	}
	pub, err := parsePublicKey(spki)
	if err != nil {
		return "", err
	}
	chain, err := parseAttestation(message.Attestation)
	if err != nil {
		return "", err
	}
	level, err := verifyAttestation(chain, store.roots, ceremony.challenge, pub, store.now())
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(spki)
	keyID := rawURL.EncodeToString(sum[:])
	if err := store.addDeviceKey(deviceKey{KeyID: keyID, PublicKey: spki, Level: level, Enrolled: store.now().UTC()}); err != nil {
		return "", err
	}
	d.logf("a remote device enrolled a device key (%s)", level)
	return keyID, nil
}

// finishDeviceAssert verifies the signature over the prefixed challenge.
func finishDeviceAssert(store *passkeyStore, ceremony *deviceCeremony, message frame) error {
	if message.KeyID != ceremony.keyID {
		return errors.New("the signature names a different key than the one asserted")
	}
	held, ok := store.findDeviceKey(ceremony.keyID)
	if !ok {
		return withReason(reasonDeviceKeyGone, errors.New("this phone's key is no longer enrolled here"))
	}
	pub, err := parsePublicKey(held.PublicKey)
	if err != nil {
		return err
	}
	signature, err := rawURL.DecodeString(message.Signature)
	if err != nil {
		return errors.New("signature is not base64url")
	}
	signed := append(append([]byte(assertPrefix), 0), ceremony.challenge...)
	digest := sha256.Sum256(signed)
	if !ecdsa.VerifyASN1(pub, digest[:], signature) {
		return errors.New("that signature does not match the enrolled key")
	}
	return nil
}

// parseAttestation reads a JSON array of base64url DER certificates, leaf first,
// or one string of them joined by commas, which base64url never contains.
func parseAttestation(raw json.RawMessage) ([]*x509.Certificate, error) {
	var items []string
	if err := json.Unmarshal(raw, &items); err != nil {
		var joined string
		if json.Unmarshal(raw, &joined) != nil {
			return nil, errors.New("attestation must be an array of base64url certificates")
		}
		items = strings.Split(joined, ",")
	}
	if len(items) == 0 || len(items) > 8 {
		return nil, errors.New("attestation must hold between one and eight certificates")
	}
	chain := make([]*x509.Certificate, 0, len(items))
	for _, item := range items {
		der, err := rawURL.DecodeString(strings.TrimSpace(item))
		if err != nil {
			return nil, errors.New("an attestation certificate is not base64url")
		}
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			return nil, errors.New("an attestation certificate is not DER")
		}
		chain = append(chain, cert)
	}
	return chain, nil
}
