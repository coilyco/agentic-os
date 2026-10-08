package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/urfave/cli/v3"
)

func seatView(name, state string) sessionView {
	return sessionView{Name: name, Role: "eng-platform", Identity: "Beetle-Ox", State: state}
}

func TestWaitTrackerPushesWhenATurnEndsAndHoldsForTheSettleTime(t *testing.T) {
	var tracker waitTracker
	start := time.Unix(1_800_000_000, 0)
	at := func(seconds float64) time.Time { return start.Add(time.Duration(seconds * float64(time.Second))) }

	if got := tracker.observe([]sessionView{seatView("a", stateIdle)}, at(0)); len(got) != 0 {
		t.Fatalf("a seat already waiting when first seen is a baseline: %+v", got)
	}
	if got := tracker.observe([]sessionView{seatView("a", stateIdle)}, at(10)); len(got) != 0 {
		t.Fatalf("a seat that stays idle never pushes: %+v", got)
	}
	tracker.observe([]sessionView{seatView("a", stateBusy)}, at(11))
	tracker.observe([]sessionView{seatView("a", stateIdle)}, at(12))
	if got := tracker.observe([]sessionView{seatView("a", stateIdle)}, at(12+pushSettle.Seconds()-0.5)); len(got) != 0 {
		t.Fatalf("idle for less than the settle time should not push yet: %+v", got)
	}
	got := tracker.observe([]sessionView{seatView("a", stateIdle)}, at(12+pushSettle.Seconds()))
	if len(got) != 1 || got[0].Kind != "done" || got[0].Session != "a" || got[0].Identity != "Beetle-Ox" {
		t.Fatalf("a finished turn should push once: %+v", got)
	}
	if got := tracker.observe([]sessionView{seatView("a", stateIdle)}, at(30)); len(got) != 0 {
		t.Fatalf("one transition pushes once: %+v", got)
	}
}

func TestWaitTrackerIgnoresAGapThatIsNotTheEndOfATurn(t *testing.T) {
	var tracker waitTracker
	start := time.Unix(1_800_000_000, 0)
	tracker.observe([]sessionView{seatView("a", stateBusy)}, start)
	tracker.observe([]sessionView{seatView("a", stateIdle)}, start.Add(time.Second))
	tracker.observe([]sessionView{seatView("a", stateBusy)}, start.Add(2*time.Second))
	if got := tracker.observe([]sessionView{seatView("a", stateBusy)}, start.Add(10*time.Second)); len(got) != 0 {
		t.Fatalf("a seat that went quiet for a beat and resumed is still working: %+v", got)
	}
	tracker.observe([]sessionView{seatView("a", stateStarting)}, start.Add(11*time.Second))
	if got := tracker.observe([]sessionView{seatView("a", stateIdle)}, start.Add(20*time.Second)); len(got) != 0 {
		t.Fatalf("starting to idle is a launch finishing, not a turn: %+v", got)
	}
}

func TestWaitTrackerPushesAPermissionCardAndRespectsTheCooldown(t *testing.T) {
	var tracker waitTracker
	start := time.Unix(1_800_000_000, 0)
	tracker.observe([]sessionView{seatView("a", stateBusy)}, start)
	tracker.observe([]sessionView{seatView("a", statePrompt)}, start.Add(time.Second))
	got := tracker.observe([]sessionView{seatView("a", statePrompt)}, start.Add(time.Second+pushSettle))
	if len(got) != 1 || got[0].Kind != "prompt" {
		t.Fatalf("a card holding the seat should push: %+v", got)
	}
	// The card is answered and the turn ends inside the cooldown.
	tracker.observe([]sessionView{seatView("a", stateBusy)}, start.Add(7*time.Second))
	tracker.observe([]sessionView{seatView("a", stateIdle)}, start.Add(8*time.Second))
	if got := tracker.observe([]sessionView{seatView("a", stateIdle)}, start.Add(8*time.Second+pushSettle)); len(got) != 0 {
		t.Fatalf("a second push about one seat inside the cooldown should be held: %+v", got)
	}
	tracker.observe([]sessionView{seatView("a", stateBusy)}, start.Add(60*time.Second))
	tracker.observe([]sessionView{seatView("a", stateIdle)}, start.Add(61*time.Second))
	if got := tracker.observe([]sessionView{seatView("a", stateIdle)}, start.Add(61*time.Second+pushSettle)); len(got) != 1 {
		t.Fatalf("after the cooldown the next turn should push: %+v", got)
	}
}

