package main

import (
	"slices"
	"strings"
	"testing"
	"time"
)

func fastSelfClose(t *testing.T) {
	t.Helper()
	old := selfCloseDelay
	selfCloseDelay = 50 * time.Millisecond
	t.Cleanup(func() { selfCloseDelay = old })
}

// The token names the session to end, so a second seat, whose name the caller
// never gives, stays live.
func TestCloseSelfEndsTheTokenHoldersSessionAndNoOther(t *testing.T) {
	testDaemon(t)
	fastSelfClose(t)
	other := dialTest(t)
	other.spawn("scientist-evie", "scientist", "Evie", `echo UP; sleep 30`)
	other.until("UP")
	self := dialTest(t)
	self.spawn("eng-platform-weevil-ox", "eng-platform", "Weevil-Ox", `echo TOKEN=$ATERM_SESSION_TOKEN; sleep 30`)
	token := self.token()
	reply, err := closeFrame(t, dialTest(t), frame{Token: token, Self: true})
	if err != nil || reply.Type != "closed" || reply.Session != "eng-platform-weevil-ox" {
		t.Fatalf("self close = %+v, %v, want closed eng-platform-weevil-ox", reply, err)
	}
	watcher := dialTest(t)
	names := liveNamesEventually(t, watcher, "eng-platform-weevil-ox")
	if slices.Contains(names, "eng-platform-weevil-ox") {
		t.Fatalf("the caller's session is still listed: %v", names)
	}
	if !slices.Contains(names, "scientist-evie") {
		t.Fatalf("another seat was closed with it: %v", names)
	}
}

// A Target on a self frame is ignored: the token alone decides, never a name.
func TestCloseSelfIgnoresAName(t *testing.T) {
	testDaemon(t)
	fastSelfClose(t)
	other := dialTest(t)
	other.spawn("scientist-evie", "scientist", "Evie", `echo UP; sleep 30`)
	other.until("UP")
	self := dialTest(t)
	self.spawn("eng-platform-weevil-ox", "eng-platform", "Weevil-Ox", `echo TOKEN=$ATERM_SESSION_TOKEN; sleep 30`)
	token := self.token()
	reply, err := closeFrame(t, dialTest(t), frame{Token: token, Self: true, Target: "scientist-evie"})
	if err != nil || reply.Session != "eng-platform-weevil-ox" {
		t.Fatalf("a named target must not redirect a self close: %+v, %v", reply, err)
	}
	if names := liveNamesEventually(t, dialTest(t), "eng-platform-weevil-ox"); !slices.Contains(names, "scientist-evie") {
		t.Fatalf("the named session was closed: %v", names)
	}
}

func TestCloseSelfOutsideASeatIsRefused(t *testing.T) {
	testDaemon(t)
	fastSelfClose(t)
	other := dialTest(t)
	other.spawn("scientist-evie", "scientist", "Evie", `echo UP; sleep 30`)
	other.until("UP")
	for name, token := range map[string]string{"no token": "", "unknown token": "not-a-live-token"} {
		reply, err := closeFrame(t, dialTest(t), frame{Token: token, Self: true})
		if err == nil || reply.Code != exitUsage || !strings.Contains(err.Error(), "needs a seat") {
			t.Fatalf("%s: want a refusal with exit %d, got %v (code %d)", name, exitUsage, err, reply.Code)
		}
	}
	if names := liveNamesEventually(t, dialTest(t), "never"); !slices.Contains(names, "scientist-evie") {
		t.Fatalf("a refused self close ended a session: %v", names)
	}
}

// The CLI refuses before it dials, so it is non-zero even with no daemon running.
func TestCloseSelfCLIOutsideASeatExitsNonZero(t *testing.T) {
	t.Setenv(sessionTokenEnv, "")
	t.Setenv(daemonSocketEnv, t.TempDir()+"/absent.sock")
	_, err := closeSelf(false)
	if err == nil || exitCodeFor(err) != exitUsage || !strings.Contains(err.Error(), "outside every session") {
		t.Fatalf("closeSelf outside a seat = %v (code %d), want a usage refusal", err, exitCodeFor(err))
	}
}

func TestCloseSelfCLIEndsTheSessionTheTokenNames(t *testing.T) {
	testDaemon(t)
	fastSelfClose(t)
	self := dialTest(t)
	self.spawn("eng-platform-weevil-ox", "eng-platform", "Weevil-Ox", `echo TOKEN=$ATERM_SESSION_TOKEN; sleep 30`)
	t.Setenv(sessionTokenEnv, self.token())
	name, err := closeSelf(false)
	if err != nil || name != "eng-platform-weevil-ox" {
		t.Fatalf("closeSelf = %q, %v", name, err)
	}
	if names := liveNamesEventually(t, dialTest(t), name); slices.Contains(names, name) {
		t.Fatalf("%s is still listed: %v", name, names)
	}
}

// The guards close does for a named target bind a self close too, and --force lifts them.
func TestCloseSelfKeepsADraftUnlessForced(t *testing.T) {
	testDaemon(t)
	fastSelfClose(t)
	self := dialTest(t)
	self.spawn("eng-platform-weevil-ox", "eng-platform", "Weevil-Ox",
		`echo TOKEN=$ATERM_SESSION_TOKEN; printf 'READY\033[?2004h\n'; cat`)
	token := self.token()
	self.until("READY")
	time.Sleep(1600 * time.Millisecond)
	if err := self.c.write(frame{Type: "input", Session: "eng-platform-weevil-ox", Data: []byte("half a thou")}); err != nil {
		t.Fatalf("type: %v", err)
	}
	self.until("half a thou")
	closer := dialTest(t)
	reply, err := closeFrame(t, closer, frame{Token: token, Self: true})
	if err == nil || reply.Code != exitUsage || !strings.Contains(err.Error(), "draft") {
		t.Fatalf("a draft must keep a self close open, got %v (code %d)", err, reply.Code)
	}
	reply, err = closeFrame(t, closer, frame{Token: token, Self: true, Force: true})
	if err != nil || reply.Session != "eng-platform-weevil-ox" {
		t.Fatalf("force should close it, got %+v, %v", reply, err)
	}
}

// close_session keeps refusing the caller's own session: it sends a target, not Self.
func TestTargetCloseStillRefusesTheCallersOwnSessionAfterSelf(t *testing.T) {
	testDaemon(t)
	self := dialTest(t)
	self.spawn("eng-platform-weevil-ox", "eng-platform", "Weevil-Ox", `echo TOKEN=$ATERM_SESSION_TOKEN; sleep 30`)
	t.Setenv(sessionTokenEnv, self.token())
	if _, _, err := closeSession("eng-platform-weevil-ox", true); err == nil || !strings.Contains(err.Error(), "is this session") {
		t.Fatalf("closeSession on the caller's own name must still refuse, got %v", err)
	}
}
