package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	// contextFeature is how a client knows the daemon puts `context` on a session.
	contextFeature = "context"
	// contextEvery is how often live seats are read, one stat each when idle.
	contextEvery = 4 * time.Second
	// contextRetry is how long a seat whose transcript was not found waits to look again.
	contextRetry = 30 * time.Second
	// contextTail is how much of a transcript's end is searched for the latest usage line.
	contextTail = 2 << 20
	// proxyUsageFormat is what Agent Proxy's session usage route answers.
	proxyUsageFormat = "agent-proxy.session-usage.v1"
	// proxyBatch is the proxy's cap on id parameters per read.
	proxyBatch = 64
	// claudeWideWindow is the window of a claude model chosen with the [1m] suffix.
	claudeWideWindow = 1_000_000
)

// agentProxyEnv is the proxy base URL, fed by daemon.env since a tailnet host is not
// tracked config. Unset turns the proxy source off.
const agentProxyEnv = "ATERM_AGENT_PROXY_URL"

// contextView is how full a seat's context is. Window is absent where the source
// does not know it, and a client shows a percent only when it is present.
type contextView struct {
	Tokens  int       `json:"tokens"`
	Window  int       `json:"window,omitempty"`
	Source  string    `json:"source"`
	Model   string    `json:"model,omitempty"`
	Updated time.Time `json:"updated"`
}

// sameReading ignores Updated, so a reading that is only re-timestamped does not push.
func sameReading(a, b *contextView) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Tokens == b.Tokens && a.Window == b.Window && a.Source == b.Source && a.Model == b.Model
}

// fileSig is what a transcript looked like when it was last read.
type fileSig struct {
	size int64
	mod  time.Time
}

func sigOf(path string) (fileSig, bool) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return fileSig{}, false
	}
	return fileSig{info.Size(), info.ModTime()}, true
}

// fileTailLines is the whole lines in the last `window` bytes of a file, oldest first.
func fileTailLines(path string, window int64) ([][]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	offset := max(info.Size()-window, 0)
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return nil, err
	}
	reader := bufio.NewReaderSize(file, 1<<20)
	if offset > 0 {
		// The first line is cut mid-way, so it is dropped.
		if _, err := reader.ReadBytes('\n'); err != nil {
			return nil, nil
		}
	}
	var lines [][]byte
	for {
		raw, err := reader.ReadBytes('\n')
		if len(bytes.TrimSpace(raw)) > 0 {
			lines = append(lines, raw)
		}
		if err != nil {
			return lines, nil
		}
	}
}

