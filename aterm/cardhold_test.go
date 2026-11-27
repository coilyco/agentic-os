package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// What a real claude showed in an untrusted directory, read off a PTY on 2026-10-02.
const trustDialog = `Accessing workspace:
/private/var/folders/x/T/clprobe-b15iu5up

Quick safety check: Is this a project you created or one you trust? (Like your
own code, a well-known open source project, or work from your team).

Claude Code'll be able to read, edit, and execute files here.

> 1. No, exit
  2. Yes, I trust this folder

Enter to confirm · Esc to cancel`

// A live claude screen with its prompt up, and nothing to answer.
const claudePrompt = `Claude Code v2.1.287
> Try "refactor <filepath>"
────────────────
⏵⏵ auto mode on (shift+tab to cycle) · ← for agents`

func TestShowsCardReadsTheTrustDialogAndNotAnOrdinaryPrompt(t *testing.T) {
	if !showsCard("claude", trustDialog) {
		t.Fatal("the trust dialog defaults to exit, so a message must not type into it")
	}
	if showsCard("claude", claudePrompt) {
		t.Fatal("an ordinary claude prompt is not a card")
	}
	if showsCard("goose", trustDialog) {
		t.Fatal("a seat with no card patterns is never held on a guess")
	}
}

func TestAMessageWaitsBehindACardAndLandsWhenItClears(t *testing.T) {
	card := `printf 'Do you want to proceed?\r\n1. Yes\r\nEsc to cancel\r\n'`
	token, _, target := spawnPair(t, `printf 'READY\033[?2004h\n'; `+card+`; sleep 5; printf '\033[3A\033[J'; exec cat`)
	target.until("Esc to cancel")
	state := sendAs(t, token, "scientist", "ping")
	if state.State != "held" || !strings.Contains(state.Reason, "prompt card") {
		t.Fatalf("state = %+v, want held behind the card, since Enter would answer it", state)
	}
	if strings.Contains(target.output.String(), "[from eng-platform") {
		t.Fatalf("the message was typed into the card:\n%q", target.output.String())
	}
	// The card clears at about 5s, and the message lands after.
	target.until("[from eng-platform Beetle-Ox] ping")
}

// A repaint erases a dialog before drawing it again. A message queued in that gap was
// typed into the dialog's free-text row, so Kai's answer was a notice (COI-3243).
func TestAMessageStaysHeldThroughACardRepaintGap(t *testing.T) {
	card := func(row string) string {
		return `printf 'Do you want to proceed?\r\n` + row + `\r\nEsc to cancel\r\n'`
	}
	erase := `printf '\033[3A\033[J'`
	script := `printf 'READY\033[?2004h\n'; ` + card("1. Yes") + `; sleep 3; ` + erase + `; sleep 0.8; ` +
		card("2. Again") + `; sleep 4; ` + erase + `; exec cat`
	token, _, target := spawnPair(t, script)
	target.until("1. Yes")
	if state := sendAs(t, token, "scientist", "ping"); state.State != "held" {
		t.Fatalf("state = %+v, want held behind the card", state)
	}
	target.until("2. Again")
	if strings.Contains(target.output.String(), "[from eng-platform") {
		t.Fatalf("the message was typed while the card was being repainted:\n%q", target.output.String())
	}
	target.until("[from eng-platform Beetle-Ox] ping")
}

// Claude Code 2.1.293 screens read off a PTY on 2026-10-08, one per AskUserQuestion page,
// plus an idle control (idle-*). Recapture after a Claude Code upgrade.
const claudeScreens = "testdata/claude-screens"

func recordedScreens(t *testing.T) map[string]string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(claudeScreens, "*.txt"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no recorded screens under %s: %v", claudeScreens, err)
	}
	screens := map[string]string{}
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		screens[filepath.Base(path)] = string(raw)
	}
	return screens
}

// recordedKind is the state a recorded screen shows, named by its file's prefix. The
// AskUserQuestion pages (q*, review*) and the folder-trust dialog are choice dialogs.
func recordedKind(name string) string {
	prefixes := map[string]string{
		"idle-": stateIdle, "busy-": stateBusy, "permission-": screenPermission,
		"mcp-": screenMCP, "resume-": screenResume, "login-": screenLogin,
	}
	for prefix, kind := range prefixes {
		if strings.HasPrefix(name, prefix) {
			return kind
		}
	}
	return screenChoice
}

// COI-3788: each recorded Claude Code screen names its own state, quiet for a minute.
func TestEveryRecordedClaudeScreenReadsAsItsState(t *testing.T) {
	for name, screen := range recordedScreens(t) {
		t.Run(name, func(t *testing.T) {
			want := recordedKind(name)
			if got := detectScreen("claude", strings.Split(screen, "\n"), true, time.Minute); got != want {
				t.Fatalf("detectScreen = %q, want %q for:\n%s", got, want, screen)
			}
			held := want != stateIdle && want != stateBusy
			if showsCard("claude", screen) != held {
				t.Fatalf("showsCard = %v, want %v, since Enter on a held screen would answer it", !held, held)
			}
		})
	}
}

