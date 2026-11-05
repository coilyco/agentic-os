package main

import (
	"testing"
	"time"
)

// COI-3427: a closed seat's native shadow waited out a 24-hour grace because nothing told
// aos the session was finished. The daemon names the ended process when it forgets it.
func TestAnEndedSessionNamesItsProcessForShadowCollection(t *testing.T) {
	d := newDaemon(func(string, ...any) {})
	var finished []int
	d.finishShadow = func(pid int) { finished = append(finished, pid) }
	s := &ptySession{name: "ended-seat", pid: 4242, started: time.Now(), d: d, clients: map[*conn]bool{}}
	d.sessions[s.name] = s
	d.forget(s)
	if len(finished) != 1 || finished[0] != 4242 {
		t.Fatalf("finished = %v, want the ended session's pid once", finished)
	}
	d.forget(&ptySession{name: "no-process", started: time.Now(), d: d, clients: map[*conn]bool{}})
	if len(finished) != 1 {
		t.Fatalf("a session with no process has no shadow to collect: %v", finished)
	}
}