func TestWaitTrackerForgetsASeatThatEnded(t *testing.T) {
	var tracker waitTracker
	start := time.Unix(1_800_000_000, 0)
	tracker.observe([]sessionView{seatView("a", stateBusy)}, start)
	tracker.observe(nil, start.Add(time.Second))
	if len(tracker.seats) != 0 {
		t.Fatalf("an ended seat should not be remembered: %+v", tracker.seats)
	}
	if got := tracker.observe([]sessionView{seatView("a", stateIdle)}, start.Add(10*time.Second)); len(got) != 0 {
		t.Fatalf("a new seat under an old name starts from a baseline: %+v", got)
	}
}

func TestPushStoreSurvivesARestartAndReplacesByEndpoint(t *testing.T) {
	path := filepath.Join(t.TempDir(), "push-subscriptions.json")
	store, err := loadPushStore(path)
	if err != nil {
		t.Fatal(err)
	}
	first, _, _ := testSubscription(t, "https://fcm.googleapis.com/fcm/send/one")
	second, _, _ := testSubscription(t, "https://fcm.googleapis.com/fcm/send/two")
	for _, sub := range []pushSubscription{first, second, first} {
		if err := store.add(sub); err != nil {
			t.Fatal(err)
		}
	}
	reloaded, err := loadPushStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.list(); len(got) != 2 {
		t.Fatalf("the same endpoint twice is one subscription: %+v", got)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("the list names devices to alert, so it is private: %v %v", info, err)
	}
	if err := reloaded.remove(first.Endpoint); err != nil {
		t.Fatal(err)
	}
	if got := reloaded.list(); len(got) != 1 || got[0].Endpoint != second.Endpoint {
		t.Fatalf("remove should drop only its endpoint: %+v", got)
	}
}

func TestPushStoreRefusesPastItsCap(t *testing.T) {
	store, _ := loadPushStore("")
	for index := range pushMaxSubscriptions {
		sub, _, _ := testSubscription(t, "https://fcm.googleapis.com/fcm/send/"+string(rune('a'+index)))
		if err := store.add(sub); err != nil {
			t.Fatalf("subscription %d: %v", index, err)
		}
	}
	extra, _, _ := testSubscription(t, "https://fcm.googleapis.com/fcm/send/extra")
	if err := store.add(extra); err == nil {
		t.Fatal("a store past its cap should refuse, so a page cannot grow it without bound")
	}
}

