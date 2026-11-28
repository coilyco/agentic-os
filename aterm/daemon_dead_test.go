package main

import (
	"strings"
	"testing"
	"time"
)

func deadEntry(name, conversation string, started time.Time) ledgerEntry {
	ended := started.Add(time.Hour)
	return ledgerEntry{Name: name, Role: "eng-platform", Seat: "claude", Conversation: conversation, Started: started, Ended: &ended, Lost: true}
}

func deadNames(dead []deadSeat) []string {
	names := make([]string, 0, len(dead))
	for _, seat := range dead {
		names = append(names, seat.Name)
	}
	return names
}

func TestDeadSeatsKeepOnlyLostSeatsWithAConversationNobodyResumed(t *testing.T) {
	now := time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)
	ended := now.Add(-time.Hour)
	reopened := now
	closed := deadEntry("closed-on-purpose", "c-closed", now.Add(-3*time.Hour))
	closed.Lost = false
	stamped := deadEntry("reopened-in-treehouse", "c-stamped", now.Add(-3*time.Hour))
	stamped.Reopened = &reopened
	entries := []ledgerEntry{
		deadEntry("too-old", "c-old", now.Add(-8*24*time.Hour)),
		deadEntry("resumed-by-hand", "c-resumed", now.Add(-5*time.Hour)),
		closed,
		stamped,
		deadEntry("no-conversation", "", now.Add(-3*time.Hour)),
		deadEntry("live-again", "c-live", now.Add(-2*time.Hour)),
		deadEntry("dead-older", "c-a", now.Add(-2*time.Hour)),
		deadEntry("dead-newer", "c-b", now.Add(-time.Hour)),
		// a later launch of c-resumed is `aterm resume` having brought it back
		{Name: "resumed-by-hand-2", Role: "eng-platform", Seat: "claude", Conversation: "c-resumed", Started: now.Add(-4 * time.Hour), Ended: &ended},
	}
	live := map[string]string{"live-again": sessionRef("live-again", now.Add(-2*time.Hour))}
	got := strings.Join(deadNames(deadSeats(entries, live, now)), ",")
	if got != "dead-newer,dead-older" {
		t.Fatalf("dead = %s, want dead-newer,dead-older", got)
	}
}

func TestDeadSeatsStopAtTheCap(t *testing.T) {
	now := time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)
	var entries []ledgerEntry
	for index := range deadSeatCap + 5 {
		entries = append(entries, deadEntry("seat-"+string(rune('a'+index)), "c-"+string(rune('a'+index)), now.Add(time.Duration(index-30)*time.Minute)))
	}
	dead := deadSeats(entries, nil, now)
	if len(dead) != deadSeatCap || dead[0].Name != entries[len(entries)-1].Name {
		t.Fatalf("dead = %v, want the %d newest, newest first", deadNames(dead), deadSeatCap)
	}
}

// A seat still "running when last seen" with no holder left is the ~30 stale records
// resume --list showed (COI-3799): a daemon starting marks it dead.
func TestADaemonStartingMarksARecordWithNoHolderDead(t *testing.T) {
	started := time.Now().UTC().Add(-time.Hour)
	stale := ledgerEntry{Name: "stale-running-seat", Role: "eng-platform", Seat: "claude", Conversation: "5d0f1c2e-1111-4222-8333-944455556666", Started: started}
	if err := writeLedger(ledgerDir(), stale); err != nil {
		t.Fatalf("seed: %v", err)
	}
	testDaemon(t)
	// The socket answers before adoption and the sweep, so both are waited on.
	deadline := time.Now().Add(10 * time.Second)
	for {
		var record ledgerEntry
		for _, entry := range readLedger(ledgerDir()) {
			if entry.Name == stale.Name {
				record = entry
			}
		}
		listed := dialTest(t).list()
		found := false
		for _, seat := range listed.Dead {
			found = found || seat.Name == stale.Name
		}
		if record.Lost && record.Ended != nil && found {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("record = %+v, dead = %v, want it marked dead and offered to reopen", record, deadNames(listed.Dead))
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestReopeningADeadSeatTakesItOffTheList(t *testing.T) {
	started := time.Now().UTC().Add(-time.Hour)
	entry := deadEntry("dead-to-reopen", "c-reopen", started)
	d, c, reopened := reopenRig(t, []ledgerEntry{entry})
	d.ledgerDir = t.TempDir()
	if err := writeLedger(d.ledgerDir, entry); err != nil {
		t.Fatalf("seed: %v", err)
	}
	d.refreshDead()
	if names := deadNames(d.deadList()); len(names) != 1 {
		t.Fatalf("dead before = %v, want the one seat", names)
	}
	reply, err := c.request(frame{Type: "reopen", Target: entry.Name + "@" + sessionRef(entry.Name, started)})
	if err != nil || reply.Type != "reopened" || len(*reopened) != 1 {
		t.Fatalf("reopen = %+v, %v, want one relaunch", reply, err)
	}
	if names := deadNames(d.deadList()); len(names) != 0 {
		t.Fatalf("dead after = %v, want none", names)
	}
	if held := readLedger(d.ledgerDir); len(held) != 1 || held[0].Reopened == nil {
		t.Fatalf("record = %+v, want it stamped reopened", held)
	}
}

func TestADeadSeatReopensWhileAnotherSeatHoldsItsPoolName(t *testing.T) {
	started := time.Now().UTC().Add(-time.Hour)
	entry := deadEntry("pool-name", "c-pool", started)
	d, c, reopened := reopenRig(t, []ledgerEntry{entry})
	d.sessions["pool-name"] = &ptySession{name: "pool-name", started: started.Add(30 * time.Minute)}
	reply, err := c.request(frame{Type: "reopen", Target: "pool-name@" + sessionRef("pool-name", started)})
	if err != nil || reply.Type != "reopened" || len(*reopened) != 1 {
		t.Fatalf("reopen = %+v, %v, want the dead seat reopened beside the live one", reply, err)
	}
}

func TestResumeListSaysDeadForALostRecord(t *testing.T) {
	started := time.Date(2026, 11, 27, 9, 0, 0, 0, time.UTC)
	var out strings.Builder
	if err := listResumable(&out, []ledgerEntry{deadEntry("lost-seat", "c-lost", started)}, map[string]bool{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "dead, holder gone, found ") || strings.Contains(out.String(), "running when last seen") {
		t.Fatalf("list = %q, want the seat shown dead", out.String())
	}
}
