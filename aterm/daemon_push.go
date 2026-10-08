package main

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/urfave/cli/v3"
)

const (
	// pushWatchEvery is how often seats are read for a waiting transition.
	pushWatchEvery = time.Second
	// pushSettle is how long a seat must stay waiting before it pushes, so a
	// silent tool call that reads as idle for a beat does not wake a phone.
	pushSettle = 4 * time.Second
	// pushCooldown is the least gap between two pushes about one seat.
	pushCooldown = 30 * time.Second
	// pushMaxSubscriptions bounds the store, one per browser profile in use.
	pushMaxSubscriptions = 32
	// pushSendTimeout bounds one fan-out, so a stalled push service ends it.
	pushSendTimeout = 20 * time.Second
)

// pushPayload is what the service worker reads. Session names the seat to focus
// on a tap, and Tag lets a second push about one seat replace the first.
type pushPayload struct {
	Kind     string `json:"kind"`
	Session  string `json:"session"`
	Role     string `json:"role,omitempty"`
	Identity string `json:"identity,omitempty"`
	Title    string `json:"title"`
	Body     string `json:"body"`
	Tag      string `json:"tag"`
}

// pushStore keeps subscriptions across a restart, since a browser subscribes
// once and would never know the daemon forgot it.
type pushStore struct {
	mu   sync.Mutex
	path string
	subs []pushSubscription
}

func pushStatePath() string {
	return filepath.Join(filepath.Dir(ledgerDir()), "push-subscriptions.json")
}

// loadPushStore reads path. An empty path keeps subscriptions in memory only.
func loadPushStore(path string) (*pushStore, error) {
	store := &pushStore{path: path}
	if path == "" {
		return store, nil
	}
	raw, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return store, nil
	case err != nil:
		return nil, err
	}
	return store, json.Unmarshal(raw, &store.subs)
}

// save writes through a temp file, so a crash leaves the old list. Caller holds mu.
func (s *pushStore) save() error {
	if s.path == "" {
		return nil
	}
	raw, err := json.Marshal(s.subs)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	temp := s.path + ".tmp"
	if err := os.WriteFile(temp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(temp, s.path)
}

// add stores sub, replacing a subscription with the same endpoint.
func (s *pushStore) add(sub pushSubscription) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sub.Added = time.Now().UTC()
	for index := range s.subs {
		if s.subs[index].Endpoint == sub.Endpoint {
			s.subs[index] = sub
			return s.save()
		}
	}
	if len(s.subs) >= pushMaxSubscriptions {
		return withExit(exitUsage, fmt.Errorf("this daemon holds %d push subscriptions already", pushMaxSubscriptions))
	}
	s.subs = append(s.subs, sub)
	return s.save()
}

func (s *pushStore) remove(endpoint string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := s.subs[:0:0]
	for _, sub := range s.subs {
		if sub.Endpoint != endpoint {
			kept = append(kept, sub)
		}
	}
	if len(kept) == len(s.subs) {
		return nil
	}
	s.subs = kept
	return s.save()
}

func (s *pushStore) list() []pushSubscription {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]pushSubscription(nil), s.subs...)
}

// pushState is the daemon's Web Push: nil key means the feature is off.
type pushState struct {
	key     *vapidKey
	store   *pushStore
	send    pushSender
	tracker waitTracker
}

func (p *pushState) enabled() bool { return p.key != nil && p.store != nil }

// setupPush turns Web Push on from an encoded VAPID key. An empty key leaves it off.
func (d *daemon) setupPush(encoded string, store *pushStore) error {
	if strings.TrimSpace(encoded) == "" {
		return nil
	}
	key, err := parseVAPIDKey(encoded)
	if err != nil {
		return err
	}
	d.push = pushState{key: key, store: store, send: sendPush}
	return nil
}

// pushFrame answers the push_* frames. The key is public, but subscribing aims
// alerts at a device, so a remote one needs its passkey (remoteLocked).
func (d *daemon) pushFrame(cl *client, message frame) error {
	if !d.push.enabled() {
		return withExit(exitUsage, errors.New("this daemon has no VAPID key, so it sends no push"))
	}
	switch message.Type {
	case "push_key":
		return cl.c.write(frame{Type: "push_key", ID: message.ID, Key: d.push.key.publicKey()})
	case "push_subscribe":
		if message.Push == nil {
			return withExit(exitUsage, errors.New("push_subscribe carries a subscription"))
		}
		if err := message.Push.validate(); err != nil {
			return withExit(exitUsage, err)
		}
		if err := d.push.store.add(*message.Push); err != nil {
			return err
		}
		d.logf("a browser subscribed to push")
		return cl.c.write(frame{Type: "push_subscribed", ID: message.ID})
	default:
		if message.Push == nil {
			return withExit(exitUsage, errors.New("push_unsubscribe carries the subscription to drop"))
		}
		if err := d.push.store.remove(message.Push.Endpoint); err != nil {
			return err
		}
		return cl.c.write(frame{Type: "push_unsubscribed", ID: message.ID})
	}
}

// waitEvent is a seat that has started waiting on a person.
type waitEvent struct {
	Kind     string
	Session  string
	Role     string
	Identity string
}

type watchedSeat struct {
	state string
	// since is when the seat entered the state that will push, zero for none.
	since  time.Time
	kind   string
	pushed time.Time
}

