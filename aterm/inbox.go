package main

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"time"
)

// inboxFeature is how a client knows the daemon answers an inbox frame.
const inboxFeature = "inbox"

// maxInbox bounds one session's inbox. A seat that never reads loses its
// oldest read messages first, then its oldest unread, rather than growing.
const maxInbox = 200

// inboxMessage is one message addressed to a seat, kept beside its delivery
// into the PTY. Text is the stamped line typed in, `[from <role> <identity>] body`.
type inboxMessage struct {
	ID       string    `json:"id"`
	From     string    `json:"from"`
	Text     string    `json:"text"`
	Received time.Time `json:"received"`
	Read     bool      `json:"read"`
}

// inbox is a session's received messages. It lives in the daemon's memory, so a
// daemon restart empties it while the session itself is adopted again.
type inbox struct {
	mu       sync.Mutex
	messages []inboxMessage
}

func (b *inbox) add(message inboxMessage) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.messages = append(b.messages, message)
	if len(b.messages) <= maxInbox {
		return
	}
	drop := slices.IndexFunc(b.messages, func(m inboxMessage) bool { return m.Read })
	b.messages = slices.Delete(b.messages, max(drop, 0), max(drop, 0)+1)
}

// take returns the unread messages, oldest first, and marks them read. With all
// set it also returns the ones already read.
func (b *inbox) take(all bool) []inboxMessage {
	b.mu.Lock()
	defer b.mu.Unlock()
	taken := []inboxMessage{}
	for index := range b.messages {
		if all || !b.messages[index].Read {
			taken = append(taken, b.messages[index])
		}
		b.messages[index].Read = true
	}
	return taken
}

// readInbox is the one path the MCP tool uses. The token names the caller's own
// session, so a seat can read no inbox but its own.
func readInbox(all bool) ([]inboxMessage, error) {
	token := strings.TrimSpace(os.Getenv(sessionTokenEnv))
	if token == "" {
		return nil, withExit(exitUsage, fmt.Errorf(
			"%s is unset: an inbox belongs to a session aterm launched, so read it from inside one", sessionTokenEnv))
	}
	c, err := dialDaemon(false)
	if err != nil {
		return nil, withExit(exitMissing, err)
	}
	defer c.Close()
	if !slices.Contains(c.features, inboxFeature) {
		return nil, fmt.Errorf("the running aterm daemon predates the inbox. " +
			"It restarts on the upgraded binary after five idle minutes")
	}
	reply, err := c.request(frame{Type: "inbox", Token: token, All: all})
	if err != nil {
		if reply.Code != 0 {
			return nil, withExit(reply.Code, err)
		}
		return nil, err
	}
	return reply.Inbox, nil
}
