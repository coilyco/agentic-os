package main

import (
	"errors"
	"fmt"
)

// reopenSession resumes a closed claude seat from its ledger entry. Target is `name` or
// `name@ref`. The seat claims a fresh name, so the reply names role and seat only.
func (d *daemon) reopenSession(cl *client, message frame) error {
	if cl.web {
		if err := d.typingRefusal(cl, "a process inside an aterm session cannot reopen a seat as Kai"); err != nil {
			return err
		}
	}
	target, ref := splitRef(message.Target)
	if target == "" {
		return withExit(exitUsage, errors.New("reopen needs the name of a closed seat"))
	}
	if d.session(target) != nil {
		return withExit(exitUsage, fmt.Errorf("%s is live, so there is nothing to reopen", target))
	}
	entry, found := reopenable(d.entries(), target, ref)
	if !found {
		return withExit(exitUsage, fmt.Errorf(
			"%s has no recorded claude conversation to resume: a new launch of its role is the only path", target))
	}
	if err := d.reopen(entry); err != nil {
		return withExit(exitSpawn, err)
	}
	return cl.c.write(frame{Type: "reopened", ID: message.ID, Session: target, Role: entry.Role, Seat: entry.Seat})
}

// reopenable is the latest ledger entry of the name that has a conversation to resume,
// narrowed to the launch `ref` names when one is given.
func reopenable(entries []ledgerEntry, name, ref string) (ledgerEntry, bool) {
	for index := len(entries) - 1; index >= 0; index-- {
		entry := entries[index]
		if entry.Name != name || entry.Conversation == "" {
			continue
		}
		if ref == "" || sessionRef(entry.Name, entry.Started) == ref {
			return entry, true
		}
	}
	return ledgerEntry{}, false
}
