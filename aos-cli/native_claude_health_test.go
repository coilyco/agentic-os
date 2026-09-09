package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// credentialAt renders the envelope the harness writes, at both expiries.
func credentialAt(access, refresh int64) []byte {
	return fmt.Appendf(nil,
		`{"claudeAiOauth":{"accessToken":"t","expiresAt":%d,"refreshTokenExpiresAt":%d}}`,
		access, refresh)
}

func writeCanonical(t *testing.T, home string, payload []byte) {
	t.Helper()
	target := canonicalClaudeCredentialPath(home)
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, payload, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestInspectCanonicalClaudeCredentialNamesEveryState(t *testing.T) {
	now := time.Date(2026, 9, 9, 22, 0, 0, 0, time.UTC)
	past := now.Add(-time.Hour).UnixMilli()
	future := now.Add(time.Hour).UnixMilli()

	for _, testCase := range []struct {
		name    string
		payload []byte
		absent  bool
		want    string
		healthy bool
	}{
		{name: "absent", absent: true, want: claudeCredentialMissing},
		{
			name:    "the trap",
			payload: credentialAt(0, 0),
			want:    claudeCredentialUnstamped,
		},
		{
			name:    "access live",
			payload: credentialAt(future, future),
			want:    claudeCredentialLive,
			healthy: true,
		},
		{
			name:    "access expired, refresh live",
			payload: credentialAt(past, future),
			want:    claudeCredentialRefreshable,
			healthy: true,
		},
		{
			name:    "both expired",
			payload: credentialAt(past, past),
			want:    claudeCredentialStale,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			home := t.TempDir()
			if !testCase.absent {
				writeCanonical(t, home, testCase.payload)
			}
			health := inspectCanonicalClaudeCredential(home, now)
			if health.State != testCase.want {
				t.Fatalf("state = %q, want %q (%s)", health.State, testCase.want, health.Detail)
			}
			if health.Healthy() != testCase.healthy {
				t.Fatalf("healthy = %v, want %v", health.Healthy(), testCase.healthy)
			}
			if health.Path != canonicalClaudeCredentialPath(home) {
				t.Fatalf("path = %q", health.Path)
			}
		})
	}
}

// The whole point is a report safe to paste, so no token material may appear.
func TestInspectCanonicalClaudeCredentialLeaksNoToken(t *testing.T) {
	home := t.TempDir()
	writeCanonical(t, home, []byte(
		`{"claudeAiOauth":{"accessToken":"sk-secret-value","refreshToken":"rt-secret",`+
			`"expiresAt":1,"refreshTokenExpiresAt":2}}`))

	health := inspectCanonicalClaudeCredential(home, time.Now())
	rendered := health.Line() + health.Path
	for _, secret := range []string{"sk-secret-value", "rt-secret"} {
		if strings.Contains(rendered, secret) {
			t.Fatalf("rendered health carries %q: %s", secret, rendered)
		}
	}
}

// A missing stamp and a corrupt payload are the same verdict, since neither
// gives the launcher anything to compare.
func TestInspectCanonicalClaudeCredentialTreatsGarbageAsUnstamped(t *testing.T) {
	home := t.TempDir()
	writeCanonical(t, home, []byte("not json at all"))

	if state := inspectCanonicalClaudeCredential(home, time.Now()).State; state != claudeCredentialUnstamped {
		t.Fatalf("state = %q, want %q", state, claudeCredentialUnstamped)
	}
}
