package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHarnessDirsOfKeepsOnlyAbsoluteHarnessPathsAndTheLastAssignment(t *testing.T) {
	got := harnessDirsOf([]string{
		"PATH=/bin", "CODEX_HOME=/old", "CODEX_HOME=/seat/codex/", "CLAUDE_CONFIG_DIR=relative/dir",
		"XDG_DATA_HOME=/seat/data", "AWS_SECRET_ACCESS_KEY=hunter2", "XDG_DATA_HOMEX=/no",
	})
	if len(got) != 2 || got["CODEX_HOME"] != "/seat/codex" || got["XDG_DATA_HOME"] != "/seat/data" {
		t.Fatalf("dirs = %v", got)
	}
	if harnessDirsOf([]string{"PATH=/bin"}) != nil {
		t.Fatal("a spawn that set none records none")
	}
}

func TestDirPrefersTheSeatsOwnVariableOverItsHome(t *testing.T) {
	entry := ledgerEntry{Home: "/shadow/home", Dirs: map[string]string{"CODEX_HOME": "/elsewhere/codex"}}
	if got := entry.dir("CODEX_HOME", ".codex"); got != "/elsewhere/codex" {
		t.Fatalf("variable should win: %q", got)
	}
	if got := entry.dir("CLAUDE_CONFIG_DIR", ".claude"); got != "/shadow/home/.claude" {
		t.Fatalf("home is the fallback: %q", got)
	}
}

func TestDaemonRecordsHarnessDirsAndNothingElseOfTheEnvironment(t *testing.T) {
	state := filepath.Join(t.TempDir(), "state")
	d := newDaemon(func(string, ...any) {})
	d.ledgerDir = state
	s := &ptySession{d: d, name: "eng-platform-dirs", role: "eng-platform", seat: "codex", started: time.Now(), done: make(chan struct{})}
	d.recordSession(s, frame{Cwd: "/", Argv: []string{"x"}, Env: []string{
		"HOME=/shadow/home", "CODEX_HOME=/seat/codex", "XDG_DATA_HOME=/seat/data", "CLAUDE_CONFIG_DIR=not-absolute",
		"AWS_SECRET_ACCESS_KEY=hunter2-should-never-land",
	}})
	entries := readLedger(state)
	if len(entries) != 1 || entries[0].Dirs["CODEX_HOME"] != "/seat/codex" || entries[0].Dirs["XDG_DATA_HOME"] != "/seat/data" || len(entries[0].Dirs) != 2 {
		t.Fatalf("record = %+v", entries)
	}
	if raw, _ := os.ReadFile(entries[0].file(state)); strings.Contains(string(raw), "hunter2") || strings.Contains(string(raw), "not-absolute") {
		t.Fatalf("the environment reached the record:\n%s", raw)
	}
}

func TestReadersFollowTheSeatsOwnHarnessDirectories(t *testing.T) {
	start := time.Date(2026, 9, 28, 23, 16, 0, 0, time.UTC)
	root := t.TempDir()

	claudeDir, id := filepath.Join(root, "claude-config"), newConversationID()
	seatTranscript(t, strings.TrimSuffix(claudeDir, "/.claude"), id, userLine("go"), assistantLine("end_turn", text("From the config dir.")))
	// seatTranscript writes under <home>/.claude, so point CLAUDE_CONFIG_DIR at that.
	claude := ledgerEntry{Name: "c", Seat: "claude", Conversation: id, Home: "/nonexistent",
		Dirs: map[string]string{"CLAUDE_CONFIG_DIR": filepath.Join(claudeDir, ".claude")}, Started: start}
	if view, err := replyOf(claude, nil); err != nil || view.Text != "From the config dir." {
		t.Fatalf("claude = %+v %v", view, err)
	}

	codexHomeDir, cwd := filepath.Join(root, "codex-home"), "/shadow/projects"
	codexRolloutFile(t, filepath.Dir(codexHomeDir)+"/x", "2026/09/28", cwd, start.Add(time.Second),
		codexEvent("task_started", map[string]any{}), codexMessage("final_answer", "wrong home"), codexEvent("task_complete", map[string]any{}))
	rollout := codexRolloutFile(t, root, "2026/09/28", cwd, start.Add(time.Second),
		codexEvent("task_started", map[string]any{}), codexMessage("final_answer", "From CODEX_HOME."), codexEvent("task_complete", map[string]any{}))
	codex := ledgerEntry{Name: "x", Seat: "codex", Cwd: cwd, Home: "/nonexistent", Started: start,
		Dirs: map[string]string{"CODEX_HOME": filepath.Join(root, ".codex")}}
	if view, err := replyOf(codex, nil); err != nil || view.Text != "From CODEX_HOME." || view.Source != rollout {
		t.Fatalf("codex = %+v %v", view, err)
	}
}

