package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/urfave/cli/v3"
)

func TestConversationOfReadsTheClaudeIdAndIgnoresAosSessionIds(t *testing.T) {
	const id = "0a1b2c3d-4e5f-4a6b-8c7d-9e0f1a2b3c4d"
	const other = "11111111-2222-4333-8444-555555555555"
	for _, testCase := range []struct {
		name string
		argv []string
		want string
	}{
		{"claude session id", []string{"aos", "_native-shadow", "--session-id", "zx44", "--", "agent-compose", "launch", "r", "claude", "--session-id", id}, id},
		{"aos id alone is not a conversation", []string{"aos", "--session-id", "zx44"}, ""},
		{"resume keeps its id", []string{"agent-compose", "launch", "r", "claude", "--", "--resume", id, "--name", "x"}, id},
		{"the last one wins", []string{"--session-id", other, "--session-id", id}, id},
		{"a flag with no value", []string{"--session-id"}, ""},
		{"nothing", []string{"/bin/sh", "-c", "true"}, ""},
	} {
		if got := conversationOf(testCase.argv); got != testCase.want {
			t.Fatalf("%s: conversationOf = %q, want %q", testCase.name, got, testCase.want)
		}
	}
}

func TestNewConversationIDIsAV4UUID(t *testing.T) {
	seen := map[string]bool{}
	for range 50 {
		id := newConversationID()
		if !conversationFlag.MatchString(id) || id[14] != '4' || !strings.ContainsRune("89ab", rune(id[19])) {
			t.Fatalf("%q is not a v4 uuid", id)
		}
		seen[id] = true
	}
	if len(seen) != 50 {
		t.Fatal("conversation ids repeated")
	}
}

func TestLaunchMintsAConversationOnlyWhereTheHarnessCanBeResumed(t *testing.T) {
	var spawns []recordedSpawn
	claude, err := runAterm(t, stubDeps(t, &spawns, true), "--dry-run", "--json", "eng-platform", "claude")
	if err != nil {
		t.Fatal(err)
	}
	var plan launchPlan
	if err := json.Unmarshal([]byte(claude), &plan); err != nil {
		t.Fatal(err)
	}
	if conversationOf(plan.Child) == "" {
		t.Fatalf("claude should carry a minted conversation id: %v", plan.Child)
	}
	for _, own := range [][]string{{"--resume", newConversationID()}, {"--session-id", newConversationID()}, {"-c"}} {
		args := append([]string{"--dry-run", "--json", "eng-platform", "claude", "--"}, own...)
		out, err := runAterm(t, stubDeps(t, &spawns, true), args...)
		if err != nil {
			t.Fatal(err)
		}
		var caller launchPlan
		if err := json.Unmarshal([]byte(out), &caller); err != nil {
			t.Fatal(err)
		}
		if count := strings.Count(strings.Join(caller.Child, " "), "--session-id"); own[0] != "--session-id" && count > 1 {
			t.Fatalf("a caller's %v must not get a second session id: %v", own, caller.Child)
		}
		if own[0] == "--session-id" && conversationOf(caller.Child) != own[1] {
			t.Fatalf("a caller's own --session-id must stand: %v", caller.Child)
		}
	}
}

func TestLedgerRoundTripsAtomicallyWithoutAnEnvironment(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sessions")
	entry := ledgerEntry{Name: "eng-platform-x-ab84", Role: "eng-platform", Seat: "claude", Cwd: "/", Argv: []string{"a"}, Conversation: newConversationID(), Started: time.Now().UTC()}
	if err := writeLedger(dir, entry); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(entry.file(dir))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("a record is private to the user: %v %v", info, err)
	}
	got := readLedger(dir)
	if len(got) != 1 || got[0].Conversation != entry.Conversation || got[0].Format != ledgerFormat {
		t.Fatalf("round trip = %+v", got)
	}
	leftovers, _ := filepath.Glob(filepath.Join(dir, ".ledger-*"))
	if len(leftovers) != 0 {
		t.Fatalf("an atomic write leaves no temp file: %v", leftovers)
	}
	if err := os.WriteFile(filepath.Join(dir, "junk.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if len(readLedger(dir)) != 1 {
		t.Fatal("a record it cannot read must not hide the rest")
	}
}

