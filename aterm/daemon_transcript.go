package main

import (
	"errors"
	"fmt"
	"time"
)

// transcriptOf reads one recorded seat's conversation from its harness's own files,
// naming the reason when it cannot.
func transcriptOf(entry ledgerEntry, all []ledgerEntry, req transcriptRequest) (transcriptReply, error) {
	var reply transcriptReply
	var err error
	switch entry.Seat {
	case "claude":
		var path string
		if path, err = transcriptPath(entry); err == nil {
			reply, err = claudeTranscript(path, req, all)
		}
	case "codex":
		reply, err = codexTranscript(entry, all, req)
	case "opencode":
		reply, err = opencodeTranscript(entry, all, req)
	default:
		err = withExit(exitUsage, fmt.Errorf("%s runs %q, which keeps no transcript aterm reads: %w", entry.Name, entry.Seat, errUnsupportedHarness))
	}
	return reply, transcriptRefusal(err)
}

// transcriptRefusal gives a failed read the reason a client can name.
func transcriptRefusal(err error) error {
	switch {
	case err == nil || reasonFor(err) != "":
		return err
	case errors.Is(err, errUnsupportedHarness):
		return withReason(reasonUnsupportedSeat, err)
	case errors.Is(err, errAmbiguousSession):
		return withReason(reasonAmbiguous, err)
	case errors.Is(err, errUnreadableFormat):
		return withReason(reasonUnreadableFormat, err)
	case errors.Is(err, errBadCursor):
		return withReason(reasonBadCursor, err)
	case errors.Is(err, errNoTranscript):
		return withReason(reasonNoTranscript, err)
	}
	return err
}

// transcript answers a transcript request: the seat's conversation, oldest first. It is
// gated like typing, since tool output can hold secrets (remoteLocked).
func (d *daemon) transcript(cl *client, message frame) error {
	s, err := d.oneTarget(firstNonEmpty(message.Session, message.Target))
	if err != nil {
		return err
	}
	d.mu.Lock()
	ledger := d.ledgerDir
	d.mu.Unlock()
	var recorded []ledgerEntry
	if ledger != "" {
		recorded = readLedger(ledger)
	}
	entry, err := findRecorded(recorded, s.name)
	if err != nil {
		return withReason(reasonNoTranscript, fmt.Errorf("%s has no ledger record to find its transcript by: %w", s.name, errNoTranscript))
	}
	entry = withLiveCwd(entry, s.pid)
	reply, err := transcriptOf(entry, recorded, transcriptRequest{Limit: message.Limit, Before: message.Before, Since: message.Since})
	if err != nil {
		return err
	}
	return cl.c.write(frame{
		Type: "transcript", ID: message.ID, Session: s.name, Reader: reply.Reader, Items: reply.Items,
		Cursor: reply.Cursor, Older: reply.Older, More: reply.More, UpdatedAt: reply.UpdatedAt.UTC().Format(time.RFC3339Nano),
	})
}
