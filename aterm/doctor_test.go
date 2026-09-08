package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func runDoctorCommand(t *testing.T, deps commandDeps, argv ...string) (string, error) {
	t.Helper()
	return runAterm(t, deps, append([]string{"doctor"}, argv...)...)
}

func doctorVerdicts(t *testing.T, out string) map[string]doctorCheck {
	t.Helper()
	var report doctorReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("doctor --json did not emit JSON: %v\n%s", err, out)
	}
	if report.Format != doctorFormat {
		t.Fatalf("format = %q", report.Format)
	}
	verdicts := map[string]doctorCheck{}
	for _, check := range report.Checks {
		verdicts[check.Name] = check
	}
	return verdicts
}

func TestDoctorPassesOnAHostThatCanLaunch(t *testing.T) {
	var spawns []recordedSpawn
	out, err := runDoctorCommand(t, stubDeps(t, &spawns, true), "--json")
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, out)
	}
	verdicts := doctorVerdicts(t, out)
	for _, name := range []string{
		"working directory", "agent-compose", "roster", "overlay",
		"identity vocabulary", "terminal", "aos", "session shadow", "launch profiles",
	} {
		check, found := verdicts[name]
		if !found {
			t.Fatalf("doctor did not run the %q check: %s", name, out)
		}
		if check.Status == doctorFail {
			t.Fatalf("%s failed on a healthy host: %s", name, check.Detail)
		}
	}
	if len(spawns) != 0 {
		t.Fatalf("doctor opened %d window(s)", len(spawns))
	}
}

// An unleased launch is correct behavior and invisible, which is the single
// line the issue calls the highest-value one. agentic-os#1257
func TestDoctorNamesAnUnleasedLaunchWithoutFailing(t *testing.T) {
	var spawns []recordedSpawn
	out, err := runDoctorCommand(t, stubDeps(t, &spawns, false), "--json")
	if err != nil {
		t.Fatalf("an unleased host still launches, so doctor should pass: %v", err)
	}
	shadow := doctorVerdicts(t, out)["session shadow"]
	if shadow.Status != doctorWarn {
		t.Fatalf("session shadow = %+v, want an unleased warning", shadow)
	}
	// Both losses, because reporting only the worktree understates the skip.
	for _, want := range []string{"unleased", "shares this checkout", "daily host convergence"} {
		if !strings.Contains(shadow.Detail, want) {
			t.Fatalf("session shadow detail %q is missing %q", shadow.Detail, want)
		}
	}
}

func TestDoctorFailsAndSaysWhichLinkBroke(t *testing.T) {
	cases := map[string]struct {
		argv  []string
		check string
	}{
		"no terminal":      {[]string{"--terminal-bin", "/missing/kitty"}, "terminal"},
		"no agent-compose": {[]string{"--agent-compose-bin", "/missing/agent-compose"}, "agent-compose"},
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			var spawns []recordedSpawn
			out, err := runDoctorCommand(t, stubDeps(t, &spawns, true), append(want.argv, "--json")...)
			if err == nil {
				t.Fatal("doctor should refuse when a link is broken")
			}
			if exitCodeFor(err) != exitFailure {
				t.Fatalf("exit = %d, want %d", exitCodeFor(err), exitFailure)
			}
			if got := doctorVerdicts(t, out)[want.check]; got.Status != doctorFail {
				t.Fatalf("%s = %+v, want a failure", want.check, got)
			}
		})
	}
}

// aos is optional, so its absence degrades the launch rather than stopping it.
func TestDoctorTreatsAMissingAOSAsADegradedLaunch(t *testing.T) {
	var spawns []recordedSpawn
	out, err := runDoctorCommand(t, stubDeps(t, &spawns, true), "--aos-bin", "/missing/aos", "--json")
	if err != nil {
		t.Fatalf("a missing aos should not fail doctor: %v", err)
	}
	verdicts := doctorVerdicts(t, out)
	for _, name := range []string{"aos", "session shadow", "launch profiles"} {
		if verdicts[name].Status != doctorWarn {
			t.Fatalf("%s = %+v, want a warning", name, verdicts[name])
		}
	}
}

// A launch profile pointing at a seat the role cannot launch is the failure
// this check exists for, and it is silent everywhere else.
func TestDoctorFailsAProfileThatNamesAnUnlaunchableSeat(t *testing.T) {
	var spawns []recordedSpawn
	deps := stubDeps(t, &spawns, true)
	inner := deps.output
	deps.output = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "_launch-agent" {
			return []byte("penpot\n"), nil
		}
		return inner(ctx, name, args...)
	}
	out, err := runDoctorCommand(t, deps, "--json")
	if err == nil {
		t.Fatal("doctor should fail a profile naming an unlaunchable seat")
	}
	profiles := doctorVerdicts(t, out)["launch profiles"]
	if profiles.Status != doctorFail || !strings.Contains(profiles.Detail, "penpot") {
		t.Fatalf("launch profiles = %+v", profiles)
	}
}

func TestDoctorRendersForAPersonByDefault(t *testing.T) {
	var spawns []recordedSpawn
	out, err := runDoctorCommand(t, stubDeps(t, &spawns, true))
	if err != nil {
		t.Fatalf("doctor: %v", err)
	}
	if strings.HasPrefix(strings.TrimSpace(out), "{") {
		t.Fatal("doctor should render for a person by default")
	}
	if !strings.Contains(out, "aterm can open a window on this host.") {
		t.Fatalf("doctor should end with a verdict: %s", out)
	}
}

// The role positional and the subcommand share the first argument slot.
func TestDoctorDoesNotShadowARolePositional(t *testing.T) {
	var spawns []recordedSpawn
	out, err := runAterm(t, stubDeps(t, &spawns, true), "--dry-run", "--json", "platform", "claude")
	if err != nil {
		t.Fatalf("a role positional should still launch: %v", err)
	}
	if !strings.Contains(out, fmt.Sprintf("%q: %q", "role", "platform")) {
		t.Fatalf("the role positional was lost: %s", out)
	}
}
