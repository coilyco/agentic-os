package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Shapes follow codex rollouts read on 2026-10-02, with made-up text. Across 40
// recent sessions every final_answer equalled its task_complete last_agent_message.
func codexEvent(kind string, fields map[string]any) string {
	fields["type"] = kind
	encoded, _ := json.Marshal(map[string]any{"type": "event_msg", "payload": fields})
	return string(encoded)
}

func codexMessage(phase, body string) string {
	encoded, _ := json.Marshal(map[string]any{"type": "response_item", "payload": map[string]any{
		"type": "message", "role": "assistant", "phase": phase,
		"content": []map[string]string{{"type": "output_text", "text": body}},
	}})
	return string(encoded)
}

func codexItem(kind string) string {
	return `{"type":"response_item","payload":{"type":"` + kind + `","output":"ok"}}`
}

func readCodex(t *testing.T, lines ...string) (string, bool) {
	t.Helper()
	got, complete, err := lastCodexReply(strings.NewReader(strings.Join(lines, "\n") + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	return got, complete
}

func TestLastCodexReplyIsTheFinalAnswerOfTheLatestTurn(t *testing.T) {
	got, complete := readCodex(t,
		codexEvent("task_started", map[string]any{}), codexMessage("final_answer", "older"), codexEvent("task_complete", map[string]any{}),
		codexEvent("task_started", map[string]any{}),
		codexMessage("commentary", "Let me look."), `{"type":"response_item","payload":{"type":"function_call","name":"x"}}`,
		codexItem("function_call_output"), codexMessage("commentary", "Found it."),
		`{"type":"response_item","payload":{"type":"reasoning","content":"a plain string, not blocks"}}`,
		codexMessage("final_answer", "The answer."),
		codexEvent("task_complete", map[string]any{"last_agent_message": "The answer."}),
	)
	if got != "The answer." || !complete {
		t.Fatalf("reply = %q complete = %v", got, complete)
	}
}

func TestLastCodexReplyMidTurnKeepsOnlyCommentarySinceTheLastToolOutput(t *testing.T) {
	got, complete := readCodex(t, codexEvent("task_started", map[string]any{}),
		codexMessage("commentary", "Before the tool."), codexItem("custom_tool_call_output"), codexMessage("commentary", "After the tool."))
	if got != "After the tool." || complete {
		t.Fatalf("reply = %q complete = %v", got, complete)
	}
	if got, complete = readCodex(t, codexEvent("task_started", map[string]any{})); got != "" || complete {
		t.Fatalf("a turn with no output yet reads empty, got %q %v", got, complete)
	}
}

func TestLastCodexReplyUsesTheLastAgentMessageWhenNoFinalAnswerLineExists(t *testing.T) {
	got, complete := readCodex(t, codexEvent("task_started", map[string]any{}), codexMessage("commentary", "Working."),
		codexEvent("task_complete", map[string]any{"last_agent_message": "  Closing words.  "}))
	if got != "Closing words." || !complete {
		t.Fatalf("reply = %q complete = %v", got, complete)
	}
	got, complete = readCodex(t, codexEvent("task_started", map[string]any{}), codexMessage("commentary", "Partial."),
		codexEvent("turn_aborted", map[string]any{"reason": "interrupted"}))
	if got != "Partial." || !complete {
		t.Fatalf("an aborted turn ends with its text so far, got %q %v", got, complete)
	}
}

func TestLastCodexReplyReadsPastAMultiMegabyteLine(t *testing.T) {
	big := `{"type":"response_item","payload":{"type":"function_call_output","output":"` + strings.Repeat("x", 3<<20) + `"}}`
	got, complete := readCodex(t, codexEvent("task_started", map[string]any{}), big, codexMessage("final_answer", "After."),
		codexEvent("task_complete", map[string]any{}))
	if got != "After." || !complete {
		t.Fatalf("reply = %q complete = %v", got, complete)
	}
}

// codexRolloutFile writes a rollout where codex keeps it, dated by its local day.
func codexRolloutFile(t *testing.T, home, day, cwd string, started time.Time, lines ...string) string {
	t.Helper()
	return writeCodexRollout(t, home, day, cwd, started, nil, lines...)
}

// writeCodexRollout also takes extra session_meta fields, such as a subagent's parent.
func writeCodexRollout(t *testing.T, home, day, cwd string, started time.Time, extra map[string]any, lines ...string) string {
	t.Helper()
	dir := filepath.Join(home, ".codex", "sessions", day)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{
		"id": "u", "cwd": cwd, "timestamp": started.UTC().Format("2006-01-02T15:04:05.000Z"), "base_instructions": strings.Repeat("i", 40000),
		"thread_source": "user",
	}
	for key, value := range extra {
		payload[key] = value
	}
	meta, _ := json.Marshal(map[string]any{"type": "session_meta", "payload": payload})
	path := filepath.Join(dir, "rollout-"+started.UTC().Format("2006-01-02T15-04-05")+"-"+newConversationID()+".jsonl")
	if err := os.WriteFile(path, []byte(string(meta)+"\n"+strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCodexRolloutMatchesTheSeatByCwdAndStartAndTakesTheNewest(t *testing.T) {
	home, cwd := t.TempDir(), "/shadow/projects"
	start := time.Date(2026, 9, 28, 23, 16, 0, 0, time.UTC)
	entry := ledgerEntry{Name: "scientist-a-1111", Seat: "codex", Cwd: cwd, Home: home, Started: start}
	codexRolloutFile(t, home, "2026/09/28", cwd, start.Add(-time.Hour), codexMessage("final_answer", "before the seat"))
	codexRolloutFile(t, home, "2026/09/28", "/elsewhere", start.Add(time.Minute), codexMessage("final_answer", "another cwd"))
	first := codexRolloutFile(t, home, "2026/09/28", cwd, start.Add(time.Second), codexEvent("task_started", map[string]any{}), codexMessage("final_answer", "first"), codexEvent("task_complete", map[string]any{}))
	// Local day is behind UTC, so a start just past midnight UTC sits in the day before.
	newest := codexRolloutFile(t, home, "2026/09/27", cwd, start.Add(2*time.Minute), codexEvent("task_started", map[string]any{}), codexMessage("final_answer", "newest"), codexEvent("task_complete", map[string]any{}))
	got, err := codexRollout(entry, nil)
	if err != nil || got != newest {
		t.Fatalf("rollout = %q (first %q) err = %v", got, first, err)
	}
	view, err := replyOf(entry, nil)
	if err != nil || view.Text != "newest" || !view.Complete || view.Source != newest {
		t.Fatalf("reply = %+v err = %v", view, err)
	}
	ended := start.Add(90 * time.Second)
	entry.Ended = &ended
	if got, err := codexRollout(entry, nil); err != nil || got != first {
		t.Fatalf("a session that ended first must not take a later rollout: %q %v", got, err)
	}
}

func TestCodexRolloutWithNoMatchIsNoTranscript(t *testing.T) {
	home := t.TempDir()
	entry := ledgerEntry{Name: "scientist-a-1111", Seat: "codex", Cwd: "/shadow/projects", Home: home, Started: time.Now()}
	if _, err := codexRollout(entry, nil); !errorsIsNoTranscript(err) || exitCodeFor(err) != exitOffRoster {
		t.Fatalf("no sessions directory: %v", err)
	}
	day := time.Now().UTC().Format("2006/01/02")
	codexRolloutFile(t, home, day, "/other", time.Now().Add(time.Second), codexMessage("final_answer", "x"))
	if _, err := codexRollout(entry, nil); !errorsIsNoTranscript(err) {
		t.Fatalf("no rollout for this cwd: %v", err)
	}
	codexRolloutFile(t, home, day, entry.Cwd, entry.Started.Add(-time.Minute), codexMessage("final_answer", "an earlier session in the same cwd"))
	if _, err := codexRollout(entry, nil); !errorsIsNoTranscript(err) {
		t.Fatalf("a rollout that began before the seat is another session's: %v", err)
	}
}

func TestCodexRolloutIgnoresSubagentRolloutsInTheSameCwd(t *testing.T) {
	home, cwd := t.TempDir(), "/shadow/projects"
	start := time.Date(2026, 9, 28, 23, 16, 0, 0, time.UTC)
	entry := ledgerEntry{Name: "scientist-a-1111", Seat: "codex", Cwd: cwd, Home: home, Started: start}
	turn := []string{codexEvent("task_started", map[string]any{}), codexMessage("final_answer", "x"), codexEvent("task_complete", map[string]any{})}
	own := codexRolloutFile(t, home, "2026/09/28", cwd, start.Add(time.Second), turn...)
	// Later than the seat's own and in its cwd: a subagent, never to be read as the seat.
	writeCodexRollout(t, home, "2026/09/28", cwd, start.Add(time.Minute), map[string]any{"parent_thread_id": "p", "thread_source": "subagent",
		"source": map[string]any{"subagent": map[string]any{"other": "guardian"}}}, turn...)
	writeCodexRollout(t, home, "2026/09/28", cwd, start.Add(2*time.Minute), map[string]any{"thread_source": "subagent"}, turn...)
	if got, err := codexRollout(entry, nil); err != nil || got != own {
		t.Fatalf("rollout = %q err = %v, want the seat's own %q", got, err, own)
	}
}
