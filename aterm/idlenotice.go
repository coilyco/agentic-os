package main

import (
	"fmt"
	"time"
)

const (
	// idleWatchMax bounds how long a delivered message's sender is kept waiting.
	idleWatchMax = 30 * time.Minute
	// idleGrace is how long a target that never showed work may sit idle before the
	// sender is told, so a message it ignored does not leave the sender waiting.
	idleGrace = 15 * time.Second
	idlePoll  = 500 * time.Millisecond
)

// idleDue is whether a watched target has gone idle after the message. It must
// have shown work or a prompt since delivery, unless idleGrace has passed.
func idleDue(state string, sawWork bool, since time.Duration) bool {
	return state == stateIdle && (sawWork || since > idleGrace)
}

// watchIdle types one daemon-stamped line into the sender when the session that
// took its message next goes idle, ends, or the watch runs out. See docs/aterm-daemon.md.
func (p *pendingSend) watchIdle() {
	snapshot, sender, d := p.snapshot(), p.sender, p.d
	if sender == nil || d == nil || snapshot.Session == "" {
		return
	}
	go func() {
		start := time.Now()
		sawWork := false
		ticker := time.NewTicker(idlePoll)
		defer ticker.Stop()
		for range ticker.C {
			select {
			case <-sender.done:
				return
			default:
			}
			target := d.session(snapshot.Session)
			since := time.Since(start)
			switch {
			case target == nil:
				tellSender(sender, snapshot.Session+" ended")
				return
			case since > idleWatchMax:
				tellSender(sender, fmt.Sprintf("idle watch on %s expired after %s", snapshot.Session, idleWatchMax))
				return
			}
			state := target.view().State
			sawWork = sawWork || state == stateBusy || state == statePrompt
			if idleDue(state, sawWork, since) {
				tellSender(sender, snapshot.Session+" is idle")
				return
			}
		}
	}()
}
