package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

func stubLookPath(name string) (string, error) { return "/stub/" + name, nil }

func TestWrapChildPutsTheHarnessBehindVt(t *testing.T) {
	child := []string{"aos", "_native-shadow", "--harness", "claude", "--", "agent-compose", "launch"}
	got, wrapped := wrapChild(child, true, stubLookPath, &bytes.Buffer{})
	trampoline := []string{"/stub/vt", "-S", "/bin/sh", "-c", renameThenRun, "sh", vibeTunnelSessionName, "/stub/vt"}
	want := append(trampoline, child...)
	// The shadow child carries its own `--`, and vt has to hand it on untouched.
	if !wrapped || !slices.Equal(got, want) {
		t.Fatalf("argv = %v, want %v", got, want)
	}
}

func TestWrapChildLeavesTheArgvAloneWhenOptedOut(t *testing.T) {
	notice := &bytes.Buffer{}
	got, wrapped := wrapChild([]string{"agent-compose", "launch"}, false, stubLookPath, notice)
	if wrapped || !slices.Equal(got, []string{"agent-compose", "launch"}) || notice.Len() != 0 {
		t.Fatalf("an opt-out should be silent and unchanged: %v %q", got, notice.String())
	}
}

func TestWrapChildFallsOpenWhenVtIsMissing(t *testing.T) {
	notice := &bytes.Buffer{}
	missing := func(string) (string, error) { return "", fmt.Errorf("not found") }
	got, wrapped := wrapChild([]string{"agent-compose", "launch"}, true, missing, notice)
	// A sidecar the operator may not have installed must not cost the session.
	if wrapped || !slices.Equal(got, []string{"agent-compose", "launch"}) {
		t.Fatalf("a missing vt should still launch the harness: %v", got)
	}
	if !strings.Contains(notice.String(), "not in VibeTunnel") {
		t.Fatalf("the missing wrapper should be named: %q", notice.String())
	}
}

func TestWithoutVibeTunnelSessionDropsOnlyTheMarker(t *testing.T) {
	got := withoutVibeTunnelSession([]string{
		"PATH=/usr/bin", vibeTunnelSessionEnv + "=fwd_1", "VIBETUNNEL_LOG_LEVEL=debug", "HOME=/h",
	})
	want := []string{"PATH=/usr/bin", "VIBETUNNEL_LOG_LEVEL=debug", "HOME=/h"}
	if !slices.Equal(got, want) {
		t.Fatalf("environ = %v, want %v", got, want)
	}
}

func TestLaunchPlanWrapsInVibeTunnelUnlessOptedOut(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		args    []string
		wrapped bool
	}{
		{"default", nil, true},
		{"opted out", []string{"--no-vibetunnel"}, false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			var spawns []recordedSpawn
			args := append([]string{"--dry-run", "--json"}, testCase.args...)
			out, err := runAterm(t, stubDeps(t, &spawns, true), append(args, "platform", "claude")...)
			if err != nil {
				t.Fatalf("dry run: %v", err)
			}
			var plan launchPlan
			if err := json.Unmarshal([]byte(out), &plan); err != nil {
				t.Fatalf("decode plan: %v", err)
			}
			if plan.VibeTunnel != testCase.wrapped {
				t.Fatalf("plan.VibeTunnel = %v, want %v", plan.VibeTunnel, testCase.wrapped)
			}
			if slices.Contains(plan.Arguments, "--vibetunnel") != testCase.wrapped {
				t.Fatalf("session flag mismatch in %v", plan.Arguments)
			}
			// The plan's child stays the compose command, so the release check
			// that asserts against it does not learn about the wrapper.
			if slices.Contains(plan.Child, "vt") {
				t.Fatalf("the wrapper belongs to the session stage: %v", plan.Child)
			}
		})
	}
}