// waitTracker turns the roster into waiting transitions. A seat already waiting
// when first seen is a baseline and never pushes.
type waitTracker struct {
	seats map[string]*watchedSeat
	// settle replaces pushSettle when set, for a test that cannot wait seconds.
	settle time.Duration
}

// observe takes one reading of every seat and returns those waiting long enough
// to tell someone: a turn ended (busy then idle) or a card holds it (prompt).
func (t *waitTracker) observe(views []sessionView, now time.Time) []waitEvent {
	if t.seats == nil {
		t.seats = map[string]*watchedSeat{}
	}
	live := map[string]bool{}
	var events []waitEvent
	for _, view := range views {
		live[view.Name] = true
		seat, known := t.seats[view.Name]
		if !known {
			t.seats[view.Name] = &watchedSeat{state: view.State}
			continue
		}
		if view.State != seat.state {
			seat.since, seat.kind = time.Time{}, ""
			switch {
			case view.State == statePrompt:
				seat.since, seat.kind = now, "prompt"
			case view.State == stateIdle && seat.state == stateBusy:
				seat.since, seat.kind = now, "done"
			}
			seat.state = view.State
		}
		if seat.since.IsZero() || now.Sub(seat.since) < cmp.Or(t.settle, pushSettle) {
			continue
		}
		seat.since = time.Time{}
		if now.Sub(seat.pushed) < pushCooldown {
			continue
		}
		seat.pushed = now
		events = append(events, waitEvent{Kind: seat.kind, Session: view.Name, Role: view.Role, Identity: view.Identity})
	}
	for name := range t.seats {
		if !live[name] {
			delete(t.seats, name)
		}
	}
	return events
}

// waitPayload words a transition for a lock screen.
func waitPayload(event waitEvent, question string) pushPayload {
	who := event.Identity
	if who == "" {
		who = event.Session
	}
	payload := pushPayload{Kind: event.Kind, Session: event.Session, Role: event.Role, Identity: event.Identity, Tag: "aterm-" + event.Session}
	switch event.Kind {
	case "prompt":
		payload.Title, payload.Body = who+" needs a decision", "A permission or choice card is waiting."
	case "ask":
		payload.Title, payload.Body = who+" is asking", question
	default:
		payload.Title, payload.Body = who+" is done", "Your turn."
	}
	return payload
}

// watchWaiting reads the roster on a timer until done closes.
func (d *daemon) watchWaiting(done <-chan struct{}, every time.Duration) {
	defer d.guard("push watcher")
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case now := <-ticker.C:
			for _, event := range d.push.tracker.observe(d.views(), now) {
				d.pushWaiting(waitPayload(event, ""))
			}
		}
	}
}

// pushAsk tells the subscribed devices that a seat called ask_choice.
func (d *daemon) pushAsk(ask choiceAsk) {
	if !d.push.enabled() {
		return
	}
	question := ask.Question
	if runes := []rune(question); len(runes) > 140 {
		question = string(runes[:139]) + "…"
	}
	d.pushWaiting(waitPayload(waitEvent{Kind: "ask", Session: ask.Session, Identity: askerName(ask)}, question))
}

// askerName is the identity half of "<role> <identity>".
func askerName(ask choiceAsk) string {
	if _, identity, ok := strings.Cut(ask.From, " "); ok {
		return identity
	}
	return ""
}

// pushWaiting sends to every subscription without blocking, page open or not,
// since a sleeping phone's socket looks live. docs/aterm-daemon.md
func (d *daemon) pushWaiting(payload pushPayload) {
	if !d.push.enabled() {
		return
	}
	subs := d.push.store.list()
	if len(subs) == 0 {
		return
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return
	}
	go func() {
		defer d.guard("push send")
		ctx, cancel := context.WithTimeout(context.Background(), pushSendTimeout)
		defer cancel()
		var wg sync.WaitGroup
		for _, sub := range subs {
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer d.guard("push send")
				status, err := d.push.send(ctx, d.push.key, sub, raw)
				switch {
				case err != nil:
					d.logf("push to a browser failed: %v", err)
				case status == http.StatusNotFound || status == http.StatusGone:
					d.logf("a browser dropped its push subscription, so it is forgotten")
					_ = d.push.store.remove(sub.Endpoint)
				case status < 200 || status > 299:
					d.logf("a push service answered %d", status)
				}
			}()
		}
		wg.Wait()
	}()
}

// newVAPIDCommand is `aterm vapid`: make the key the daemon signs push with.
func newVAPIDCommand() *cli.Command {
	return &cli.Command{
		Name:  "vapid",
		Usage: "make a VAPID key for Web Push, writing the private half to a file and printing the public half",
		Description: "The file holds the secret and is never printed. Stash it in SSM at\n" +
			"/coilysiren/aterm/vapid-key, and feed it to the daemon as ATERM_VAPID_KEY.",
		Flags: []cli.Flag{&cli.StringFlag{Name: "out", Required: true, Usage: "file to create, mode 0600, refused when it exists"}},
		Action: func(_ context.Context, cmd *cli.Command) error {
			key, encoded, err := newVAPIDKey()
			if err != nil {
				return err
			}
			file, err := os.OpenFile(cmd.String("out"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
			if err != nil {
				return withExit(exitUsage, err)
			}
			if _, err := file.WriteString(encoded + "\n"); err != nil {
				_ = file.Close()
				return err
			}
			if err := file.Close(); err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.Root().Writer, key.publicKey())
			return err
		},
	}
}
