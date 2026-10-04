package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Shapes follow a claude transcript and a codex rollout read on 2026-10-04, with
// made-up numbers.
func claudeUsage(kind, model string, sidechain bool, input, created, read, output int) string {
	encoded, _ := json.Marshal(map[string]any{
		"type": kind, "isSidechain": sidechain, "timestamp": "2026-10-04T18:00:00.000Z",
		"message": map[string]any{"model": model, "usage": map[string]int{
			"input_tokens": input, "cache_creation_input_tokens": created,
			"cache_read_input_tokens": read, "output_tokens": output,
		}},
	})
	return string(encoded)
}

func codexTokens(info any) string {
	encoded, _ := json.Marshal(map[string]any{
		"type": "event_msg", "timestamp": "2026-10-04T18:00:00.000Z",
		"payload": map[string]any{"type": "token_count", "info": info},
	})
	return string(encoded)
}

func codexInfo(last, window int) map[string]any {
	return map[string]any{"last_token_usage": map[string]int{"total_tokens": last}, "model_context_window": window}
}

func writeLines(t *testing.T, path string, lines ...string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestClaudeContextSumsTheLatestMainThreadRequest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.jsonl")
	writeLines(t, path,
		claudeUsage("assistant", "claude-opus-5-5", false, 2, 36, 535004, 442),
		`{"type":"user","message":{"content":"ok"}}`,
		claudeUsage("assistant", "claude-opus-5-5", false, 2, 499, 535040, 179),
		claudeUsage("assistant", "claude-haiku-4-5", true, 9, 0, 1000, 9),
		claudeUsage("assistant", "<synthetic>", false, 0, 0, 0, 0),
	)
	got, err := claudeContext(path, 0)
	if err != nil || got == nil {
		t.Fatalf("reading = %v, err = %v", got, err)
	}
	if got.Tokens != 2+499+535040+179 || got.Source != "claude" || got.Model != "claude-opus-5-5" || got.Window != 0 {
		t.Fatalf("reading = %+v", got)
	}
	if want := time.Date(2026, 10, 4, 18, 0, 0, 0, time.UTC); !got.Updated.Equal(want) {
		t.Fatalf("updated = %v, want the line's own time", got.Updated)
	}
}

func TestClaudeContextReadsTheTailOfAFileLargerThanTheWindow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.jsonl")
	filler := `{"type":"user","message":{"content":"` + strings.Repeat("x", 4096) + `"}}`
	lines := []string{claudeUsage("assistant", "m", false, 1, 0, 0, 1)}
	for range contextTail/len(filler) + 8 {
		lines = append(lines, filler)
	}
	lines = append(lines, claudeUsage("assistant", "m", false, 7, 0, 100, 3))
	writeLines(t, path, lines...)
	got, err := claudeContext(path, claudeWideWindow)
	if err != nil || got == nil || got.Tokens != 110 || got.Window != claudeWideWindow {
		t.Fatalf("reading = %+v, err = %v", got, err)
	}
}

func TestClaudeContextIsNilWhenNoRequestHasReportedUsage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.jsonl")
	writeLines(t, path, `{"type":"user","message":{"content":"hi"}}`, claudeUsage("assistant", "<synthetic>", false, 0, 0, 0, 0))
	if got, err := claudeContext(path, 0); got != nil || err != nil {
		t.Fatalf("reading = %+v, err = %v", got, err)
	}
}

func TestClaudeWindowIsKnownOnlyFromAnExplicitWideModel(t *testing.T) {
	cases := map[string][]string{
		"wide":      {"claude", "--model", "opus[1m]"},
		"short":     {"claude", "-m", "opus"},
		"plain":     {"claude", "--model", "opus"},
		"no flag":   {"claude"},
		"flag last": {"claude", "--model"},
	}
	want := map[string]int{"wide": claudeWideWindow}
	for name, argv := range cases {
		if got := claudeWindow(argv); got != want[name] {
			t.Errorf("%s: window = %d, want %d", name, got, want[name])
		}
	}
}

