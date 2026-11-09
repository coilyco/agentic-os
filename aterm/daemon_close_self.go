package main

import (
	"fmt"
	"time"
)

// selfCloseDelay is how long a self close waits after replying before it ends the
// session, so the caller, which runs inside it, can read the reply first.
var selfCloseDelay = 500 * time.Millisecond

// closeHeld refuses to close a session holding Kai's unsent draft or undelivered
// messages unless forced. Both close paths share it.
func closeHeld(s *ptySession, force bool) error {
	view := s.view()
	if force || (!view.Drafted && view.Pending == 0) {
		return nil
	}
	held := "Kai's unsent draft"
	if !view.Drafted {
		held = fmt.Sprintf("%d undelivered message(s)", view.Pending)
	}
	return withExit(exitUsage, fmt.Errorf("%s holds %s, so it stays open unless forced", s.name, held))
}

// closeSelf ends the session the caller's token names, never a Target (COI-3403). The
// caller runs inside it, so the reply goes first and the end follows selfCloseDelay on.
func (d *daemon) closeSelf(cl *client, message frame) error {
	s := d.byToken(message.Token)
	if s == nil {
		return withExit(exitUsage, fmt.Errorf("aterm close self needs a seat: no live session answers to this caller's token"))
	}
	if err := closeHeld(s, message.Force); err != nil {
		return err
	}
	d.logf("closing session %s (pid %d) for itself", s.name, s.pid)
	if err := cl.c.write(frame{Type: "closed", ID: message.ID, Session: s.name}); err != nil {
		return err
	}
	go func() {
		defer d.guard("close self")
		time.Sleep(selfCloseDelay)
		s.end()
	}()
	return nil
}