func TestOpencodeFollowsXDGDataHome(t *testing.T) {
	needSqlite(t)
	data, cwd := t.TempDir(), "/shadow/projects"
	start := time.Date(2026, 9, 28, 23, 15, 0, 0, time.UTC)
	ms := func(offset time.Duration) string { return itoa(start.Add(offset).UnixMilli()) }
	makeOpencodeDBIn(t, filepath.Join(data, "opencode"),
		`INSERT INTO session VALUES ('ses_a', '`+cwd+`', NULL, `+ms(time.Second)+`);`,
		`INSERT INTO message VALUES ('msg_1', 'ses_a', `+ms(2*time.Second)+`, '{"role":"user"}');`,
		`INSERT INTO message VALUES ('msg_2', 'ses_a', `+ms(3*time.Second)+`, '{"role":"assistant","finish":"stop"}');`,
		`INSERT INTO part VALUES ('prt_1', 'msg_2', 'ses_a', `+ms(3*time.Second)+`, '{"type":"text","text":"From XDG_DATA_HOME."}');`)
	entry := ledgerEntry{Name: "o", Seat: "opencode", Cwd: cwd, Home: "/nonexistent", Started: start, Dirs: map[string]string{"XDG_DATA_HOME": data}}
	if view, err := replyOf(entry, nil); err != nil || view.Text != "From XDG_DATA_HOME." {
		t.Fatalf("opencode = %+v %v", view, err)
	}
}

// Two seats on one harness in one cwd, as when neither launched in its own shadow.
func TestCodexSeatsSharingACwdEachGetTheirOwnRolloutOrARefusal(t *testing.T) {
	home, cwd := t.TempDir(), "/shared/projects"
	t0 := time.Date(2026, 9, 28, 23, 0, 0, 0, time.UTC)
	t1 := t0.Add(10 * time.Minute)
	a := ledgerEntry{Name: "scientist-a-1111", Seat: "codex", Cwd: cwd, Home: home, Started: t0}
	b := ledgerEntry{Name: "scientist-b-2222", Seat: "codex", Cwd: cwd, Home: home, Started: t1}
	all := []ledgerEntry{a, b}
	turn := []string{codexEvent("task_started", map[string]any{}), codexMessage("final_answer", "x"), codexEvent("task_complete", map[string]any{})}
	rolloutA := codexRolloutFile(t, home, "2026/09/28", cwd, t0.Add(2*time.Second), turn...)
	rolloutB := codexRolloutFile(t, home, "2026/09/28", cwd, t1.Add(2*time.Second), turn...)
	if got, err := codexRollout(a, all); err != nil || got != rolloutA {
		t.Fatalf("the earlier seat takes its own rollout, not the newest: %q %v", got, err)
	}
	if got, err := codexRollout(b, all); err != nil || got != rolloutB {
		t.Fatalf("the later seat takes the one after it started: %q %v", got, err)
	}
	// A second rollout inside the earlier seat's window cannot be attributed.
	codexRolloutFile(t, home, "2026/09/28", cwd, t0.Add(5*time.Minute), turn...)
	if _, err := codexRollout(a, all); !errorsIsNoTranscript(err) || !strings.Contains(err.Error(), "cannot be told") {
		t.Fatalf("two candidates for one seat with a peer must be refused: %v", err)
	}
	// Without the peer list the newest in the cwd wins, which here is the other seat's,
	// and is why the verb passes the whole ledger.
	if got, err := codexRollout(a, nil); err != nil || got != rolloutB {
		t.Fatalf("with no peer list the newest wins: %q %v", got, err)
	}
}

