package main

import (
	"fmt"
	"testing"
	"time"
)

// busyHost spawns a sender and enough other sessions that one sessions frame is
// larger than a socket buffer, which is what a real host with a dozen seats sends.
func busyHost(t *testing.T) (token string) {
	t.Helper()
	testDaemon(t)
	sender := dialTest(t)
	sender.spawn("eng-platform-beetle-ox", "eng-platform", "Beetle-Ox", `echo TOKEN=$ATERM_SESSION_TOKEN; sleep 120`)
	token = sender.token()
	for i := 2; i < 18; i++ {
		dialTest(t).spawn(fmt.Sprintf("frontend-eng-imp-dragonfly-%d", i), "frontend-eng", "Imp-Dragonfly", `printf 'UP\033[?2004h\n'; cat >/dev/null`)
	}
	return token
}

// A subscriber that stopped reading must not hold up the daemon, so a send is
// answered well under the 5s a blocked write waits.
func TestBroadcastDoesNotWaitOnAStalledSubscriber(t *testing.T) {
	token := busyHost(t)
	stalled := dialTest(t)
	if err := stalled.c.write(frame{Type: "subscribe", ID: "s", Channel: "sessions"}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	for i := 0; i < 2; i++ {
		started := time.Now()
		sendAs(t, token, fmt.Sprintf("frontend-eng-imp-dragonfly-%d", i+2), "x")
		if took := time.Since(started); took > 3*time.Second {
			t.Fatalf("send %d took %s behind a subscriber that reads nothing", i, took)
		}
	}
}

// send --new reads its watch during the launch. Unread, the watch fills and is
// dropped, and the sender hears "launching" with no session (COI-2300).
func TestSendNewNamesTheInstanceWhenTheLaunchFloodsTheWatch(t *testing.T) {
	token := busyHost(t)
	t.Setenv(sessionTokenEnv, token)
	earlier := openRole
	t.Cleanup(func() { openRole = earlier })
	openRole = func(role, _ string) error {
		// A launch is slow and the host stays busy while it runs, so each send
		// to a live seat pushes a sessions frame at every subscriber.
		for i := 0; i < 12; i++ {
			sendAs(t, token, fmt.Sprintf("frontend-eng-imp-dragonfly-%d", i+2), "x")
		}
		dialTest(t).spawn("scientist-evie-cd12", "scientist", "Evie", `printf 'UP\033[?2004h\n'; cat >/dev/null`)
		return nil
	}
	state, err := sendMessage("scientist", "hello", sendOptions{Fresh: true})
	if err != nil {
		t.Fatalf("sendMessage: %v", err)
	}
	if state.Session != "scientist-evie-cd12" {
		t.Fatalf("sendMessage = %+v, want the instance that took the message named", state)
	}
}
