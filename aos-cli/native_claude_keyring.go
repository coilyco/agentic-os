// Claude Code namespaces its macOS Keychain credential by a digest of
// CLAUDE_CONFIG_DIR. See docs/native-claude-credentials.md.

package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"
)

const claudeCredentialService = "Claude Code-credentials"

var (
	errClaudeKeyringNotFound    = errors.New("Claude Code keyring credential not found")
	errClaudeKeyringUnsupported = errors.New("Claude Code keyring is unsupported")
)

// nativeClaudeKeychainService mirrors Claude Code's own naming: the default
// directory keeps the bare service, every other one takes a digest suffix.
func nativeClaudeKeychainService(home, configDir string) string {
	configDir = strings.TrimSpace(configDir)
	if configDir == "" || configDir == filepath.Join(home, ".claude") {
		return claudeCredentialService
	}
	digest := sha256.Sum256([]byte(configDir))
	return fmt.Sprintf("%s-%x", claudeCredentialService, digest[:4])
}

// nativeClaudeKeychainAccount matches the account Claude Code records, which is
// the operating-system user name.
func nativeClaudeKeychainAccount() string {
	if current, err := user.Current(); err == nil &&
		strings.TrimSpace(current.Username) != "" {
		return current.Username
	}
	return strings.TrimSpace(os.Getenv("USER"))
}

// canonicalClaudeCredentialPath is the one file every session links back to.
func canonicalClaudeCredentialPath(home string) string {
	return filepath.Join(home, ".claude", ".credentials.json")
}

// seedCanonicalClaudeCredential writes the Keychain login when canonical could
// not carry a launch, never over one that could. Doc page above.
func seedCanonicalClaudeCredential(
	ctx context.Context,
	read claudeKeyringReader,
	home string,
	now time.Time,
) (bool, error) {
	target := canonicalClaudeCredentialPath(home)
	usable, err := claudeCredentialUsable(target, now)
	if err != nil {
		return false, err
	}
	if usable {
		return false, nil
	}

	secret, err := read(ctx, claudeCredentialService, nativeClaudeKeychainAccount())
	if errors.Is(err, errClaudeKeyringUnsupported) ||
		errors.Is(err, errClaudeKeyringNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if len(secret) == 0 {
		return false, nil
	}
	// An absent target takes anything, and a worthless one is replaced only by
	// a payload that is worth more rather than merely stamped later.
	fresher, err := claudeCredentialOutlives(secret, target, now)
	if err != nil || !fresher {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return false, fmt.Errorf("create %s: %w", filepath.Dir(target), err)
	}
	if err := os.WriteFile(target, secret, 0o600); err != nil {
		return false, fmt.Errorf("write %s: %w", target, err)
	}
	return true, nil
}

// claudeCredentialUsable reports whether the file could carry a launch, which
// is what disarms the seed. Why not the stamp: doc page above.
func claudeCredentialUsable(path string, now time.Time) (bool, error) {
	payload, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read %s: %w", path, err)
	}
	return claudeCredentialWorthOf(payload, now).usable(now), nil
}

// reclaimSessionClaudeCredential recovers a rotated token when a session left a
// regular file where its symlink was. docs/native-claude-credentials.md.
func reclaimSessionClaudeCredential(sessionHome, home string) (bool, error) {
	if strings.TrimSpace(sessionHome) == "" {
		return false, nil
	}
	source := filepath.Join(sessionHome, ".claude", ".credentials.json")
	info, err := os.Lstat(source)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect %s: %w", source, err)
	}
	// Still a symlink means the session wrote through it, or never wrote.
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return false, nil
	}
	secret, err := os.ReadFile(source)
	if err != nil {
		return false, fmt.Errorf("read %s: %w", source, err)
	}
	if len(secret) == 0 {
		return false, nil
	}
	target := canonicalClaudeCredentialPath(home)
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return false, fmt.Errorf("create %s: %w", filepath.Dir(target), err)
	}
	if err := os.WriteFile(target, secret, 0o600); err != nil {
		return false, fmt.Errorf("write %s: %w", target, err)
	}
	return true, nil
}

