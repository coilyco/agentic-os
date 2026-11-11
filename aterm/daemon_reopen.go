package main

import (
	"errors"
	"fmt"
)

// reopenSession resumes a closed seat from its ledger entry. Target is `name` or
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
			"%s has no recorded seat to resume: a new launch of its role is the only path", target))
	}
	// A claude seat's id is in its record. Another harness's is in its own files, and a seat
	// with none there is refused here, not after a launch that cannot resume it.
	if _, err := resumeHarnessArgs(entry, d.entries()); err != nil {
		return withExit(exitUsage, err)
	}
	if err := d.reopen(entry); err != nil {
		return withExit(exitSpawn, err)
	}
	return cl.c.write(frame{Type: "reopened", ID: message.ID, Session: target, Role: entry.Role, Seat: entry.Seat})
}

// reopenable is the latest ledger entry of the name, narrowed to `ref` when given. Only a
// claude seat with no id is skipped here, the other harnesses find theirs at resume.
func reopenable(entries []ledgerEntry, name, ref string) (ledgerEntry, bool) {
	for index := len(entries) - 1; index >= 0; index-- {
		entry := entries[index]
		if entry.Name != name || (entry.Seat == "claude" && entry.Conversation == "") {
			continue
		}
		if ref == "" || sessionRef(entry.Name, entry.Started) == ref {
			return entry, true
		}
	}
	return ledgerEntry{}, false
}
