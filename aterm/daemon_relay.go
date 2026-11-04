package main

import (
	"fmt"
	"strings"
	"time"
)

// directorRole is the seat a send_user_question card lands on. See docs/aterm-daemon.md.
const directorRole = "prod-director"

// directorCandidate is one live director seat, as pickDirector weighs it.
type directorCandidate struct {
	name      string
	clients   int
	lastInput time.Time
	started   time.Time
}

// pickDirector prefers a director a client has open, then the one typed into last, then
// the oldest, so the card lands where Kai already is. It is "" when none is up.
func pickDirector(candidates []directorCandidate) string {
	best := -1
	for index, candidate := range candidates {
		if best < 0 || betterDirector(candidate, candidates[best]) {
			best = index
		}
	}
	if best < 0 {
		return ""
	}
	return candidates[best].name
}

func betterDirector(a, b directorCandidate) bool {
	switch {
	case (a.clients > 0) != (b.clients > 0):
		return a.clients > 0
	case !a.lastInput.Equal(b.lastInput):
		return a.lastInput.After(b.lastInput)
	case !a.started.Equal(b.started):
		return a.started.Before(b.started)
	}
	return a.name < b.name
}

// directorFor names the live director seat other than the asker. With none up it is "",
// and the card shows on the asker's own pane, as an ask_choice card does.
func (d *daemon) directorFor(asker *ptySession) string {
	d.mu.Lock()
	seats := make([]*ptySession, 0, len(d.sessions))
	for _, s := range d.sessions {
		if s.role == directorRole && s != asker {
			seats = append(seats, s)
		}
	}
	d.mu.Unlock()
	candidates := make([]directorCandidate, 0, len(seats))
	for _, s := range seats {
		select {
		case <-s.done:
			continue
		default:
		}
		s.mu.Lock()
		candidates = append(candidates, directorCandidate{s.name, len(s.clients), s.lastInput, s.started})
		s.mu.Unlock()
	}
	return pickDirector(candidates)
}

// rehomeAsks moves the card of every pending relayed ask off a director that ended, onto
// another director or, with none, back onto the asker. Every client hears the ask again.
func (d *daemon) rehomeAsks(gone string) {
	d.mu.Lock()
	var ids []string
	for id, pending := range d.asks {
		if pending.ask.CardSeat == gone {
			ids = append(ids, id)
		}
	}
	d.mu.Unlock()
	for _, id := range ids {
		d.mu.Lock()
		pending := d.asks[id]
		asker := ""
		if pending != nil {
			asker = pending.ask.Session
		}
		d.mu.Unlock()
		if pending == nil {
			continue
		}
		next := ""
		if seat := d.session(asker); seat != nil {
			next = d.directorFor(seat)
		}
		d.mu.Lock()
		if d.asks[id] != pending {
			d.mu.Unlock()
			continue
		}
		pending.ask.CardSeat = next
		moved := pending.ask
		d.mu.Unlock()
		d.broadcast(frame{Type: "ask", Ask: &moved})
	}
}

// tellDirector queues one daemon-stamped line in the director whose pane showed a relayed
// card, so it sees what Kai answered. A director that ended meanwhile hears nothing.
func (d *daemon) tellDirector(ask choiceAsk, answer choiceAnswer) {
	if !ask.Relay || ask.CardSeat == "" {
		return
	}
	if director := d.session(ask.CardSeat); director != nil {
		tellSender(director, relayText(ask, answer))
	}
}

// relayText is the director's line: the asker, the question, and how it ended.
func relayText(ask choiceAsk, answer choiceAnswer) string {
	question := oneLine(ask.Question, 120)
	if answer.State != "answered" {
		return fmt.Sprintf("%s's question \"%s\" ended unanswered: %s", ask.From, question, strings.ReplaceAll(answer.State, "_", " "))
	}
	given := append([]string{}, answer.Labels...)
	if answer.Text != "" {
		given = append(given, "typed: "+answer.Text)
	}
	return fmt.Sprintf("Kai answered %s's question \"%s\": %s", ask.From, question, oneLine(strings.Join(given, ", "), 200))
}

// oneLine folds whitespace and cuts to limit runes, for a one-line notice.
func oneLine(text string, limit int) string {
	text = strings.Join(strings.Fields(text), " ")
	if runes := []rune(text); len(runes) > limit {
		return string(runes[:limit-1]) + "…"
	}
	return text
}
