package main

import (
	"os"
	"strings"
	"testing"
	"time"
)

func fastEnters(t *testing.T) {
	t.Helper()
	gap, enters, park := enterGap, claudeEnters, parkEnterGap
	t.Cleanup(func() { enterGap, claudeEnters, parkEnterGap = gap, enters, park })
	enterGap, claudeEnters, parkEnterGap = 100*time.Millisecond, 2, 150*time.Millisecond
}

// COI-2619: a seat mid-turn held its Enter back past the retries, and the daemon called
// the message unsent although the seat took it later.
func TestMessageThatABusySeatTakesLateIsDeliveredNotFailed(t *testing.T) {
	fastEnters(t)
	token, target := fakeBusyBoxSeat(t, 200, 0, 0, 3500)
	state := sendWaiting(t, token, "eng-junior", "do the thing", 30)
	if state.State != "delivered" {
		t.Fatalf("state = %+v, want delivered once the busy seat takes the Enter", state)
	}
	target.until("SUBMITTED [from eng-platform Beetle-Ox] do the thing\r\n")
}

// While one message waits in the box, the next waits behind it, so two never share a box.
func TestMessageBehindAParkedOneIsHeldThenDeliveredInOrder(t *testing.T) {
	fastEnters(t)
	token, target := fakeBusyBoxSeat(t, 200, 0, 0, 3500)
	first := sendWaiting(t, token, "eng-junior", "first thing", 1)
	if first.State == "failed" {
		t.Fatalf("first = %+v, want it not failed while the seat works", first)
	}
	sendWaiting(t, token, "eng-junior", "second thing", 30)
	target.until("SUBMITTED [from eng-platform Beetle-Ox] second thing\r\n")
	out := target.output.String()
	one := strings.Index(out, "SUBMITTED [from eng-platform Beetle-Ox] first thing\r\n")
	two := strings.Index(out, "SUBMITTED [from eng-platform Beetle-Ox] second thing\r\n")
	if one < 0 || two < 0 || one > two {
		t.Fatalf("first at %d, second at %d, want both and first before second:\n%s", one, two, out)
	}
}

// A seat that never takes the Enter still gets a verdict, after parkWindow.
func TestParkedMessageTheSeatNeverTakesFailsAfterTheWindow(t *testing.T) {
	fastEnters(t)
	defer func(w time.Duration) { parkWindow = w }(parkWindow)
	parkWindow = 1500 * time.Millisecond
	token, target := fakeBusyBoxSeat(t, 200, 0, 0, 600000)
	state := sendWaiting(t, token, "eng-junior", "do the thing", 30)
	if state.State != "failed" || !strings.Contains(state.Reason, "not Enter") {
		t.Fatalf("state = %+v, want failed naming the unsent text", state)
	}
	if strings.Contains(target.output.String(), "SUBMITTED") {
		t.Fatal("the fake took an Enter it should have dropped")
	}
}

// Enters pressed into a box a person has typed in would submit her words too.
func TestParkedMessageFailsWhenAPersonTypesIntoTheBox(t *testing.T) {
	fastEnters(t)
	token, target := fakeBusyBoxSeat(t, 200, 0, 0, 600000)
	typed := make(chan struct{})
	go func() {
		defer close(typed)
		waitTextInBox(t, "eng-junior-beetle-ox", "do the thing")
		_ = target.c.write(frame{Type: "input", Session: "eng-junior-beetle-ox", Data: []byte("hold on")})
	}()
	state := sendWaiting(t, token, "eng-junior", "do the thing", 30)
	<-typed
	if state.State != "failed" || !strings.Contains(state.Reason, "someone typed") {
		t.Fatalf("state = %+v, want failed because a person typed into the box", state)
	}
}

// waitTextInBox returns once nothing is pending and the screen shows text: a message
// typed into the box and waiting there.
func waitTextInBox(t *testing.T, session, text string) {
	t.Helper()
	for deadline := time.Now().Add(40 * time.Second); ; time.Sleep(200 * time.Millisecond) {
		asker, err := dialDaemon(false)
		if err != nil {
			t.Error(err)
			return
		}
		reply, err := asker.request(frame{Type: "status", Target: session, Lines: 40})
		_ = asker.Close()
		if err == nil && reply.Status.Pending == 0 && strings.Contains(strings.Join(reply.Status.Screen, "\n"), text) {
			return
		}
		if time.Now().After(deadline) {
			t.Errorf("%q never sat in %s's box", text, session)
			return
		}
	}
}

// Claude Code 2.1.295 screen read off a PTY on 2026-10-09: three messages queued behind a
// long Bash call, each drawn above the spinner, and the box showing only the queue hint.
func TestRecordedScreenWithQueuedMessagesInAToolCallIsASentMessage(t *testing.T) {
	raw, err := os.ReadFile("testdata/claude-transcripts/queued-in-tool-call.txt")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	text := "[from eng-platform Beetle-Ox] COI-2619 scratch seat, new task. Wait three minutes inside " +
		"ONE foreground Bash tool call, so you stay inside a single long-running tool call. Choose " +
		"the command yourself. When it returns, reply with the single word waited3."
	readable, holds := claudeBoxHolds(lines, text)
	if !readable || holds {
		t.Fatalf("readable=%v holds=%v, want the queue hint in the box read as an empty box", readable, holds)
	}
	if got := echoCount(lines, text); got != 1 {
		t.Fatalf("a message queued behind a tool call counted %d, want 1", got)
	}
	if got := classify("claude", lines, true, 10*time.Second); got != stateBusy {
		t.Fatalf("state = %q, want busy from the spinner alone", got)
	}
}
