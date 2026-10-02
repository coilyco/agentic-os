package main

import (
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
