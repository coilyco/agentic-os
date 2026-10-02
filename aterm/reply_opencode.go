package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// sqliteTimeout bounds a read, since a harness may be writing the same file.
const sqliteTimeout = 10 * time.Second

var opencodeSessionID = regexp.MustCompile(`^ses_[A-Za-z0-9]+$`)

// opencodeRow is one part of one assistant message after the last user message.
type opencodeRow struct {
	Message   string `json:"mid"`
	Finish    string `json:"finish"`
	PartType  string `json:"ptype"`
	Synthetic any    `json:"synthetic"`
	Text      string `json:"text"`
}

// sqliteJSON runs one read-only query through the system sqlite3, with no shell. It reads
// only session, message and part, since the same file holds account tokens.
func sqliteJSON(db, query string, out any) error {
	ctx, cancel := context.WithTimeout(context.Background(), sqliteTimeout)
	defer cancel()
	raw, err := exec.CommandContext(ctx, "sqlite3", "-readonly", "-json", db, query).Output()
	if err != nil {
		var missing *exec.Error
		if errors.As(err, &missing) {
			return withExit(exitMissing, fmt.Errorf("opencode keeps sessions in SQLite and sqlite3 is not on PATH: %w", errNoTranscript))
		}
		return withExit(exitOffRoster, fmt.Errorf("could not read %s: %v: %w", db, err, errNoTranscript))
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return json.Unmarshal([]byte("[]"), out)
	}
	return json.Unmarshal(raw, out)
}

// sqlQuote is a SQL string literal. The input is a path from the ledger, so a
// quote is doubled and a NUL, which would end the statement early, is refused.
func sqlQuote(value string) (string, error) {
	if strings.ContainsRune(value, 0) {
		return "", errors.New("a NUL byte in a SQL value")
	}
	return "'" + strings.ReplaceAll(value, "'", "''") + "'", nil
}

// opencodeSession is the newest top-level session in the seat's directory that
// began no earlier than the seat. A subagent session has a parent and is skipped.
func opencodeSession(db string, entry ledgerEntry) (string, error) {
	dir, err := sqlQuote(entry.Cwd)
	if err != nil {
		return "", withExit(exitUsage, fmt.Errorf("%s: %v: %w", entry.Name, err, errNoTranscript))
	}
	query := fmt.Sprintf("SELECT id FROM session WHERE directory = %s AND parent_id IS NULL AND time_created >= %d",
		dir, entry.Started.Add(-5*time.Second).UnixMilli())
	if entry.Ended != nil {
		query += fmt.Sprintf(" AND time_created <= %d", entry.Ended.Add(5*time.Second).UnixMilli())
	}
	var rows []struct {
		ID string `json:"id"`
	}
	if err := sqliteJSON(db, query+" ORDER BY time_created DESC LIMIT 1", &rows); err != nil {
		return "", err
	}
	if len(rows) == 0 || !opencodeSessionID.MatchString(rows[0].ID) {
		return "", withExit(exitOffRoster, fmt.Errorf("no opencode session for %s in %s: %w", entry.Name, entry.Cwd, errNoTranscript))
	}
	return rows[0].ID, nil
}

// opencodeRows reads the assistant parts since the last user message, in order.
// The id is checked against a pattern before it is placed in the query.
func opencodeRows(db, session string) ([]opencodeRow, error) {
	if !opencodeSessionID.MatchString(session) {
		return nil, fmt.Errorf("%q is not an opencode session id", session)
	}
	query := fmt.Sprintf(`SELECT m.id AS mid, json_extract(m.data,'$.finish') AS finish,
  json_extract(p.data,'$.type') AS ptype, json_extract(p.data,'$.synthetic') AS synthetic,
  json_extract(p.data,'$.text') AS text
FROM message m LEFT JOIN part p ON p.message_id = m.id
WHERE m.session_id = '%[1]s' AND json_extract(m.data,'$.role') = 'assistant'
  AND m.time_created > COALESCE((SELECT max(time_created) FROM message
    WHERE session_id = '%[1]s' AND json_extract(data,'$.role') = 'user'), 0)
ORDER BY m.time_created, m.id, p.time_created, p.id`, session)
	var rows []opencodeRow
	return rows, sqliteJSON(db, query, &rows)
}

// replyFromOpencodeRows is the last assistant message's text and whether it finished.
// Finish tool-calls means a tool call follows, and a synthetic part is the harness's own.
func replyFromOpencodeRows(rows []opencodeRow) (string, bool) {
	var parts []string
	last, finish := "", ""
	for _, row := range rows {
		if row.Message != last {
			last, parts = row.Message, nil
		}
		finish = row.Finish
		if text := strings.TrimSpace(row.Text); row.PartType == "text" && text != "" && row.Synthetic != true && row.Synthetic != float64(1) {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n\n"), finish != "" && finish != "tool-calls"
}

func opencodeReply(entry ledgerEntry) (replyView, error) {
	home := entry.Home
	if home == "" {
		home = realHome()
	}
	db := filepath.Join(home, ".local", "share", "opencode", "opencode.db")
	session, err := opencodeSession(db, entry)
	if err != nil {
		return replyView{}, err
	}
	rows, err := opencodeRows(db, session)
	if err != nil {
		return replyView{}, err
	}
	text, complete := replyFromOpencodeRows(rows)
	return replyView{Session: entry.Name, Text: text, Complete: complete, Source: db + "#" + session}, nil
}
