package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/urfave/cli/v3"
)

const (
	codexThread  = "019a6d3e-7b1c-7f42-9a2e-5c8d1e0f4a63"
	opencodeSess = "ses_0a1b2c3d4e5f"
	gooseSess    = "20261110_3"
)

var seatStart = time.Date(2026, 11, 10, 21, 0, 0, 0, time.UTC)

// codexSeat is a codex seat whose home holds one rollout, built from the captured
// session_meta line (testdata/resume/codex-rollout-session_meta.json).
func codexSeat(t *testing.T) ledgerEntry {
	t.Helper()
	home := t.TempDir()
	meta, err := os.ReadFile("testdata/resume/codex-rollout-session_meta.json")
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, "sessions", "2026", "11", "10")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	rollout := string(meta) + `{"type":"response_item","payload":{"type":"message","role":"assistant","phase":"final_answer","content":[{"type":"output_text","text":"done"}]}}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "rollout-2026-11-10T21-00-00-"+codexThread+".jsonl"), []byte(rollout), 0o600); err != nil {
		t.Fatal(err)
	}
	return ledgerEntry{Name: "scientist-a-1111", Role: "scientist", Seat: "codex", Cwd: "/work/demo", Started: seatStart, Dirs: map[string]string{"CODEX_HOME": home}}
}

// gooseSeat has goose 1.52.0's real sessions table (testdata/resume/) and rows for the
// seat's own session, a subagent, scheduled, archived, other-directory and earlier ones.
func gooseSeat(t *testing.T) ledgerEntry {
	t.Helper()
	needSqlite(t)
	data := t.TempDir()
	dir := filepath.Join(data, "goose", "sessions")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	schema, err := os.ReadFile("testdata/resume/goose-1.52.0-sessions.sql")
	if err != nil {
		t.Fatal(err)
	}
	insert := func(id, kind, workdir, created, parent, archived string) string {
		return "INSERT INTO sessions (id, name, working_dir, session_type, created_at, parent_session_id, archived_at) VALUES (" +
			strings.Join([]string{q(id), q(id), q(workdir), q(kind), q(created), orNull(parent), orNull(archived)}, ", ") + ");\n"
	}
	rows := insert(gooseSess, "user", "/work/demo", "2026-11-10 21:00:30", "", "") +
		insert("20261110_4", "sub_agent", "/work/demo", "2026-11-10 21:01:00", "", "") +
		insert("20261110_5", "user", "/work/demo", "2026-11-10 21:02:00", gooseSess, "") +
		insert("20261110_6", "scheduled", "/work/demo", "2026-11-10 21:03:00", "", "") +
		insert("20261110_7", "user", "/work/demo", "2026-11-10 21:04:00", "", "2026-11-10 22:00:00") +
		insert("20261110_8", "user", "/elsewhere", "2026-11-10 21:05:00", "", "") +
		insert("20261109_1", "user", "/work/demo", "2026-11-09 08:00:00", "", "")
	command := exec.Command("sqlite3", filepath.Join(dir, "sessions.db"))
	command.Stdin = strings.NewReader(string(schema) + "\n" + rows)
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("sqlite3: %v\n%s", err, out)
	}
	return ledgerEntry{Name: "prod-manager-egret-goose", Role: "prod-manager", Seat: "goose", Cwd: "/work/demo", Started: seatStart, Dirs: map[string]string{"XDG_DATA_HOME": data}}
}

func q(value string) string { return "'" + strings.ReplaceAll(value, "'", "''") + "'" }

func orNull(value string) string {
	if value == "" {
		return "NULL"
	}
	return q(value)
}

func opencodeSeat(t *testing.T) ledgerEntry {
	t.Helper()
	needSqlite(t)
	data := t.TempDir()
	makeOpencodeDBIn(t, filepath.Join(data, "opencode"),
		"INSERT INTO session VALUES ('"+opencodeSess+"', '/work/demo', NULL, "+itoa(seatStart.Add(30*time.Second).UnixMilli())+");",
		"INSERT INTO session VALUES ('ses_sub000000', '/work/demo', '"+opencodeSess+"', "+itoa(seatStart.Add(time.Minute).UnixMilli())+");",
		"INSERT INTO session VALUES ('ses_other0000', '/elsewhere', NULL, "+itoa(seatStart.Add(time.Minute).UnixMilli())+");",
		"INSERT INTO session VALUES ('ses_before0000', '/work/demo', NULL, "+itoa(seatStart.Add(-time.Hour).UnixMilli())+");")
	return ledgerEntry{Name: "scientist-b-2222", Role: "scientist", Seat: "opencode", Cwd: "/work/demo", Started: seatStart, Dirs: map[string]string{"XDG_DATA_HOME": data}}
}

func TestResumeReopensACodexSeatFromItsRolloutWithCodexsOwnVerb(t *testing.T) {
	entry := codexSeat(t)
	got, err := resumeArgs(entry, nil, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"--headless", "scientist", "codex", "--", "resume", codexThread}; !slices.Equal(got, want) {
		t.Fatalf("args = %v, want %v", got, want)
	}
}

func TestResumeReopensAGooseSeatFromItsOwnSessionAndNotASubagentOrArchivedOrForeignOne(t *testing.T) {
	entry := gooseSeat(t)
	got, err := resumeArgs(entry, nil, true, false)
	if err != nil {
		t.Fatal(err)
	}
	// session is the verb agent-compose reads first, then goose's own resume of that one id.
	if want := []string{"--headless", "prod-manager", "goose", "--", "session", "--resume", "--session-id", gooseSess}; !slices.Equal(got, want) {
		t.Fatalf("args = %v, want %v", got, want)
	}
}

func TestResumeReopensAnOpencodeSeatBySessionId(t *testing.T) {
	entry := opencodeSeat(t)
	got, err := resumeArgs(entry, nil, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"scientist", "opencode", "--", "--session", opencodeSess}; !slices.Equal(got, want) {
		t.Fatalf("args = %v, want %v", got, want)
	}
}

func TestAGooseSessionOfAnEndedSeatMustBeginInsideItsLifeAndAnIdWithAFlagInItIsRefused(t *testing.T) {
	entry := gooseSeat(t)
	ended := seatStart.Add(10 * time.Second)
	entry.Ended = &ended
	if _, err := resumeArgs(entry, nil, false, false); exitCodeFor(err) != exitOffRoster || !strings.Contains(err.Error(), "no goose conversation to resume") {
		t.Fatalf("a session that began after the seat ended is not its own: %v", err)
	}
	other := gooseSeat(t)
	command := exec.Command("sqlite3", gooseDB(other))
	command.Stdin = strings.NewReader("DELETE FROM sessions; INSERT INTO sessions (id, name, working_dir, session_type, created_at) VALUES ('--debug', 'x', '/work/demo', 'user', '2026-11-10 21:00:30');")
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("sqlite3: %v\n%s", err, out)
	}
	if _, err := resumeArgs(other, nil, false, false); err == nil {
		t.Fatal("an id that reads as a flag must never reach a command line")
	}
}

func TestAResumedSeatsRecordNamesItsConversationSoTheNextResumeDoesNotSearch(t *testing.T) {
	shadow := []string{"/usr/bin/env", "AGENT_COMPOSE_IDENTITY=Egret-Goose", "aos", "_native-shadow", "--harness", "goose", "--role", "prod-manager",
		"--session-id", "gs46", "--assigned-role", "--", "agent-compose", "launch", "prod-manager"}
	cases := map[string]struct {
		seat  string
		argv  []string
		want  string
		after []string
	}{
		"goose":                        {"goose", append(slices.Clone(shadow), "goose", "session", "--resume", "--session-id", gooseSess), gooseSess, nil},
		"codex":                        {"codex", append(slices.Clone(shadow), "codex", "resume", codexThread), codexThread, nil},
		"opencode":                     {"opencode", append(slices.Clone(shadow), "opencode", "--session", opencodeSess), opencodeSess, nil},
		"codex through a double dash":  {"codex", append(slices.Clone(shadow), "codex", "--", "resume", codexThread), codexThread, nil},
		"a first launch names nothing": {"goose", append(slices.Clone(shadow), "goose"), "", nil},
		"the shadow's own session id is not goose's": {"goose", append(slices.Clone(shadow), "goose", "session"), "", nil},
		"a goose session with no resume is not one":  {"goose", append(slices.Clone(shadow), "goose", "session", "--session-id", "20261110_9"), "", nil},
		"a codex id that is not a uuid":              {"codex", append(slices.Clone(shadow), "codex", "resume", "--help"), "", nil},
		"an opencode id that is not one":             {"opencode", append(slices.Clone(shadow), "opencode", "--session", "../x"), "", nil},
	}
	for name, c := range cases {
		if got := harnessConversation(c.seat, c.argv); got != c.want {
			t.Errorf("%s: conversation = %q, want %q", name, got, c.want)
		}
	}
	// A claude record keeps its own rule.
	id := newConversationID()
	if got := harnessConversation("claude", []string{"claude", "--session-id", id}); got != id {
		t.Errorf("claude: %q, want %q", got, id)
	}
	// And a record that names it needs no files at all.
	entry := ledgerEntry{Name: "n", Role: "r", Seat: "goose", Conversation: gooseSess, Dirs: map[string]string{"XDG_DATA_HOME": t.TempDir()}}
	if got, err := resumeHarnessArgs(entry, nil); err != nil || !slices.Equal(got, []string{"session", "--resume", "--session-id", gooseSess}) {
		t.Errorf("a recorded conversation resumes with no database: %v, %v", got, err)
	}
}

func TestResumeVerbRelaunchesAGooseAndACodexSeatWithNoPermissionModeAndListsTheOnesWithNoFiles(t *testing.T) {
	state := t.TempDir()
	t.Setenv(stateDirEnv, state)
	t.Setenv(daemonSocketEnv, filepath.Join(t.TempDir(), "none.sock"))
	goose, codex := gooseSeat(t), codexSeat(t)
	ended := seatStart.Add(time.Hour)
	goose.Ended, codex.Ended = &ended, &ended
	goose.Started, codex.Started = seatStart, seatStart.Add(time.Second)
	lost := ledgerEntry{Name: "scientist-lost-9999", Role: "lost", Seat: "codex", Cwd: "/work/demo", Started: seatStart, Ended: &ended, Dirs: map[string]string{"CODEX_HOME": t.TempDir()}}
	for _, entry := range []ledgerEntry{goose, codex, lost} {
		if err := writeLedger(state, entry); err != nil {
			t.Fatal(err)
		}
	}
	var captured []string
	earlier := resumeRun
	resumeRun = func(_ string, args []string, _, _ io.Writer) error {
		captured = args
		return nil
	}
	t.Cleanup(func() { resumeRun = earlier })
	run := func(args ...string) (string, error) {
		out := &bytes.Buffer{}
		command := &cli.Command{Name: "aterm", Writer: out, Commands: []*cli.Command{newResumeCommand()}}
		err := command.Run(context.Background(), append([]string{"aterm", "resume"}, args...))
		return out.String(), err
	}
	for role, want := range map[string][]string{
		"prod-manager": {"prod-manager", "goose", "--", "session", "--resume", "--session-id", gooseSess},
		"scientist":    nil,
	} {
		if want == nil {
			continue
		}
		if _, err := run(role); err != nil {
			t.Fatalf("resume %s: %v", role, err)
		}
		if !slices.Equal(captured, want) {
			t.Fatalf("resume %s relaunch = %v, want %v with no --permission-mode", role, captured, want)
		}
	}
	if _, err := run("scientist-a-1111"); err != nil {
		t.Fatal(err)
	}
	if want := []string{"scientist", "codex", "--", "resume", codexThread}; !slices.Equal(captured, want) {
		t.Fatalf("codex relaunch = %v, want %v", captured, want)
	}
	listed, err := run("--list")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSpace(listed), "\n") {
		resumable := !strings.Contains(line, "not resumable")
		if got := strings.HasPrefix(line, "scientist-lost-9999"); got == resumable {
			t.Fatalf("only the seat whose files are gone is not resumable: %q", line)
		}
	}
}

// shadowArgv is the argv a seat leaves when `aos _native-shadow` runs it, as read from a
// real ledger record on the Mac.
func shadowArgv(id string) []string {
	return []string{"/Users/kai/.local/bin/aos", "_native-shadow", "--harness", "goose", "--role", "prod-manager",
		"--session-id", id, "--assigned-role", "--", "/opt/homebrew/bin/agent-compose", "launch", "prod-manager", "goose"}
}

func sqliteExec(t *testing.T, db, statements string) {
	t.Helper()
	command := exec.Command("sqlite3", db)
	command.Stdin = strings.NewReader(statements)
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("sqlite3: %v\n%s", err, out)
	}
}

func TestAShadowSeatsConversationIsFoundInTheShadowDirectoryAndNotAnotherShadows(t *testing.T) {
	const here, other = "/private/tmp/native/cf65/projects", "/private/tmp/native/zz99/projects"
	goose := gooseSeat(t)
	goose.Cwd, goose.Argv = "/launch/named/this", shadowArgv("cf65")
	sqliteExec(t, gooseDB(goose), "UPDATE sessions SET working_dir = '"+here+"' WHERE id = '"+gooseSess+"';"+
		"INSERT INTO sessions (id, name, working_dir, session_type, created_at) VALUES ('20261110_9', 'x', '"+other+"', 'user', '2026-11-10 21:00:40');"+
		"INSERT INTO sessions (id, name, working_dir, session_type, created_at) VALUES ('20261110_10', 'x', '/private/tmp/native/cf65/projects-old', 'user', '2026-11-10 21:00:50');")
	if got, err := resumeHarnessArgs(goose, nil); err != nil || !slices.Equal(got, []string{"session", "--resume", "--session-id", gooseSess}) {
		t.Fatalf("goose: %v, %v", got, err)
	}

	opencode := opencodeSeat(t)
	opencode.Cwd, opencode.Argv = "/launch/named/this", shadowArgv("cf65")
	sqliteExec(t, opencodeDB(opencode), "UPDATE session SET directory = '"+here+"/coilyco/repo' WHERE id = '"+opencodeSess+"';")
	if got, err := resumeHarnessArgs(opencode, nil); err != nil || !slices.Equal(got, []string{"--session", opencodeSess}) {
		t.Fatalf("opencode, a repo under the shadow projects: %v, %v", got, err)
	}

	codex := codexSeat(t)
	codex.Cwd, codex.Argv = "/launch/named/this", shadowArgv("cf65")
	files, _ := filepath.Glob(filepath.Join(codex.Dirs["CODEX_HOME"], "sessions", "*", "*", "*", "rollout-*.jsonl"))
	if len(files) != 1 {
		t.Fatalf("rollout files: %v", files)
	}
	body, _ := os.ReadFile(files[0])
	if err := os.WriteFile(files[0], []byte(strings.Replace(string(body), `"cwd":"/work/demo"`, `"cwd":"`+here+`"`, 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := resumeHarnessArgs(codex, nil); err != nil || !slices.Equal(got, []string{"resume", codexThread}) {
		t.Fatalf("codex: %v, %v", got, err)
	}

	stranger := gooseSeat(t)
	stranger.Cwd, stranger.Argv = "/launch/named/this", shadowArgv("ab12")
	sqliteExec(t, gooseDB(stranger), "UPDATE sessions SET working_dir = '"+here+"' WHERE id = '"+gooseSess+"';")
	if _, err := resumeHarnessArgs(stranger, nil); exitCodeFor(err) != exitOffRoster {
		t.Fatalf("a session in cf65's shadow is not ab12's: %v", err)
	}
}

func TestSeatsInTwoShadowsAreNotPeersEvenWhenTheyNamedOneDirectory(t *testing.T) {
	a, b := ledgerEntry{Name: "a", Seat: "goose", Cwd: "/Users/kai/projects", Argv: shadowArgv("cf65"), Started: seatStart},
		ledgerEntry{Name: "b", Seat: "goose", Cwd: "/Users/kai/projects", Argv: shadowArgv("bu95"), Started: seatStart.Add(time.Minute)}
	same := func(ledgerEntry) string { return "db" }
	if peers := peersOf(a, []ledgerEntry{a, b}, same); len(peers) != 0 {
		t.Fatalf("two shadows are two stores of conversations, got %v", peers)
	}
	a.Argv, b.Argv = nil, nil
	if peers := peersOf(a, []ledgerEntry{a, b}, same); len(peers) != 1 {
		t.Fatalf("two seats with no shadow in one directory still are peers, got %v", peers)
	}
	if id := (ledgerEntry{Argv: []string{"x", "_native-shadow", "--", "--session-id", "cf65"}}).shadowID(); id != "" {
		t.Fatalf("a --session-id after the harness's own `--` is not the shadow's: %q", id)
	}
}

func TestAClosedSeatsWALDatabaseIsReadableOnceNoProcessHoldsItOpen(t *testing.T) {
	needSqlite(t)
	db := filepath.Join(t.TempDir(), "sessions.db")
	sqliteExec(t, db, "PRAGMA journal_mode=wal; CREATE TABLE sessions (id TEXT); INSERT INTO sessions VALUES ('20261111_1');")
	for _, suffix := range []string{"-wal", "-shm"} {
		if err := os.Remove(db + suffix); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}
	var rows []struct {
		ID string `json:"id"`
	}
	if err := sqliteJSON(db, "SELECT id FROM sessions", &rows); err != nil || len(rows) != 1 || rows[0].ID != "20261111_1" {
		t.Fatalf("rows = %v, err = %v", rows, err)
	}
}