// A screen no signature knows reads unknown, never a guess, and stays idle in the
// coarse state as it always read.
func TestAnUnseenScreenReadsUnknown(t *testing.T) {
	unseen := [][]string{
		{"  Resume session", "", "  ✶ Loading sessions…"},
		{"Some program printed this and stopped."},
	}
	for _, lines := range unseen {
		if got := detectScreen("claude", lines, true, time.Minute); got != screenUnknown {
			t.Fatalf("detectScreen(%q) = %q, want unknown", lines, got)
		}
		if got := classify("claude", lines, true, time.Minute); got != stateIdle {
			t.Fatalf("classify(%q) = %q, want idle", lines, got)
		}
	}
	if got := detectScreen("goose", []string{"anything"}, true, time.Minute); got != screenUnknown {
		t.Fatalf("a harness with no idle mark matched reads unknown, got %q", got)
	}
}

// A phone-sized terminal scrolls a menu's heading away, and its footer still names it.
func TestAMenuWhoseHeadingScrolledAwayStillReads(t *testing.T) {
	for name, want := range map[string]string{"resume-picker.txt": screenResume, "mcp-menu.txt": screenMCP} {
		raw, err := os.ReadFile(filepath.Join(claudeScreens, name))
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
		tail := lines[len(lines)-8:]
		if got := detectScreen("claude", tail, true, time.Minute); got != want {
			t.Fatalf("%s, last 8 rows: detectScreen = %q, want %q:\n%s", name, got, want, strings.Join(tail, "\n"))
		}
	}
}

// A sign-in screen is why a seat never gets ready, so it reads before readiness does.
func TestALoginScreenReadsBeforeTheSeatIsReady(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(claudeScreens, "login-method.txt"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(raw), "\n")
	if got := detectScreen("claude", lines, false, time.Minute); got != screenLogin {
		t.Fatalf("detectScreen = %q, want login", got)
	}
	if got := classify("claude", lines, false, time.Minute); got != stateStarting {
		t.Fatalf("classify = %q, the coarse state still says starting", got)
	}
	if got := detectScreen("claude", []string{"booting"}, false, time.Minute); got != stateStarting {
		t.Fatalf("detectScreen = %q, want starting", got)
	}
}

// The review page draws no footer, and Enter on it submits the answers, so a message
// typed there would answer for Kai.
func TestAMessageWaitsBehindTheAskUserQuestionReviewPage(t *testing.T) {
	review, err := filepath.Abs(filepath.Join(claudeScreens, "review.txt"))
	if err != nil {
		t.Fatal(err)
	}
	script := `printf 'READY\033[?2004h\n'; cat ` + review + `; sleep 5; printf '\033[2J\033[H'; exec cat`
	token, _, target := spawnPair(t, script)
	target.until("Ready to submit your answers?")
	state := sendAs(t, token, "scientist", "ping")
	if state.State != "held" || !strings.Contains(state.Reason, "prompt card") {
		t.Fatalf("state = %+v, want held behind the review page, since Enter would submit it", state)
	}
	if strings.Contains(target.output.String(), "[from eng-platform") {
		t.Fatalf("the message was typed into the review page:\n%q", target.output.String())
	}
	target.until("[from eng-platform Beetle-Ox] ping")
}

// COI-2653 gate: three sends to a seat at a card are each held, nothing is typed into the
// card, and all three arrive in order once it clears.
func TestThreeMessagesWaitBehindACardAndLandInOrder(t *testing.T) {
	card := `printf 'Do you want to proceed?\r\n1. Yes\r\nEsc to cancel\r\n'`
	token, _, target := spawnPair(t, `printf 'READY\033[?2004h\n'; `+card+`; sleep 8; printf '\033[3A\033[J'; exec cat`)
	target.until("Esc to cancel")
	for _, word := range []string{"one", "two", "three"} {
		state := sendWaiting(t, token, "scientist", word, 1)
		if state.State != "held" || !strings.Contains(state.Reason, "prompt card") {
			t.Fatalf("%s: state = %+v, want held behind the card", word, state)
		}
	}
	if strings.Contains(target.output.String(), "[from eng-platform") {
		t.Fatalf("a message was typed into the card:\n%q", target.output.String())
	}
	target.until("[from eng-platform Beetle-Ox] three")
	out := target.output.String()
	first, second, third := strings.Index(out, "] one"), strings.Index(out, "] two"), strings.Index(out, "] three")
	if first < 0 || first > second || second > third {
		t.Fatalf("messages out of order (%d, %d, %d):\n%q", first, second, third, out)
	}
}