func TestPeersAreOnlySeatsSharingTheHarnessCwdStoreAndTime(t *testing.T) {
	t0 := time.Date(2026, 9, 28, 23, 0, 0, 0, time.UTC)
	ended, before := t0.Add(time.Minute), t0.Add(-time.Minute)
	later := t0.Add(time.Hour)
	self := ledgerEntry{Name: "a", Seat: "codex", Cwd: "/p", Home: "/h", Started: t0}
	all := []ledgerEntry{
		self,
		{Name: "peer", Seat: "codex", Cwd: "/p", Home: "/h", Started: t0.Add(time.Second)},
		{Name: "other-cwd", Seat: "codex", Cwd: "/q", Home: "/h", Started: t0},
		{Name: "other-seat", Seat: "opencode", Cwd: "/p", Home: "/h", Started: t0},
		{Name: "other-store", Seat: "codex", Cwd: "/p", Home: "/h", Started: t0, Dirs: map[string]string{"CODEX_HOME": "/elsewhere"}},
		{Name: "ended-before", Seat: "codex", Cwd: "/p", Home: "/h", Started: t0.Add(-time.Hour), Ended: &before},
		{Name: "starts-after-end", Seat: "codex", Cwd: "/p", Home: "/h", Started: later},
	}
	self.Ended = &ended
	got := peersOf(self, all, codexHome)
	if len(got) != 1 || got[0].Name != "peer" {
		names := []string{}
		for _, p := range got {
			names = append(names, p.Name)
		}
		t.Fatalf("peers = %v, want only peer", names)
	}
}

func TestOpencodeSeatsSharingACwdAreToldApartByStart(t *testing.T) {
	needSqlite(t)
	home, cwd := t.TempDir(), "/shared/projects"
	t0 := time.Date(2026, 9, 28, 23, 0, 0, 0, time.UTC)
	t1 := t0.Add(10 * time.Minute)
	ms := func(at time.Time, offset time.Duration) string { return itoa(at.Add(offset).UnixMilli()) }
	stmts := []string{}
	for _, s := range []struct {
		id    string
		at    time.Time
		reply string
	}{{"ses_a", t0, "reply of a"}, {"ses_b", t1, "reply of b"}} {
		stmts = append(stmts,
			`INSERT INTO session VALUES ('`+s.id+`', '`+cwd+`', NULL, `+ms(s.at, 2*time.Second)+`);`,
			`INSERT INTO message VALUES ('msg_`+s.id+`u', '`+s.id+`', `+ms(s.at, 3*time.Second)+`, '{"role":"user"}');`,
			`INSERT INTO message VALUES ('msg_`+s.id+`a', '`+s.id+`', `+ms(s.at, 4*time.Second)+`, '{"role":"assistant","finish":"stop"}');`,
			`INSERT INTO part VALUES ('prt_`+s.id+`', 'msg_`+s.id+`a', '`+s.id+`', `+ms(s.at, 4*time.Second)+`, '{"type":"text","text":"`+s.reply+`"}');`)
	}
	makeOpencodeDB(t, home, stmts...)
	a := ledgerEntry{Name: "dev-advocate-a-1111", Seat: "opencode", Cwd: cwd, Home: home, Started: t0}
	b := ledgerEntry{Name: "dev-advocate-b-2222", Seat: "opencode", Cwd: cwd, Home: home, Started: t1}
	all := []ledgerEntry{a, b}
	if view, err := replyOf(a, all); err != nil || view.Text != "reply of a" {
		t.Fatalf("a = %+v %v", view, err)
	}
	if view, err := replyOf(b, all); err != nil || view.Text != "reply of b" {
		t.Fatalf("b = %+v %v", view, err)
	}
	makeOpencodeDB(t, home,
		`INSERT INTO session VALUES ('ses_a2', '`+cwd+`', NULL, `+ms(t0, 5*time.Minute)+`);`)
	if _, err := replyOf(a, all); !errorsIsNoTranscript(err) || !strings.Contains(err.Error(), "cannot be told") {
		t.Fatalf("a second session in a's window is ambiguous: %v", err)
	}
}