func TestPruneLedgerDropsOnlyLongEndedRecords(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC()
	old, recent := now.Add(-ledgerKeep-time.Hour), now.Add(-time.Hour)
	for _, entry := range []ledgerEntry{
		{Name: "old-ended", Started: old, Ended: &old},
		{Name: "recent-ended", Started: recent, Ended: &recent},
		{Name: "old-never-ended", Started: old},
	} {
		if err := writeLedger(dir, entry); err != nil {
			t.Fatal(err)
		}
	}
	pruneLedger(dir, now)
	var names []string
	for _, entry := range readLedger(dir) {
		names = append(names, entry.Name)
	}
	slices.Sort(names)
	if !slices.Equal(names, []string{"old-never-ended", "recent-ended"}) {
		t.Fatalf("kept %v, a record that never ended may still be resumable", names)
	}
}

// The daemon records a spawn and stamps its exit, and the record holds nothing
// from the environment, which carries credentials.
func TestDaemonRecordsASessionForResumeWithoutItsEnvironment(t *testing.T) {
	state := filepath.Join(t.TempDir(), "state")
	t.Setenv(stateDirEnv, state)
	testDaemon(t)
	id := newConversationID()
	tc := dialTest(t)
	_, err := tc.c.request(frame{
		Type: "spawn", Session: "eng-platform-led", Role: "eng-platform", Identity: "Beetle-Ox", Seat: "claude",
		Argv: []string{"/bin/sh", "-c", "exit 3", "x", "--session-id", id},
		Env:  append(os.Environ(), "AWS_SECRET_ACCESS_KEY=hunter2-should-never-land"), Cwd: "/", Rows: 24, Cols: 80,
	})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "an ended record", 10*time.Second, func() bool {
		entries := readLedger(state)
		return len(entries) == 1 && entries[0].Ended != nil
	})
	entry := readLedger(state)[0]
	if entry.Conversation != id || entry.Role != "eng-platform" || entry.Seat != "claude" || entry.Code == nil || *entry.Code != 3 {
		t.Fatalf("record = %+v", entry)
	}
	raw, _ := os.ReadFile(entry.file(state))
	if bytes.Contains(raw, []byte("hunter2")) || bytes.Contains(raw, []byte("AWS_SECRET")) {
		t.Fatalf("the environment reached the record:\n%s", raw)
	}
}

// A child that exits at once finishes before its record is written, so its end
// stamp must not depend on the record already existing. CI hit this with `exit 3`.
func TestLedgerEndStampSurvivesAChildThatExitsBeforeItsRecord(t *testing.T) {
	for _, order := range []string{"record then end", "end then record"} {
		t.Run(order, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "state")
			d := newDaemon(func(string, ...any) {})
			d.ledgerDir = dir
			s := &ptySession{d: d, name: "eng-platform-race", role: "eng-platform", seat: "claude",
				started: time.Now(), done: make(chan struct{}), exitCode: 3}
			close(s.done)
			message := frame{Cwd: "/", Argv: []string{"x", "--session-id", newConversationID()}}
			if order == "record then end" {
				d.recordSession(s, message)
				d.markEnded(s)
			} else {
				d.markEnded(s)
				d.recordSession(s, message)
			}
			entries := readLedger(dir)
			if len(entries) != 1 || entries[0].Ended == nil || entries[0].Code == nil || *entries[0].Code != 3 {
				t.Fatalf("%s: record = %+v", order, entries)
			}
		})
	}
}

