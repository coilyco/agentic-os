package main

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestANonCoordinatorSeatSendsNoPushAndTheCoordinatorStillDoes(t *testing.T) {
	notifyRoster(t, "eng-platform")
	d := newDaemon(func(string, ...any) {})
	store, _ := loadPushStore("")
	if err := d.local().setupPush(mustKey(t), store); err != nil {
		t.Fatal(err)
	}
	sub, _, _ := testSubscription(t, "https://fcm.googleapis.com/fcm/send/phone")
	_ = store.add(sub)
	pushed, brokered := make(chan pushPayload, 8), make(chan waitEvent, 8)
	d.local().push.send = func(_ context.Context, _ *vapidKey, _ pushSubscription, _ []byte) (int, error) {
		pushed <- pushPayload{}
		return http.StatusCreated, nil
	}
	d.local().brokerEvent = func(e waitEvent) { brokered <- e }
	d.local().push.tracker.settle = 20 * time.Millisecond
	long := time.Now().Add(-time.Hour)
	worker := &ptySession{d: d, name: "scientist-evie", role: "scientist", identity: "Evie", seat: "claude", started: long, promptSeen: long, lastOutput: time.Now()}
	d.sessions[worker.name] = worker
	done := make(chan struct{})
	t.Cleanup(func() { close(done) })
	go d.local().watchWaiting(done, 10*time.Millisecond)
	time.Sleep(60 * time.Millisecond)

	d.users.NotifyAsk(choiceAsk{Question: "ship it?", Session: worker.name, From: "scientist Evie"})
	worker.mu.Lock()
	worker.lastOutput = time.Now().Add(-time.Minute)
	worker.mu.Unlock()
	select {
	case <-pushed:
		t.Fatal("a seat with no coordinator attribute sends no push, for an ask or a finished turn")
	case event := <-brokered:
		t.Fatalf("and tells the broker nothing: %+v", event)
	case <-time.After(400 * time.Millisecond):
	}

	d.users.NotifyAsk(choiceAsk{Question: "ship it?", Session: "eng-platform-beetle-ox", From: "eng-platform Beetle-Ox"})
	select {
	case <-pushed:
	case <-time.After(5 * time.Second):
		t.Fatal("the coordinator's ask never pushed")
	}
}

// flaggedRoster is the fixture roster with one role pinned_by_default, for a daemon whose
// roster is a function and not the agent-compose binary.
func flaggedRoster(t *testing.T, role string) func(context.Context) (listedRoster, error) {
	t.Helper()
	document, err := parseRoster(fixture(t, "roster.json"))
	if err != nil {
		t.Fatal(err)
	}
	for index := range document.Items {
		document.Items[index].PinnedByDefault = document.Items[index].Slug == role
	}
	return func(context.Context) (listedRoster, error) { return listRoster(document), nil }
}
