package main

import (
	"testing"
	"time"
)

func stateSeat(seat string) *ptySession {
	return &ptySession{
		seat: seat, started: time.Now().Add(-time.Hour), scr: newScreen(5, 40),
		clients: map[*conn]bool{},
	}
}

func TestCurrentStateKeepsWhenItBeganAndStartsOverOnAFlip(t *testing.T) {
	s := stateSeat("goose")
	t0 := time.Now()
	if got := s.currentState(t0, nil, true, 10*time.Second); got != stateIdle || !s.stateSince.Equal(t0) {
		t.Fatalf("first reading = %q since %v, want idle since %v", got, s.stateSince, t0)
	}
	s.currentState(t0.Add(5*time.Second), nil, true, 15*time.Second)
	if !s.stateSince.Equal(t0) {
		t.Fatalf("the same state moved its start to %v, want %v", s.stateSince, t0)
	}
	t1 := t0.Add(8 * time.Second)
	if got := s.currentState(t1, nil, true, 0); got != stateBusy || !s.stateSince.Equal(t1) {
		t.Fatalf("after output = %q since %v, want busy since %v", got, s.stateSince, t1)
	}
}

func TestAStartingSeatCountsFromItsStart(t *testing.T) {
	s := stateSeat("goose")
	if got := s.currentState(time.Now(), nil, false, 0); got != stateStarting || !s.stateSince.Equal(s.started) {
		t.Fatalf("starting = %q since %v, want since the start %v", got, s.stateSince, s.started)
	}
}

// A repaint erases a card before it draws it again, which would otherwise reset
// "waiting 4m" to nothing each time the screen flickers (COI-3243).
func TestAHeldCardKeepsTheSeatAtPromptThroughARepaintGap(t *testing.T) {
	s := stateSeat("claude")
	now := time.Now()
	s.cardUp = true
	if got := s.currentState(now, nil, true, 10*time.Second); got != statePrompt {
		t.Fatalf("a card up = %q, want prompt", got)
	}
	s.cardUp, s.cardLost = false, now.Add(-100*time.Millisecond)
	if got := s.currentState(now, nil, true, 10*time.Second); got != statePrompt || !s.stateSince.Equal(now) {
		t.Fatalf("a card gone 100ms = %q since %v, want prompt kept since %v", got, s.stateSince, now)
	}
	s.cardLost = now.Add(-time.Minute)
	if got := s.currentState(now, nil, true, 10*time.Second); got != stateIdle {
		t.Fatalf("a card gone a minute = %q, want idle", got)
	}
}

func TestAViewCarriesHowLongTheStateHasLasted(t *testing.T) {
	s := stateSeat("goose")
	s.lastOutput = time.Now().Add(-time.Hour)
	view := s.view()
	if view.StateSeconds == nil || *view.StateSeconds < 0 {
		t.Fatalf("the view should say how long its state has lasted: %+v", view.StateSeconds)
	}
}

// Nothing but the sampler pushes when a state flips, so a client need not poll for it.
func TestASessionsFrameIsPushedWhenAStateFlipsWithNoOneAsking(t *testing.T) {
	testDaemon(t)
	seat := dialTest(t)
	seat.spawn("scientist-evie", "scientist", "Evie", `printf 'READY\033[?2004h\n'; sleep 30`)
	seat.until("READY")
	client := subscribed(t)
	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		_ = client.raw.SetReadDeadline(deadline)
		message, err := client.read()
		if err != nil {
			t.Fatalf("waiting for the flip to idle: %v", err)
		}
		for _, view := range message.Sessions {
			if message.Type == "sessions" && view.Name == "scientist-evie" && view.State == stateIdle {
				if view.StateSeconds == nil {
					t.Fatalf("the pushed view should carry state_seconds: %+v", view)
				}
				return
			}
		}
	}
	t.Fatal("no sessions frame announced the seat going idle")
}
