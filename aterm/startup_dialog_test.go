package main

import (
	"strings"
	"testing"
	"time"
)

// dialogSeat is a seat with a screen and no PTY, for the readiness rules.
func dialogSeat(seat string) *ptySession {
	return &ptySession{
		seat: seat, started: time.Now().Add(-time.Hour), scr: newScreen(5, 40),
		clients: map[*conn]bool{},
	}
}

func TestAStartupDialogHoldsMessagesUntilThePromptMarkOrAFreshWait(t *testing.T) {
	s := dialogSeat("claude")
	now := time.Now()
	if !s.pastStartupDialog(now) {
		t.Fatal("a seat that showed no dialog has nothing to wait past")
	}
	s.startupCard, s.cardUp = true, true
	if s.pastStartupDialog(now) {
		t.Fatal("a dialog still up is not past")
	}
	s.cardUp, s.cardLost = false, now.Add(-5*time.Second)
	if s.pastStartupDialog(now) {
		t.Fatal("a dialog gone five seconds, with no prompt mark yet, is not past")
	}
	s.cardLost = now.Add(-promptWait - time.Second)
	if !s.pastStartupDialog(now) {
		t.Fatal("a dialog gone for the whole wait is past, for a harness that words its mark differently")
	}
	s.cardLost, s.promptSeen = now, now
	if !s.pastStartupDialog(now) {
		t.Fatal("the prompt mark showing is past, whenever the dialog left")
	}
}

func TestOnlyACardBeforeThePromptMarkIsAStartupDialog(t *testing.T) {
	card := []byte("Do you want to proceed?\r\n1. Yes\r\nEsc to cancel")
	early := dialogSeat("claude")
	early.scr.write(card)
	early.noteCard()
	if !early.startupCard {
		t.Fatal("a card drawn before the prompt mark is a startup dialog")
	}
	late := dialogSeat("claude")
	late.promptSeen = time.Now()
	late.scr.write(card)
	late.noteCard()
	if late.startupCard {
		t.Fatal("a card after the prompt is an ordinary permission card, held by the card rule")
	}
	codex := dialogSeat("codex")
	codex.scr.write([]byte("Would you like to run this command?\r\nyes, proceed"))
	codex.noteCard()
	if codex.startupCard || !codex.pastStartupDialog(time.Now()) {
		t.Fatal("a harness with no prompt mark has no startup rule, or every later card would delay its messages")
	}
}

// A dialog's wake: the fallback clock had run out while it was up, so the message was
// typed as the dialog cleared, before the harness drew its prompt (COI-3343).
func TestAMessageWaitsForThePromptAfterAStartupDialogClears(t *testing.T) {
	saved := promptWait
	promptWait = 5 * time.Second
	t.Cleanup(func() { promptWait = saved })
	card := `printf 'Allow external imports?\r\n1. No\r\n2. Yes\r\nEnter to confirm \xc2\xb7 Esc to cancel\r\n'`
	script := `printf 'READY\033[?2004h\n'; ` + card + `; sleep 3; printf '\033[4A\033[J'; sleep 3; printf 'Try "x"\r'; exec cat`
	token, _, target := spawnPair(t, script)
	target.until("Esc to cancel")
	if state := sendAs(t, token, "scientist", "ping"); state.State != "held" && state.State != "queued" {
		t.Fatalf("state = %+v, want it waiting behind the dialog", state)
	}
	target.until(`Try "x"`)
	if strings.Contains(target.output.String(), "[from eng-platform") {
		t.Fatalf("the message was typed before the harness drew its prompt:\n%q", target.output.String())
	}
	target.until("[from eng-platform Beetle-Ox] ping")
}