func TestCodexContextTakesTheLatestCountWithInfo(t *testing.T) {
	path := filepath.Join(t.TempDir(), "r.jsonl")
	writeLines(t, path,
		codexTokens(codexInfo(116857, 258400)),
		codexTokens(codexInfo(120000, 258400)),
		codexTokens(nil),
		`{"type":"response_item","payload":{"type":"message"}}`,
	)
	got, err := codexContext(path)
	if err != nil || got == nil || got.Tokens != 120000 || got.Window != 258400 || got.Source != "codex" {
		t.Fatalf("reading = %+v, err = %v", got, err)
	}
}

func TestCodexContextIsNilBeforeTheFirstRequest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "r.jsonl")
	writeLines(t, path, codexTokens(nil))
	if got, err := codexContext(path); got != nil || err != nil {
		t.Fatalf("reading = %+v, err = %v", got, err)
	}
}

func proxyServer(t *testing.T, handler func(w http.ResponseWriter, ids []string)) proxyFetcher {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/sessions/usage" {
			http.NotFound(w, r)
			return
		}
		handler(w, r.URL.Query()["id"])
	}))
	t.Cleanup(server.Close)
	fetch, err := newProxyFetcher(server.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	return fetch
}

func TestProxyFetcherAsksForEveryNameAndReadsTheAnswer(t *testing.T) {
	var asked []string
	fetch := proxyServer(t, func(w http.ResponseWriter, ids []string) {
		asked = ids
		fmt.Fprint(w, `{"format":"agent-proxy.session-usage.v1","sessions":{"goose-a":{"context_tokens":4260,"context_window":1000000,"model":"evaluation/x","last_seen":"2026-10-04T18:00:00Z"}}}`)
	})
	got, err := fetch([]string{"goose-a", "opencode-b"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(asked, ",") != "goose-a,opencode-b" {
		t.Fatalf("asked %v", asked)
	}
	if usage, ok := got["goose-a"]; !ok || usage.ContextTokens != 4260 || *usage.ContextWindow != 1000000 || len(got) != 1 {
		t.Fatalf("got %+v", got)
	}
}

func TestProxyFetcherSplitsAReadAtTheIdCap(t *testing.T) {
	var sizes []int
	fetch := proxyServer(t, func(w http.ResponseWriter, ids []string) {
		sizes = append(sizes, len(ids))
		fmt.Fprint(w, `{"format":"agent-proxy.session-usage.v1","sessions":{}}`)
	})
	names := make([]string, proxyBatch+1)
	for index := range names {
		names[index] = fmt.Sprintf("s%d", index)
	}
	if _, err := fetch(names); err != nil {
		t.Fatal(err)
	}
	if len(sizes) != 2 || sizes[0] != proxyBatch || sizes[1] != 1 {
		t.Fatalf("batches %v", sizes)
	}
}

func TestProxyFetcherRefusesAnAnswerItDoesNotRecognise(t *testing.T) {
	for name, body := range map[string]string{"wrong format": `{"format":"other","sessions":{}}`, "not json": `<html>`} {
		fetch := proxyServer(t, func(w http.ResponseWriter, _ []string) { fmt.Fprint(w, body) })
		if _, err := fetch([]string{"a"}); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
	fetch := proxyServer(t, func(w http.ResponseWriter, _ []string) { http.Error(w, "no", http.StatusBadGateway) })
	if _, err := fetch([]string{"a"}); err == nil || !strings.Contains(err.Error(), "502") {
		t.Errorf("a 502 should name its status, got %v", err)
	}
}

func TestNewProxyFetcherRefusesAnythingButAnHTTPURL(t *testing.T) {
	for _, base := range []string{"", "proxy.example", "ftp://proxy.example", "http://"} {
		if _, err := newProxyFetcher(base); err == nil {
			t.Errorf("%q should be refused", base)
		}
	}
}

func TestProxyContextDropsTheLastHoldersOfAReusedName(t *testing.T) {
	started := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	window := 200000
	fresh := proxyUsage{ContextTokens: 900, ContextWindow: &window, Model: "m", LastSeen: started.Add(time.Minute)}
	if got := proxyContext(fresh, started); got == nil || got.Tokens != 900 || got.Window != window || got.Source != "proxy" {
		t.Fatalf("reading = %+v", got)
	}
	if proxyContext(proxyUsage{ContextTokens: 900, LastSeen: started.Add(-time.Minute)}, started) != nil {
		t.Fatal("a reading from before the session began is another session's")
	}
	if got := proxyContext(proxyUsage{ContextTokens: 5, LastSeen: started.Add(time.Second)}, started); got == nil || got.Window != 0 {
		t.Fatalf("an unknown window stays absent, got %+v", got)
	}
	if proxyContext(proxyUsage{LastSeen: started.Add(time.Second)}, started) != nil {
		t.Fatal("zero tokens is no reading")
	}
}

// contextDaemon is a daemon with three live seats and a ledger holding the two
// whose transcripts it can read.
type contextDaemon struct {
	d       *daemon
	claude  *ptySession
	codex   *ptySession
	goose   *ptySession
	claudeF string
	codexF  string
	logs    *[]string
}

func newContextDaemon(t *testing.T) contextDaemon {
	t.Helper()
	root := t.TempDir()
	started := time.Now().UTC().Add(-time.Hour)
	conversation := "0b1c2d3e-4f50-4a6b-8c7d-9e0f1a2b3c4d"
	claudeHome, codexHome := filepath.Join(root, "claude"), filepath.Join(root, "codex")
	claudeFile := filepath.Join(claudeHome, "projects", "-work", conversation+".jsonl")
	codexFile := filepath.Join(codexHome, "sessions", started.Format("2006/01/02"), "rollout-x.jsonl")
	meta, _ := json.Marshal(map[string]any{"type": "session_meta", "payload": map[string]any{
		"cwd": "/work", "timestamp": started.Add(time.Second).Format(time.RFC3339Nano)}})
	writeLines(t, claudeFile, claudeUsage("assistant", "claude-opus-5-5", false, 2, 10, 1000, 38))
	writeLines(t, codexFile, string(meta), codexTokens(codexInfo(5000, 258400)))
	ledger := filepath.Join(root, "ledger")
	for _, entry := range []ledgerEntry{
		{Name: "claude-a", Role: "r", Seat: "claude", Cwd: "/work", Conversation: conversation, Started: started,
			Argv: []string{"claude", "--model", "opus[1m]"}, Dirs: map[string]string{"CLAUDE_CONFIG_DIR": claudeHome}},
		{Name: "codex-a", Role: "r", Seat: "codex", Cwd: "/work", Started: started, Dirs: map[string]string{"CODEX_HOME": codexHome}},
	} {
		if err := writeLedger(ledger, entry); err != nil {
			t.Fatal(err)
		}
	}
	var logs []string
	d := newDaemon(func(format string, args ...any) { logs = append(logs, fmt.Sprintf(format, args...)) })
	d.ledgerDir = ledger
	c := contextDaemon{d: d, claudeF: claudeFile, codexF: codexFile, logs: &logs,
		claude: &ptySession{d: d, name: "claude-a", seat: "claude", started: started},
		codex:  &ptySession{d: d, name: "codex-a", seat: "codex", started: started},
		goose:  &ptySession{d: d, name: "goose-a", seat: "goose", started: started}}
	for _, s := range []*ptySession{c.claude, c.codex, c.goose} {
		d.sessions[s.name] = s
	}
	return c
}

func TestRefreshFoldsClaudeCodexAndProxyIntoOneField(t *testing.T) {
	c := newContextDaemon(t)
	window := 1000000
	c.d.proxyFetch = func(names []string) (map[string]proxyUsage, error) {
		if strings.Join(names, ",") != "goose-a" {
			t.Errorf("only the proxy-backed seat is asked, got %v", names)
		}
		return map[string]proxyUsage{"goose-a": {ContextTokens: 4260, ContextWindow: &window, Model: "evaluation/x", LastSeen: time.Now()}}, nil
	}
	c.d.refreshContexts()
	for seat, want := range map[*ptySession]contextView{
		c.claude: {Tokens: 1050, Window: claudeWideWindow, Source: "claude"},
		c.codex:  {Tokens: 5000, Window: 258400, Source: "codex"},
		c.goose:  {Tokens: 4260, Window: window, Source: "proxy"},
	} {
		got := seat.view().Context
		if got == nil || got.Tokens != want.Tokens || got.Window != want.Window || got.Source != want.Source {
			t.Errorf("%s context = %+v, want %+v", seat.name, got, want)
		}
	}
	if len(*c.logs) != 0 {
		t.Fatalf("a clean round logs nothing, got %v", *c.logs)
	}
}

func TestRefreshFollowsATurnAndIgnoresAnUnchangedFile(t *testing.T) {
	c := newContextDaemon(t)
	c.d.refreshContexts()
	first := c.claude.view().Context
	c.d.refreshContexts()
	if c.claude.view().Context != first {
		t.Fatal("an unchanged transcript must not be read again")
	}
	writeLines(t, c.claudeF, claudeUsage("assistant", "claude-opus-5-5", false, 2, 10, 1000, 38),
		claudeUsage("assistant", "claude-opus-5-5", false, 2, 10, 3000, 90))
	c.d.refreshContexts()
	if got := c.claude.view().Context; got == first || got.Tokens != 3102 {
		t.Fatalf("after a turn: %+v", got)
	}
}

func TestRefreshPushesTheRosterOnlyWhenAReadingMoved(t *testing.T) {
	c := newContextDaemon(t)
	watcher := &conn{writeLine: func([]byte) error { return nil }}
	var pushed int
	watcher.writeLine = func([]byte) error { pushed++; return nil }
	c.d.subscribers[watcher] = true
	c.d.refreshContexts()
	if pushed != 1 {
		t.Fatalf("first reading pushes once, got %d", pushed)
	}
	c.d.refreshContexts()
	if pushed != 1 {
		t.Fatalf("nothing moved, yet pushed %d", pushed)
	}
}

func TestRefreshKeepsTheLastReadingWhenAnAnswerFailsAndSaysSo(t *testing.T) {
	c := newContextDaemon(t)
	window := 0
	answer := map[string]proxyUsage{"goose-a": {ContextTokens: 700, ContextWindow: &window, LastSeen: time.Now()}}
	var failing bool
	c.d.proxyFetch = func([]string) (map[string]proxyUsage, error) {
		if failing {
			return nil, fmt.Errorf("agent proxy answered 502 Bad Gateway")
		}
		return answer, nil
	}
	c.d.refreshContexts()
	failing = true
	c.d.refreshContexts()
	if got := c.goose.view().Context; got == nil || got.Tokens != 700 || got.Window != 0 {
		t.Fatalf("the reading survives a failed round: %+v", got)
	}
	if len(*c.logs) != 1 || !strings.Contains((*c.logs)[0], "502") {
		t.Fatalf("a failed round logs once, got %v", *c.logs)
	}
}

func TestRefreshLeavesASeatWithNoTranscriptBlankAndSilent(t *testing.T) {
	c := newContextDaemon(t)
	// A claude seat launched without --session-id has no conversation to find.
	if err := writeLedger(c.d.ledgerDir, ledgerEntry{Name: "claude-b", Role: "r", Seat: "claude", Cwd: "/work", Started: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	loose := &ptySession{d: c.d, name: "claude-b", seat: "claude", started: time.Now().UTC()}
	c.d.sessions[loose.name] = loose
	c.d.refreshContexts()
	c.d.refreshContexts()
	if loose.view().Context != nil || len(*c.logs) != 0 {
		t.Fatalf("context = %+v, logs = %v", loose.view().Context, *c.logs)
	}
}

func TestSessionViewCarriesContextOnTheWireOnlyWhenRead(t *testing.T) {
	bare, _ := json.Marshal(sessionView{Name: "a"})
	if strings.Contains(string(bare), "context") {
		t.Fatalf("an unread seat omits the field: %s", bare)
	}
	read, _ := json.Marshal(sessionView{Name: "a", Context: &contextView{Tokens: 12, Source: "proxy"}})
	if !strings.Contains(string(read), `"context":{"tokens":12,"source":"proxy","updated":`) {
		t.Fatalf("wire shape: %s", read)
	}
}
