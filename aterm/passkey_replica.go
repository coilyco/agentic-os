package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
)

// passkeysSSMName is where the enrolling daemon publishes its store and a replica's
// ExternalSecret reads it. references/passkey-replica.md, COI-3542
const passkeysSSMName = "/coilysiren/aterm/passkeys"

var errFollowing = errors.New("this daemon follows the passkeys another one publishes, so enroll or revoke there")

// newFollowingStore is a read-only store that tracks the file a secret mount holds.
func newFollowingStore(path, rpID string, origins []string) (*passkeyStore, error) {
	store, err := newPasskeyStore("", rpID, origins)
	if err != nil {
		return nil, err
	}
	store.follow = path
	store.mu.Lock()
	store.sync()
	store.mu.Unlock()
	return store, nil
}

// sync reloads the followed file when it changed. The caller holds s.mu. A missing file
// honors no passkey, and one that will not parse keeps what is held.
func (s *passkeyStore) sync() {
	if s.follow == "" {
		return
	}
	info, err := os.Stat(s.follow)
	if errors.Is(err, os.ErrNotExist) {
		s.user, s.followed = passkeyUser{}, ""
		return
	}
	if err != nil {
		return
	}
	stamp := fmt.Sprintf("%d:%d", info.ModTime().UnixNano(), info.Size())
	if stamp == s.followed {
		return
	}
	raw, err := os.ReadFile(s.follow)
	if err != nil {
		return
	}
	var user passkeyUser
	if json.Unmarshal(raw, &user) != nil {
		return
	}
	s.user, s.followed = user, stamp
}

// publishedPasskeys is the store without attestation, which a login never reads.
// An empty store publishes a revoke.
func publishedPasskeys(raw []byte) ([]byte, error) {
	var user passkeyUser
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &user); err != nil {
			return nil, err
		}
	}
	for i := range user.Credentials {
		user.Credentials[i].Attestation = webauthn.CredentialAttestation{}
	}
	return json.Marshal(&user)
}

// putSSM writes the value through a mode 600 file, so it stays out of argv.
func putSSM(name string, value []byte) error {
	aws, err := exec.LookPath("aws")
	if err != nil {
		return errors.New("aws is not on PATH here, so run `aterm passkey publish` where it is")
	}
	dir, err := os.MkdirTemp("", "aterm-passkeys")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	file := filepath.Join(dir, "value")
	if err := os.WriteFile(file, value, 0o600); err != nil {
		return err
	}
	out, err := exec.Command(aws, "ssm", "put-parameter", "--region", "us-east-1", "--name", name,
		"--type", "SecureString", "--tier", "Intelligent-Tiering", "--overwrite", "--value", "file://"+file).CombinedOutput()
	if err != nil {
		return fmt.Errorf("aws ssm put-parameter %s: %v: %s", name, err, out)
	}
	return nil
}

// publishPasskeys puts this host's stored passkeys, or an empty store once revoked,
// where replicas follow them.
func publishPasskeys(name string) error {
	raw, err := os.ReadFile(passkeyStatePath())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	value, err := publishedPasskeys(raw)
	if err != nil {
		return err
	}
	return putSSM(name, value)
}

// storeStamp is the passkey file's change marker, empty when there is none.
func storeStamp() string {
	info, err := os.Stat(passkeyStatePath())
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%d:%d", info.ModTime().UnixNano(), info.Size())
}

// waitForEnrollment returns once the stored passkeys change, or after the code's life.
func waitForEnrollment(before string, life time.Duration) bool {
	deadline := time.Now().Add(life)
	for time.Now().Before(deadline) {
		if storeStamp() != before {
			return true
		}
		time.Sleep(2 * time.Second)
	}
	return false
}
