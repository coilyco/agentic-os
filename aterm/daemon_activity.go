package main

import (
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// maxActivityRunes is the widest "what I'm doing" line a phone shows (measured on
// COI-3336). Counted in characters, so a wide glyph can still overrun the row.
const maxActivityRunes = 57

// activityFeature is how a client knows a view carries `activity` and `activity_at`,
// and that the daemon answers an `activity` frame. See references/seat-state.md.
const activityFeature = "activity"

// cleanActivity is the stored form of a seat's line: unsafe characters dropped, white
// space read as one space, then cut to maxActivityRunes. See references/seat-state.md.
func cleanActivity(text string) string {
	text = strings.ToValidUTF8(text, "")
	var b strings.Builder
	space := false
	for _, r := range text {
		switch {
		case r == '\n' || r == '\r' || r == '\t' || unicode.IsSpace(r):
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
	if utf8.RuneCountInString(out) <= maxActivityRunes {
		return out
	}
	return strings.TrimRight(string([]rune(out)[:maxActivityRunes]), " ")
}

// setActivity stores a seat's own line and when it was set. A repeat of the same
// text still moves the time, since that is how a seat says the line is current.
func (s *ptySession) setActivity(text string, now time.Time) string {
	text = cleanActivity(text)
	s.mu.Lock()
	s.activity, s.activityAt = text, now
	if text == "" {
		s.activityAt = time.Time{}
	}
	s.mu.Unlock()
	return text
}

// activity answers an `activity` frame. Only the token names the session, never a
// target or the screen, so a seat sets its own line and nothing else can.
func (d *daemon) activity(cl *client, message frame) error {
	s := d.byToken(message.Token)
	if s == nil {
		return withExit(exitUsage, errors.New("the token names no live session, and an activity line belongs to one"))
	}
	text := s.setActivity(message.Text, time.Now())
	d.pushSessions()
	return cl.c.write(frame{Type: "activity", ID: message.ID, Session: s.name, Text: text})
}
