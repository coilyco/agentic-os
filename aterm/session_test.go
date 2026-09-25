package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestParseSessionArgsSplitsOnTheFirstDash(t *testing.T) {
	options, err := parseSessionArgs(
		[]string{"--hold", "--", "aos", "_native-shadow", "--", "agent-compose"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !options.Hold {
		t.Fatal("--hold should be recognized")
	}
	// The child's own `--` has to survive, or the shadow loses its command.
	if strings.Join(options.Argv, " ") != "aos _native-shadow -- agent-compose" {
		t.Fatalf("argv = %v", options.Argv)
	}
}

func TestParseSessionArgsRejectsGarbage(t *testing.T) {
	for _, argv := range [][]string{
		{}, {"--hold"}, {"--nope", "--", "x"}, {"--card"}, {"--card", "not-base64!", "--", "x"},
	} {
		if _, err := parseSessionArgs(argv); err == nil {
			t.Fatalf("%v should be rejected", argv)
		}
	}
}

func TestRunSessionPassesTheChildExitCodeThrough(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fixture uses a POSIX shell")
	}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code := runSession(
		sessionOptions{Argv: []string{"/bin/sh", "-c", "exit 7"}},
		strings.NewReader(""),
		stdout,
		stderr,
	)
	if code != 7 {
		t.Fatalf("exit code = %d, want 7", code)
	}
	// A failing launch always holds, whatever --hold said, so the window keeps
	// the reason on screen instead of closing over it.
	if !strings.Contains(stdout.String(), "Session failed") {
		t.Fatalf("a failure should be announced: %q", stdout.String())
	}
}

func TestRunSessionStaysQuietOnACleanExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fixture uses a POSIX shell")
	}
	stdout := &bytes.Buffer{}
	code := runSession(sessionOptions{Argv: []string{"/bin/sh", "-c", "exit 0"}}, strings.NewReader(""), stdout, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if stdout.String() != "" {
		t.Fatalf("a clean exit should print nothing: %q", stdout.String())
	}
}

func TestRunSessionHoldsACleanExitOnRequest(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fixture uses a POSIX shell")
	}
	stdout := &bytes.Buffer{}
	if code := runSession(sessionOptions{Hold: true, Argv: []string{"/bin/sh", "-c", "exit 0"}}, strings.NewReader(""), stdout, &bytes.Buffer{}); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if !strings.Contains(stdout.String(), "Session ended") {
		t.Fatalf("--hold should announce the end: %q", stdout.String())
	}
}

func TestRunSessionReportsAChildThatCannotStart(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code := runSession(sessionOptions{Argv: []string{"/nonexistent/harness"}}, strings.NewReader(""), stdout, stderr)
	if code == 0 {
		t.Fatal("a child that cannot start is a failure")
	}
	if !strings.Contains(stderr.String(), "/nonexistent/harness") {
		t.Fatalf("the failure should name the child: %q", stderr.String())
	}
}

func TestRunSessionRejectsAnEmptyCommand(t *testing.T) {
	if code := runSession(sessionOptions{}, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}); code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
}

func TestUpdateClaudeSkipsWhenClaudeIsNotOnPath(t *testing.T) {
	called := false
	stderr := &bytes.Buffer{}
	updateClaude(
		func(string) (string, error) { return "", errors.New("not found") },
		func(string, ...string) ([]byte, error) { called = true; return nil, nil },
		stderr,
	)
	if called {
		t.Fatal("update should not run when claude is not on PATH")
	}
	if stderr.String() != "" {
		t.Fatalf("a missing claude is not a failure to report: %q", stderr.String())
	}
}

func TestUpdateClaudeStaysQuietOnSuccess(t *testing.T) {
	var gotName string
	var gotArgs []string
	stderr := &bytes.Buffer{}
	updateClaude(
		func(string) (string, error) { return "/usr/local/bin/claude", nil },
		func(name string, args ...string) ([]byte, error) {
			gotName, gotArgs = name, args
			return []byte("Already on the latest version.\n"), nil
		},
		stderr,
	)
	if gotName != "claude" || strings.Join(gotArgs, " ") != "update" {
		t.Fatalf("command = %s %v, want claude update", gotName, gotArgs)
	}
	if stderr.String() != "" {
		t.Fatalf("a clean update should print nothing: %q", stderr.String())
	}
}

func TestUpdateClaudeReportsAFailureWithoutPanicking(t *testing.T) {
	stderr := &bytes.Buffer{}
	updateClaude(
		func(string) (string, error) { return "/usr/local/bin/claude", nil },
		func(string, ...string) ([]byte, error) {
			return []byte("network unreachable\n"), errors.New("exit status 1")
		},
		stderr,
	)
	if !strings.Contains(stderr.String(), "claude update") || !strings.Contains(stderr.String(), "network unreachable") {
		t.Fatalf("failure should name the command and its detail: %q", stderr.String())
	}
}

func TestUnderCardRunsTheStepsBesideTheCardAndHoldsTheirNotices(t *testing.T) {
	const span = 300 * time.Millisecond
	stderr := &bytes.Buffer{}
	early := ""
	began := time.Now()
	underCard(stderr, func() {
		time.Sleep(span)
		early = stderr.String()
	}, func(notice io.Writer) {
		time.Sleep(span)
		fmt.Fprintln(notice, "first")
	}, func(notice io.Writer) {
		// Finishes ahead of the first step, which must not reorder the notices.
		time.Sleep(span / 3)
		fmt.Fprintln(notice, "second")
	})
	// In series this is three spans. Beside the card it is one.
	if elapsed := time.Since(began); elapsed > 2*span {
		t.Fatalf("the steps ran after the card: %v", elapsed)
	}
	if early != "" {
		t.Fatalf("a notice landed inside the card: %q", early)
	}
	if stderr.String() != "first\nsecond\n" {
		t.Fatalf("notices = %q, want them in step order", stderr.String())
	}
}
