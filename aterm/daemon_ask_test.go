package main

import (
	"net"
	"os"
	"strings"
	"testing"
	"time"
)

// nextFrame reads until a frame of the wanted type, failing on a deadline.
func nextFrame(t *testing.T, c *conn, want string) frame {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		_ = c.raw.SetReadDeadline(deadline)
		message, err := c.read()
		if err != nil {
			t.Fatalf("waiting for %s: %v", want, err)
		}
		if message.Type == want {
			return message
		}
	}
}

// askingSeat is a live session whose token can ask, plus a connection to ask on.
func askingSeat(t *testing.T) (*testClient, string) {
	t.Helper()
	testDaemon(t)
	seat := dialTest(t)
	seat.spawn("eng-platform-beetle-ox", "eng-platform", "Beetle-Ox", `echo TOKEN=$ATERM_SESSION_TOKEN; sleep 30`)
	return seat, seat.token()
}

func subscribed(t *testing.T) *conn {
	t.Helper()
	client := dialTest(t)
	if err := client.c.write(frame{Type: "subscribe", ID: "s", Channel: "sessions"}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	nextFrame(t, client.c, "sessions")
	return client.c
}

func startAsk(t *testing.T, token string, ask choiceAsk) (*conn, chan frame) {
	t.Helper()
	asker := dialTest(t).c
	replies := make(chan frame, 1)
	if err := asker.write(frame{Type: "ask", ID: "q", Token: token, Ask: &ask}); err != nil {
		t.Fatalf("ask: %v", err)
	}
	go func() {
		for {
			message, err := asker.read()
			if err != nil {
				close(replies)
				return
			}
			if message.ID == "q" {
				replies <- message
				return
			}
		}
	}()
	return asker, replies
}

var colors = choiceAsk{Question: "Which color?", Options: []choiceOption{{Label: "red"}, {Label: "green", Description: "the calm one"}}}

func TestAskReachesClientsStampedAndReturnsThePick(t *testing.T) {
	_, token := askingSeat(t)
	client := subscribed(t)
	_, replies := startAsk(t, token, colors)
	shown := nextFrame(t, client, "ask").Ask
	if shown.From != "eng-platform Beetle-Ox" || shown.Session != "eng-platform-beetle-ox" || shown.Options[1].Description != "the calm one" {
		t.Fatalf("the ask should arrive stamped with the asker's seat: %+v", shown)
	}
	if err := client.write(frame{Type: "answer", ID: "a", AskID: shown.ID, Picks: []int{1}}); err != nil {
		t.Fatalf("answer: %v", err)
	}
	if settled := nextFrame(t, client, "asked"); settled.AskID != shown.ID || settled.State != "answered" {
		t.Fatalf("every client should hear it settle: %+v", settled)
	}
	reply := <-replies
	if reply.Type != "answered" || reply.Answer.State != "answered" || strings.Join(reply.Answer.Labels, ",") != "green" {
		t.Fatalf("the asker should get the pick back: %+v %+v", reply, reply.Answer)
	}
}

func TestAskReplaysToAClientThatSubscribesLate(t *testing.T) {
	_, token := askingSeat(t)
	early := subscribed(t)
	startAsk(t, token, colors)
	id := nextFrame(t, early, "ask").Ask.ID
	late := subscribed(t)
	if replayed := nextFrame(t, late, "ask").Ask; replayed.ID != id {
		t.Fatalf("a pending ask should replay on subscribe: %+v", replayed)
	}
}

func TestAskIsCancelledWhenTheAskerGoesAway(t *testing.T) {
	_, token := askingSeat(t)
	client := subscribed(t)
	asker, _ := startAsk(t, token, colors)
	id := nextFrame(t, client, "ask").Ask.ID
	_ = asker.Close()
	if settled := nextFrame(t, client, "asked"); settled.AskID != id || settled.State != "cancelled" {
		t.Fatalf("an asker that left should cancel its ask: %+v", settled)
	}
}

func TestAnswerIsHeldToWhatTheAskOffered(t *testing.T) {
	_, token := askingSeat(t)
	client := subscribed(t)
	startAsk(t, token, colors)
	id := nextFrame(t, client, "ask").Ask.ID
	for name, bad := range map[string]frame{
		"a pick past the options": {Picks: []int{2}},
		"two picks on one-pick":   {Picks: []int{0, 1}},
		"text with no other":      {Text: "blue"},
		"nothing at all":          {},
	} {
		bad.Type, bad.ID, bad.AskID = "answer", "bad", id
		if err := client.write(bad); err != nil {
			t.Fatalf("write: %v", err)
		}
		if reply := nextFrame(t, client, "error"); reply.Code != exitUsage {
			t.Fatalf("%s should be refused with exit 2: %+v", name, reply)
		}
	}
}

// The timeout is the daemon's own, so this test serves one over pipes
// rather than changing a value another test's daemon is reading.
func TestAskTimesOut(t *testing.T) {
	d := newDaemon(func(string, ...any) {})
	d.holdDir = testHoldDir(t)
	d.askTimeout = 300 * time.Millisecond
	t.Cleanup(d.endAll)
	pipe := func() *conn {
		client, server := net.Pipe()
		go d.serveConn(newConn(server), peerStanding{pids: []int{0}})
		c := newConn(client)
		t.Cleanup(func() { _ = c.Close() })
		if err := c.write(frame{Type: "hello", Format: daemonFormat}); err != nil {
			t.Fatalf("hello: %v", err)
		}
		nextFrame(t, c, "welcome")
		return c
	}
	seat := pipe()
	if _, err := seat.request(frame{
		Type: "spawn", Session: "eng-platform-beetle-ox", Role: "eng-platform", Identity: "Beetle-Ox",
		Argv: []string{"/bin/sh", "-c", "sleep 30"}, Env: os.Environ(), Cwd: "/",
	}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	token := d.session("eng-platform-beetle-ox").token
	reply, err := pipe().request(frame{Type: "ask", Token: token, Ask: &colors})
	if err != nil || reply.Answer == nil || reply.Answer.State != "timed_out" {
		t.Fatalf("an unanswered ask should time out: %+v %v", reply.Answer, err)
	}
}

func TestAskRefusesATokenItNeverIssued(t *testing.T) {
	testDaemon(t)
	c := dialTest(t)
	reply, err := c.c.request(frame{Type: "ask", Token: "forged", Ask: &colors})
	if err == nil || reply.Code != exitUsage {
		t.Fatalf("a forged token must not ask: %v (code %d)", err, reply.Code)
	}
}
