package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// realPlaywrightEnv names an @playwright/mcp cli.js. The test skips without it and
// a real Chromium, which CI images lack. The browser reference says how to run it.
const realPlaywrightEnv = "ATERM_REAL_PLAYWRIGHT_MCP"

// realPlaywright starts the real Playwright MCP as a harness would, with only the
// endpoint in its environment. Its pid is recorded for the daemon's process table.
func realPlaywright(t *testing.T, h *proxyHarness, mcpPID *atomic.Int64) *mcp.ClientSession {
	t.Helper()
	cli := os.Getenv(realPlaywrightEnv)
	if cli == "" || chromiumPath() == "" {
		t.Skipf("set %s to an @playwright/mcp cli.js, and have a Chrome or Chromium", realPlaywrightEnv)
	}
	command := exec.Command("node", cli, "--headless")
	// The MCP writes its console and snapshot files into its working directory.
	command.Dir = t.TempDir()
	command.Env = append(browserEnviron(), cdpEndpointEnv+"="+h.d.cdpEndpoint(h.token))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	session, err := mcp.NewClient(&mcp.Implementation{Name: "real-playwright", Version: "0"}, nil).
		Connect(ctx, &mcp.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatalf("start the Playwright MCP: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	mcpPID.Store(int64(command.Process.Pid))
	return session
}

func playwrightTool(t *testing.T, session *mcp.ClientSession, name string, arguments map[string]any) string {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	var text strings.Builder
	for _, content := range result.Content {
		if piece, ok := content.(*mcp.TextContent); ok {
			text.WriteString(piece.Text)
		}
	}
	if result.IsError {
		return "ERROR: " + text.String()
	}
	return text.String()
}

// realProxy is a daemon with a real Chromium behind it and a real Playwright MCP
// pointed at it through the proxy.
type realProxy struct {
	h       *proxyHarness
	pages   *httptest.Server
	session *mcp.ClientSession
	address string
}

func startRealProxy(t *testing.T) *realProxy {
	t.Helper()
	pages := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, `<!doctype html><title>page %s</title><h1 id=h>hello from %s</h1><button onclick="location.href='/clicked'">go</button>`, r.URL.Path, r.URL.Path)
	}))
	t.Cleanup(pages.Close)

	h := newProxyHarness(t)
	h.d.browserLaunch = nil
	// A real process table, with the MCP's parent rewritten to the session's
	// process, since the MCP is a child of this test and not of a harness.
	real := listProcesses
	var mcpPID atomic.Int64
	sessionPID := h.d.session("scientist-evie").pid
	h.d.processes = func() ([]processEntry, error) {
		entries, err := real()
		for i := range entries {
			if int64(entries[i].PID) == mcpPID.Load() {
				entries[i].PPID = sessionPID
			}
		}
		return entries, err
	}
	h.d.peerLookup = tcpPeerPIDs
	address := h.url[:strings.Index(h.url, cdpPrefix)]
	h.d.loopbackAddr = strings.TrimPrefix(address, "ws://")
	return &realProxy{h: h, pages: pages, session: realPlaywright(t, h, &mcpPID), address: address}
}

// screen is a person's client watching the session's browser.
func (r *realProxy) screen(t *testing.T) wsClient {
	t.Helper()
	person, _ := dialLoopbackClient(t, r.address)
	person.send(frame{Type: "browser_watch", Session: "scientist-evie"})
	return person
}

func (r *realProxy) tool(t *testing.T, name string, arguments map[string]any) string {
	t.Helper()
	return playwrightTool(t, r.session, name, arguments)
}

