package main

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
)

// A caller distinguishing a stale role from a missing binary should branch on
// the code rather than grep the prose.
func TestFailuresCarryTheirOwnExitCode(t *testing.T) {
	cases := map[string]struct {
		argv []string
		want int
	}{
		"off-roster role":   {[]string{"--dry-run", "engineer", "codex"}, exitOffRoster},
		"off-roster seat":   {[]string{"--dry-run", "platform", "goose"}, exitOffRoster},
		"unlaunchable seat": {[]string{"--dry-run", "frontend", "penpot"}, exitOffRoster},
		"unsafe slug":       {[]string{"--dry-run", "Platform"}, exitUsage},
		"json on launch":    {[]string{"--json", "platform", "claude"}, exitUsage},
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			var spawns []recordedSpawn
			_, err := runAterm(t, stubDeps(t, &spawns, true), want.argv...)
			if err == nil {
				t.Fatal("expected a refusal")
			}
			if got := exitCodeFor(err); got != want.want {
				t.Fatalf("exit = %d, want %d (%v)", got, want.want, err)
			}
		})
	}
}

func TestMissingDependencyAndSpawnFailureSplitApart(t *testing.T) {
	var spawns []recordedSpawn
	_, err := runAterm(t, stubDeps(t, &spawns, true),
		"--agent-compose-bin", "/missing/agent-compose", "platform", "claude")
	if err == nil || exitCodeFor(err) != exitMissing {
		t.Fatalf("a missing binary should exit %d: %v", exitMissing, err)
	}
	failing := stubDeps(t, &spawns, true)
	failing.spawn = func(context.Context, string, ...string) error {
		return fmt.Errorf("kitty refused its own arguments")
	}
	_, err = runAterm(t, failing, "platform", "claude")
	if err == nil || exitCodeFor(err) != exitSpawn {
		t.Fatalf("a spawn failure should exit %d: %v", exitSpawn, err)
	}
}

// A generic failure keeps the old code, so nothing silently changes meaning.
func TestUnclassifiedFailuresStayAtOne(t *testing.T) {
	if got := exitCodeFor(fmt.Errorf("something broke")); got != exitFailure {
		t.Fatalf("exit = %d, want %d", got, exitFailure)
	}
	wrapped := fmt.Errorf("open the window: %w", withExit(exitSpawn, fmt.Errorf("kitty died")))
	if got := exitCodeFor(wrapped); got != exitSpawn {
		t.Fatalf("wrapping lost the code: %d", got)
	}
}

// The operator form has to name the identity and the colors without the caller
// reading JSON, and the machine form has to stay available behind --json.
func TestDryRunRendersForAPersonAndStillEmitsJSON(t *testing.T) {
	var spawns []recordedSpawn
	human, err := runAterm(t, stubDeps(t, &spawns, true), "--dry-run", "platform", "claude")
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if strings.HasPrefix(strings.TrimSpace(human), "{") {
		t.Fatal("--dry-run should render for a person by default")
	}
	for _, want := range []string{
		"Angie", "claude", "acting", "#9c8b31", "tenacious", "grounded", "_native-shadow",
	} {
		if !strings.Contains(human, want) {
			t.Fatalf("the rendered plan should name %q:\n%s", want, human)
		}
	}
	machine, err := runAterm(t, stubDeps(t, &spawns, true), "--dry-run", "--json", "platform", "claude")
	if err != nil {
		t.Fatalf("dry run --json: %v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(machine), "{") {
		t.Fatalf("--dry-run --json should stay machine-readable: %s", machine)
	}
	if len(spawns) != 0 {
		t.Fatalf("a dry run opened %d window(s)", len(spawns))
	}
}

func TestRenderPlanSkipsEmptyFields(t *testing.T) {
	document := platformOverlay(t)
	plan, err := buildLaunchPlan(
		document,
		launchRequest{Role: "platform", Seat: "claude", Expression: "acting", TerminalBin: "kitty"},
		t.TempDir(), "/stub/aterm", "/stub/agent-compose", "/stub/aos", false,
	)
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	out := &bytes.Buffer{}
	if err := renderPlan(out, document, plan); err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(out.String(), "not a checkout") {
		t.Fatalf("an absent workspace should say so:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "none, the window runs Agent Compose directly and skips daily host convergence") {
		t.Fatalf("an unshadowed plan should say so:\n%s", out.String())
	}
}
