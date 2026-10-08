package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

const (
	// The only relying party. A page on another origin cannot assert, so the
	// daemon-served client on the tailnet name stays read-only. See docs/aterm-daemon.md.
	passkeyRPID   = "coilyco.dev"
	passkeyOrigin = "https://coilyco.dev"
	// An enrollment code is single use, and five wrong guesses burn it.
	enrollCodeTTL  = 10 * time.Minute
	enrollAttempts = 5
	// Letters and digits that survive being read aloud, no 0/o/1/l/i.
	enrollAlphabet = "abcdefghjkmnpqrstuvwxyz23456789"
	passkeyFeature = "passkey"
)

// passkeyUser is the one person who can enroll, held as WebAuthn's user.
type passkeyUser struct {
	ID          []byte                `json:"id"`
	Credentials []webauthn.Credential `json:"credentials"`
}

func (u *passkeyUser) WebAuthnID() []byte                         { return u.ID }
func (u *passkeyUser) WebAuthnName() string                       { return "kai" }
func (u *passkeyUser) WebAuthnDisplayName() string                { return "Kai" }
func (u *passkeyUser) WebAuthnCredentials() []webauthn.Credential { return u.Credentials }

// passkeyStore holds the enrolled credentials on disk and the enrollment code
// in memory, so a restart keeps the passkeys and drops an unspent code.
type passkeyStore struct {
	mu   sync.Mutex
	path string
	rp   *webauthn.WebAuthn
	user passkeyUser

	code     string
	expires  time.Time
	failures int
	now      func() time.Time
}

// newPasskeyStore loads path when set. An empty path keeps credentials in memory only.
func newPasskeyStore(path, rpID string, origins []string) (*passkeyStore, error) {
	rp, err := webauthn.New(&webauthn.Config{
		RPDisplayName: "aterm",
		RPID:          rpID,
		RPOrigins:     origins,
		// Assertion without user verification would be possession of the device alone.
		AuthenticatorSelection: protocol.AuthenticatorSelection{UserVerification: protocol.VerificationRequired},
	})
	if err != nil {
		return nil, err
	}
	store := &passkeyStore{path: path, rp: rp, now: time.Now}
	if path == "" {
		return store, nil
	}
	raw, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return store, nil
	case err != nil:
		return nil, err
	}
	if err := json.Unmarshal(raw, &store.user); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return store, nil
}

func (s *passkeyStore) enrolled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.user.Credentials) > 0
}

// save writes the credentials through a rename, 0600, since a lost half-write
// would lock Kai out of her own seats.
func (s *passkeyStore) save() error {
	if s.path == "" {
		return nil
	}
	raw, err := json.Marshal(&s.user)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	temp := s.path + ".tmp"
	if err := os.WriteFile(temp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(temp, s.path)
}

// mintCode issues the one live enrollment code, replacing any earlier one.
func (s *passkeyStore) mintCode() (string, time.Duration, error) {
	random := make([]byte, 8)
	if _, err := rand.Read(random); err != nil {
		return "", 0, err
	}
	code := make([]byte, len(random))
	for i, b := range random {
		code[i] = enrollAlphabet[int(b)%len(enrollAlphabet)]
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.code, s.expires, s.failures = string(code), s.now().Add(enrollCodeTTL), 0
	return string(code), enrollCodeTTL, nil
}

func normalizeCode(code string) string {
	return strings.ToLower(strings.NewReplacer("-", "", " ", "").Replace(code))
}

// spendCode consumes the code when it matches, and burns it after too many misses.
func (s *passkeyStore) spendCode(given string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.code == "" || !s.now().Before(s.expires) {
		s.code = ""
		return errors.New("no enrollment code is live, run `aterm passkey enroll` in a terminal")
	}
	if subtle.ConstantTimeCompare([]byte(normalizeCode(given)), []byte(s.code)) != 1 {
		if s.failures++; s.failures >= enrollAttempts {
			s.code = ""
		}
		return errors.New("that is not the enrollment code")
	}
	s.code = ""
	return nil
}

func (s *passkeyStore) beginEnroll() (*protocol.CredentialCreation, *webauthn.SessionData, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.user.ID) == 0 {
		s.user.ID = make([]byte, 64)
		if _, err := rand.Read(s.user.ID); err != nil {
			return nil, nil, err
		}
	}
	var exclude []protocol.CredentialDescriptor
	for _, credential := range s.user.Credentials {
		exclude = append(exclude, credential.Descriptor())
	}
	return s.rp.BeginRegistration(&s.user, webauthn.WithExclusions(exclude))
}

func (s *passkeyStore) finishEnroll(session webauthn.SessionData, raw []byte) error {
	parsed, err := protocol.ParseCredentialCreationResponseBytes(raw)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	credential, err := s.rp.CreateCredential(&s.user, session, parsed)
	if err != nil {
		return err
	}
	s.user.Credentials = append(s.user.Credentials, *credential)
	return s.save()
}

func (s *passkeyStore) beginAssert() (*protocol.CredentialAssertion, *webauthn.SessionData, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.user.Credentials) == 0 {
		return nil, nil, errors.New("no passkey is enrolled, run `aterm passkey enroll` in a terminal")
	}
	return s.rp.BeginLogin(&s.user)
}

// finishAssert verifies the signature, the origin, and user verification, then
// stores the credential back so its signature counter moves forward.
func (s *passkeyStore) finishAssert(session webauthn.SessionData, raw []byte) error {
	parsed, err := protocol.ParseCredentialRequestResponseBytes(raw)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	credential, err := s.rp.ValidateLogin(&s.user, session, parsed)
	if err != nil {
		return err
	}
	for i, held := range s.user.Credentials {
		if string(held.ID) == string(credential.ID) {
			s.user.Credentials[i] = *credential
		}
	}
	return s.save()
}

// revoke forgets every passkey, which is how a lost device stops mattering.
func (s *passkeyStore) revoke() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.user = passkeyUser{}
	if s.path == "" {
		return nil
	}
	if err := os.Remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// passkeyStatePath is beside the session ledger, under the real home.
func passkeyStatePath() string {
	return filepath.Join(filepath.Dir(ledgerDir()), "passkeys.json")
}

func defaultPasskeys() *passkeyStore {
	store, err := newPasskeyStore("", passkeyRPID, []string{passkeyOrigin})
	if err != nil {
		panic("aterm: the passkey relying party is a constant and failed to build: " + err.Error())
	}
	return store
}
