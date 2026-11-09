package main

import (
	"context"
	"errors"
	"time"
)

const (
	maxNotifyTitle = 60
	maxNotifyBody  = 140
	// notifyGap is the least time between two notifies from one seat.
	notifyGap = 10 * time.Second
)

// notifyEvent is what a client plays once per id. Not replayed on subscribe.
type notifyEvent struct {
	ID       string `json:"id"`
	Session  string `json:"session"`
	Role     string `json:"role"`
	Identity string `json:"identity"`
	Title    string `json:"title"`
	Body     string `json:"body"`
	At       string `json:"at"`
}

// notify answers a coordinator seat's ping for Kai. The token names the seat, and the
// roster must mark its role pinned_by_default. See references/seat-state.md.
func (d *daemon) notify(cl *client, message frame) error {
	sender := d.byToken(message.Token)
	if sender == nil {
		return withExit(exitUsage, errors.New("aterm notify speaks for a session aterm launched, and this token names no live one"))
	}
	if message.Target != "" || message.Session != "" {
		return withExit(exitUsage, errors.New("notify never names a seat: it speaks for the session its token belongs to"))
	}
	roster, err := d.roster(context.Background())
	if err != nil {
		return withReason(reasonRosterUnavailable, withExit(exitFailure,
			errors.New("the role roster could not be read, so no seat may notify")))
	}
	if !coordinatorRole(roster, sender.role) {
		return withReason(reasonNotCoordinator, withExit(exitUsage,
			errors.New("only a coordinator role may notify Kai, and this seat's role is not one")))
	}
	title, body := cleanLine(message.Title, maxNotifyTitle), cleanLine(message.Body, maxNotifyBody)
	if title == "" || body == "" {
		return withReason(reasonBadText, withExit(exitUsage, errors.New("notify needs a title and a body with text in them")))
	}
	now := time.Now()
	if !d.reserveNotify(sender.name, now) {
		return withReason(reasonTooSoon, withExit(exitUsage, errors.New("this seat notified a moment ago, so try again after ten seconds")))
	}
	event := notifyEvent{
		ID: randomID(8), Session: sender.name, Role: sender.role, Identity: sender.identity,
		Title: title, Body: body, At: now.UTC().Format(time.RFC3339),
	}
	delivered := d.subscriberCount()
	d.broadcast(frame{Type: "notify", Notify: &event})
	d.users.NotifyPush(pushPayload{
		Kind: "notify", Session: event.Session, Role: event.Role, Identity: event.Identity,
		Title: title, Body: body, Tag: "aterm-" + event.Session,
	})
	return cl.c.write(frame{Type: "notified", ID: message.ID, NotifyID: event.ID, Session: event.Session, Delivered: delivered})
}

// coordinatorRole is whether the roster marks slug as one whose seats start pinned.
func coordinatorRole(roster listedRoster, slug string) bool {
	for _, role := range roster.Roles {
		if role.Slug == slug {
			return role.PinnedByDefault
		}
	}
	return false
}

// reserveNotify takes the seat's slot, or says it already notified within notifyGap.
func (d *daemon) reserveNotify(name string, now time.Time) bool {
	d.notifyMu.Lock()
	defer d.notifyMu.Unlock()
	if last, ok := d.notifyLast[name]; ok && now.Sub(last) < notifyGap {
		return false
	}
	if d.notifyLast == nil {
		d.notifyLast = map[string]time.Time{}
	}
	d.notifyLast[name] = now
	return true
}

func (d *daemon) subscriberCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.subscribers)
}
