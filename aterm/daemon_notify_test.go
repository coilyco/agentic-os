package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// notifyRig is a daemon with two roles in its roster, one carrying the coordinator
// attribute, a client subscribed to the sessions channel, and a phone subscribed to push.
type notifyRig struct {
	t        *testing.T
	d        *daemon
	watcher  *conn
	pushed   chan pushPayload
	brokered chan waitEvent
}

func newNotifyRig(t *testing.T) *notifyRig {
	t.Helper()
	d, _ := wsDaemon(t)
	d.roster = func(context.Context) (listedRoster, error) {
		return listedRoster{Roles: []listedRole{{Slug: "lead", PinnedByDefault: true}, {Slug: "worker"}}}, nil
	}
	store, _ := loadPushStore("")
	if err := d.local().setupPush(mustKey(t), store); err != nil {
		t.Fatal(err)
	}
	sub, _, _ := testSubscription(t, "https://fcm.googleapis.com/fcm/send/phone")
	_ = store.add(sub)
	r := &notifyRig{t: t, d: d, pushed: make(chan pushPayload, 8), brokered: make(chan waitEvent, 8)}
	d.local().push.send = func(_ context.Context, _ *vapidKey, _ pushSubscription, payload []byte) (int, error) {
		var got pushPayload
		_ = json.Unmarshal(payload, &got)
		r.pushed <- got
		return http.StatusCreated, nil
	}
	d.local().brokerEvent = func(e waitEvent) { r.brokered <- e }
	r.watcher = pipeClient(t, d, peerStanding{pids: []int{os.Getpid()}})
	nextFrame(t, r.watcher, "welcome")
	if _, err := r.watcher.request(frame{Type: "subscribe", Channel: "sessions"}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	return r
}

func (r *notifyRig) seat(name, role string) *ptySession {
	r.t.Helper()
	s, err := startPTYSession(r.d, name, frame{
		Role: role, Identity: name, Seat: "codex",
		Argv: []string{"/bin/sh", "-c", "cat"}, Env: []string{"PATH=" + os.Getenv("PATH")}, Cwd: r.t.TempDir(),
	})
	if err != nil {
		r.t.Fatalf("start: %v", err)
	}
	r.d.mu.Lock()
	r.d.sessions[s.name] = s
	r.d.tokens[s.token] = s
	r.d.mu.Unlock()
	r.t.Cleanup(func() { s.end() })
	return s
}

func (r *notifyRig) notify(s *ptySession, title, body string) (frame, error) {
	r.t.Helper()
	c := pipeClient(r.t, r.d, peerStanding{pids: []int{os.Getpid()}})
	nextFrame(r.t, c, "welcome")
	return c.request(frame{Type: "notify", Token: s.token, Title: title, Body: body})
}

func TestNotifyOnlyACoordinatorRolePingsKaiAndEverySurfaceHearsIt(t *testing.T) {
	r := newNotifyRig(t)
	worker, lead := r.seat("worker-ana", "worker"), r.seat("lead-bea", "lead")

	if reply, err := r.notify(worker, "Deploy done", "all green"); err == nil || reply.Reason != reasonNotCoordinator {
		t.Fatalf("a role with no coordinator attribute is refused: %+v, %v", reply, err)
	}
	reply, err := r.notify(lead, "Pick a database", "Postgres or SQLite, I need your call.")
	if err != nil || reply.Type != "notified" || reply.Delivered != 1 {
		t.Fatalf("a coordinator's notice goes out to the one subscribed client: %+v, %v", reply, err)
	}

	got := nextFrame(t, r.watcher, "notify").Notify
	if got == nil || got.Session != lead.name || got.Role != "lead" || got.Title != "Pick a database" || got.Body != "Postgres or SQLite, I need your call." || got.ID == "" || got.At.IsZero() {
		t.Fatalf("the first notice a client hears is the coordinator's, stamped from its token: %+v", got)
	}
	select {
	case push := <-r.pushed:
		if push.Kind != "notify" || push.Session != lead.name || push.Title != got.Title || push.Body != got.Body || push.Tag != "aterm-"+lead.name {
			t.Fatalf("the phone gets the coordinator's words and a tap that names its seat: %+v", push)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the phone was never pushed")
	}
	if event := <-r.brokered; event.Kind != "ask" || event.Session != lead.name || event.Question != got.Body {
		t.Fatalf("the broker hears it as an ask, since its kinds are closed: %+v", event)
	}
	select {
	case push := <-r.pushed:
		t.Fatalf("the refused seat pushed nothing: %+v", push)
	case <-time.After(150 * time.Millisecond):
	}
}

func TestNotifyRefusesWhenTheRosterCannotBeReadTooSoonAndWithoutATitle(t *testing.T) {
	r := newNotifyRig(t)
	lead := r.seat("lead-bea", "lead")

	if reply, err := r.notify(lead, " \n\t ", "body"); err == nil || reply.Reason != reasonBadText {
		t.Fatalf("a notice with no title is refused: %+v, %v", reply, err)
	}
	r.d.roster = func(context.Context) (listedRoster, error) { return listedRoster{}, errors.New("no agent-compose") }
	other := r.seat("lead-cal", "lead")
	if reply, err := r.notify(other, "Hello", ""); err == nil || reply.Reason != reasonNoRoster {
		t.Fatalf("an unreadable roster fails closed, even for a coordinator: %+v, %v", reply, err)
	}
	if _, err := r.notify(lead, "x", ""); err == nil {
		t.Fatal("the bad-title try still counts, so a seat cannot probe faster than the limit")
	}
	r.d.roster = func(context.Context) (listedRoster, error) {
		return listedRoster{Roles: []listedRole{{Slug: "lead", PinnedByDefault: true}}}, nil
	}
	third := r.seat("lead-dee", "lead")
	if _, err := r.notify(third, "First", ""); err != nil {
		t.Fatal(err)
	}
	if reply, err := r.notify(third, "Second", ""); err == nil || reply.Reason != reasonTooSoon {
		t.Fatalf("a second try inside 10 seconds is held: %+v, %v", reply, err)
	}
}

func TestNotifyCleansAndCutsTheWordsAndNeedsALiveToken(t *testing.T) {
	r := newNotifyRig(t)
	lead := r.seat("lead-bea", "lead")
	long := strings.Repeat("é", 200)
	if _, err := r.notify(lead, "Needs\r\nyou\x07 now", long); err != nil {
		t.Fatal(err)
	}
	got := nextFrame(t, r.watcher, "notify").Notify
	if got.Title != "Needs you now" || len([]rune(got.Body)) != maxNotifyBody {
		t.Fatalf("control characters are dropped, white space is one space, and the body is cut to %d: %q %d", maxNotifyBody, got.Title, len([]rune(got.Body)))
	}
	c := pipeClient(t, r.d, peerStanding{pids: []int{os.Getpid()}})
	nextFrame(t, c, "welcome")
	if _, err := c.request(frame{Type: "notify", Token: "not-a-token", Title: "x"}); err == nil {
		t.Fatal("a token that names no live seat cannot ping")
	}
}
