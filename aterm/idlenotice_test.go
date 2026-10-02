package main

import (
	"strings"
	"testing"
	"time"
)

// sendNotifying is a send that asks for an idle notice back.
func sendNotifying(t *testing.T, token, target, body string) peerMessage {
	t.Helper()
	c, err := dialDaemon(false)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	reply, err := c.request(frame{Type: "send", Token: token, Target: target, Body: body, NotifyIdle: true})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	return *reply.Message
}

func TestIdleDueNeedsWorkOrGrace(t *testing.T) {
	for _, row := range []struct {
		state   string
		sawWork bool
		since   time.Duration
		want    bool
	}{
		{stateIdle, true, time.Second, true},
		{stateIdle, false, time.Second, false},
		{stateIdle, false, idleGrace + time.Second, true},
		{stateBusy, true, time.Hour, false},
		{statePrompt, true, time.Hour, false},
		{stateStarting, false, time.Hour, false},
	} {
		if got := idleDue(row.state, row.sawWork, row.since); got != row.want {
			t.Errorf("idleDue(%s, work=%v, %v) = %v, want %v", row.state, row.sawWork, row.since, got, row.want)
		}
	}
}

func TestNotifyIdleTypesOneLineWhenTheTargetGoesQuiet(t *testing.T) {
	token, sender, target := spawnPair(t, pasteCat)
	time.Sleep(600 * time.Millisecond)
	if state := sendNotifying(t, token, "scientist", "do the thing"); state.State != "delivered" {
		t.Fatalf("state = %+v, want delivered at once", state)
	}
	target.until("do the thing")
	// The echo counts as work, then the screen goes quiet for the busy window.
	sender.until("[from aterm daemon] scientist-evie is idle")
	// One notice shows twice, since the sender's terminal echoes what is typed and its
	// cat prints it again. A second notice would make it four.
	sender.drain(6 * time.Second)
	if got := strings.Count(sender.output.String(), "scientist-evie is idle"); got > 2 {
		t.Fatalf("more than one notice came, %d mentions:\n%q", got, sender.output.String())
	}
}

func TestNoIdleNoticeWithoutTheAsk(t *testing.T) {
	token, sender, target := spawnPair(t, pasteCat)
	time.Sleep(600 * time.Millisecond)
	if state := sendAs(t, token, "scientist", "ping"); state.State != "delivered" {
		t.Fatalf("state = %+v, want delivered at once", state)
	}
	target.until("ping")
	sender.drain(6 * time.Second)
	if strings.Contains(sender.output.String(), "is idle") {
		t.Fatalf("a send that did not ask owes no idle notice:\n%q", sender.output.String())
	}
}

func TestNotifyIdleNamesATargetThatEndsInstead(t *testing.T) {
	token, sender, _ := spawnPair(t, `printf 'READY\033[?2004h\n'; read -r _`)
	time.Sleep(600 * time.Millisecond)
	sendNotifying(t, token, "scientist", "last words")
	sender.until("[from aterm daemon] scientist-evie ended")
}
