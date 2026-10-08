package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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

func TestShowsCardReadsEveryRecordedAskUserQuestionPage(t *testing.T) {
	for name, screen := range recordedScreens(t) {
		t.Run(name, func(t *testing.T) {
			// An idle-* screen is the control: a claude with nothing to answer.
			if want := !strings.HasPrefix(name, "idle-"); showsCard("claude", screen) != want {
				t.Fatalf("showsCard = %v, want %v for:\n%s", !want, want, screen)
			}
		})
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