func TestCDPProxyWithTheRealPlaywrightMCPAndARealChromium(t *testing.T) {
	r := startRealProxy(t)
	if got := r.tool(t, "browser_navigate", map[string]any{"url": r.pages.URL + "/first"}); strings.HasPrefix(got, "ERROR") {
		t.Fatalf("browser_navigate through the proxy: %s", got)
	}

	// The client's Browser pane watches the same browser the agent just drove.
	person := r.screen(t)
	person.untilState("live", func(m frame) bool { return strings.HasSuffix(m.URL, "/first") })
	person.untilFrame("a frame of the agent's page", func(m frame) bool { return m.Type == "browser_frame" })

	// And what the person does is what the agent's next snapshot reads, once handed back.
	person.send(frame{Type: "browser_control", Session: "scientist-evie", Take: true})
	person.untilState("live", func(m frame) bool { return m.Driver == "person" })
	person.send(frame{Type: "browser_navigate", Session: "scientist-evie", URL: r.pages.URL + "/second"})
	person.untilState("live", func(m frame) bool { return strings.HasSuffix(m.URL, "/second") })
	person.send(frame{Type: "browser_control", Session: "scientist-evie", Take: false})
	person.untilState("live", func(m frame) bool { return m.Driver == "agent" })
	if snapshot := r.tool(t, "browser_snapshot", map[string]any{}); !strings.Contains(snapshot, "hello from /second") {
		t.Fatalf("the agent's snapshot should show the person's page:\n%s", snapshot)
	}
}

// buttonRef is the snapshot's reference for the page's one button.
func buttonRef(t *testing.T, snapshot string) string {
	t.Helper()
	for _, line := range strings.Split(snapshot, "\n") {
		if _, after, found := strings.Cut(line, "[ref="); found && strings.Contains(line, "button") {
			ref, _, _ := strings.Cut(after, "]")
			return ref
		}
	}
	t.Fatalf("no button in the snapshot:\n%s", snapshot)
	return ""
}

func TestCDPProxyRefusesTheRealPlaywrightWhileAPersonHoldsControl(t *testing.T) {
	r := startRealProxy(t)
	if got := r.tool(t, "browser_navigate", map[string]any{"url": r.pages.URL + "/first"}); strings.HasPrefix(got, "ERROR") {
		t.Fatalf("browser_navigate: %s", got)
	}
	ref := buttonRef(t, r.tool(t, "browser_snapshot", map[string]any{}))
	person := r.screen(t)
	person.untilState("live", nil)
	person.send(frame{Type: "browser_control", Session: "scientist-evie", Take: true})
	person.untilState("live", func(m frame) bool { return m.Driver == "person" })

	// A ref target is rewritten by Playwright's own "ref not found", so the click
	// names the button by selector. The others reach the tool's error as sent.
	refused := map[string]map[string]any{
		"browser_click":    {"element": "go", "target": "button"},
		"browser_navigate": {"url": r.pages.URL + "/agent"},
		"browser_snapshot": {},
	}
	for name, arguments := range refused {
		if got := r.tool(t, name, arguments); !strings.Contains(got, personHasControl) {
			t.Fatalf("%s while a person holds control should fail with the handback message, got:\n%s", name, got)
		}
	}
	person.send(frame{Type: "browser_control", Session: "scientist-evie", Take: false})
	person.untilState("live", func(m frame) bool { return m.Driver == "agent" })
	if got := r.tool(t, "browser_click", map[string]any{"element": "go", "target": ref}); strings.HasPrefix(got, "ERROR") {
		t.Fatalf("browser_click after the hand back: %s", got)
	}
	person.untilState("live", func(m frame) bool { return strings.HasSuffix(m.URL, "/clicked") })
}

func TestCDPProxyLetsARealPlaywrightConnectWhileAPersonHoldsControl(t *testing.T) {
	r := startRealProxy(t)
	person := r.screen(t)
	person.untilState("live", nil)
	person.send(frame{Type: "browser_control", Session: "scientist-evie", Take: true})
	person.untilState("live", func(m frame) bool { return m.Driver == "person" })

	got := r.tool(t, "browser_navigate", map[string]any{"url": r.pages.URL + "/first"})
	if !strings.Contains(got, personHasControl) {
		t.Fatalf("a first tool call during person control should reach the handback message, got:\n%s", got)
	}
	person.send(frame{Type: "browser_control", Session: "scientist-evie", Take: false})
	person.untilState("live", func(m frame) bool { return m.Driver == "agent" })
	if got := r.tool(t, "browser_navigate", map[string]any{"url": r.pages.URL + "/first"}); strings.HasPrefix(got, "ERROR") {
		t.Fatalf("browser_navigate after the hand back: %s", got)
	}
}
