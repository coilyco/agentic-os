package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// notifyRoster points the roster at a stand-in agent-compose printing the fixture roster
// with the flagged role pinned_by_default. Empty flags none, as an older release does.
func notifyRoster(t *testing.T, flagged string) {
	t.Helper()
	var document map[string]any
	if err := json.Unmarshal(fixture(t, "roster.json"), &document); err != nil {
		t.Fatal(err)
	}
	for _, item := range document["items"].([]any) {
		if role := item.(map[string]any); role["slug"] == flagged {
			role["pinned_by_default"] = true
		}
	}
	raw, _ := json.Marshal(document)
	dir := t.TempDir()
	data := filepath.Join(dir, "roster.json")
	script := filepath.Join(dir, "agent-compose")
	if err := os.WriteFile(data, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(script, []byte("#!/bin/sh\ncat '"+data+"'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENT_COMPOSE_BIN", script)
}

// notifyRig is a daemon with a coordinator seat and a worker seat, and their tokens.
func notifyRig(t *testing.T, options daemonOptions) (coordinator, worker string) {
	t.Helper()
	notifyRoster(t, "eng-platform")
	testDaemonWith(t, options)
	boss := dialTest(t)
	boss.spawn("eng-platform-beetle-ox", "eng-platform", "Beetle-Ox", `echo TOKEN=$ATERM_SESSION_TOKEN; sleep 60`)
	coordinator = boss.token()
	hand := dialTest(t)
	hand.spawn("scientist-evie", "scientist", "Evie", `echo TOKEN=$ATERM_SESSION_TOKEN; sleep 60`)
	return coordinator, hand.token()
}

func notifyFrame(t *testing.T, token, title, body string) (frame, error) {
	t.Helper()
	return dialTest(t).c.request(frame{Type: "notify", Token: token, Title: title, Body: body})
}

func subscribedClient(t *testing.T) *conn {
	t.Helper()
	c := dialTest(t).c
	if _, err := c.request(frame{Type: "subscribe", Channel: "sessions"}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	return c
}

func TestNotifyFromACoordinatorSeatReachesASubscribedClientOnce(t *testing.T) {
	coordinator, _ := notifyRig(t, daemonOptions{})
	client := subscribedClient(t)
	reply, err := notifyFrame(t, coordinator, "Decision needed", "Ship the pin bump?")
	if err != nil || reply.Type != "notified" || reply.Session != "eng-platform-beetle-ox" || reply.NotifyID == "" || reply.Delivered < 1 {
		t.Fatalf("notify = %+v, %v, want notified with an id and a delivery count", reply, err)
	}
	event := nextFrame(t, client, "notify").Notify
	if event == nil || event.ID != reply.NotifyID || event.Session != "eng-platform-beetle-ox" || event.Role != "eng-platform" ||
		event.Identity != "Beetle-Ox" || event.Title != "Decision needed" || event.Body != "Ship the pin bump?" {
		t.Fatalf("the client's event = %+v", event)
	}
	if _, err := time.Parse(time.RFC3339, event.At); err != nil {
		t.Fatalf("at %q is not RFC 3339: %v", event.At, err)
	}
	// Not replayed: a client that subscribes afterwards is told nothing of it.
	late := subscribedClient(t)
	deadline := time.Now().Add(700 * time.Millisecond)
	for time.Now().Before(deadline) {
		_ = late.raw.SetReadDeadline(deadline)
		message, err := late.read()
		if err != nil {
			break
		}
		if message.Type == "notify" {
			t.Fatalf("a late subscriber was replayed %+v", message.Notify)
		}
	}
}

func TestNotifyIsRefusedWithAReasonAClientCanBranchOn(t *testing.T) {
	coordinator, worker := notifyRig(t, daemonOptions{})
	// In order: only the fourth succeeds, and the fifth is its second within ten seconds.
	for _, test := range []struct {
		name, token, title, body, reason string
		refused                          bool
	}{
		{"a worker seat", worker, "Hello", "World", reasonNotCoordinator, true},
		{"no body", coordinator, "Hello", " \t\n ", reasonBadText, true},
		{"only control characters", coordinator, "\x1b\x07", "World", reasonBadText, true},
		{"the first allowed", coordinator, "Hello", "World", "", false},
		{"a second at once", coordinator, "Hello again", "World", reasonTooSoon, true},
		{"a token that names no seat", "not-a-token", "Hello", "World", "", true},
	} {
		reply, err := notifyFrame(t, test.token, test.title, test.body)
		if !test.refused {
			if err != nil || reply.Type != "notified" {
				t.Fatalf("%s = %+v, %v, want notified", test.name, reply, err)
			}
			continue
		}
		if err == nil || reply.Reason != test.reason {
			t.Fatalf("%s = %+v, %v, want an error with reason %q", test.name, reply, err, test.reason)
		}
	}
}

func TestNotifyCleansAndBoundsItsText(t *testing.T) {
	coordinator, _ := notifyRig(t, daemonOptions{})
	client := subscribedClient(t)
	long := strings.Repeat("é", 200)
	if _, err := notifyFrame(t, coordinator, "a\x1b[31mb\nc"+long, "x\ty\x00"+long); err != nil {
		t.Fatalf("notify: %v", err)
	}
	event := nextFrame(t, client, "notify").Notify
	if strings.ContainsAny(event.Title+event.Body, "\x1b\x00\n\t") || !strings.HasPrefix(event.Title, "a[31mb c") {
		t.Fatalf("control characters should drop and white space read as one space: %q / %q", event.Title, event.Body)
	}
	if len([]rune(event.Title)) > maxNotifyTitle || len([]rune(event.Body)) > maxNotifyBody {
		t.Fatalf("title %d and body %d runes, want at most %d and %d", len([]rune(event.Title)), len([]rune(event.Body)), maxNotifyTitle, maxNotifyBody)
	}
}

// An agent-compose that predates the attribute marks no role, so nobody may notify.
func TestNotifyRefusesEverySeatWhileTheRosterMarksNoCoordinator(t *testing.T) {
	notifyRoster(t, "")
	testDaemon(t)
	seat := dialTest(t)
	seat.spawn("eng-platform-beetle-ox", "eng-platform", "Beetle-Ox", `echo TOKEN=$ATERM_SESSION_TOKEN; sleep 30`)
	reply, err := notifyFrame(t, seat.token(), "Hello", "World")
	if err == nil || reply.Reason != reasonNotCoordinator {
		t.Fatalf("notify = %+v, %v, want not_coordinator", reply, err)
	}
}

func TestNotifyFailsClosedWhenTheRosterCannotBeRead(t *testing.T) {
	t.Setenv("AGENT_COMPOSE_BIN", filepath.Join(t.TempDir(), "absent"))
	testDaemon(t)
	seat := dialTest(t)
	seat.spawn("eng-platform-beetle-ox", "eng-platform", "Beetle-Ox", `echo TOKEN=$ATERM_SESSION_TOKEN; sleep 30`)
	reply, err := notifyFrame(t, seat.token(), "Hello", "World")
	if err == nil || reply.Reason != reasonRosterUnavailable {
		t.Fatalf("notify = %+v, %v, want roster_unavailable", reply, err)
	}
}

func TestNotifyPushesToABrowserThatIsNotConnected(t *testing.T) {
	t.Setenv(stateDirEnv, t.TempDir())
	bodies := standInPushService(t)
	coordinator, _ := notifyRig(t, daemonOptions{VAPIDKey: mustKey(t)})
	browser := dialTest(t).c
	sub, ua, auth := testSubscription(t, "https://fcm.googleapis.com/fcm/send/phone")
	if _, err := browser.request(frame{Type: "push_subscribe", Push: &sub}); err != nil {
		t.Fatal(err)
	}
	_ = browser.Close()
	if _, err := notifyFrame(t, coordinator, "Decision needed", "Ship it?"); err != nil {
		t.Fatalf("notify: %v", err)
	}
	select {
	case body := <-bodies:
		var payload pushPayload
		if err := json.Unmarshal(decryptPush(t, ua, auth, body), &payload); err != nil {
			t.Fatal(err)
		}
		want := pushPayload{Kind: "notify", Session: "eng-platform-beetle-ox", Role: "eng-platform", Identity: "Beetle-Ox",
			Title: "Decision needed", Body: "Ship it?", Tag: "aterm-eng-platform-beetle-ox"}
		if payload != want {
			t.Fatalf("push = %+v, want %+v", payload, want)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("no push reached the subscribed browser")
	}
}

func TestWelcomeOffersNotify(t *testing.T) {
	testDaemon(t)
	if features := dialTest(t).c.features; !slices.Contains(features, notifyFeature) {
		t.Fatalf("welcome features %v lack %q", features, notifyFeature)
	}
}