func TestRunSessionRunsTheChildThroughVtNamedAndKeepsItsExitCode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fixture is a POSIX script")
	}
	dir := t.TempDir()
	record, titles := filepath.Join(dir, "vt-args"), filepath.Join(dir, "vt-titles")
	// Stands in for vt: records a rename, and otherwise runs the command the way
	// `vt -S` does.
	script := "#!/bin/sh\nif [ \"$1\" = title ]; then echo \"$*\" >> " + titles + "; exit 0; fi\n" +
		"echo \"$*\" > " + record + "\nshift\nexec \"$@\"\n"
	if err := os.WriteFile(filepath.Join(dir, "vt"), []byte(script), 0o700); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	code := runSession(
		sessionOptions{VibeTunnel: true, Argv: []string{"/bin/sh", "-c", "exit 7"}},
		strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{},
	)
	if code != 7 {
		t.Fatalf("exit code = %d, want 7", code)
	}
	raw, err := os.ReadFile(record)
	if err != nil {
		t.Fatalf("the session never reached vt: %v", err)
	}
	if got := strings.TrimSpace(string(raw)); !strings.HasSuffix(got, "sh aterm "+filepath.Join(dir, "vt")+" /bin/sh -c exit 7") {
		t.Fatalf("vt argv = %q", got)
	}
	// The name is set from inside the session, before the harness starts.
	if got, _ := os.ReadFile(titles); strings.TrimSpace(string(got)) != "title aterm" {
		t.Fatalf("the session was not named: %q", got)
	}
}

func TestParseSessionArgsTakesTheVibeTunnelFlag(t *testing.T) {
	options, err := parseSessionArgs([]string{"--vibetunnel", "--", "agent-compose"})
	if err != nil || !options.VibeTunnel {
		t.Fatalf("--vibetunnel should be recognized: %v %+v", err, options)
	}
}

func TestSpawnWindowDropsTheVibeTunnelSessionMarker(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fixture is a POSIX script")
	}
	for _, name := range []string{"outside a shadow", "inside a shadow"} {
		t.Run(name, func(t *testing.T) {
			clearShadowEnv(t)
			if strings.HasPrefix(name, "inside") {
				root := t.TempDir()
				t.Setenv(nativeSessionEnv, "zr87")
				t.Setenv(nativeSessionRootEnv, root)
				t.Setenv(canonicalHomeEnv, "/home/canonical")
				t.Setenv(canonicalProjectsEnv, "/home/canonical/projects")
			}
			t.Setenv(vibeTunnelSessionEnv, "fwd_1")
			dump := filepath.Join(t.TempDir(), "environ")
			terminal := filepath.Join(t.TempDir(), "terminal")
			body := "#!/bin/sh\nenv > " + dump + "\n"
			if err := os.WriteFile(terminal, []byte(body), 0o700); err != nil {
				t.Fatalf("write: %v", err)
			}
			if err := spawnWindow(terminal, nil); err != nil {
				t.Fatalf("spawn: %v", err)
			}
			// spawnWindow detaches and returns at its own deadline, so the
			// terminal may not have written yet on a loaded host.
			var raw []byte
			var err error
			for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
				if raw, err = os.ReadFile(dump); err == nil && len(raw) > 0 {
					break
				}
				time.Sleep(20 * time.Millisecond)
			}
			if err != nil {
				t.Fatalf("the terminal never ran: %v", err)
			}
			// vt refuses a nested session, so a marker that reached the window
			// would kill the harness before it started.
			if strings.Contains(string(raw), vibeTunnelSessionEnv+"=") {
				t.Fatalf("the window inherited the marker:\n%s", raw)
			}
		})
	}
}

func TestDoctorNamesVibeTunnelAsAWarningNeverAFailure(t *testing.T) {
	var spawns []recordedSpawn
	deps := stubDeps(t, &spawns, true)
	out, err := runDoctorCommand(t, deps, "--json")
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, out)
	}
	if check := doctorVerdicts(t, out)["vibetunnel"]; check.Status != doctorOK {
		t.Fatalf("a running server should pass: %+v", check)
	}
	inner := deps.lookPath
	deps.lookPath = func(name string) (string, error) {
		if name == vibeTunnelBin {
			return "", fmt.Errorf("not found")
		}
		return inner(name)
	}
	out, err = runDoctorCommand(t, deps, "--json")
	if err != nil {
		t.Fatalf("a missing vt must not fail the launch chain: %v\n%s", err, out)
	}
	if check := doctorVerdicts(t, out)["vibetunnel"]; check.Status != doctorWarn {
		t.Fatalf("a missing vt should warn: %+v", check)
	}
}
