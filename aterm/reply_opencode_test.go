package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestReplyFromOpencodeRowsIsTheLastAssistantMessageAndWhetherItFinished(t *testing.T) {
	rows := []opencodeRow{
		{Message: "m1", Finish: "tool-calls", PartType: "text", Text: "narration before a tool"},
		{Message: "m1", Finish: "tool-calls", PartType: "tool"},
		{Message: "m2", Finish: "stop", PartType: "reasoning", Text: "hidden"},
		{Message: "m2", Finish: "stop", PartType: "text", Text: "The answer."},
		{Message: "m2", Finish: "stop", PartType: "text", Text: "synthetic note", Synthetic: float64(1)},
		{Message: "m2", Finish: "stop", PartType: "text", Text: "other note", Synthetic: true},
		{Message: "m2", Finish: "stop", PartType: "text", Text: "  Second part. "},
	}
	if got, complete := replyFromOpencodeRows(rows); got != "The answer.\n\nSecond part." || !complete {
		t.Fatalf("reply = %q complete = %v", got, complete)
	}
	if got, complete := replyFromOpencodeRows(rows[:2]); got != "narration before a tool" || complete {
		t.Fatalf("mid-turn = %q %v", got, complete)
	}
	if got, complete := replyFromOpencodeRows(nil); got != "" || complete {
		t.Fatalf("no rows = %q %v", got, complete)
	}
	// An assistant message with no parts yet comes back as one row with null part columns.
	if got, complete := replyFromOpencodeRows([]opencodeRow{{Message: "m3", Finish: ""}}); got != "" || complete {
		t.Fatalf("a message still being written = %q %v", got, complete)
	}
}

func TestSQLQuoteDoublesQuotesAndRefusesNUL(t *testing.T) {
	if got, err := sqlQuote("/it's/a dir"); err != nil || got != "'/it''s/a dir'" {
		t.Fatalf("quote = %q %v", got, err)
	}
	if _, err := sqlQuote("a\x00b"); err == nil {
		t.Fatal("a NUL byte must be refused")
	}
}

func TestOpencodeRowsRefusesAnIdThatIsNotASessionId(t *testing.T) {
	if _, err := opencodeRows("/nonexistent.db", "ses_x'; DROP TABLE message; --"); err == nil || !strings.Contains(err.Error(), "not an opencode session id") {
		t.Fatalf("an unvalidated id must never reach the query: %v", err)
	}
}

func needSqlite(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 is not on PATH, so the real-database test cannot run")
	}
}

// makeOpencodeDB builds a database with the tables and columns the reader touches.
func makeOpencodeDB(t *testing.T, home string, statements ...string) {
	t.Helper()
	makeOpencodeDBIn(t, filepath.Join(home, ".local", "share", "opencode"), statements...)
}

func makeOpencodeDBIn(t *testing.T, dir string, statements ...string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	schema := `CREATE TABLE IF NOT EXISTS session (id text PRIMARY KEY, directory text NOT NULL, parent_id text, time_created integer NOT NULL);
CREATE TABLE IF NOT EXISTS message (id text PRIMARY KEY, session_id text NOT NULL, time_created integer NOT NULL, data text NOT NULL);
CREATE TABLE IF NOT EXISTS part (id text PRIMARY KEY, message_id text NOT NULL, session_id text NOT NULL, time_created integer NOT NULL, data text NOT NULL);
CREATE TABLE IF NOT EXISTS control_account (email text PRIMARY KEY, access_token text);
INSERT OR IGNORE INTO control_account VALUES ('a@b', 'token-that-must-never-be-read');
`
	command := exec.Command("sqlite3", filepath.Join(dir, "opencode.db"))
	command.Stdin = strings.NewReader(schema + strings.Join(statements, "\n"))
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("sqlite3: %v\n%s", err, out)
	}
}

