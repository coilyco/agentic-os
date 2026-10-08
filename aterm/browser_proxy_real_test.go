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

func TestCDPProxyWithTheRealPlaywrightMCPAndARealChromium(t *testing.T) {
	pages := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, `<!doctype html><title>page %s</title><h1 id=h>hello from %s</h1><button onclick="document.title='clicked'">go</button>`, r.URL.Path, r.URL.Path)
	}))
	defer pages.Close()

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
	h.d.loopbackAddr = strings.TrimPrefix(h.url[:strings.Index(h.url, cdpPrefix)], "ws://")

	session := realPlaywright(t, h, &mcpPID)
	if got := playwrightTool(t, session, "browser_navigate", map[string]any{"url": pages.URL + "/first"}); strings.HasPrefix(got, "ERROR") {
		t.Fatalf("browser_navigate through the proxy: %s", got)
	}

	// The client's Browser pane watches the same browser the agent just drove.
	person, _ := dialLoopbackClient(t, h.url[:strings.Index(h.url, cdpPrefix)])
	person.send(frame{Type: "browser_watch", Session: "scientist-evie"})
	person.untilState("live", func(m frame) bool { return strings.HasSuffix(m.URL, "/first") })
	person.untilFrame("a frame of the agent's page", func(m frame) bool { return m.Type == "browser_frame" })

	// And what the person does is what the agent's next snapshot reads.
	person.send(frame{Type: "browser_control", Session: "scientist-evie", Take: true})
	person.untilState("live", func(m frame) bool { return m.Driver == "person" })
	person.send(frame{Type: "browser_navigate", Session: "scientist-evie", URL: pages.URL + "/second"})
	person.untilState("live", func(m frame) bool { return strings.HasSuffix(m.URL, "/second") })
	deadline := time.Now().Add(10 * time.Second)
	var snapshot string
	for time.Now().Before(deadline) {
		snapshot = playwrightTool(t, session, "browser_snapshot", map[string]any{})
		if strings.Contains(snapshot, "hello from /second") {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("the agent's snapshot never showed the person's page:\n%s", snapshot)
}
