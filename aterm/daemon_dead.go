package main

import (
	"slices"
	"time"
)

// A dead seat is one whose holder went without an exit, seen or found by the sweep.
// See .agents/skills/tooling-aterm-client/references/dead-seats.md.
const (
	deadSweepEvery = 30 * time.Second
	// deadSeatWindow and deadSeatCap bound what the sessions frame offers to reopen.
	deadSeatWindow = 7 * 24 * time.Hour
	deadSeatCap    = 12
)

// deadSeat is one entry of the `dead` list beside a sessions frame's seats.
type deadSeat struct {
	Name     string    `json:"name"`
	Ref      string    `json:"ref"`
	Role     string    `json:"role"`
	Identity string    `json:"identity,omitempty"`
	Seat     string    `json:"seat"`
	Started  time.Time `json:"started"`
	Ended    time.Time `json:"ended"`
}

// watchDead sweeps until done. The startup sweep runs in runDaemon, after adoption.
func (d *daemon) watchDead(done <-chan struct{}, every time.Duration) {
	defer d.guard("dead seat sweep")
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			d.sweepDead()
		}
	}
}

// sweepDead marks dead every open record whose session this daemon does not hold.
// It runs only after adoptHolders, so by then a holder still alive is a session here.
func (d *daemon) sweepDead() {
	if d.ledgerDir == "" {
		return
	}
	// A holder that answered but failed adoption still runs its seat, and its name
	// is unknown, so no record can be called dead until a restart adopts it.
	if n := d.unadopted.Load(); n > 0 {
		d.logf("%d holder(s) could not be adopted, so no seat is marked dead", n)
		d.refreshDead()
		return
	}
	d.ledgerMu.Lock()
	entries := readLedger(d.ledgerDir)
	live := d.liveStarts()
	now := time.Now().UTC()
	for _, entry := range entries {
		if entry.Ended != nil {
			continue
		}
		if started, ok := live[entry.Name]; ok && started.Equal(entry.Started) {
			continue
		}
		entry.Ended, entry.Lost = &now, true
		if err := writeLedger(d.ledgerDir, entry); err != nil {
			d.logf("could not mark %s dead: %v", entry.Name, err)
			continue
		}
		d.logf("marked %s dead: no holder answers for it", entry.Name)
	}
	d.ledgerMu.Unlock()
	d.refreshDead()
}

// liveStarts is each held seat's name and start, the pair a ledger record carries.
func (d *daemon) liveStarts() map[string]time.Time {
	d.mu.Lock()
	defer d.mu.Unlock()
	live := make(map[string]time.Time, len(d.sessions))
	for name, s := range d.sessions {
		live[name] = s.started
	}
	return live
}

// refreshDead rereads the ledger into the list the sessions frames carry, and
// pushes the roster when the list changed.
func (d *daemon) refreshDead() {
	if d.ledgerDir == "" {
		return
	}
	refs := map[string]string{}
	for name, started := range d.liveStarts() {
		refs[name] = sessionRef(name, started)
	}
	dead := deadSeats(readLedger(d.ledgerDir), refs, time.Now())
	d.mu.Lock()
	changed := !slices.Equal(dead, d.dead)
	d.dead = dead
	d.mu.Unlock()
	if changed {
		d.pushSessions()
	}
}

func (d *daemon) deadList() []deadSeat {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.dead
}

// deadSeats is the newest dead records that still have a conversation to resume,
// leaving out one reopened since, by Treehouse or by a later launch of its conversation.
func deadSeats(entries []ledgerEntry, liveRefs map[string]string, now time.Time) []deadSeat {
	resumedAt := map[string]time.Time{}
	for _, entry := range entries {
		if entry.Conversation != "" && entry.Started.After(resumedAt[entry.Conversation]) {
			resumedAt[entry.Conversation] = entry.Started
		}
	}
	var dead []deadSeat
	for index := len(entries) - 1; index >= 0 && len(dead) < deadSeatCap; index-- {
		entry := entries[index]
		if !entry.Lost || entry.Ended == nil || entry.Reopened != nil || now.Sub(entry.Started) > deadSeatWindow {
			continue
		}
		ref := sessionRef(entry.Name, entry.Started)
		if liveRefs[entry.Name] == ref {
			continue
		}
		if entry.Conversation != "" && resumedAt[entry.Conversation].After(entry.Started) {
			continue
		}
		if _, err := resumeHarnessArgs(entry, entries); err != nil {
			continue
		}
		dead = append(dead, deadSeat{
			Name: entry.Name, Ref: ref, Role: entry.Role, Identity: entry.Identity, Seat: entry.Seat,
			Started: entry.Started.UTC(), Ended: entry.Ended.UTC(),
		})
	}
	return dead
}

// stampReopened marks a record reopened, so it leaves the dead list for good.
func (d *daemon) stampReopened(entry ledgerEntry) {
	if d.ledgerDir == "" {
		return
	}
	d.ledgerMu.Lock()
	for _, held := range readLedger(d.ledgerDir) {
		if held.Name != entry.Name || !held.Started.Equal(entry.Started) {
			continue
		}
		now := time.Now().UTC()
		held.Reopened = &now
		if err := writeLedger(d.ledgerDir, held); err != nil {
			d.logf("could not mark %s reopened: %v", held.Name, err)
		}
	}
	d.ledgerMu.Unlock()
	d.refreshDead()
}