func TestOpencodeReplyReadsTheSeatsOwnTopLevelSessionFromARealDatabase(t *testing.T) {
	needSqlite(t)
	home, cwd := t.TempDir(), "/shadow/it's/projects"
	start := time.Date(2026, 9, 28, 23, 15, 0, 0, time.UTC)
	ms := func(offset time.Duration) int64 { return start.Add(offset).UnixMilli() }
	makeOpencodeDB(t, home,
		`INSERT INTO session VALUES ('ses_old', '/shadow/it''s/projects', NULL, `+itoa(ms(-time.Hour))+`);`,
		`INSERT INTO session VALUES ('ses_child', '/shadow/it''s/projects', 'ses_mine', `+itoa(ms(2*time.Minute))+`);`,
		`INSERT INTO session VALUES ('ses_mine', '/shadow/it''s/projects', NULL, `+itoa(ms(time.Second))+`);`,
		`INSERT INTO session VALUES ('ses_other', '/elsewhere', NULL, `+itoa(ms(time.Minute))+`);`,
		`INSERT INTO message VALUES ('msg_1', 'ses_mine', `+itoa(ms(2*time.Second))+`, '{"role":"user"}');`,
		`INSERT INTO message VALUES ('msg_2', 'ses_mine', `+itoa(ms(3*time.Second))+`, '{"role":"assistant","finish":"tool-calls"}');`,
		`INSERT INTO part VALUES ('prt_1', 'msg_2', 'ses_mine', `+itoa(ms(3*time.Second))+`, '{"type":"text","text":"narration"}');`,
		`INSERT INTO message VALUES ('msg_3', 'ses_mine', `+itoa(ms(4*time.Second))+`, '{"role":"user"}');`,
		`INSERT INTO message VALUES ('msg_4', 'ses_mine', `+itoa(ms(5*time.Second))+`, '{"role":"assistant","finish":"tool-calls"}');`,
		`INSERT INTO part VALUES ('prt_2', 'msg_4', 'ses_mine', `+itoa(ms(5*time.Second))+`, '{"type":"text","text":"Looking."}');`,
		`INSERT INTO message VALUES ('msg_5', 'ses_mine', `+itoa(ms(6*time.Second))+`, '{"role":"assistant","finish":"stop"}');`,
		`INSERT INTO part VALUES ('prt_3', 'msg_5', 'ses_mine', `+itoa(ms(6*time.Second))+`, '{"type":"reasoning","text":"hidden"}');`,
		`INSERT INTO part VALUES ('prt_4', 'msg_5', 'ses_mine', `+itoa(ms(7*time.Second))+`, '{"type":"text","text":"The whole answer."}');`,
		`INSERT INTO part VALUES ('prt_5', 'msg_5', 'ses_mine', `+itoa(ms(8*time.Second))+`, '{"type":"text","synthetic":true,"text":"harness note"}');`,
	)
	entry := ledgerEntry{Name: "dev-advocate-a-1111", Seat: "opencode", Cwd: cwd, Home: home, Started: start}
	view, err := replyOf(entry, nil)
	if err != nil || view.Text != "The whole answer." || !view.Complete || !strings.HasSuffix(view.Source, "opencode.db#ses_mine") {
		t.Fatalf("reply = %+v err = %v", view, err)
	}
	if strings.Contains(view.Text+view.Source, "token-that-must-never-be-read") {
		t.Fatal("the reader must never touch the account table")
	}
	// A prompt with no answer yet reads empty and not complete.
	makeOpencodeDB(t, home, `INSERT OR IGNORE INTO message VALUES ('msg_6', 'ses_mine', `+itoa(ms(9*time.Second))+`, '{"role":"user"}');`)
	if view, err = replyOf(entry, nil); err != nil || view.Text != "" || view.Complete {
		t.Fatalf("a new prompt = %+v err = %v", view, err)
	}
	entry.Cwd = "/nobody/here"
	if _, err = replyOf(entry, nil); !errorsIsNoTranscript(err) || exitCodeFor(err) != exitOffRoster {
		t.Fatalf("no session in the cwd: %v", err)
	}
}

func TestOpencodeWithoutSqlite3OrADatabaseIsNoTranscript(t *testing.T) {
	entry := ledgerEntry{Name: "s", Seat: "opencode", Cwd: "/x", Home: t.TempDir(), Started: time.Now()}
	if _, err := replyOf(entry, nil); !errorsIsNoTranscript(err) {
		t.Fatalf("no database file: %v", err)
	}
	t.Setenv("PATH", t.TempDir())
	if _, err := replyOf(entry, nil); !errorsIsNoTranscript(err) || exitCodeFor(err) != exitMissing || !strings.Contains(err.Error(), "sqlite3 is not on PATH") {
		t.Fatalf("no sqlite3 binary: %v", err)
	}
}

func itoa(value int64) string { return strconv.FormatInt(value, 10) }
