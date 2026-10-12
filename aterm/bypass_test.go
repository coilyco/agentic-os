package main

import (
	"bytes"
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestHarnessArgsAskForBypassUnlessTheCallerOrKeepSaysOtherwise(t *testing.T) {
	cases := map[string]struct {
		mode  string
		extra []string
		want  []string
	}{
		"default asks for bypass":      {defaultPermissionMode, nil, []string{"--permission-mode", "bypassPermissions"}},
		"extras follow the mode":       {defaultPermissionMode, []string{"--model", "opus"}, []string{"--permission-mode", "bypassPermissions", "--model", "opus"}},
		"keep leaves the stored mode":  {keepPermissionMode, []string{"--model", "opus"}, []string{"--model", "opus"}},
		"empty asks for nothing":       {"", nil, nil},
		"caller mode wins":             {defaultPermissionMode, []string{"--permission-mode", "plan"}, []string{"--permission-mode", "plan"}},
		"caller inline mode wins":      {defaultPermissionMode, []string{"--permission-mode=plan"}, []string{"--permission-mode=plan"}},
		"caller skip flag wins":        {defaultPermissionMode, []string{"--dangerously-skip-permissions"}, []string{"--dangerously-skip-permissions"}},
		"another mode is passed as is": {"acceptEdits", nil, []string{"--permission-mode", "acceptEdits"}},
	}
	for name, c := range cases {
		if got := harnessArgs(c.mode, c.extra); !slices.Equal(got, c.want) {
			t.Errorf("%s: got %v, want %v", name, got, c.want)
		}
	}
}

func bypassFixture() (bypassDeps, *[]string) {
	var calls []string
	entry := func(name string) ledgerEntry {
		return ledgerEntry{Name: name, Role: "r", Seat: "claude", Conversation: "conv-" + name}
	}
	deps := bypassDeps{
		views: func() ([]sessionView, error) {
			return []sessionView{
				{Name: "b-idle", Role: "r", Seat: "claude", State: "idle"},
				{Name: "a-busy", Role: "r", Seat: "claude", State: "busy"},
				{Name: "c-me", Role: "r", Seat: "claude", State: "idle"},
				{Name: "d-draft", Role: "r", Seat: "claude", State: "idle", Drafted: true},
				{Name: "e-none", Role: "r", Seat: "claude", State: "idle"},
				{Name: "f-done", Role: "r", Seat: "claude", State: "idle"},
				{Name: "g-codex", Role: "r", Seat: "codex", State: "idle"},
			}, nil
		},
		entries: func() []ledgerEntry {
			return []ledgerEntry{entry("a-busy"), entry("b-idle"), entry("c-me"), entry("d-draft"), entry("f-done"),
				{Name: "e-none", Seat: "claude"}}
		},
		inBypass:  func(name string) bool { return name == "f-done" },
		closeSeat: func(name string) error { calls = append(calls, "close "+name); return nil },
		gone:      func(string) bool { return true },
		relaunch:  func(e ledgerEntry) error { calls = append(calls, "relaunch "+e.Conversation); return nil },
		self:      "c-me",
	}
	return deps, &calls
}

func TestBypassMovesOnlyTheIdleSeatsThatAreNotAlreadyThereAndNamesTheRest(t *testing.T) {
	deps, calls := bypassFixture()
	out := &bytes.Buffer{}
	if err := runBypass(out, deps, false); err != nil {
		t.Fatal(err)
	}
	if want := []string{"close b-idle", "relaunch conv-b-idle"}; !slices.Equal(*calls, want) {
		t.Fatalf("calls = %v, want %v", *calls, want)
	}
	for _, line := range []string{
		"moved   b-idle", "kept    a-busy: state is busy", "kept    c-me: this is the calling session",
		"kept    d-draft: holds a draft", "kept    e-none: no recorded conversation", "kept    f-done: already in bypass",
	} {
		if !strings.Contains(out.String(), line) {
			t.Errorf("output lacks %q:\n%s", line, out)
		}
	}
	if strings.Contains(out.String(), "g-codex") {
		t.Errorf("a codex seat is not this verb's business:\n%s", out)
	}
}

func TestBypassBusyFlagIncludesMidTurnSeatsButNeverTheCaller(t *testing.T) {
	deps, calls := bypassFixture()
	deps.includeBusy = true
	if err := runBypass(&bytes.Buffer{}, deps, false); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(*calls, "close a-busy") || slices.Contains(*calls, "close c-me") {
		t.Fatalf("calls = %v", *calls)
	}
}

func TestBypassDryRunClosesNothing(t *testing.T) {
	deps, calls := bypassFixture()
	out := &bytes.Buffer{}
	if err := runBypass(out, deps, true); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 0 || !strings.Contains(out.String(), "would move b-idle") {
		t.Fatalf("calls = %v, output:\n%s", *calls, out)
	}
}

func TestBypassReportsAFailedCloseAndAFailedReopenAndExitsNonZero(t *testing.T) {
	deps, _ := bypassFixture()
	deps.closeSeat = func(string) error { return errors.New("holds a draft") }
	out := &bytes.Buffer{}
	if err := runBypass(out, deps, false); err == nil || !strings.Contains(out.String(), "FAILED  b-idle: close: holds a draft") {
		t.Fatalf("err = %v, output:\n%s", err, out)
	}
	deps, _ = bypassFixture()
	deps.relaunch = func(ledgerEntry) error { return errors.New("no window") }
	out.Reset()
	if err := runBypass(out, deps, false); err == nil || !strings.Contains(out.String(), "`aterm resume b-idle` retries it") {
		t.Fatalf("a closed seat that was not reopened must say how to get it back:\n%s", out)
	}
}

func TestBypassWithNoDaemonReportsNoSeats(t *testing.T) {
	deps, _ := bypassFixture()
	deps.views = func() ([]sessionView, error) { return nil, nil }
	out := &bytes.Buffer{}
	if err := runBypass(out, deps, false); err != nil || !strings.Contains(out.String(), "no live claude seats") {
		t.Fatalf("err = %v, output:\n%s", err, out)
	}
}
