package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The suite closes fake seats that never read a typed exit, so the grace is
// short here and the tests that care about the wait set their own.
func init() { cleanExitGrace = 500 * time.Millisecond }

func claimFrame(t *testing.T, tc *testClient, base string, peek bool) string {
	t.Helper()
	reply, err := tc.c.request(frame{Type: "claim", Session: base, Peek: peek})
	if err != nil {
		t.Fatalf("claim %s: %v", base, err)
	}
	if reply.Type != "claimed" {
		t.Fatalf("claim reply = %+v, want claimed", reply)
	}
	return reply.Session
}

func TestClaimGrantsConcurrentLaunchesDistinctPoolNames(t *testing.T) {
	testDaemon(t)
	tc := dialTest(t)
	base := "eng-platform-beetle-ox"
	for _, want := range []string{base, base + "-2", base + "-3"} {
		if got := claimFrame(t, tc, base, false); got != want {
			t.Fatalf("claim = %q, want %q: two launches at once must never share a name", got, want)
		}
	}
}

func TestClaimPeekAnswersWithoutHoldingTheName(t *testing.T) {
	testDaemon(t)
	tc := dialTest(t)
	base := "eng-platform-beetle-ox"
	for range 3 {
		if got := claimFrame(t, tc, base, true); got != base {
			t.Fatalf("peek = %q, want %q every time", got, base)
		}
	}
	if got := claimFrame(t, tc, base, false); got != base {
		t.Fatalf("claim after peeks = %q, want %q", got, base)
	}
}

func TestClaimedNameIsConsumedBySpawnAndReturnsWhenTheSessionCloses(t *testing.T) {
	testDaemon(t)
	room := dialTest(t)
	base := "eng-platform-beetle-ox"
	if got := claimFrame(t, room, base, false); got != base {
		t.Fatalf("first claim = %q, want %q", got, base)
	}
	room.spawn(base, "eng-platform", "Beetle-Ox", `echo UP; sleep 30`)
	room.until("UP")
	if got := claimFrame(t, room, base, true); got != base+"-2" {
		t.Fatalf("with %s live the next name = %q, want %s-2", base, got, base)
	}
	if _, err := closeFrame(t, dialTest(t), frame{Target: base}); err != nil {
		t.Fatalf("close: %v", err)
	}
	liveNamesEventually(t, room, base)
	if got := claimFrame(t, room, base, true); got != base {
		t.Fatalf("after close the bare name = %q, want %q back", got, base)
	}
}

func TestFiveRelaunchesOfOneRoleReuseTheBareName(t *testing.T) {
	testDaemon(t)
	tc := dialTest(t)
	base := "eng-platform-beetle-ox"
	for launch := range 5 {
		if got := claimFrame(t, tc, base, false); got != base {
			t.Fatalf("launch %d claimed %q, want the bare %q", launch+1, got, base)
		}
		session := dialTest(t)
		session.spawn(base, "eng-platform", "Beetle-Ox", `echo UP; sleep 30`)
		session.until("UP")
		if _, err := closeFrame(t, tc, frame{Target: base}); err != nil {
			t.Fatalf("close %d: %v", launch+1, err)
		}
		if names := liveNamesEventually(t, tc, base); len(names) != 0 {
			t.Fatalf("launch %d left %v live", launch+1, names)
		}
	}
}

func TestTwoConcurrentInstancesTakeTheBareNameAndTwoThenTheBareNameAgain(t *testing.T) {
	testDaemon(t)
	tc := dialTest(t)
	base := "eng-platform-beetle-ox"
	names := make([]string, 2)
	for index := range names {
		names[index] = claimFrame(t, tc, base, false)
		dialTest(t).spawn(names[index], "eng-platform", "Beetle-Ox", `echo UP; sleep 30`)
	}
	if names[0] != base || names[1] != base+"-2" {
		t.Fatalf("two concurrent instances = %v, want [%s %s-2]", names, base, base)
	}
	for _, name := range names {
		if _, err := closeFrame(t, tc, frame{Target: name}); err != nil {
			t.Fatalf("close %s: %v", name, err)
		}
		liveNamesEventually(t, tc, name)
	}
	if got := claimFrame(t, tc, base, false); got != base {
		t.Fatalf("relaunch after both closed = %q, want the bare %q", got, base)
	}
}

func TestClaimHoldLapsesSoADeadLaunchFreesItsName(t *testing.T) {
	d := newDaemon(func(string, ...any) {})
	base := "eng-platform-beetle-ox"
	first, _ := d.claimName(base, false)
	d.claims[first] = time.Now().Add(-time.Second)
	if again, _ := d.claimName(base, false); again != first {
		t.Fatalf("a lapsed claim of %q was not freed: next grant %q", first, again)
	}
}

func TestClaimRefusesAnUnsluggedBase(t *testing.T) {
	d := newDaemon(func(string, ...any) {})
	for _, base := range []string{"", "Eng Platform", "eng-platform-", "a/b"} {
		if _, err := d.claimName(base, false); err == nil || !strings.Contains(err.Error(), "slugged") {
			t.Fatalf("claim of %q = %v, want a refusal", base, err)
		}
	}
}

func TestHolderSocketIsUniquePerSpawnSoAReusedNameCannotLoseIt(t *testing.T) {
	socket := testDaemon(t)
	holdDir := filepath.Join(filepath.Dir(socket), "hold")
	for range 2 {
		dialTest(t).spawn("eng-platform-beetle-ox", "eng-platform", "Beetle-Ox", `echo UP; sleep 30`)
		found, _ := filepath.Glob(filepath.Join(holdDir, "*.sock"))
		if len(found) != 1 || found[0] == holdSocketPath(holdDir, "eng-platform-beetle-ox") {
			t.Fatalf("holder sockets = %v, want one that is not the name's own path", found)
		}
		if _, err := closeFrame(t, dialTest(t), frame{Target: "eng-platform-beetle-ox"}); err != nil {
			t.Fatalf("close: %v", err)
		}
		waitFor(t, "the holder socket to go", 5*time.Second, func() bool {
			left, _ := filepath.Glob(filepath.Join(holdDir, "*.sock"))
			return len(left) == 0
		})
	}
}
