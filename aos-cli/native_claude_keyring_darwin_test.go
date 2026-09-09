//go:build darwin

package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// A stub cannot cover the reader reaching a real Keychain item under the
// service name the digest builds. docs/native-claude-credentials.md
func TestHarvestSessionClaudeKeychainReadsARealKeychainItem(t *testing.T) {
	home := t.TempDir()
	sessionHome := t.TempDir()
	configDir := filepath.Join(sessionHome, ".claude")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	// Derived rather than hand-written, so drift in the digest fails here too.
	service := nativeClaudeKeychainService(home, configDir)
	account := nativeClaudeKeychainAccount()
	canonical := canonicalClaudeCredentialPath(home)
	if err := os.MkdirAll(filepath.Dir(canonical), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(canonical, stampedCredential(100), 0o600); err != nil {
		t.Fatal(err)
	}

	// Negative control: with nothing under that service a real read must report
	// a miss, so the positive case below cannot pass on an unconditional write.
	harvested, err := harvestSessionClaudeKeychain(
		context.Background(), readClaudeKeyring, sessionHome, home)
	if err != nil {
		t.Fatalf("harvest before the item exists: %v", err)
	}
	if harvested {
		t.Fatal("harvest reported a write with no Keychain item present")
	}

	addScratchKeychainItem(t, service, account, string(stampedCredential(200)))

	harvested, err = harvestSessionClaudeKeychain(
		context.Background(), readClaudeKeyring, sessionHome, home)
	if err != nil {
		t.Fatalf("harvest: %v", err)
	}
	if !harvested {
		t.Fatal("harvest reported no write with the item in place")
	}
	body, err := os.ReadFile(canonical)
	if err != nil {
		t.Fatalf("read canonical: %v", err)
	}
	if string(body) != string(stampedCredential(200)) {
		t.Fatalf("canonical body = %q, want the harvested token", body)
	}
}

// The expiry comparison is what stops a reaped session retiring a live token,
// and it has to hold over the real reader rather than only over a stub.
func TestHarvestSessionClaudeKeychainKeepsTheLongerLivedRealToken(t *testing.T) {
	home := t.TempDir()
	sessionHome := t.TempDir()
	configDir := filepath.Join(sessionHome, ".claude")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	canonical := canonicalClaudeCredentialPath(home)
	if err := os.MkdirAll(filepath.Dir(canonical), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(canonical, stampedCredential(300), 0o600); err != nil {
		t.Fatal(err)
	}
	addScratchKeychainItem(
		t,
		nativeClaudeKeychainService(home, configDir),
		nativeClaudeKeychainAccount(),
		string(stampedCredential(200)),
	)

	harvested, err := harvestSessionClaudeKeychain(
		context.Background(), readClaudeKeyring, sessionHome, home)
	if err != nil {
		t.Fatalf("harvest: %v", err)
	}
	if harvested {
		t.Fatal("harvest retired a token that outlives the candidate")
	}
	body, err := os.ReadFile(canonical)
	if err != nil {
		t.Fatalf("read canonical: %v", err)
	}
	if string(body) != string(stampedCredential(300)) {
		t.Fatalf("canonical body = %q, want the longer-lived token", body)
	}
}

// addScratchKeychainItem writes one synthetic item to the login keychain and
// removes it at test end. The payload is not a credential.
func addScratchKeychainItem(t *testing.T, service, account, secret string) {
	t.Helper()
	add := exec.Command(
		"/usr/bin/security", "add-generic-password",
		"-s", service, "-a", account, "-w", secret,
		// Only the binary the production reader shells out to needs access.
		"-T", "/usr/bin/security",
	)
	if output, err := add.CombinedOutput(); err != nil {
		t.Fatalf("add keychain item %q: %v: %s", service, err, output)
	}
	t.Cleanup(func() {
		remove := exec.Command(
			"/usr/bin/security", "delete-generic-password",
			"-s", service, "-a", account,
		)
		if output, err := remove.CombinedOutput(); err != nil {
			t.Errorf("delete keychain item %q: %v: %s", service, err, output)
		}
	})
}
