package main

import (
	"errors"
	"strings"
	"time"
)

// A claude seat can read a paste long after it was written, and draw its box later still
// (COI-2300), so an empty box proves nothing. See docs/aterm-daemon.md.
var (
	// enterGap is the first Enter's time. Each retry doubles it, to enterCeiling.
	enterGap = time.Second
	// claudeEnters is how many more Enters follow the first before giving up.
	claudeEnters = 8
	// arrivalWindow is how long a message may go unseen or unsent, on a loaded host.
	arrivalWindow = 90 * time.Second
	// boxGrace is how long a screen with no box waits before it is an unknown layout.
	boxGrace = 20 * time.Second
)

const enterCeiling = 8 * time.Second

var errNotSeen = errors.New("its screen never showed the message, in the prompt box or the transcript, so it may not have arrived")

// confirmClaude returns once claude's transcript shows the typed message, pressing Enter
// again while the text sits in the box. Caller holds writeMu.
func (s *ptySession) confirmClaude(text string, before int) error {
	start := time.Now()
	lastEnter, holdsSince := start, time.Time{}
	retries, clean, seen := 0, 0, false
	for ; ; time.Sleep(submitPoll) {
		select {
		case <-s.done:
			return nil
		default:
		}
		readable, holds, echoes := s.readClaude(text)
		now := time.Now()
		seen = seen || readable
		switch {
		case !readable:
			clean = 0
			if !seen && now.Sub(start) > boxGrace {
				// A layout this cannot read is unknown, and costs a send no more than the grace.
				return nil
			}
		case holds:
			clean = 0
			if holdsSince.IsZero() {
				// The Enter written with the paste may be queued behind it, so it gets its window.
				holdsSince, lastEnter = now, now
			}
			if now.Sub(lastEnter) >= min(enterGap<<retries, enterCeiling) && !s.holdsCard() {
				if retries == claudeEnters {
					return errNotSubmitted
				}
				retries++
				if err := s.writePTY([]byte("\r")); err != nil {
					return err
				}
				lastEnter = now
			}
		case echoes > before:
			holdsSince = time.Time{}
			if clean++; clean == 2 {
				return nil
			}
		default:
			holdsSince, clean = time.Time{}, 0
		}
		if now.Sub(start) > arrivalWindow {
			if holds {
				return errNotSubmitted
			}
			return errNotSeen
		}
	}
}

// holdsCard is whether a permission or choice card is up, which an Enter would answer.
func (s *ptySession) holdsCard() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cardReason() != ""
}

// readClaude reads the screen under the lock: whether claude's box can be found, whether
// it still holds the text, and how many times the text shows above it.
func (s *ptySession) readClaude(text string) (readable, holds bool, echoes int) {
	s.mu.Lock()
	var lines []string
	if s.scr != nil {
		lines = s.scr.text()
	}
	s.mu.Unlock()
	readable, holds = claudeBoxHolds(lines, text)
	return readable, holds, echoCount(lines, text)
}

// echoCount is how often the end of text, or a folded paste, shows above claude's box,
// which is where a submitted message lands, a queued one included.
func echoCount(lines []string, text string) int {
	var rules []int
	for index, line := range lines {
		if isRule(line) {
			rules = append(rules, index)
		}
	}
	if len(rules) < 2 {
		return 0
	}
	above := strings.Join(lines[:rules[len(rules)-2]], "\n")
	count := len(pastedPlaceholder.FindAllString(above, -1))
	if tail := []rune(squeeze(text)); len(tail) > 0 {
		count += strings.Count(squeeze(above), string(tail[max(len(tail)-40, 0):]))
	}
	return count
}