func TestPushWaitingDropsASubscriptionThePushServiceForgot(t *testing.T) {
	d := newDaemon(func(string, ...any) {})
	store, _ := loadPushStore("")
	if err := d.setupPush(mustKey(t), store); err != nil {
		t.Fatal(err)
	}
	gone, _, _ := testSubscription(t, "https://fcm.googleapis.com/fcm/send/gone")
	kept, _, _ := testSubscription(t, "https://fcm.googleapis.com/fcm/send/kept")
	_ = store.add(gone)
	_ = store.add(kept)
	var mu sync.Mutex
	sent := map[string]int{}
	done := make(chan struct{}, 2)
	d.push.send = func(_ context.Context, _ *vapidKey, sub pushSubscription, _ []byte) (int, error) {
		mu.Lock()
		sent[sub.Endpoint]++
		mu.Unlock()
		defer func() { done <- struct{}{} }()
		if sub.Endpoint == gone.Endpoint {
			return http.StatusGone, nil
		}
		return http.StatusCreated, nil
	}
	d.pushWaiting(waitPayload(waitEvent{Kind: "done", Session: "a", Identity: "Beetle-Ox"}, ""))
	<-done
	<-done
	for range 100 {
		if len(store.list()) == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := store.list(); len(got) != 1 || got[0].Endpoint != kept.Endpoint {
		t.Fatalf("a 410 means the browser dropped it, so the daemon should too: %+v", got)
	}
}

func TestPushIsOffWithoutAKey(t *testing.T) {
	d := newDaemon(func(string, ...any) {})
	if err := d.setupPush("", nil); err != nil || d.push.enabled() {
		t.Fatalf("no key means no push: %v", err)
	}
	called := false
	d.push.send = func(_ context.Context, _ *vapidKey, _ pushSubscription, _ []byte) (int, error) {
		called = true
		return 201, nil
	}
	d.pushWaiting(pushPayload{})
	d.pushAsk(choiceAsk{Question: "q"})
	if called {
		t.Fatal("a daemon without a VAPID key should send nothing")
	}
	if err := d.setupPush("not a key", nil); err == nil {
		t.Fatal("a malformed key should be reported, not silently turn push off")
	}
}

func TestWaitPayloadNamesTheSeatAndWhatItWantsOfYou(t *testing.T) {
	done := waitPayload(waitEvent{Kind: "done", Session: "eng-platform-2", Identity: "Beetle-Ox"}, "")
	if done.Title != "Beetle-Ox is done" || done.Tag != "aterm-eng-platform-2" || done.Session != "eng-platform-2" {
		t.Fatalf("done: %+v", done)
	}
	ask := waitPayload(waitEvent{Kind: "ask", Session: "s"}, "Which color?")
	if ask.Title != "s is asking" || ask.Body != "Which color?" {
		t.Fatalf("a seat with no identity falls back to its session name: %+v", ask)
	}
}

// standInPushService answers any push endpoint the daemon dials, and hands each
// request body to the test. The daemon's allowlist still judges the endpoint.
func standInPushService(t *testing.T) <-chan []byte {
	t.Helper()
	bodies := make(chan []byte, 4)
	service := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		bodies <- body
		w.WriteHeader(http.StatusCreated)
	}))
	t.Cleanup(service.Close)
	roots := x509.NewCertPool()
	roots.AddCert(service.Certificate())
	previous := pushClient.Transport
	pushClient.Transport = &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, service.Listener.Addr().String())
		},
		// The test server's certificate names example.com, so verification stays on.
		TLSClientConfig: &tls.Config{RootCAs: roots, ServerName: "example.com"},
	}
	t.Cleanup(func() { pushClient.Transport = previous })
	return bodies
}

