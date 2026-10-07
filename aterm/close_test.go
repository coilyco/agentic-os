package main

import (
	"net"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestDescendsFromFollowsTheParentChain(t *testing.T) {
	d := newDaemon(func(string, ...any) {})
	d.processes = func() ([]processEntry, error) {
		return []processEntry{{PID: 100, PPID: 50}, {PID: 101, PPID: 100}, {PID: 200, PPID: 1}}, nil
	}
	if !d.descendsFrom(101, map[int]bool{100: true}) {
		t.Fatal("a child of the root runs under it")
	}
	if !d.descendsFrom(100, map[int]bool{100: true}) {
		t.Fatal("the root itself counts")
	}
	if d.descendsFrom(200, map[int]bool{100: true}) {
		t.Fatal("an unrelated process does not")
	}
	if !d.descendsFrom(0, map[int]bool{100: true}) {
		t.Fatal("an unread pid is treated as under, the cautious answer")
	}
}

func closeFrame(t *testing.T, tc *testClient, message frame) (frame, error) {
	t.Helper()
	message.Type = "close"
	return tc.c.request(message)
}

// liveNamesEventually polls the session list, since the table drops a session
// just after its process ends.
func liveNamesEventually(t *testing.T, tc *testClient, gone string) []string {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		reply, err := tc.c.request(frame{Type: "list"})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		var names []string
		for _, view := range reply.Sessions {
			names = append(names, view.Name)
		}
		if !slices.Contains(names, gone) || time.Now().After(deadline) {
			return names
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestDaemonClosesASessionAndDropsItFromTheTable(t *testing.T) {
	testDaemon(t)
	room := dialTest(t)
	room.spawn("scientist-evie", "scientist", "Evie", `echo UP; sleep 30`)
	room.until("UP")
	closer := dialTest(t)
	reply, err := closeFrame(t, closer, frame{Target: "scientist"})
	if err != nil {
		t.Fatalf("close: %v", err)
	}
	if reply.Type != "closed" || reply.Session != "scientist-evie" {
		t.Fatalf("reply = %+v, want closed scientist-evie", reply)
	}
	if names := liveNamesEventually(t, closer, "scientist-evie"); slices.Contains(names, "scientist-evie") {
		t.Fatalf("scientist-evie is still listed after close: %v", names)
	}
	if _, err := closeFrame(t, closer, frame{Target: "scientist-evie"}); err == nil {
		t.Fatal("a second close of the same session should find nothing")
	}
}

func TestDaemonRefusesToCloseTheCallersOwnSession(t *testing.T) {
	testDaemon(t)
	self := dialTest(t)
	self.spawn("eng-platform-beetle-ox", "eng-platform", "Beetle-Ox", `echo TOKEN=$ATERM_SESSION_TOKEN; sleep 30`)
	token := self.token()
	closer := dialTest(t)
	reply, err := closeFrame(t, closer, frame{Token: token, Target: "eng-platform"})
	if err == nil || reply.Code != exitUsage || !strings.Contains(err.Error(), "is this session") {
		t.Fatalf("closing the caller's own session must refuse with exit 2, got %v (code %d)", err, reply.Code)
	}
}

func TestDaemonRefusesAnAmbiguousCloseTarget(t *testing.T) {
	testDaemon(t)
	first := dialTest(t)
	first.spawn("eng-junior-beetle-ox-aa11", "eng-junior", "Beetle-Ox", `echo UP; sleep 30`)
	first.until("UP")
	second := dialTest(t)
	second.spawn("eng-junior-beetle-ox-bb22", "eng-junior", "Beetle-Ox", `echo UP; sleep 30`)
	second.until("UP")
	reply, err := closeFrame(t, first, frame{Target: "eng-junior"})
	if err == nil || reply.Code != exitUsage || !strings.Contains(err.Error(), "name one") {
		t.Fatalf("a role matching two sessions must refuse, got %v (code %d)", err, reply.Code)
	}
}

func TestDaemonKeepsASessionHoldingKaisDraftUnlessForced(t *testing.T) {
	testDaemon(t)
	target := dialTest(t)
	target.spawn("scientist-evie", "scientist", "Evie", `printf 'READY\033[?2004h\n'; cat`)
	target.until("READY")
	time.Sleep(1600 * time.Millisecond)
	if err := target.c.write(frame{Type: "input", Session: "scientist-evie", Data: []byte("half a thou")}); err != nil {
		t.Fatalf("type: %v", err)
	}
	target.until("half a thou")
	closer := dialTest(t)
	reply, err := closeFrame(t, closer, frame{Target: "scientist-evie"})
	if err == nil || reply.Code != exitUsage || !strings.Contains(err.Error(), "draft") {
		t.Fatalf("a session holding a draft must stay open, got %v (code %d)", err, reply.Code)
	}
	reply, err = closeFrame(t, closer, frame{Target: "scientist-evie", Force: true})
	if err != nil || reply.Session != "scientist-evie" {
		t.Fatalf("force should close it, got %+v, %v", reply, err)
	}
}

// A pool name recurs, so "closed" has to mean the name is free. The owner stops
// reading, so its exit frame holds the session between process end and forget.
func TestClosedReplyMeansTheNameIsFree(t *testing.T) {
	d := newDaemon(func(string, ...any) {})
	d.holdDir = testHoldDir(t)
	t.Cleanup(d.endAll)
	pipe := func(browser bool) *conn {
		client, server := net.Pipe()
		go d.serveConn(newConn(server), 0, browser)
		c := newConn(client)
		t.Cleanup(func() { _ = c.Close() })
		if err := c.write(frame{Type: "hello", Format: daemonFormat}); err != nil {
			t.Fatalf("hello: %v", err)
		}
		nextFrame(t, c, "welcome")
		return c
	}
	owner := pipe(false)
	if _, err := owner.request(frame{
		Type: "spawn", Session: "close-pooled", Role: "frontend-eng", Identity: "Imp",
		Argv: []string{"/bin/sh", "-c", "exec sleep 30"}, Env: os.Environ(), Cwd: "/",
	}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	go func() {
		time.Sleep(400 * time.Millisecond)
		for {
			if _, err := owner.read(); err != nil {
				return
			}
		}
	}()
	if _, err := pipe(true).request(frame{Type: "close", Target: "close-pooled", Force: true}); err != nil {
		t.Fatalf("close: %v", err)
	}
	if d.session("close-pooled") != nil {
		t.Fatal("close replied while the daemon still held the name, so a respawn of it is refused")
	}
}
