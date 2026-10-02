package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/urfave/cli/v3"
)

// Shapes follow a real claude transcript read on 2026-10-02, with made-up text:
// one line per content block, and a tool result is a user line.
func userLine(text string) string {
	encoded, _ := json.Marshal(map[string]any{"type": "user", "message": map[string]any{"role": "user", "content": text}})
	return string(encoded)
}

func toolResultLine() string {
	return `{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"ok"}]}}`
}

func assistantLine(stop string, blocks ...contentBlock) string {
	encoded, _ := json.Marshal(map[string]any{"type": "assistant", "message": map[string]any{"role": "assistant", "content": blocks, "stop_reason": stop}})
	return string(encoded)
}

func text(body string) contentBlock { return contentBlock{Type: "text", Text: body} }

func readReply(t *testing.T, lines ...string) (string, bool) {
	t.Helper()
	got, complete, err := lastReply(strings.NewReader(strings.Join(lines, "\n") + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	return got, complete
}

func TestLastReplyIsTheClosingMessageOfTheTurn(t *testing.T) {
	got, complete := readReply(t,
		userLine("older prompt"), assistantLine("end_turn", text("older answer")),
		userLine("do the thing"),
		assistantLine("tool_use", contentBlock{Type: "thinking", Text: "hidden"}),
		assistantLine("tool_use", text("Let me look.")),
		assistantLine("tool_use", contentBlock{Type: "tool_use"}),
		toolResultLine(),
		assistantLine("end_turn", text("It is done."), text("   "), text("Two parts.")),
	)
	if got != "It is done.\n\nTwo parts." || !complete {
		t.Fatalf("reply = %q complete = %v", got, complete)
	}
}

func TestLastReplyMidTurnIsTheTextSoFarAndNotComplete(t *testing.T) {
	got, complete := readReply(t, userLine("go"), assistantLine("tool_use", text("Looking now.")))
	if got != "Looking now." || complete {
		t.Fatalf("reply = %q complete = %v", got, complete)
	}
	got, complete = readReply(t, userLine("go"), assistantLine("end_turn", text("Done.")), userLine("and again"))
	if got != "" || complete {
		t.Fatalf("a new prompt with no answer yet reads empty, got %q %v", got, complete)
	}
	if got, complete = readReply(t); got != "" || complete {
		t.Fatalf("an empty file has no reply, got %q %v", got, complete)
	}
}

func TestLastReplySkipsSidechainsBadLinesAndAHalfWrittenTail(t *testing.T) {
	side := `{"type":"assistant","isSidechain":true,"message":{"content":[{"type":"text","text":"subagent"}],"stop_reason":"end_turn"}}`
	got, complete, err := lastReply(strings.NewReader(strings.Join([]string{
		userLine("go"), side, "{not json", assistantLine("end_turn", text("Real.")),
	}, "\n") + "\n" + `{"type":"assistant","message":{"content":[{"type":"te`))
	if err != nil || got != "Real." || !complete {
		t.Fatalf("reply = %q complete = %v err = %v", got, complete, err)
	}
}

func TestLastReplyReadsPastAMultiMegabyteToolResult(t *testing.T) {
	big := `{"type":"user","message":{"role":"user","content":[{"type":"tool_result","content":"` + strings.Repeat("x", 3<<20) + `"}]}}`
	got, complete := readReply(t, userLine("go"), big, assistantLine("end_turn", text("After the big one.")))
	if got != "After the big one." || !complete {
		t.Fatalf("reply = %q complete = %v", got, complete)
	}
}

func TestHomeOfTakesTheLastAssignment(t *testing.T) {
	if got := homeOf([]string{"PATH=/bin", "HOME=/a", "HOME=/shadow/home", "HOMEX=/no"}); got != "/shadow/home" {
		t.Fatalf("homeOf = %q", got)
	}
	if got := homeOf([]string{"PATH=/bin"}); got != "" {
		t.Fatalf("homeOf with none = %q", got)
	}
}

// seatTranscript writes a transcript where a shadow home would keep it.
func seatTranscript(t *testing.T, home, id string, lines ...string) {
	t.Helper()
	dir := filepath.Join(home, ".claude", "projects", "-some-project")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, id+".jsonl"), []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestTranscriptPathFindsTheSeatsOwnHomeAndRefusesWhatItCannotRead(t *testing.T) {
	home, id := filepath.Join(t.TempDir(), "[shadow]", "home"), newConversationID()
	seatTranscript(t, home, id, userLine("hi"))
	entry := ledgerEntry{Name: "eng-platform-a-1111", Seat: "claude", Conversation: id, Home: home}
	if got, err := transcriptPath(entry); err != nil || !strings.HasSuffix(got, id+".jsonl") {
		t.Fatalf("path = %q err = %v", got, err)
	}
	for name, bad := range map[string]ledgerEntry{
		"codex":      {Name: "s", Seat: "codex", Conversation: id, Home: home},
		"no id":      {Name: "s", Seat: "claude", Home: home},
		"traversal":  {Name: "s", Seat: "claude", Conversation: "../../etc/passwd", Home: home},
		"other home": {Name: "s", Seat: "claude", Conversation: id, Home: t.TempDir()},
		"unknown id": {Name: "s", Seat: "claude", Conversation: newConversationID(), Home: home},
	} {
		if _, err := transcriptPath(bad); err == nil {
			t.Fatalf("%s: a transcript that is not there must not be found", name)
		}
	}
	if _, err := transcriptPath(ledgerEntry{Name: "s", Seat: "codex", Conversation: id}); exitCodeFor(err) != exitUsage {
		t.Fatalf("a codex seat is a usage error: %v", err)
	}
}

func TestDaemonRecordsTheSeatsHomeButNotTheRestOfItsEnvironment(t *testing.T) {
	state := filepath.Join(t.TempDir(), "state")
	d := newDaemon(func(string, ...any) {})
	d.ledgerDir = state
	s := &ptySession{d: d, name: "eng-platform-home", role: "eng-platform", seat: "claude",
		started: time.Now(), done: make(chan struct{})}
	d.recordSession(s, frame{Cwd: "/", Argv: []string{"x", "--session-id", newConversationID()},
		Env: []string{"HOME=/shadow/home", "AWS_SECRET_ACCESS_KEY=hunter2-should-never-land"}})
	entries := readLedger(state)
	if len(entries) != 1 || entries[0].Home != "/shadow/home" {
		t.Fatalf("record = %+v", entries)
	}
	if raw, _ := os.ReadFile(entries[0].file(state)); bytes.Contains(raw, []byte("hunter2")) {
		t.Fatalf("the environment reached the record:\n%s", raw)
	}
}

func TestReplyVerbReadsByNameOrRoleAndSaysWhenATurnIsRunning(t *testing.T) {
	state, home, id := t.TempDir(), filepath.Join(t.TempDir(), "home"), newConversationID()
	t.Setenv(stateDirEnv, state)
	seatTranscript(t, home, id, userLine("go"), assistantLine("end_turn", text("The whole answer.")))
	started := time.Now().UTC()
	if err := writeLedger(state, ledgerEntry{Name: "eng-platform-a-1111", Role: "eng-platform", Seat: "claude", Conversation: id, Home: home, Started: started}); err != nil {
		t.Fatal(err)
	}
	t.Setenv(daemonSocketEnv, filepath.Join(t.TempDir(), "none.sock"))
	if err := writeLedger(state, ledgerEntry{Name: "scientist-b-2222", Role: "scientist", Seat: "codex", Cwd: "/x", Home: t.TempDir(), Started: started}); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) (string, string, error) {
		out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
		command := &cli.Command{Name: "aterm", Writer: out, ErrWriter: errOut, Commands: []*cli.Command{newReplyCommand()}}
		err := command.Run(context.Background(), append([]string{"aterm", "reply"}, args...))
		return out.String(), errOut.String(), err
	}
	for _, target := range []string{"eng-platform-a-1111", "eng-platform"} {
		if out, errOut, err := run(target); err != nil || out != "The whole answer.\n" || errOut != "" {
			t.Fatalf("%s: out = %q err = %q %v", target, out, errOut, err)
		}
	}
	out, _, err := run("--json", "eng-platform")
	var view replyView
	if err != nil || json.Unmarshal([]byte(out), &view) != nil || view.Session != "eng-platform-a-1111" || !view.Complete {
		t.Fatalf("json = %q %v", out, err)
	}
	seatTranscript(t, home, id, userLine("go"), assistantLine("tool_use", text("Working.")))
	if out, errOut, err := run("eng-platform"); err != nil || out != "Working.\n" || !strings.Contains(errOut, "mid-turn") {
		t.Fatalf("mid-turn: out = %q err = %q %v", out, errOut, err)
	}
	if _, errOut, err := run("scientist"); exitCodeFor(err) != exitMissing || !strings.Contains(err.Error(), "no codex rollout") || !strings.Contains(errOut, "visible screen") {
		t.Fatalf("no rollout and no daemon should say both: %q %v", errOut, err)
	}
	if _, _, err := run("nobody"); exitCodeFor(err) != exitOffRoster {
		t.Fatalf("an unknown name is off the roster: %v", err)
	}
	if _, _, err := run(); exitCodeFor(err) != exitUsage {
		t.Fatalf("reply with no name is a usage error: %v", err)
	}
}

func errorsIsNoTranscript(err error) bool { return errors.Is(err, errNoTranscript) }

func TestReplyVerbFallsBackToTheVisibleScreenWhenNoTranscriptIsFound(t *testing.T) {
	state := t.TempDir()
	t.Setenv(stateDirEnv, state)
	testDaemon(t)
	dialTest(t).spawn("reply-fallback", "eng-platform", "Beetle-Ox", `printf 'rows the screen shows\r\n'; exec cat`)
	if err := writeLedger(state, ledgerEntry{Name: "reply-fallback", Role: "eng-platform", Seat: "goose", Cwd: "/", Home: t.TempDir(), Started: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	var out, errOut string
	waitFor(t, "the spawned screen", 8*time.Second, func() bool {
		stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
		command := &cli.Command{Name: "aterm", Writer: stdout, ErrWriter: stderr, Commands: []*cli.Command{newReplyCommand()}}
		if err := command.Run(context.Background(), []string{"aterm", "reply", "--json", "reply-fallback"}); err != nil {
			t.Fatal(err)
		}
		out, errOut = stdout.String(), stderr.String()
		return strings.Contains(out, "rows the screen shows")
	})
	var view replyView
	if err := json.Unmarshal([]byte(out), &view); err != nil || view.Source != "screen" || !strings.Contains(errOut, "visible screen") {
		t.Fatalf("view = %+v err = %v stderr = %q", view, err, errOut)
	}
}
