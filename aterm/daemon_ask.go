package main

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"
)

// defaultAskTimeout bounds how long a seat waits on Kai.
const defaultAskTimeout = 15 * time.Minute

const maxChoiceOptions = 12

type choiceOption struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// choiceAsk is a question a seat puts to whoever is at a client. The daemon
// stamps Session and From from the asker's token, like a peer message.
type choiceAsk struct {
	ID         string         `json:"id"`
	Session    string         `json:"session"`
	From       string         `json:"from"`
	Question   string         `json:"question"`
	Header     string         `json:"header,omitempty"`
	Options    []choiceOption `json:"options"`
	AllowOther bool           `json:"allow_other,omitempty"`
	Multi      bool           `json:"multi,omitempty"`
	Asked      time.Time      `json:"asked"`
}

// choiceAnswer is how an ask ended: answered, cancelled, or timed_out.
type choiceAnswer struct {
	State  string   `json:"state"`
	Picks  []int    `json:"picks,omitempty"`
	Labels []string `json:"labels,omitempty"`
	Text   string   `json:"text,omitempty"`
	Reason string   `json:"reason,omitempty"`
}

type pendingAsk struct {
	ask  choiceAsk
	done chan choiceAnswer
	once sync.Once
}

func validateAsk(ask choiceAsk) error {
	switch {
	case strings.TrimSpace(ask.Question) == "":
		return errors.New("an ask needs a question")
	case len(ask.Options) == 0 && !ask.AllowOther:
		return errors.New("an ask needs options, or allow_other for free text")
	case len(ask.Options) > maxChoiceOptions:
		return fmt.Errorf("an ask takes at most %d options", maxChoiceOptions)
	}
	for index, option := range ask.Options {
		if strings.TrimSpace(option.Label) == "" {
			return fmt.Errorf("option %d has no label", index)
		}
	}
	return nil
}

// validateAnswer holds a client to what the ask offered.
func validateAnswer(ask choiceAsk, picks []int, text string) error {
	if !ask.Multi && len(picks) > 1 {
		return errors.New("this ask takes one pick")
	}
	if text != "" && !ask.AllowOther {
		return errors.New("this ask takes no free text")
	}
	if len(picks) == 0 && text == "" {
		return errors.New("an answer needs a pick or text")
	}
	for _, pick := range picks {
		if pick < 0 || pick >= len(ask.Options) {
			return fmt.Errorf("pick %d is not an option", pick)
		}
	}
	return nil
}

// ask registers the question, broadcasts it, and answers the asker when it
// settles. The asker's connection holds the id, so its loss cancels the ask.
func (d *daemon) ask(cl *client, message frame) error {
	asker := d.byToken(message.Token)
	if asker == nil {
		return withExit(exitUsage, errors.New("aterm ask speaks for a session aterm launched, and this token names no live one"))
	}
	if message.Ask == nil {
		return withExit(exitUsage, errors.New("an ask frame carries an ask"))
	}
	ask := *message.Ask
	if err := validateAsk(ask); err != nil {
		return withExit(exitUsage, err)
	}
	ask.ID = randomID(6)
	ask.Session = asker.name
	ask.From = asker.role + " " + asker.identity
	ask.Asked = time.Now().UTC()
	pending := &pendingAsk{ask: ask, done: make(chan choiceAnswer, 1)}
	d.mu.Lock()
	d.asks[ask.ID] = pending
	d.mu.Unlock()
	cl.asks = append(cl.asks, ask.ID)
	d.broadcast(frame{Type: "ask", Ask: &pending.ask})
	go func() {
		defer d.guard("ask")
		var answer choiceAnswer
		select {
		case answer = <-pending.done:
		case <-time.After(d.askTimeout):
			d.settle(ask.ID, choiceAnswer{State: "timed_out", Reason: "no answer within " + d.askTimeout.String()})
			answer = <-pending.done
		}
		_ = cl.c.write(frame{Type: "answered", ID: message.ID, AskID: ask.ID, Answer: &answer})
	}()
	return nil
}

// answer is a person's pick, so it takes the same guard as typing.
func (d *daemon) answer(cl *client, message frame) error {
	if !cl.browser && d.insideSession(cl.peerPID) {
		return errors.New("a process inside an aterm session cannot answer an ask")
	}
	d.mu.Lock()
	pending := d.asks[message.AskID]
	d.mu.Unlock()
	if pending == nil {
		return withExit(exitOffRoster, fmt.Errorf("no pending ask %q", message.AskID))
	}
	if err := validateAnswer(pending.ask, message.Picks, message.Text); err != nil {
		return withExit(exitUsage, err)
	}
	labels := make([]string, 0, len(message.Picks))
	for _, pick := range message.Picks {
		labels = append(labels, pending.ask.Options[pick].Label)
	}
	answer := choiceAnswer{State: "answered", Picks: message.Picks, Labels: labels, Text: message.Text}
	if !d.settle(message.AskID, answer) {
		return withExit(exitOffRoster, fmt.Errorf("ask %q was already settled", message.AskID))
	}
	return cl.c.write(frame{Type: "asked", ID: message.ID, AskID: message.AskID, State: "answered"})
}

// settle ends an ask once, tells the asker, and tells every client to drop it.
func (d *daemon) settle(id string, answer choiceAnswer) bool {
	d.mu.Lock()
	pending := d.asks[id]
	delete(d.asks, id)
	d.mu.Unlock()
	if pending == nil {
		return false
	}
	settled := false
	pending.once.Do(func() {
		pending.done <- answer
		settled = true
	})
	if settled {
		d.broadcast(frame{Type: "asked", AskID: id, State: answer.State})
	}
	return settled
}

// settleWhere cancels every pending ask that matches, for an asker that left.
func (d *daemon) settleWhere(match func(choiceAsk) bool, reason string) {
	d.mu.Lock()
	var ids []string
	for id, pending := range d.asks {
		if match(pending.ask) {
			ids = append(ids, id)
		}
	}
	d.mu.Unlock()
	for _, id := range ids {
		d.settle(id, choiceAnswer{State: "cancelled", Reason: reason})
	}
}

// pendingAsks is what a client joining late replays, oldest first.
func (d *daemon) pendingAsks() []choiceAsk {
	d.mu.Lock()
	asks := make([]choiceAsk, 0, len(d.asks))
	for _, pending := range d.asks {
		asks = append(asks, pending.ask)
	}
	d.mu.Unlock()
	slices.SortFunc(asks, func(a, b choiceAsk) int { return a.Asked.Compare(b.Asked) })
	return asks
}
