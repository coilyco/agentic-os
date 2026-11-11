package main

import (
	"fmt"
	"path/filepath"
	"regexp"
	"time"
)

// gooseSessionID is a goose session id, such as 20260110_1. It reaches a command line,
// so a leading dash is refused and an id never reads as a flag.
var gooseSessionID = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z_-]{0,63}$`)

// gooseDB is the database a seat's goose writes, under its own XDG_DATA_HOME.
func gooseDB(entry ledgerEntry) string {
	return filepath.Join(entry.dir("XDG_DATA_HOME", ".local", "share"), "goose", "sessions", "sessions.db")
}

// gooseSession finds a seat's own session: its directory, begun no earlier than the seat.
// Subagent, scheduled and archived ones are skipped. It reads only id and time.
func gooseSession(db string, entry ledgerEntry, all []ledgerEntry) (string, error) {
	dir, err := sqlQuote(entry.Cwd)
	if err != nil {
		return "", withExit(exitUsage, fmt.Errorf("%s: %v: %w", entry.Name, err, errNoTranscript))
	}
	query := fmt.Sprintf("SELECT id, CAST(strftime('%%s', created_at) AS INTEGER) AS created FROM sessions "+
		"WHERE working_dir = %s AND session_type = 'user' AND parent_session_id IS NULL AND archived_at IS NULL "+
		"AND CAST(strftime('%%s', created_at) AS INTEGER) >= %d", dir, entry.Started.Add(-5*time.Second).Unix())
	if entry.Ended != nil {
		query += fmt.Sprintf(" AND CAST(strftime('%%s', created_at) AS INTEGER) <= %d", entry.Ended.Add(5*time.Second).Unix())
	}
	var rows []struct {
		ID      string `json:"id"`
		Created int64  `json:"created"`
	}
	if err := sqliteJSON(db, query, &rows); err != nil {
		return "", err
	}
	var cands []candidate
	for _, row := range rows {
		if gooseSessionID.MatchString(row.ID) {
			cands = append(cands, candidate{ref: row.ID, started: time.Unix(row.Created, 0)})
		}
	}
	if len(cands) == 0 {
		return "", withExit(exitOffRoster, fmt.Errorf("no goose session for %s in %s: %w", entry.Name, entry.Cwd, errNoTranscript))
	}
	own, err := ownCandidate(entry, peersOf(entry, all, gooseDB), cands)
	return own.ref, err
}