// claudeUsageLine is the part of a claude transcript line that carries usage.
// The format is claude's and unversioned, so a field not named here is never read.
type claudeUsageLine struct {
	Type        string `json:"type"`
	IsSidechain bool   `json:"isSidechain"`
	Timestamp   string `json:"timestamp"`
	Message     struct {
		Model string `json:"model"`
		Usage struct {
			Input         int `json:"input_tokens"`
			CacheCreation int `json:"cache_creation_input_tokens"`
			CacheRead     int `json:"cache_read_input_tokens"`
			Output        int `json:"output_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

// claudeContext sums the latest main-thread request's input and output, which is
// what the next one starts from. Subagent and synthetic lines are not this window.
func claudeContext(path string, window int) (*contextView, error) {
	lines, err := fileTailLines(path, contextTail)
	if err != nil {
		return nil, err
	}
	for index := len(lines) - 1; index >= 0; index-- {
		var line claudeUsageLine
		if json.Unmarshal(lines[index], &line) != nil || line.Type != "assistant" || line.IsSidechain {
			continue
		}
		usage := line.Message.Usage
		tokens := usage.Input + usage.CacheCreation + usage.CacheRead + usage.Output
		if tokens == 0 || line.Message.Model == "<synthetic>" {
			continue
		}
		return &contextView{Tokens: tokens, Window: window, Source: "claude", Model: line.Message.Model, Updated: stampOf(line.Timestamp, path)}, nil
	}
	return nil, nil
}

// claudeWindow is known only from a [1m] suffix on --model, since the transcript
// records the resolved model id and no window.
func claudeWindow(argv []string) int {
	for index := 0; index+1 < len(argv); index++ {
		if (argv[index] == "--model" || argv[index] == "-m") && strings.HasSuffix(argv[index+1], "[1m]") {
			return claudeWideWindow
		}
	}
	return 0
}

// codexUsageLine is the part of a codex rollout line that carries a token count.
type codexUsageLine struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	Payload   struct {
		Type string `json:"type"`
		Info *struct {
			Last struct {
				Total int `json:"total_tokens"`
			} `json:"last_token_usage"`
			Window int `json:"model_context_window"`
		} `json:"info"`
	} `json:"payload"`
}

// codexContext is the latest token_count event with an info block. Codex sends one
// with null info before the first request, which says nothing yet.
func codexContext(path string) (*contextView, error) {
	lines, err := fileTailLines(path, contextTail)
	if err != nil {
		return nil, err
	}
	for index := len(lines) - 1; index >= 0; index-- {
		var line codexUsageLine
		if json.Unmarshal(lines[index], &line) != nil || line.Type != "event_msg" || line.Payload.Type != "token_count" || line.Payload.Info == nil {
			continue
		}
		if line.Payload.Info.Last.Total == 0 {
			continue
		}
		return &contextView{Tokens: line.Payload.Info.Last.Total, Window: line.Payload.Info.Window, Source: "codex", Updated: stampOf(line.Timestamp, path)}, nil
	}
	return nil, nil
}

// stampOf is a line's own time, else the file's.
func stampOf(stamp, path string) time.Time {
	if parsed, err := time.Parse(time.RFC3339Nano, stamp); err == nil {
		return parsed.UTC()
	}
	if sig, ok := sigOf(path); ok {
		return sig.mod.UTC()
	}
	return time.Time{}
}

// proxyUsage is one session in Agent Proxy's usage answer.
type proxyUsage struct {
	ContextTokens int       `json:"context_tokens"`
	ContextWindow *int      `json:"context_window"`
	Model         string    `json:"model"`
	LastSeen      time.Time `json:"last_seen"`
}

// proxyFetcher reads usage for session names, absent names left out.
type proxyFetcher func(names []string) (map[string]proxyUsage, error)

// newProxyFetcher asks the proxy at base, one read per proxyBatch names.
func newProxyFetcher(base string) (proxyFetcher, error) {
	parsed, err := url.Parse(strings.TrimSpace(base))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, fmt.Errorf("%s is not an http(s) URL", agentProxyEnv)
	}
	client := &http.Client{Timeout: 3 * time.Second}
	endpoint := strings.TrimRight(parsed.String(), "/") + "/v1/sessions/usage"
	return func(names []string) (map[string]proxyUsage, error) {
		found := map[string]proxyUsage{}
		for start := 0; start < len(names); start += proxyBatch {
			query := url.Values{}
			for _, name := range names[start:min(start+proxyBatch, len(names))] {
				query.Add("id", name)
			}
			response, err := client.Get(endpoint + "?" + query.Encode())
			if err != nil {
				return nil, err
			}
			var body struct {
				Format   string                `json:"format"`
				Sessions map[string]proxyUsage `json:"sessions"`
			}
			err = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&body)
			_ = response.Body.Close()
			if response.StatusCode != http.StatusOK {
				return nil, fmt.Errorf("agent proxy answered %s", response.Status)
			}
			if err != nil || body.Format != proxyUsageFormat {
				return nil, fmt.Errorf("agent proxy answered no %s", proxyUsageFormat)
			}
			for name, usage := range body.Sessions {
				found[name] = usage
			}
		}
		return found, nil
	}, nil
}

// proxyContext drops a reading from before the session began, since a resume
// reuses the name of the session that made it.
func proxyContext(usage proxyUsage, started time.Time) *contextView {
	if usage.ContextTokens <= 0 || usage.LastSeen.Before(started) {
		return nil
	}
	view := &contextView{Tokens: usage.ContextTokens, Source: "proxy", Model: usage.Model, Updated: usage.LastSeen.UTC()}
	if usage.ContextWindow != nil && *usage.ContextWindow > 0 {
		view.Window = *usage.ContextWindow
	}
	return view
}

// contextSource is what a session remembers between reads, so an unchanged file is
// not parsed again and a codex rollout is not searched for twice.
type contextSource struct {
	path string
	sig  fileSig
}

// setContext keeps a new reading and says whether it differs. A round with no
// reading keeps the last one rather than flickering out.
func (s *ptySession) setContext(next *contextView) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if next == nil || sameReading(s.context, next) {
		return false
	}
	s.context = next
	return true
}

// readContext reads a seat's own transcript, answering nil when it has none or
// the file has not changed.
func (s *ptySession) readContext(entry ledgerEntry, all []ledgerEntry) (*contextView, error) {
	s.mu.Lock()
	source := s.contextFile
	s.mu.Unlock()
	if source.path == "" {
		s.mu.Lock()
		wait := time.Now().Before(s.contextRetry)
		s.mu.Unlock()
		if wait {
			return nil, nil
		}
		var err error
		switch s.seat {
		case "claude":
			source.path, err = transcriptPath(entry)
		case "codex":
			source.path, err = codexRollout(entry, all)
		}
		if err != nil || source.path == "" {
			s.mu.Lock()
			s.contextRetry = time.Now().Add(contextRetry)
			s.mu.Unlock()
			if errors.Is(err, errNoTranscript) {
				err = nil
			}
			return nil, err
		}
	}
	sig, ok := sigOf(source.path)
	if !ok {
		s.mu.Lock()
		s.contextFile = contextSource{}
		s.mu.Unlock()
		return nil, nil
	}
	if sig == source.sig {
		return nil, nil
	}
	var view *contextView
	var err error
	if s.seat == "claude" {
		view, err = claudeContext(source.path, claudeWindow(entry.Argv))
	} else {
		view, err = codexContext(source.path)
	}
	if err == nil {
		source.sig = sig
	}
	s.mu.Lock()
	s.contextFile = source
	s.mu.Unlock()
	return view, err
}

// refreshContexts reads every live seat once and pushes the roster if a reading
// moved. Seats other than claude and codex are asked of Agent Proxy.
func (d *daemon) refreshContexts() {
	d.mu.Lock()
	sessions := make([]*ptySession, 0, len(d.sessions))
	for _, s := range d.sessions {
		sessions = append(sessions, s)
	}
	fetch := d.proxyFetch
	ledger := d.ledgerDir
	d.mu.Unlock()
	var recorded []ledgerEntry
	if ledger != "" {
		recorded = readLedger(ledger)
	}
	changed := false
	var proxied []*ptySession
	for _, s := range sessions {
		switch s.seat {
		case "claude", "codex":
			entry, err := findRecorded(recorded, s.name)
			if err != nil {
				continue
			}
			view, err := s.readContext(entry, recorded)
			if err != nil {
				d.logf("context of %s: %v", s.name, err)
			}
			changed = s.setContext(view) || changed
		default:
			proxied = append(proxied, s)
		}
	}
	if fetch != nil && len(proxied) > 0 {
		names := make([]string, 0, len(proxied))
		for _, s := range proxied {
			names = append(names, s.name)
		}
		found, err := fetch(names)
		if err != nil {
			d.logf("context from agent proxy: %v", err)
		}
		for _, s := range proxied {
			if usage, ok := found[s.name]; ok {
				changed = s.setContext(proxyContext(usage, s.started)) || changed
			}
		}
	}
	if changed {
		d.pushSessions()
	}
}

// watchContext reads on a timer until done closes.
func (d *daemon) watchContext(done <-chan struct{}, every time.Duration) {
	defer d.guard("context")
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			d.refreshContexts()
		}
	}
}
