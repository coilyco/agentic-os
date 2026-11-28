package main

import (
	"errors"
	"fmt"
	"time"
)

// reloadFeature is the daemon's `reload` frame: restart a live seat on its own
// conversation, so the harness reads its MCP servers and connectors again (COI-3787).
const reloadFeature = "reload"

// reloadWait is how long a reload waits for the reopened seat to come back idle, and
// reloadPoll how often it looks. Vars, for tests.
var (
	reloadWait = 90 * time.Second
	reloadPoll = 250 * time.Millisecond
)

// reloadSession resumes an idle seat on its conversation, so the harness reads its
// connectors again. Every check runs before anything ends. See references/seat-state.md.
func (d *daemon) reloadSession(cl *client, message frame) error {
	if cl.web {
		if err := d.typingRefusal(cl, "a process inside an aterm session cannot reload a seat as Kai"); err != nil {
			return err
		}
	}
	target, ref := splitRef(message.Target)
	s, err := d.oneTarget(target)
	if err != nil {
		return err
	}
	caller := d.byToken(message.Token)
	if caller == s {
		return withExit(exitUsage, fmt.Errorf("%s is this session, and reloading it would end the caller", s.name))
	}
	if err := d.staleTarget(caller, ref, s); err != nil {
		return err
	}
	if d.clientDescendsFrom(cl, map[int]bool{s.pid: true}) {
		return withExit(exitUsage, fmt.Errorf("this caller runs inside %s, so reloading it would end the caller too", s.name))
	}
	view := s.view()
	if view.TUI != stateIdle && !message.Force {
		return withExit(exitUsage, fmt.Errorf("%s reads %s, not idle, and a reload would end what it is doing. Reload it once it is idle", s.name, view.TUI))
	}
	if err := closeHeld(s, message.Force); err != nil {
		return err
	}
	entry, found := reopenable(d.entries(), s.name, sessionRef(s.name, s.started))
	if !found {
		return withExit(exitUsage, fmt.Errorf("%s has no recorded conversation to resume, so a reload would lose it", s.name))
	}
	if _, err := resumeHarnessArgs(entry, d.entries()); err != nil {
		return withExit(exitUsage, err)
	}
	by := "a client outside every session"
	if caller != nil {
		by = caller.name
	}
	d.logf("reloading session %s (pid %d) for %s", s.name, s.pid, by)
	s.endFor(endedByReload)
	select {
	case <-s.done:
	default:
		return fmt.Errorf("%s (pid %d) did not end after SIGKILL, so it was not reopened", s.name, s.pid)
	}
	select {
	case <-s.forgotten:
	case <-time.After(forgetGrace):
		return fmt.Errorf("%s ended but the daemon still holds its name, so it was not reopened. `aterm resume %s` retries it", s.name, s.name)
	}
	if err := d.reopen(entry); err != nil {
		return withExit(exitSpawn, fmt.Errorf("%s closed but did not reopen, `aterm resume %s` retries it: %w", s.name, s.name, err))
	}
	back, state := d.awaitReload(entry, s.started)
	reply := frame{Type: "reloaded", ID: message.ID, Session: back, Role: entry.Role, Seat: entry.Seat, State: state}
	switch {
	case back == "":
		reply.Text = fmt.Sprintf("%s reopened, and no seat of its identity is listed yet.", s.name)
	case state != stateIdle:
		reply.Text = fmt.Sprintf("%s reopened and reads %s, not yet idle.", back, state)
	default:
		reply.Text = fmt.Sprintf("%s is back on its conversation and idle.", back)
	}
	// The daemon cannot see a harness's tool list, so it says so rather than guess.
	reply.Text += " Whether a connector added since shows in its tools is not checked here. Ask the seat to list them."
	return cl.c.write(reply)
}

// awaitReload is the seat that came back for entry, found by role and identity, and
// its reading once idle or when reloadWait runs out.
func (d *daemon) awaitReload(entry ledgerEntry, before time.Time) (string, string) {
	deadline := time.Now().Add(reloadWait)
	name, state := "", ""
	for {
		for _, view := range d.views() {
			if view.Role == entry.Role && view.Identity == entry.Identity && view.Started.After(before) {
				name, state = view.Name, view.TUI
			}
		}
		if state == stateIdle || !time.Now().Before(deadline) {
			return name, state
		}
		time.Sleep(reloadPoll)
	}
}

var errNoReload = errors.New("the running aterm daemon predates reload. It restarts on the upgraded binary after five idle minutes")
