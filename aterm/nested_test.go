package main

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestLaunchRefusesFromInsideANativeSessionShadow(t *testing.T) {
	var spawns []recordedSpawn
	t.Setenv(nativeSessionEnv, "ds74")
	out, err := runAtermRaw(t, stubDeps(t, &spawns, true), "platform", "claude")
	if err == nil {
		t.Fatalf("a nested launch should refuse\n%s", out)
	}
	if code := exitCodeFor(err); code != exitNested {
		t.Fatalf("exit code = %d, want %d: %v", code, exitNested, err)
	}
	if !strings.Contains(err.Error(), "ds74") {
		t.Fatalf("the refusal should name the session it refused from: %v", err)
	}
	if len(spawns) != 0 {
		t.Fatalf("a refused launch should open no window, got %d", len(spawns))
	}
}

// The refusal has to stay clear of the read-only paths, because --dry-run is
// how the identity card is inspected from inside a session. agentic-os#1456
func TestDryRunStillWorksFromInsideANativeSessionShadow(t *testing.T) {
	var spawns []recordedSpawn
	t.Setenv(nativeSessionEnv, "ds74")
	if _, err := runAtermRaw(t, stubDeps(t, &spawns, true), "--dry-run", "platform", "claude"); err != nil {
		t.Fatalf("dry run inside a shadow: %v", err)
	}
	if _, err := runAtermRaw(t, stubDeps(t, &spawns, true), "--list"); err != nil {
		t.Fatalf("list inside a shadow: %v", err)
	}
	if len(spawns) != 0 {
		t.Fatalf("neither reads a window open, got %d", len(spawns))
	}
}

func TestLaunchProceedsWithoutANativeSessionShadow(t *testing.T) {
	var spawns []recordedSpawn
	t.Setenv(nativeSessionEnv, "")
	if _, err := runAtermRaw(t, stubDeps(t, &spawns, true), "platform", "claude"); err != nil {
		t.Fatalf("top-level launch: %v", err)
	}
	if len(spawns) != 1 {
		t.Fatalf("spawned %d windows, want 1", len(spawns))
	}
}

func TestDoctorFailsInsideANativeSessionShadow(t *testing.T) {
	var spawns []recordedSpawn
	t.Setenv(nativeSessionEnv, "ds74")
	out, err := runAtermRaw(t, stubDeps(t, &spawns, true), "doctor", "--json")
	if err == nil {
		t.Fatalf("doctor should fail the launch chain from inside a shadow\n%s", out)
	}
	check, ok := doctorVerdicts(t, out)["native session"]
	if !ok {
		t.Fatalf("doctor reported no native session check\n%s", out)
	}
	if check.Status != doctorFail {
		t.Fatalf("native session status = %q, want %q", check.Status, doctorFail)
	}
	if !strings.Contains(check.Detail, "ds74") {
		t.Fatalf("the check should name the session: %q", check.Detail)
	}
}

func TestDoctorPassesTheNativeSessionCheckAtTheTopLevel(t *testing.T) {
	var spawns []recordedSpawn
	out, err := runAterm(t, stubDeps(t, &spawns, true), "doctor", "--json")
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, out)
	}
	if check := doctorVerdicts(t, out)["native session"]; check.Status != doctorOK {
		t.Fatalf("native session status = %q, want %q", check.Status, doctorOK)
	}
}

func TestRefusalUnwrapsToItsExitCode(t *testing.T) {
	t.Setenv(nativeSessionEnv, "ds74")
	err := refuseNestedLaunch()
	if err == nil {
		t.Fatal("refuseNestedLaunch returned nil inside a shadow")
	}
	var typed exitError
	if !errors.As(err, &typed) {
		t.Fatalf("the refusal should carry an exit code: %v", err)
	}
}

// A bare invocation used to die on the working directory, which is the same
// root cause reported as its symptom. agentic-os#1460
func TestRefusalOutrunsTheWorkingDirectoryCheck(t *testing.T) {
	var spawns []recordedSpawn
	t.Setenv(nativeSessionEnv, "ds74")
	t.Setenv(defaultWorkingEnvVar, "")
	t.Setenv("HOME", filepath.Join(t.TempDir(), "no-projects-here"))
	stdout := &bytes.Buffer{}
	command := newCommand(stubDeps(t, &spawns, true))
	command.Writer = stdout
	err := command.Run(context.Background(), []string{"aterm", "platform", "claude"})
	if err == nil {
		t.Fatalf("a nested launch should refuse\n%s", stdout.String())
	}
	if code := exitCodeFor(err); code != exitNested {
		t.Fatalf("exit code = %d, want %d: %v", code, exitNested, err)
	}
}
