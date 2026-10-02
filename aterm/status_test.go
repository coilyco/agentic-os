package main

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
	"time"
)

const promptScript = `printf '\033[?2004h'; printf 'Bash command\r\n  git push\r\nDo you want to proceed?\r\n1. Yes\r\n2. No\r\nEsc to cancel\r\n'; exec cat`

func TestStatusReportsAPromptWithItsTextAndNeverTypes(t *testing.T) {
	testDaemon(t)
	tc := dialTest(t)
	tc.spawn("status-test", "eng-platform", "Beetle-Ox", promptScript)
	var got sessionStatus
	waitFor(t, "a prompt status", 8*time.Second, func() bool {
		status, err := sessionStatusOf("status-test", 0)
		if err != nil {
			t.Fatalf("status: %v", err)
		}
		got = status
		return status.State == statePrompt
	})
	if !strings.Contains(strings.Join(got.Prompt, "\n"), "Do you want to proceed?") {
		t.Fatalf("the prompt text is missing: %q", got.Prompt)
	}
	if len(got.Screen) == 0 || got.Screen[len(got.Screen)-1] != "Esc to cancel" {
		t.Fatalf("the screen should end at the prompt's last row: %q", got.Screen)
	}
	if got.InputSeconds != -1 {
		t.Fatalf("nobody typed, yet input_seconds = %d", got.InputSeconds)
	}
	if got.Rows != 24 || got.Cols != 200 {
		t.Fatalf("screen is %dx%d, want the 24x200 the session was spawned at", got.Rows, got.Cols)
	}
	again, _ := sessionStatusOf("status-test", 0)
	if again.InputSeconds != -1 || again.Pending != 0 {
		t.Fatalf("a status must change nothing: %+v", again)
	}
}

func TestStatusRefusesAnUnknownOrAmbiguousTarget(t *testing.T) {
	testDaemon(t)
	tc := dialTest(t)
	tc.spawn("amb-one", "eng-platform", "Beetle-Ox", "exec cat")
	tc.spawn("amb-two", "eng-platform", "Beetle-Ox", "exec cat")
	if _, err := sessionStatusOf("nobody-here", 0); err == nil || !strings.Contains(err.Error(), "no live session") {
		t.Fatalf("an unknown target should name the live sessions: %v", err)
	}
	if _, err := sessionStatusOf("eng-platform", 0); err == nil || !strings.Contains(err.Error(), "matches") {
		t.Fatalf("a role with two sessions should refuse and name them: %v", err)
	}
}

func TestStatusLinesAreBoundedAndTailed(t *testing.T) {
	rows := make([]string, 300)
	for index := range rows {
		rows[index] = "row"
	}
	if got := len(tailLines(rows, 0)); got != defaultStatusLines {
		t.Fatalf("default is %d rows, want %d", got, defaultStatusLines)
	}
	if got := len(tailLines(rows, 9999)); got != maxStatusLines {
		t.Fatalf("an oversize ask gave %d rows, want %d", got, maxStatusLines)
	}
	if got := tailLines([]string{"a", "b", "c"}, 2); !reflect.DeepEqual(got, []string{"b", "c"}) {
		t.Fatalf("tail: %q", got)
	}
	if got := tailLines(nil, 5); got == nil || len(got) != 0 {
		t.Fatalf("an empty screen should be an empty list, not null: %#v", got)
	}
}

func TestPromptLinesKeepContextAroundTheFirstMatch(t *testing.T) {
	lines := []string{"a", "b", "c", "d", "e", "Do you want to proceed?", "1. Yes", "2. No"}
	got := promptLines("claude", lines)
	if len(got) == 0 || got[len(got)-1] != "2. No" || got[0] != "b" {
		t.Fatalf("got %q", got)
	}
	if promptLines("claude", []string{"nothing here"}) != nil {
		t.Fatal("no prompt should give no lines")
	}
}

func TestWriteStatusNamesWhatHoldsTheSession(t *testing.T) {
	var out bytes.Buffer
	err := writeStatus(&out, sessionStatus{
		Name: "s", State: statePrompt, QuietSeconds: 90, InputSeconds: -1, Drafted: true, Pending: 2,
		Rows: 24, Cols: 80, Prompt: []string{"Do you want to proceed?"}, Screen: []string{"x"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"s  prompt  quiet 1m30s, no input yet, Kai drafting, 2 pending", "-- prompt --", "Do you want to proceed?", "last 1 of 24x80"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output lacks %q:\n%s", want, out.String())
		}
	}
}
