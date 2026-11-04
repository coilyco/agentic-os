package main

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestSessionRefChangesWhenANameIsReusedAndSplitRefTakesItOff(t *testing.T) {
	first := time.Date(2026, 11, 4, 22, 0, 0, 0, time.UTC)
	a, b := sessionRef("beetle-ox-15", first), sessionRef("beetle-ox-15", first.Add(time.Minute))
	if a == b || len(a) != 8 || a != sessionRef("beetle-ox-15", first) {
		t.Fatalf("a ref should be 8 hex, stable, and differ for a reused name: %q %q", a, b)
	}
	cases := []struct{ in, name, ref string }{
		{"beetle-ox-15", "beetle-ox-15", ""},
		{"beetle-ox-15@" + a, "beetle-ox-15", a},
		{"beetle-ox-15@zzzzzzzz", "beetle-ox-15@zzzzzzzz", ""},
		{"beetle-ox-15@abc", "beetle-ox-15@abc", ""},
		{"@" + a, "@" + a, ""},
	}
	for _, c := range cases {
		if name, ref := splitRef(c.in); name != c.name || ref != c.ref {
			t.Errorf("splitRef(%q) = %q, %q, want %q, %q", c.in, name, ref, c.name, c.ref)
		}
	}
}

func listedRef(t *testing.T, rig *clearRig, token, name string) string {
	t.Helper()
	reply, err := rig.tc.c.request(frame{Type: "list", Token: token})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, view := range reply.Sessions {
		if view.Name == name {
			return view.Ref
		}
	}
	t.Fatalf("%s is not listed", name)
	return ""
}

func closeAs(rig *clearRig, token, target string) (frame, error) {
	return rig.tc.c.request(frame{Type: "close", Token: token, Target: target, Force: true})
}

func TestASeatClosesOnlyANameItListedAndKaisClientIsNotHeldToIt(t *testing.T) {
	rig := newClearRig(t)
	rig.target("close-target", "claude", rawEcho)
	if _, err := closeAs(rig, rig.director, "close-target"); err == nil || !strings.Contains(err.Error(), "not in a listing you took") {
		t.Fatalf("a seat that never listed the name should be refused, got %v", err)
	}
	listedRef(t, rig, rig.director, "close-target")
	if reply, err := closeAs(rig, rig.director, "close-target"); err != nil || reply.Type != "closed" {
		t.Fatalf("after listing it, the close should go through: %+v %v", reply, err)
	}
	rig.target("close-target-two", "claude", rawEcho)
	if reply, err := closeAs(rig, "", "close-target-two"); err != nil || reply.Type != "closed" {
		t.Fatalf("a caller outside every session is not held to a listing: %+v %v", reply, err)
	}
}

// COI-3403: a name from an old listing reached the seat that took it later.
func TestACloseOfAReassignedNameIsRefusedUntilItNamesTheNewSessionByRef(t *testing.T) {
	rig := newClearRig(t)
	rig.target("reused-seat", "claude", rawEcho)
	listedRef(t, rig, rig.director, "reused-seat")
	if _, err := closeAs(rig, "", "reused-seat"); err != nil {
		t.Fatalf("closing the first holder: %v", err)
	}
	time.Sleep(1100 * time.Millisecond) // a second holder starts later, so its ref differs
	rig.target("reused-seat", "claude", rawEcho)
	_, err := closeAs(rig, rig.director, "reused-seat")
	if err == nil || !strings.Contains(err.Error(), "different session now") {
		t.Fatalf("a name reassigned since the listing should be refused, got %v", err)
	}
	if _, err := closeAs(rig, rig.director, "reused-seat@00000000"); err == nil || !strings.Contains(err.Error(), "is not a session") {
		t.Fatalf("a wrong ref should be refused, got %v", err)
	}
	ref := listedRef(t, rig, rig.director, "reused-seat")
	if reply, err := closeAs(rig, rig.director, "reused-seat@"+ref); err != nil || reply.Type != "closed" {
		t.Fatalf("naming the current session by ref should close it: %+v %v", reply, err)
	}
}

func TestAnEndedNameStaysOutOfThePoolForADay(t *testing.T) {
	d := newDaemon(func(string, ...any) {})
	now := time.Now()
	d.mu.Lock()
	d.retire("beetle-ox", now.Add(-time.Hour))
	d.mu.Unlock()
	if name, _ := d.claimName("beetle-ox", true); name != "beetle-ox-2" {
		t.Fatalf("a name that ended an hour ago should be skipped, got %q", name)
	}
	d.mu.Lock()
	d.retired["beetle-ox"] = now.Add(-nameRetire - time.Minute)
	d.mu.Unlock()
	if name, _ := d.claimName("beetle-ox", true); name != "beetle-ox" {
		t.Fatalf("a name that ended over a day ago is free again, got %q", name)
	}
}

func TestRetirementSurvivesADaemonRestartThroughTheLedger(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	recent, old := now.Add(-2*time.Hour), now.Add(-30*time.Hour)
	for _, entry := range []ledgerEntry{
		{Format: ledgerFormat, Name: "imp-dragonfly-13", Started: now.Add(-3 * time.Hour), Ended: &recent},
		{Format: ledgerFormat, Name: "imp-dragonfly-9", Started: now.Add(-40 * time.Hour), Ended: &old},
		{Format: ledgerFormat, Name: "imp-dragonfly-4", Started: now.Add(-time.Hour)},
	} {
		if err := writeLedger(dir, entry); err != nil {
			t.Fatal(err)
		}
	}
	d := newDaemon(func(string, ...any) {})
	d.ledgerDir = dir
	d.loadRetired(now)
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.retiredName("imp-dragonfly-13", now) || d.retiredName("imp-dragonfly-9", now) || d.retiredName("imp-dragonfly-4", now) {
		t.Fatalf("only a name that ended within the day should come back retired: %v", d.retired)
	}
}

// An older aterm in a running seat lists without a token, so its listing counts by the
// process tree. This process descends from `go test`, which stands in for the seat.
func TestAListFromAnOlderClientCountsForTheSeatItRunsUnder(t *testing.T) {
	d := newDaemon(func(string, ...any) {})
	seat := &ptySession{name: "seat-one", pid: os.Getppid(), started: time.Now()}
	d.sessions["seat-one"] = seat
	if got := d.callerSession(&client{peerStanding: peerStanding{pids: []int{os.Getpid()}}}, ""); got != seat {
		t.Fatalf("a client under the seat's process should be that seat, got %v", got)
	}
	if got := d.callerSession(&client{peerStanding: peerStanding{pids: []int{1}}}, ""); got != nil {
		t.Fatalf("a client under no seat is outside every session, got %v", got)
	}
}
