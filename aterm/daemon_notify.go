package main

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	// notifyFeature says the daemon answers `notify` and sends the `notify` event.
	notifyFeature  = "notify"
	maxNotifyTitle = 60
	maxNotifyBody  = 140
	// notifyEvery is the least gap between two tries from one seat.
	notifyEvery = 10 * time.Second

	reasonNotCoordinator = "not_coordinator"
	reasonTooSoon        = "too_soon"
	reasonBadText        = "bad_text"
	reasonNoRoster       = "roster_unavailable"
)

// kaiNotice is what a client plays and shows: the chime, a desktop notification, and a
// tap that opens Session. The daemon stamps everything but Title and Body.
type kaiNotice struct {
	ID       string    `json:"id"`
	Session  string    `json:"session"`
	Role     string    `json:"role,omitempty"`
	Identity string    `json:"identity,omitempty"`
	Title    string    `json:"title"`
	Body     string    `json:"body,omitempty"`
	At       time.Time `json:"at"`
}

// cleanNotice drops control characters, reads white space as one space, and cuts to
// limit characters.
func cleanNotice(text string, limit int) string {
	text = strings.ToValidUTF8(text, "")
	var b strings.Builder
	space := false
	for _, r := range text {
		switch {
		case unicode.IsSpace(r):
			space = true
		case unicode.IsControl(r) || unicode.Is(unicode.Cf, r):
		default:
			if space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			space = false
			b.WriteRune(r)
		}
	}
	out := b.String()
	if utf8.RuneCountInString(out) <= limit {
		return out
	}
	return strings.TrimRight(string([]rune(out)[:limit]), " ")
}

// isCoordinator reads the role's attribute from the roster, the one COI-3346 pins by.
// No role name is compared, and any failure to read it is a refusal.
func (d *daemon) isCoordinator(role string) (bool, error) {
	roster, err := d.roster(context.Background())
	if err != nil {
		return false, err
	}
	for _, listed := range roster.Roles {
		if listed.Slug == role {
			return listed.PinnedByDefault, nil
		}
	}
	return false, nil
}

// notifyKai answers `notify`. Only the token names the sender, and only a seat whose
// role carries the coordinator attribute gets a notice out. docs/aterm-daemon.md
func (d *daemon) notifyKai(cl *client, message frame) error {
	s := d.byToken(message.Token)
	if s == nil {
		return withExit(exitUsage, errors.New("aterm notify speaks for a session aterm launched, and this token names no live one"))
	}
	now := time.Now()
	s.mu.Lock()
	tooSoon := now.Sub(s.notifyTried) < notifyEvery
	if !tooSoon {
		s.notifyTried = now
	}
	s.mu.Unlock()
	if tooSoon {
		return withReason(reasonTooSoon, errors.New("a notice about Kai's attention goes out at most once every 10 seconds"))
	}
	title, body := cleanNotice(message.Title, maxNotifyTitle), cleanNotice(message.Body, maxNotifyBody)
	if title == "" {
		return withReason(reasonBadText, errors.New("a notice needs a title"))
	}
	ok, err := d.isCoordinator(s.role)
	if err != nil {
		return withReason(reasonNoRoster, errors.New("the daemon could not read the role roster, so it cannot tell a coordinator"))
	}
	if !ok {
		return withReason(reasonNotCoordinator, errors.New("only a coordinator seat can ping Kai, and this role is not one"))
	}
	notice := kaiNotice{ID: randomID(6), Session: s.name, Role: s.role, Identity: s.identity, Title: title, Body: body, At: now.UTC()}
	d.mu.Lock()
	delivered := len(d.subscribers)
	d.mu.Unlock()
	d.broadcast(frame{Type: "notify", Notify: &notice})
	d.users.NotifyKai(notice)
	d.logf("%s pinged Kai, shown to %d clients", s.name, delivered)
	return cl.c.write(frame{Type: "notified", ID: message.ID, Session: s.name, Delivered: delivered})
}