func TestDaemonPushesAnAskToASubscribedBrowserThatIsNotConnected(t *testing.T) {
	t.Setenv(stateDirEnv, t.TempDir())
	bodies := standInPushService(t)
	key := mustKey(t)
	testDaemonWith(t, daemonOptions{VAPIDKey: key})

	seat := dialTest(t)
	seat.spawn("eng-platform-beetle-ox", "eng-platform", "Beetle-Ox", `echo TOKEN=$ATERM_SESSION_TOKEN; sleep 30`)
	token := seat.token()

	browser := dialTest(t).c
	if err := browser.write(frame{Type: "push_key", ID: "k"}); err != nil {
		t.Fatal(err)
	}
	if reply := nextFrame(t, browser, "push_key"); reply.Key == "" {
		t.Fatalf("a client needs the public key to subscribe against: %+v", reply)
	}
	sub, ua, auth := testSubscription(t, "https://fcm.googleapis.com/fcm/send/phone")
	if err := browser.write(frame{Type: "push_subscribe", ID: "s", Push: &sub}); err != nil {
		t.Fatal(err)
	}
	nextFrame(t, browser, "push_subscribed")
	// The browser closes. Nothing is attached when the seat asks.
	_ = browser.Close()

	startAsk(t, token, colors)
	select {
	case body := <-bodies:
		var payload pushPayload
		if err := json.Unmarshal(decryptPush(t, ua, auth, body), &payload); err != nil {
			t.Fatal(err)
		}
		if payload.Kind != "ask" || payload.Session != "eng-platform-beetle-ox" || payload.Body != "Which color?" || payload.Title != "Beetle-Ox is asking" {
			t.Fatalf("the phone should be told which seat asks what: %+v", payload)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("no push reached the subscribed browser")
	}
}

func TestDaemonRefusesPushFramesItCannotServe(t *testing.T) {
	t.Setenv(stateDirEnv, t.TempDir())
	testDaemon(t)
	client := dialTest(t).c
	if err := client.write(frame{Type: "push_key", ID: "k"}); err != nil {
		t.Fatal(err)
	}
	if reply := nextFrame(t, client, "error"); reply.ID != "k" {
		t.Fatalf("a daemon without a key should say so: %+v", reply)
	}

	testDaemonWith(t, daemonOptions{VAPIDKey: mustKey(t)})
	client = dialTest(t).c
	for name, sub := range map[string]pushSubscription{
		"loopback":    {Endpoint: "https://127.0.0.1/x"},
		"metadata ip": {Endpoint: "https://169.254.169.254/x"},
	} {
		if err := client.write(frame{Type: "push_subscribe", ID: name, Push: &sub}); err != nil {
			t.Fatal(err)
		}
		if reply := nextFrame(t, client, "error"); reply.ID != name {
			t.Fatalf("%s: a subscription aiming the daemon off the push services should be refused: %+v", name, reply)
		}
	}
	if err := client.write(frame{Type: "push_subscribe", ID: "none"}); err != nil {
		t.Fatal(err)
	}
	nextFrame(t, client, "error")
}

func TestRemoteDeviceNeedsItsPasskeyToSubscribe(t *testing.T) {
	for _, frameType := range []string{"push_subscribe", "push_unsubscribe"} {
		if !remoteLocked[frameType] {
			t.Errorf("%s aims alerts at a device, so a remote one asserts its passkey first", frameType)
		}
	}
	if remoteLocked["push_key"] {
		t.Error("the VAPID public key is public, so asking for it is not locked")
	}
}

func TestVAPIDCommandWritesTheKeyPrivatelyAndPrintsOnlyThePublicHalf(t *testing.T) {
	out := filepath.Join(t.TempDir(), "vapid-key")
	run := func() (string, error) {
		var stdout bytes.Buffer
		app := &cli.Command{Name: "aterm", Writer: &stdout, ErrWriter: &stdout, Commands: []*cli.Command{newVAPIDCommand()}}
		err := app.Run(context.Background(), []string{"aterm", "vapid", "--out", out})
		return stdout.String(), err
	}
	printed, err := run()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Stat(out); info.Mode().Perm() != 0o600 {
		t.Fatalf("the secret half is private: %v", info.Mode())
	}
	key, err := parseVAPIDKey(string(raw))
	if err != nil {
		t.Fatalf("the file should hold a key the daemon reads: %v", err)
	}
	if printed != key.publicKey()+"\n" {
		t.Fatalf("stdout should be the public key alone, got %q", printed)
	}
	if _, err := run(); err == nil {
		t.Fatal("an existing key file should be refused, since overwriting loses the old key")
	}
}

func mustKey(t *testing.T) string {
	t.Helper()
	_, encoded, err := newVAPIDKey()
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func TestWatchWaitingPushesWhenALiveSeatEndsItsTurn(t *testing.T) {
	d := newDaemon(func(string, ...any) {})
	store, _ := loadPushStore("")
	if err := d.setupPush(mustKey(t), store); err != nil {
		t.Fatal(err)
	}
	sub, _, _ := testSubscription(t, "https://fcm.googleapis.com/fcm/send/phone")
	_ = store.add(sub)
	sent := make(chan []byte, 4)
	d.push.send = func(_ context.Context, _ *vapidKey, _ pushSubscription, payload []byte) (int, error) {
		sent <- payload
		return http.StatusCreated, nil
	}
	d.push.tracker.settle = 20 * time.Millisecond
	long := time.Now().Add(-time.Hour)
	seat := &ptySession{d: d, name: "eng-platform-beetle-ox", role: "eng-platform", identity: "Beetle-Ox", seat: "claude", started: long, promptSeen: long, lastOutput: time.Now()}
	d.sessions[seat.name] = seat

	done := make(chan struct{})
	t.Cleanup(func() { close(done) })
	go d.watchWaiting(done, 10*time.Millisecond)
	time.Sleep(60 * time.Millisecond)
	select {
	case payload := <-sent:
		t.Fatalf("a seat that is working has nothing to tell anyone yet: %s", payload)
	default:
	}
	seat.mu.Lock()
	seat.lastOutput = time.Now().Add(-time.Minute)
	seat.mu.Unlock()
	select {
	case payload := <-sent:
		var got pushPayload
		if err := json.Unmarshal(payload, &got); err != nil || got.Kind != "done" || got.Session != seat.name {
			t.Fatalf("the watcher should push the finished turn: %s %v", payload, err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a live seat that went quiet never pushed")
	}
}
