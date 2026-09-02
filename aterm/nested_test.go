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

func clearShadowEnv(t *testing.T) {
	t.Helper()
	for _, variable := range []string{
		nativeSessionEnv, nativeSessionRootEnv, nativeSessionProjectsEnv,
		canonicalHomeEnv, canonicalProjectsEnv,
	} {
		t.Setenv(variable, "")
	}
}

func shadowEnv(t *testing.T) canonicalLaunch {
	t.Helper()
	launch := canonicalLaunch{
		Session:  "ds74",
		Root:     "/tmp/aos/native/ds74",
		Home:     "/Users/kai",
		Projects: "/Users/kai/projects",
	}
	t.Setenv(nativeSessionEnv, launch.Session)
	t.Setenv(nativeSessionRootEnv, launch.Root)
	t.Setenv(nativeSessionProjectsEnv, launch.Root+"/projects")
	t.Setenv(canonicalHomeEnv, launch.Home)
	t.Setenv(canonicalProjectsEnv, launch.Projects)
	return launch
}

// The refusal survives only for an aos too old to publish the canonical values,
// which is the case the guard was built for. agentic-os#1460
func TestLaunchProceedsInsideAShadowThatPublishesTheCanonicalValues(t *testing.T) {
	var spawns []recordedSpawn
	shadowEnv(t)
	if _, err := runAtermRaw(t, stubDeps(t, &spawns, true), "platform", "claude"); err != nil {
		t.Fatalf("launch inside a complete shadow: %v", err)
	}
	if len(spawns) != 1 {
		t.Fatalf("spawned %d windows, want 1", len(spawns))
	}
}

func TestCanonicalEnvironReplacesTheShadowWithWhatItReplaced(t *testing.T) {
	launch := shadowEnv(t)
	environ := []string{
		"HOME=/tmp/aos/native/ds74/home",
		"USERPROFILE=/tmp/aos/native/ds74/home",
		"XDG_CONFIG_HOME=/tmp/aos/native/ds74/home/.config",
		"CLAUDE_CONFIG_DIR=/tmp/aos/native/ds74/home/.claude",
		nativeSessionEnv + "=ds74",
		nativeSessionRootEnv + "=/tmp/aos/native/ds74",
		nativeSessionProjectsEnv + "=/tmp/aos/native/ds74/projects",
		canonicalHomeEnv + "=/Users/kai",
		canonicalProjectsEnv + "=/Users/kai/projects",
		"PATH=/tmp/aos/native/ds74/home/.local/bin:/opt/homebrew/bin:/usr/bin",
		"EDITOR=vim",
		"NOTE=/tmp/aos/native/ds74 is where the shadow lives",
		childSessionEnv + "=1",
	}
	got := map[string]string{}
	for _, entry := range canonicalEnviron(environ, launch) {
		name, value, _ := strings.Cut(entry, "=")
		got[name] = value
	}
	if got["HOME"] != launch.Home {
		t.Fatalf("HOME = %q, want %q", got["HOME"], launch.Home)
	}
	if got[defaultWorkingEnvVar] != launch.Projects {
		t.Fatalf("%s = %q, want %q", defaultWorkingEnvVar, got[defaultWorkingEnvVar], launch.Projects)
	}
	for _, gone := range []string{
		"USERPROFILE", "XDG_CONFIG_HOME", "CLAUDE_CONFIG_DIR",
		nativeSessionEnv, nativeSessionRootEnv, nativeSessionProjectsEnv,
		canonicalHomeEnv, canonicalProjectsEnv, childSessionEnv,
	} {
		if _, still := got[gone]; still {
			t.Fatalf("%s should not reach the new session, got %q", gone, got[gone])
		}
	}
	if got["PATH"] != "/opt/homebrew/bin:/usr/bin" {
		t.Fatalf("PATH kept a shadow entry: %q", got["PATH"])
	}
	if got["EDITOR"] != "vim" {
		t.Fatalf("an unrelated variable was dropped: %q", got["EDITOR"])
	}
	// A value that mentions the root without being a path under it is kept,
	// because the rule is a path match rather than a substring match.
	if got["NOTE"] == "" {
		t.Fatal("a value merely mentioning the root should survive")
	}
}

func TestCanonicalEnvironStaysOutOfTheWayWhenItCannotHelp(t *testing.T) {
	clearShadowEnv(t)
	if environ := canonicalEnviron([]string{"HOME=/Users/kai"}, readCanonicalLaunch()); environ != nil {
		t.Fatalf("a top-level launch should inherit unchanged, got %v", environ)
	}
	t.Setenv(nativeSessionEnv, "ds74")
	if environ := canonicalEnviron([]string{"HOME=/x"}, readCanonicalLaunch()); environ != nil {
		t.Fatalf("an incomplete shadow has nothing to build from, got %v", environ)
	}
}

func TestDoctorPassesInsideAShadowThatPublishesTheCanonicalValues(t *testing.T) {
	var spawns []recordedSpawn
	shadowEnv(t)
	out, err := runAtermRaw(t, stubDeps(t, &spawns, true), "doctor", "--json")
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, out)
	}
	if check := doctorVerdicts(t, out)["native session"]; check.Status != doctorOK {
		t.Fatalf("native session status = %q, want %q", check.Status, doctorOK)
	}
}

// The launch resolves its own working directory before it builds the child's
// environment, and inside a shadow both fallbacks point at an empty home.
func TestDefaultWorkingDirectoryPrefersTheCanonicalProjectsRoot(t *testing.T) {
	launch := shadowEnv(t)
	t.Setenv(defaultWorkingEnvVar, "/tmp/aos/native/ds74/home/projects")
	if got := defaultWorkingDirectory(); got != launch.Projects {
		t.Fatalf("default working directory = %q, want %q", got, launch.Projects)
	}
	clearShadowEnv(t)
	t.Setenv(defaultWorkingEnvVar, "/Users/kai/projects")
	if got := defaultWorkingDirectory(); got != "/Users/kai/projects" {
		t.Fatalf("a top-level launch should keep %s, got %q", defaultWorkingEnvVar, got)
	}
}