// harvestSessionClaudeKeychain recovers a rotated token from the per-session
// Keychain item. docs/native-claude-credentials.md.
func harvestSessionClaudeKeychain(
	ctx context.Context,
	read claudeKeyringReader,
	sessionHome, home string,
	now time.Time,
) (bool, error) {
	if strings.TrimSpace(sessionHome) == "" {
		return false, nil
	}
	configDir := filepath.Join(sessionHome, ".claude")
	service := nativeClaudeKeychainService(home, configDir)
	// A session on the default service shares the host's item, so there is
	// nothing session-scoped to recover.
	if service == claudeCredentialService {
		return false, nil
	}
	// A link or a file still present is reclaimSessionClaudeCredential's case.
	switch _, err := os.Lstat(filepath.Join(configDir, ".credentials.json")); {
	case err == nil:
		return false, nil
	case !os.IsNotExist(err):
		return false, fmt.Errorf("inspect %s: %w", configDir, err)
	}

	secret, err := read(ctx, service, nativeClaudeKeychainAccount())
	if errors.Is(err, errClaudeKeyringUnsupported) ||
		errors.Is(err, errClaudeKeyringNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if len(secret) == 0 {
		return false, nil
	}

	target := canonicalClaudeCredentialPath(home)
	fresher, err := claudeCredentialOutlives(secret, target, now)
	if err != nil || !fresher {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return false, fmt.Errorf("create %s: %w", filepath.Dir(target), err)
	}
	if err := os.WriteFile(target, secret, 0o600); err != nil {
		return false, fmt.Errorf("write %s: %w", target, err)
	}
	return true, nil
}

// claudeKeyringDeleter is the write half of the port, back for one caller after
// 5db49ff0 removed the lease machinery it used to belong to.
type claudeKeyringDeleter func(ctx context.Context, service, account string) error

// dropSessionClaudeKeychain removes the item a finished session minted, which
// nothing else can name or remove. docs/native-claude-credentials.md.
func dropSessionClaudeKeychain(
	ctx context.Context,
	drop claudeKeyringDeleter,
	sessionHome, home string,
) (bool, error) {
	if strings.TrimSpace(sessionHome) == "" {
		return false, nil
	}
	service := nativeClaudeKeychainService(home, filepath.Join(sessionHome, ".claude"))
	// The shared host item is the login every other session reads. A session
	// that landed on the default service borrowed it rather than minting one.
	if service == claudeCredentialService {
		return false, nil
	}
	err := drop(ctx, service, nativeClaudeKeychainAccount())
	if errors.Is(err, errClaudeKeyringUnsupported) ||
		errors.Is(err, errClaudeKeyringNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// claudeCredentialOutlives refuses to retire a token worth more than the
// candidate, which is how the retired per-session harvest lost rotations.
func claudeCredentialOutlives(candidate []byte, target string, now time.Time) (bool, error) {
	candidateWorth := claudeCredentialWorthOf(candidate, now)
	// A payload carrying no token repairs nothing, however it is stamped.
	if !candidateWorth.tokens {
		return false, nil
	}
	current, err := os.ReadFile(target)
	if os.IsNotExist(err) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("read %s: %w", target, err)
	}
	return candidateWorth.outranks(claudeCredentialWorthOf(current, now)), nil
}

// claudeCredentialWorth ranks a payload by what makes it usable rather than by
// one stamp (teable:coilyco-flight-deck/agentic-os#7258).
type claudeCredentialWorth struct {
	tokens      bool
	refreshable bool
	expiresAt   int64
}

// outranks decides on token presence first, then on a refresh token that has
// not itself lapsed. The access stamp only separates two equal payloads.
func (worth claudeCredentialWorth) outranks(other claudeCredentialWorth) bool {
	if worth.tokens != other.tokens {
		return worth.tokens
	}
	if worth.refreshable != other.refreshable {
		return worth.refreshable
	}
	return worth.expiresAt > other.expiresAt
}

// usable reports whether a launch could start logged in: a token to present,
// and either an unlapsed one or a refresh that can mint one.
func (worth claudeCredentialWorth) usable(now time.Time) bool {
	return worth.tokens && (worth.refreshable || worth.expiresAt > now.UnixMilli())
}

// claudeCredentialWorthOf reads the four fields worth depends on. An unparsable
// payload is worth nothing rather than reading as a zero stamp.
func claudeCredentialWorthOf(payload []byte, now time.Time) claudeCredentialWorth {
	var envelope struct {
		OAuth struct {
			AccessToken      string `json:"accessToken"`
			RefreshToken     string `json:"refreshToken"`
			ExpiresAt        int64  `json:"expiresAt"`
			RefreshExpiresAt int64  `json:"refreshTokenExpiresAt"`
		} `json:"claudeAiOauth"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return claudeCredentialWorth{}
	}
	access := strings.TrimSpace(envelope.OAuth.AccessToken) != ""
	refresh := strings.TrimSpace(envelope.OAuth.RefreshToken) != ""
	return claudeCredentialWorth{
		tokens:      access || refresh,
		refreshable: refresh && envelope.OAuth.RefreshExpiresAt > now.UnixMilli(),
		expiresAt:   envelope.OAuth.ExpiresAt,
	}
}
