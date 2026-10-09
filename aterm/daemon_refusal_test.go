package main

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestAdmitPeerOnATaggedNodeMatchesNoUntaggedDevice(t *testing.T) {
	// A tagged node has no owner, so the empty owner must match nothing.
	for _, peer := range []tailnetPeer{peerWith("kai@example.com"), peerWith("")} {
		if err := admitPeer(peer, "", []string{"tag:physical"}); err == nil {
			t.Fatalf("a tagged node admitted the untagged device %+v", peer)
		}
	}
	if err := admitPeer(peerWith("tagged-devices", "tag:physical"), "", []string{"tag:physical"}); err != nil {
		t.Fatalf("a physical tower was refused: %v", err)
	}
}

func TestReadTailnetNodeGivesATaggedNodeNoOwner(t *testing.T) {
	calls := fakeTailscale(t, "Running")
	tailnetGate = func() error { return nil }
	script := "#!/bin/sh\n" + `echo '{"BackendState":"Running","Self":{"DNSName":"mac.example.ts.net.","TailscaleIPs":["100.64.0.1"],"UserID":7,"Tags":["tag:physical"]},"User":{"7":{"LoginName":"mac.example.ts.net"}}}'` + "\n"
	if err := os.WriteFile(filepath.Join(filepath.Dir(calls), "tailscale"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	node, err := readTailnetNode()
	if err != nil || node.FQDN != testFQDN || node.Owner != "" {
		t.Fatalf("a tagged node read as %+v, %v", node, err)
	}
}

type logCapture struct {
	mu    sync.Mutex
	lines []string
}

func (c *logCapture) logf(format string, args ...any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lines = append(c.lines, format)
}

func (c *logCapture) refusals() (out []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, line := range c.lines {
		if strings.HasPrefix(line, "refused ") {
			out = append(out, line)
		}
	}
	return out
}

func TestTailnetRefusalNamesItsLayerLogsOnceAndIsReadableByAnAllowedPage(t *testing.T) {
	prev := refusalEvery
	t.Cleanup(func() { refusalEvery = prev })
	refusalEvery = time.Hour

	base, client, d := tailnetServer(t, func(string) (tailnetPeer, error) { return peerWith("guest@example.com"), nil })
	logs := &logCapture{}
	d.logf = logs.logf
	get := func(origin string) *http.Response {
		t.Helper()
		request, _ := http.NewRequest(http.MethodGet, base+reachPath, nil)
		if origin != "" {
			request.Header.Set("Origin", origin)
		}
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { response.Body.Close() })
		return response
	}

	response := get("https://coilyco.dev")
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusForbidden || response.Header.Get(refusalHeader) != "ownership" || !strings.Contains(string(body), "another tailnet user") {
		t.Fatalf("a refused device read as %d %q layer %q", response.StatusCode, body, response.Header.Get(refusalHeader))
	}
	if response.Header.Get("Access-Control-Allow-Origin") != "https://coilyco.dev" || !strings.Contains(response.Header.Get("Access-Control-Expose-Headers"), refusalHeader) {
		t.Fatalf("an allowed page cannot read the refusal: %v", response.Header)
	}
	if other := get("https://evil.example"); other.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("a page the daemon would not give a socket can read the refusal: %v", other.Header)
	}
	get("https://coilyco.dev")
	if got := logs.refusals(); len(got) != 1 {
		t.Fatalf("three refusals from one peer logged %d lines %v, want one", len(got), got)
	}
}

func TestTailnetReachAnswersAnAdmittedDeviceAndNamesAWrongHost(t *testing.T) {
	base, client, d := tailnetServer(t, func(string) (tailnetPeer, error) { return peerWith("kai@example.com"), nil })
	logs := &logCapture{}
	d.logf = logs.logf

	request, _ := http.NewRequest(http.MethodGet, base+reachPath, nil)
	request.Header.Set("Origin", "https://coilyco.dev")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusOK || string(body) != `{"layer":"ok"}` || response.Header.Get("Access-Control-Allow-Origin") != "https://coilyco.dev" {
		t.Fatalf("an admitted device read as %d %q %v", response.StatusCode, body, response.Header)
	}

	request, _ = http.NewRequest(http.MethodGet, base+reachPath, nil)
	request.Host = "127.0.0.1"
	request.Header.Set("Origin", "https://coilyco.dev")
	if response, err = client.Do(request); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusForbidden || response.Header.Get(refusalHeader) != "name" {
		t.Fatalf("a request by address read as %d layer %q", response.StatusCode, response.Header.Get(refusalHeader))
	}
}

func TestTailnetWebsocketOriginRefusalIsLogged(t *testing.T) {
	base, client, d := tailnetServer(t, func(string) (tailnetPeer, error) { return peerWith("kai@example.com"), nil })
	logs := &logCapture{}
	d.logf = logs.logf
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, response, err := websocket.Dial(ctx, "wss"+strings.TrimPrefix(base, "https")+"/", &websocket.DialOptions{HTTPClient: client, HTTPHeader: http.Header{"Origin": {"https://evil.example"}}})
	if err == nil || response == nil || response.Header.Get(refusalHeader) != "origin" {
		t.Fatalf("a foreign origin read as %v %v", err, response)
	}
	if got := logs.refusals(); len(got) != 1 {
		t.Fatalf("an origin refusal logged %v, want one line", got)
	}
}

func TestTailnetAdmitsADeviceTaggedAfterItWasRefused(t *testing.T) {
	prev := whoisRefusedTTL
	t.Cleanup(func() { whoisRefusedTTL = prev })
	whoisRefusedTTL = 50 * time.Millisecond

	var calls atomic.Int32
	var tagged atomic.Bool
	base, client, _ := tailnetServer(t, func(string) (tailnetPeer, error) {
		calls.Add(1)
		if tagged.Load() {
			return peerWith("tagged-devices", "tag:physical"), nil
		}
		return peerWith("guest@example.com"), nil
	})
	status := func() int {
		t.Helper()
		response, err := client.Get(base + "/")
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		return response.StatusCode
	}

	if got := status(); got != http.StatusForbidden {
		t.Fatalf("an untagged guest read as %d", got)
	}
	if got := status(); got != http.StatusForbidden || calls.Load() != 1 {
		t.Fatalf("a burst of refusals made %d whois calls and read %d, want one call", calls.Load(), got)
	}
	tagged.Store(true)
	time.Sleep(80 * time.Millisecond)
	if got := status(); got != http.StatusOK {
		t.Fatalf("a device tagged after its refusal still read %d", got)
	}
}
