package main

import (
	"context"
	"strings"
	"testing"
)

// acompose once took the integrated capability flags and ran, so a standalone
// container could read as Ward-brokered. See agentic-os#810.
func runAOS(t *testing.T, args ...string) error {
	t.Helper()
	cmd := newCommandWithDefaults("aos", launchDefaults{})
	return cmd.Run(context.Background(), append([]string{"aos"}, args...))
}

func assertRefused(t *testing.T, err error, wantFlags ...string) {
	t.Helper()
	if err == nil {
		t.Fatal("acompose accepted an integrated capability flag it does not honor")
	}
	for _, flag := range wantFlags {
		if !strings.Contains(err.Error(), flag) {
			t.Fatalf("error does not name %s: %v", flag, err)
		}
	}
	// The error has to point somewhere, or the operator retries the same shape.
	if !strings.Contains(err.Error(), "aos --warded --guarded --composed --role") {
		t.Fatalf("error does not show the canonical root launch: %v", err)
	}
}

func TestAcomposeRefusesCapabilityFlagsAfterTheToken(t *testing.T) {
	err := runAOS(t,
		"acompose", "--warded", "--guarded", "--composed",
		"--role", "tpm", "--agent", "codex", "--", "codex")

	assertRefused(t, err, "--warded", "--guarded", "--agent")
}

func TestAcomposeRefusesCapabilityFlagsBeforeTheToken(t *testing.T) {
	// Parser ordering must not reopen the bypass.
	err := runAOS(t,
		"--warded", "--guarded", "--composed",
		"--role", "tpm", "--agent", "codex", "acompose", "--", "codex")

	assertRefused(t, err, "--warded", "--guarded", "--agent")
}

func TestAcomposeRefusesEachCapabilityFlagAlone(t *testing.T) {
	for _, flag := range []string{"--warded", "--guarded"} {
		if err := runAOS(t, "acompose", flag, "--role", "tpm", "--", "codex"); err == nil {
			t.Fatalf("acompose accepted %s alone", flag)
		}
	}
	err := runAOS(t, "acompose", "--agent", "codex", "--role", "tpm", "--", "codex")
	assertRefused(t, err, "--agent")
}

func TestAcomposeStopsBeforeItNeedsARoleOrACommand(t *testing.T) {
	// Fail closed means before materialization, so the refusal precedes the
	// argument checks that would otherwise mask it.
	err := runAOS(t, "acompose", "--warded")

	assertRefused(t, err, "--warded")
	if strings.Contains(err.Error(), "needs --role") {
		t.Fatalf("argument validation ran before the capability refusal: %v", err)
	}
}

func TestComposedAloneIsNotRefused(t *testing.T) {
	// --composed is a no-op on every path, so refusing it would break the
	// standalone form for no gain. It reaches the role check instead.
	err := runAOS(t, "acompose", "--composed")

	if err == nil || !strings.Contains(err.Error(), "needs --role") {
		t.Fatalf("--composed should fall through to argument validation, got: %v", err)
	}
}

func TestAcomposeCheckinStillHonorsAgent(t *testing.T) {
	// Checkin resolves its layout FROM --agent, so the refusal must not reach
	// it. No --role, so this stops at validation rather than launching.
	err := runAOS(t, "acompose-checkin", "--agent", "codex")

	if err == nil || !strings.Contains(err.Error(), "needs --role") {
		t.Fatalf("acompose-checkin should reach argument validation, got: %v", err)
	}
}
