package main

import (
	"errors"
	"strings"
	"time"
)

// A seat mid-turn can hold the Enter back past the retries (COI-2619), so "unsent" was
// wrong when it took it later. Its message is parked instead. See docs/aterm-daemon.md.
var (
	// parkWindow is how long a parked message is watched before it fails.
	parkWindow = 30 * time.Minute
	// parkEnterGap is the time between Enters while the box still holds a parked message.
	parkEnterGap = 10 * time.Second
)

var errTypedOver = errors.New("someone typed into its prompt while the message waited there, so it sits there unsent")

// parkedError is what confirmClaude returns for a message the seat is still working
// around. before is how often the text showed above the box when it was typed.
type parkedError struct{ before int }

func (*parkedError) Error() string { return errNotSubmitted.Error() }

// parkedSend is a message whose text sits in a busy seat's box. Only deliverNext's
// goroutine touches since, lastEnter and clean.
type parkedSend struct {
	p                *pendingSend
	text             string
	before           int
	since, lastEnter time.Time
	clean            int
}

// unsent is the verdict for text the seat did not take: parked when it is mid-turn,
// since a turn can hold the Enter back, and unsent when it is idle and should have.
func (s *ptySession) unsent(before int) error {
	if s.busyNow() {
		return &parkedError{before: before}
	}
	return errNotSubmitted
}

// busyNow is whether the seat's spinner is on screen. It ignores how recently the seat
// wrote, because typing the message makes an idle seat echo it.
func (s *ptySession) busyNow() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.scr == nil {
		return false
	}
	mark, ok := busyScreens[s.seat]
	return ok && mark.MatchString(strings.Join(s.scr.text(), "\n"))
}

// park keeps a message that submit gave up on, and tells the sender it is held.
func (s *ptySession) park(p *pendingSend, before int) {
	now := time.Now()
	s.mu.Lock()
	select {
	case <-s.done:
		s.mu.Unlock()
		p.setState("failed", s.name+" ended before it was delivered")
		return
	default:
	}
	s.parked = &parkedSend{p: p, text: p.text, before: before, since: now, lastEnter: now}
	s.mu.Unlock()
	p.setState("held", s.name+" is mid-turn and has not taken the message, it submits when the turn ends")
	s.d.logf("message to %s parked: its box holds the text and the seat is working", s.name)
}

// settleParked reads the screen for the parked message: delivered once it shows above the
// box, failed on parkWindow or a person typing, and Enter again while the box holds it.
func (s *ptySession) settleParked(now time.Time) {
	s.mu.Lock()
	pk := s.parked
	typed := pk != nil && s.lastInput.After(pk.since)
	s.mu.Unlock()
	if pk == nil {
		return
	}
	readable, holds, echoes := s.readClaude(pk.text)
	if readable && !holds && echoes > pk.before {
		if pk.clean++; pk.clean == 2 {
			s.endParked(pk, nil, now)
		}
		return
	}
	pk.clean = 0
	switch {
	case typed:
		s.endParked(pk, errTypedOver, now)
	case now.Sub(pk.since) > parkWindow:
		s.endParked(pk, errNotSubmitted, now)
	case readable && holds && now.Sub(pk.lastEnter) >= parkEnterGap && !s.holdsCard():
		s.writeMu.Lock()
		err := s.writePTY([]byte("\r"))
		s.writeMu.Unlock()
		pk.lastEnter = now
		if err != nil {
			s.endParked(pk, err, now)
		}
	}
}

func (s *ptySession) endParked(pk *parkedSend, err error, now time.Time) {
	s.mu.Lock()
	if s.parked == pk {
		s.parked = nil
	}
	s.mu.Unlock()
	waited := now.Sub(pk.since).Round(time.Second)
	switch {
	case err == nil:
		s.d.logf("message to %s taken %s after it was parked", s.name, waited)
		pk.p.setState("delivered", "")
		s.d.pushSessions()
	case errors.Is(err, errNotSubmitted), errors.Is(err, errTypedOver):
		s.d.logf("message to %s still unsent after %s parked: %v", s.name, waited, err)
		pk.p.setState("failed", s.name+": "+err.Error())
	default:
		pk.p.setState("failed", "writing to "+s.name+": "+err.Error())
	}
}

// holdBehindParked tells the next message why it waits: typing it now would put two
// messages in one box.
func (s *ptySession) holdBehindParked() {
	s.mu.Lock()
	var next *pendingSend
	if len(s.pending) > 0 {
		next = s.pending[0]
	}
	s.mu.Unlock()
	if next != nil {
		next.setState("held", s.name+" still has an earlier message waiting in its prompt")
	}
}
