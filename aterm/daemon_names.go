package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// nameRetire is how long an ended session's name stays out of the pool, so a name in an
// old listing is not handed to a new seat the moment it frees (COI-3403).
var nameRetire = 24 * time.Hour

// sessionRef names one session, not one name: it changes when the name is reused. It is
// stable across a daemon restart, since both inputs are in the ledger.
func sessionRef(name string, started time.Time) string {
	sum := sha256.Sum256([]byte(name + "|" + started.UTC().Format(time.RFC3339Nano)))
	return hex.EncodeToString(sum[:4])
}

// splitRef takes `name@ref` apart. A target without a well-formed ref is all name.
func splitRef(target string) (name, ref string) {
	at := strings.LastIndex(target, "@")
	if at <= 0 || len(target)-at-1 != 8 {
		return target, ""
	}
	if _, err := hex.DecodeString(target[at+1:]); err != nil {
		return target, ""
	}
	return target[:at], target[at+1:]
}

// noteSeen records the roster a seat just listed, so a later close can tell a name it
// saw from a name that has been reassigned since. Caller is nil outside a session.
func (d *daemon) noteSeen(caller *ptySession, views []sessionView) {
	if caller == nil {
		return
	}
	seen := make(map[string]string, len(views))
	for _, view := range views {
		seen[view.Name] = view.Ref
	}
	d.mu.Lock()
	d.seen[caller.name] = seen
	d.mu.Unlock()
}

// noteSeenOne adds one session a seat was told about, such as a send's target.
func (d *daemon) noteSeenOne(caller, s *ptySession) {
	if caller == nil || s == nil {
		return
	}
	ref := sessionRef(s.name, s.started)
	d.mu.Lock()
	if d.seen[caller.name] == nil {
		d.seen[caller.name] = map[string]string{}
	}
	d.seen[caller.name][s.name] = ref
	d.mu.Unlock()
}

// callerSession is the seat a request came from: by its token, else by the process
// tree for a client that sends none. Nil for a caller outside every session.
func (d *daemon) callerSession(cl *client, token string) *ptySession {
	if seat := d.byToken(token); seat != nil {
		return seat
	}
	if cl == nil || cl.remote || len(cl.pids) == 0 {
		return nil
	}
	d.mu.Lock()
	seats := make([]*ptySession, 0, len(d.sessions))
	for _, s := range d.sessions {
		seats = append(seats, s)
	}
	d.mu.Unlock()
	for _, s := range seats {
		for _, pid := range cl.pids {
			if d.descendsFrom(pid, map[int]bool{s.pid: true}) {
				return s
			}
		}
	}
	return nil
}

// staleTarget refuses a seat's close or clear of a name it never listed, since names are
// reused. `name@ref` passes when current. Kai's client, outside every session, is free.
func (d *daemon) staleTarget(caller *ptySession, ref string, s *ptySession) error {
	if caller == nil {
		return nil
	}
	current := sessionRef(s.name, s.started)
	if ref != "" {
		if ref != current {
			return withExit(exitUsage, fmt.Errorf("%s@%s is not a session: %s is %s now. Run `aterm agents` for its ref", s.name, ref, s.name, current))
		}
		return nil
	}
	d.mu.Lock()
	saw := d.seen[caller.name][s.name]
	d.mu.Unlock()
	switch saw {
	case current:
		return nil
	case "":
		return withExit(exitUsage, fmt.Errorf("%s was not in a listing you took, and names are reused. Run `aterm agents` first, or use %s@%s", s.name, s.name, current))
	}
	return withExit(exitUsage, fmt.Errorf("%s is a different session now (started %s) than the one you last saw under that name. Run `aterm agents`, or use %s@%s", s.name, s.started.Local().Format("15:04:05"), s.name, current))
}

// retire keeps a name out of the pool until nameRetire has passed. Caller holds mu.
func (d *daemon) retire(name string, at time.Time) {
	if d.retired[name].Before(at) {
		d.retired[name] = at
	}
}

// retiredName is whether the name ended within nameRetire. Caller holds mu.
func (d *daemon) retiredName(name string, now time.Time) bool {
	at, ok := d.retired[name]
	return ok && now.Sub(at) < nameRetire
}

// loadRetired restores retirement across a daemon restart from the ledger, whose records
// carry when each session ended.
func (d *daemon) loadRetired(now time.Time) {
	if d.ledgerDir == "" {
		return
	}
	entries := readLedger(d.ledgerDir)
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, entry := range entries {
		if entry.Ended != nil && now.Sub(*entry.Ended) < nameRetire {
			d.retire(entry.Name, *entry.Ended)
		}
	}
}
