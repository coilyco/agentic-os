// Claude Code namespaces its macOS Keychain credential by a digest of
// CLAUDE_CONFIG_DIR. See docs/native-claude-credentials.md.

package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
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

// seedCanonicalClaudeCredential writes the Keychain login to the canonical file
// when absent. Never overwrites. See docs/native-claude-credentials.md.
func seedCanonicalClaudeCredential(
	ctx context.Context,
	read claudeKeyringReader,
	home string,
) (bool, error) {
	target := canonicalClaudeCredentialPath(home)
	switch _, err := os.Lstat(target); {
	case err == nil:
		return false, nil
	case !os.IsNotExist(err):
		return false, fmt.Errorf("inspect %s: %w", target, err)
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
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return false, fmt.Errorf("create %s: %w", filepath.Dir(target), err)
	}
	if err := os.WriteFile(target, secret, 0o600); err != nil {
		return false, fmt.Errorf("write %s: %w", target, err)
	}
	return true, nil
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