func TestFindResumablePicksByNameThenByRoleAndRefusesALiveSession(t *testing.T) {
	older, newer := time.Now().Add(-2*time.Hour), time.Now().Add(-time.Hour)
	entries := []ledgerEntry{
		{Name: "eng-platform-a-1111", Role: "eng-platform", Started: older},
		{Name: "eng-platform-a-2222", Role: "eng-platform", Started: newer},
		{Name: "scientist-b-3333", Role: "scientist", Started: older},
	}
	live := map[string]bool{"scientist-b-3333": true}
	if got, err := findResumable(entries, live, "eng-platform-a-1111"); err != nil || got.Name != "eng-platform-a-1111" {
		t.Fatalf("by name: %v %v", got, err)
	}
	if got, err := findResumable(entries, live, "eng-platform"); err != nil || got.Name != "eng-platform-a-2222" {
		t.Fatalf("a role takes its most recent: %v %v", got, err)
	}
	if _, err := findResumable(entries, live, "scientist-b-3333"); err == nil || !strings.Contains(err.Error(), "aterm attach") {
		t.Fatalf("a live session points at attach: %v", err)
	}
	if _, err := findResumable(entries, live, "scientist"); err == nil {
		t.Fatal("a role whose only session is live has nothing to resume")
	}
	if _, err := findResumable(entries, live, "nobody"); exitCodeFor(err) != exitOffRoster {
		t.Fatalf("an unknown name is off the roster: %v", err)
	}
}

func TestResumeArgsReopenTheConversationUnderItsOldName(t *testing.T) {
	id := newConversationID()
	dir := t.TempDir()
	entry := ledgerEntry{Name: "eng-platform-a-1111", Role: "eng-platform", Seat: "claude", Cwd: dir, Conversation: id}
	got, err := resumeArgs(entry, true, true)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"--headless", "--dry-run", "--working-directory", dir, "eng-platform", "claude", "--", "--resume", id, "--name", "eng-platform-a-1111"}
	if !slices.Equal(got, want) {
		t.Fatalf("args = %v, want %v", got, want)
	}
	entry.Cwd = filepath.Join(dir, "gone")
	if got, _ := resumeArgs(entry, false, false); slices.Contains(got, "--working-directory") {
		t.Fatalf("a directory that is gone must not be demanded: %v", got)
	}
	for _, unresumable := range []ledgerEntry{
		{Name: "n", Role: "r", Seat: "codex", Conversation: id},
		{Name: "n", Role: "r", Seat: "claude"},
	} {
		if _, err := resumeArgs(unresumable, false, false); exitCodeFor(err) != exitUsage {
			t.Fatalf("%+v should refuse with usage: %v", unresumable, err)
		}
	}
}

func TestResumeVerbListsAndRelaunchesThroughTheNormalPath(t *testing.T) {
	state := t.TempDir()
	t.Setenv(stateDirEnv, state)
	t.Setenv(daemonSocketEnv, filepath.Join(t.TempDir(), "none.sock"))
	id := newConversationID()
	ended := time.Now().UTC()
	for _, entry := range []ledgerEntry{
		{Name: "eng-platform-a-1111", Role: "eng-platform", Seat: "claude", Cwd: "/", Conversation: id, Started: ended, Ended: &ended},
		{Name: "scientist-b-2222", Role: "scientist", Seat: "codex", Cwd: "/", Started: ended, Ended: &ended},
	} {
		if err := writeLedger(state, entry); err != nil {
			t.Fatal(err)
		}
	}
	run := func(args ...string) (string, error) {
		out := &bytes.Buffer{}
		command := &cli.Command{Name: "aterm", Writer: out, Commands: []*cli.Command{newResumeCommand()}}
		err := command.Run(context.Background(), append([]string{"aterm", "resume"}, args...))
		return out.String(), err
	}
	listed, err := run("--list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(listed, "eng-platform-a-1111") || !strings.Contains(listed, "not resumable") {
		t.Fatalf("the list should show both, marking the codex one:\n%s", listed)
	}
	var captured []string
	earlier := resumeRun
	resumeRun = func(_ string, args []string, _, _ io.Writer) error {
		captured = args
		return nil
	}
	t.Cleanup(func() { resumeRun = earlier })
	if _, err := run("eng-platform"); err != nil {
		t.Fatal(err)
	}
	want := []string{"--working-directory", "/", "eng-platform", "claude", "--", "--resume", id, "--name", "eng-platform-a-1111"}
	if !slices.Equal(captured, want) {
		t.Fatalf("relaunch args = %v, want %v", captured, want)
	}
	if _, err := run("scientist"); exitCodeFor(err) != exitUsage {
		t.Fatalf("a codex session is not resumable yet: %v", err)
	}
	if _, err := run(); exitCodeFor(err) != exitUsage {
		t.Fatalf("resume with no name is a usage error: %v", err)
	}
}
