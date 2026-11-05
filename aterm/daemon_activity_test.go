package main

import (
	"net"
	"os"
	"strings"
	"testing"
	"time"
)

func TestCleanActivity(t *testing.T) {
	long := strings.Repeat("é", 80)
	cases := []struct{ name, in, want string }{
		{"plain", "reading the diff", "reading the diff"},
		{"newline becomes a space", "building\nthe daemon half", "building the daemon half"},
		{"carriage return and tab", "a\r\n\tb", "a b"},
		{"control characters dropped", "run\x1b[31m tests\x07", "run[31m tests"},
		{"bidi override dropped", "ok‮gnp.exe", "okgnp.exe"},
		{"invalid utf-8 dropped", "ab\xffcd", "abcd"},
		{"edges trimmed", "  hi  ", "hi"},
		{"only whitespace is empty", " \n\t ", ""},
		{"exactly 57 kept", strings.Repeat("x", 57), strings.Repeat("x", 57)},
		{"58 cut to 57", strings.Repeat("x", 58), strings.Repeat("x", 57)},
		{"cut counts characters not bytes", long, strings.Repeat("é", 57)},
		{"a cut that lands on a space leaves no trailing space", strings.Repeat("x", 56) + " yyy", strings.Repeat("x", 56)},
	}
	for _, c := range cases {
		if got := cleanActivity(c.in); got != c.want {
			t.Errorf("%s: cleanActivity(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

// activityViewOf reads a session's view back with a list, as a late client would.
func activityViewOf(t *testing.T, name string) sessionView {
	t.Helper()
	c := dialTest(t).c
	reply, err := c.request(frame{Type: "list"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, view := range reply.Sessions {
		if view.Name == name {
			return view
		}
	}
	t.Fatalf("no session %q in %+v", name, reply.Sessions)
	return sessionView{}
}

func setActivityAs(t *testing.T, token, text string) frame {
	t.Helper()
	c := dialTest(t).c
	reply, err := c.request(frame{Type: "activity", Token: token, Text: text})
	if err != nil {
		t.Fatalf("activity: %v", err)
	}
	return reply
}

func TestASeatSetsItsActivityAndTheViewCarriesItWithTheTime(t *testing.T) {
	_, token := askingSeat(t)
	before := time.Now().Add(-time.Second)
	if reply := setActivityAs(t, token, "writing the activity line"); reply.Text != "writing the activity line" {
		t.Fatalf("reply = %+v, want the stored text", reply)
	}
	view := activityViewOf(t, "eng-platform-beetle-ox")
	if view.Activity != "writing the activity line" {
		t.Fatalf("view activity = %q", view.Activity)
	}
	if view.ActivityAt == nil || view.ActivityAt.Before(before) || view.ActivityAt.After(time.Now().Add(time.Second)) {
		t.Fatalf("activity_at = %v, want about now", view.ActivityAt)
	}
}

func TestAnOverlongActivityIsCutTo57AndANewlineIsStripped(t *testing.T) {
	_, token := askingSeat(t)
	reply := setActivityAs(t, token, strings.Repeat("a", 70))
	if reply.Text != strings.Repeat("a", 57) {
		t.Fatalf("overlong stored as %q (%d), want 57 characters", reply.Text, len(reply.Text))
	}
	setActivityAs(t, token, "one\ntwo")
	if got := activityViewOf(t, "eng-platform-beetle-ox").Activity; got != "one two" || strings.ContainsAny(got, "\r\n") {
		t.Fatalf("newline left as %q, want one line", got)
	}
}

func TestAnEmptyActivityClearsIt(t *testing.T) {
	_, token := askingSeat(t)
	setActivityAs(t, token, "busy")
	setActivityAs(t, token, "")
	view := activityViewOf(t, "eng-platform-beetle-ox")
	if view.Activity != "" || view.ActivityAt != nil {
		t.Fatalf("after a clear: %q at %v, want neither", view.Activity, view.ActivityAt)
	}
}

func TestOnlyASessionsOwnTokenSetsItsActivity(t *testing.T) {
	askingSeat(t)
	c := dialTest(t).c
	for _, token := range []string{"", "not-a-token"} {
		if _, err := c.request(frame{Type: "activity", Token: token, Text: "forged"}); err == nil {
			t.Fatalf("token %q set an activity line", token)
		}
	}
	// A target in the frame is ignored: the line belongs to the token's session.
	if got := activityViewOf(t, "eng-platform-beetle-ox").Activity; got != "" {
		t.Fatalf("a refused frame left %q", got)
	}
}

// A seat's output is not a way to set it: the line is whatever `activity` last said.
func TestTheScreenCannotSetTheActivity(t *testing.T) {
	testDaemon(t)
	seat := dialTest(t)
	seat.spawn("eng-platform-beetle-ox", "eng-platform", "Beetle-Ox",
		`printf 'activity: from the screen\n\033]0;from the title\007\033]7750;activity=from an osc\007DONE\n'; sleep 30`)
	seat.until("DONE")
	if got := activityViewOf(t, "eng-platform-beetle-ox").Activity; got != "" {
		t.Fatalf("the screen set the activity line to %q", got)
	}
}

func TestAnActivityChangeIsPushedToAWatcher(t *testing.T) {
	_, token := askingSeat(t)
	watcher := subscribed(t)
	setActivityAs(t, token, "pushed to watchers")
	deadline := time.Now().Add(10 * time.Second)
	for {
		_ = watcher.raw.SetReadDeadline(deadline)
		message, err := watcher.read()
		if err != nil {
			t.Fatalf("waiting for the push: %v", err)
		}
		for _, view := range message.Sessions {
			if message.Type == "sessions" && view.Name == "eng-platform-beetle-ox" && view.Activity == "pushed to watchers" {
				if view.ActivityAt == nil {
					t.Fatalf("the pushed view should carry activity_at: %+v", view)
				}
				return
			}
		}
	}
}

func TestTheActivityEndsWithTheSession(t *testing.T) {
	d := newDaemon(func(string, ...any) {})
	d.holdDir = testHoldDir(t)
	t.Cleanup(d.endAll)
	client, server := net.Pipe()
	go d.serveConn(newConn(server), peerStanding{pids: []int{0}})
	c := newConn(client)
	t.Cleanup(func() { _ = c.Close() })
	if err := c.write(frame{Type: "hello", Format: daemonFormat}); err != nil {
		t.Fatalf("hello: %v", err)
	}
	nextFrame(t, c, "welcome")
	spawned, err := c.request(frame{
		Type: "spawn", Session: "eng-platform-beetle-ox", Role: "eng-platform", Identity: "Beetle-Ox", Seat: "claude",
		Argv: []string{"/bin/sh", "-c", `read line; exit 0`}, Env: os.Environ(), Cwd: "/", Rows: 24, Cols: 80,
	})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	s := d.session(spawned.Session)
	if s == nil {
		t.Fatalf("no session %q after the spawn", spawned.Session)
	}
	s.setActivity("about to exit", time.Now())
	s.mu.Lock()
	token := s.token
	s.mu.Unlock()
	if reply, err := c.request(frame{Type: "activity", Token: token, Text: "still going"}); err != nil || reply.Text != "still going" {
		t.Fatalf("activity on the live session: %+v %v", reply, err)
	}
	if err := s.typeInput([]byte("\n")); err != nil {
		t.Fatalf("end the session: %v", err)
	}
	// The pipe is synchronous, so the owner must read the exit frame for the end to finish.
	nextFrame(t, c, "exit")
	select {
	case <-s.forgotten:
	case <-time.After(10 * time.Second):
		t.Fatal("the session never ended")
	}
	s.mu.Lock()
	line, at := s.activity, s.activityAt
	s.mu.Unlock()
	if line != "" || !at.IsZero() {
		t.Fatalf("an ended session kept %q set at %v", line, at)
	}
	if after, err := c.request(frame{Type: "list"}); err != nil || len(after.Sessions) != 0 {
		t.Fatalf("an ended session is still listed: %+v %v", after.Sessions, err)
	}
	if _, err := c.request(frame{Type: "activity", Token: token, Text: "late"}); err == nil {
		t.Fatal("the ended session's token still set an activity line")
	}
}

func TestActivityNeedsASeatsToken(t *testing.T) {
	t.Setenv(sessionTokenEnv, "")
	if _, err := setActivityLine("anything"); err == nil || !strings.Contains(err.Error(), sessionTokenEnv) {
		t.Fatalf("outside a seat = %v, want an error naming %s", err, sessionTokenEnv)
	}
}
