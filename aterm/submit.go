package main

import (
	"errors"
	"regexp"
	"strings"
	"time"
)

// A seat still settling can drop the Enter after a paste (COI-2474), so submit
// reads the box back before a message is delivered. See docs/aterm-daemon.md.
var (
	// submitWindow is how long one Enter gets to empty the box.
	submitWindow = time.Second
	// enterRetries is how many more Enters follow the first before giving up.
	enterRetries = 3
)

const (
	submitPoll = 100 * time.Millisecond
	// unreadableGrace is how long a box that never shows up is waited on.
	unreadableGrace = 300 * time.Millisecond
)

var errNotSubmitted = errors.New("its prompt took the text but not Enter, so the message sits there unsent")

// pastedPlaceholder is how claude shows a paste it folded out of the box.
var pastedPlaceholder = regexp.MustCompile(`\[Pasted text #\d+`)

// submit types a message, then presses Enter again for as long as the text is
// still in the seat's prompt box. Caller holds writeMu.
func (s *ptySession) submit(text string, paste bool) error {
	if err := s.inject(text, paste); err != nil {
		return err
	}
	for retry := 0; ; retry++ {
		if s.boxCleared(text) {
			return nil
		}
		if retry == enterRetries {
			return errNotSubmitted
		}
		if err := s.writePTY([]byte("\r")); err != nil {
			return err
		}
	}
}

// boxCleared polls until the text has left the box on two reads running, so a screen
// caught mid-repaint is not taken for it, or until submitWindow ends.
func (s *ptySession) boxCleared(text string) bool {
	if s.seat != "claude" {
		return true
	}
	clean, seen := 0, false
	start := time.Now()
	deadline := start.Add(submitWindow)
	for ; ; time.Sleep(submitPoll) {
		select {
		case <-s.done:
			return true
		default:
		}
		s.mu.Lock()
		var lines []string
		if s.scr != nil {
			lines = s.scr.text()
		}
		s.mu.Unlock()
		readable, holds := boxHolds(s.seat, lines, text)
		seen = seen || readable
		if readable && !holds {
			if clean++; clean == 2 {
				return true
			}
		} else {
			clean = 0
		}
		now := time.Now()
		if !seen && now.Sub(start) > unreadableGrace {
			// A layout this cannot read is unknown, and costs a send no more than the grace.
			return true
		}
		if now.After(deadline) {
			return !holds
		}
	}
}

// boxHolds is whether claude's box, between the last two rules, can be found and still
// holds the end of text or a folded paste. The same words above it are already sent.
func boxHolds(seat string, lines []string, text string) (readable, holds bool) {
	if seat != "claude" {
		return false, false
	}
	var rules []int
	for index, line := range lines {
		if isRule(line) {
			rules = append(rules, index)
		}
	}
	if len(rules) < 2 {
		return false, false
	}
	box := strings.Join(lines[rules[len(rules)-2]+1:rules[len(rules)-1]], "\n")
	if pastedPlaceholder.MatchString(box) {
		return true, true
	}
	// The box wraps a long line and indents its continuation, so compare without whitespace.
	tail := []rune(squeeze(text))
	tail = tail[max(len(tail)-40, 0):]
	return true, len(tail) > 0 && strings.Contains(squeeze(box), string(tail))
}

// isRule is a row drawn as a horizontal rule.
func isRule(line string) bool {
	runes := []rune(strings.TrimSpace(line))
	if len(runes) < 10 {
		return false
	}
	drawn := 0
	for _, r := range runes {
		if r == '─' || r == '━' {
			drawn++
		}
	}
	return drawn*10 >= len(runes)*8
}

// squeeze drops whitespace and the vertical borders an older box draws.
func squeeze(text string) string {
	return strings.Map(func(r rune) rune {
		if r == '│' || r == '┃' || strings.ContainsRune(" \t\r\n", r) {
			return -1
		}
		return r
	}, text)
}
