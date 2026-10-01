package main

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func peerWith(login string, tags ...string) tailnetPeer {
	var peer tailnetPeer
	peer.UserProfile.LoginName = login
	peer.Node.Tags = tags
	return peer
}

func TestAdmitPeerTakesTheOwnerAndAllowedTagsOnly(t *testing.T) {
	allow := []string{"tag:physical"}
	for _, testCase := range []struct {
		name  string
		peer  tailnetPeer
		admit bool
	}{
		{"the owner's own phone", peerWith("kai@example.com"), true},
		{"another tailnet user", peerWith("guest@example.com"), false},
		{"an untagged device with no login", peerWith(""), false},
		{"a physical tower", peerWith("tagged-devices", "tag:kai-tower-3026", "tag:physical"), true},
		{"a CI runner", peerWith("tagged-devices", "tag:ci"), false},
		{"a tagged device claiming the owner's login", peerWith("kai@example.com", "tag:proxy"), false},
	} {
		err := admitPeer(testCase.peer, "kai@example.com", allow)
		if (err == nil) != testCase.admit {
			t.Fatalf("%s: admitted=%v, want %v (%v)", testCase.name, err == nil, testCase.admit, err)
		}
	}
}

const testFQDN = "mac.example.ts.net"

// tailnetServer serves the tailnet policy over TLS on loopback, and a client
// that reaches it by the tailnet name, as a browser on another device would.
func tailnetServer(t *testing.T, whois func(string) (tailnetPeer, error)) (string, *http.Client, *daemon) {
	t.Helper()
	d := newDaemon(func(string, ...any) {})
	d.clientDir = t.TempDir()
	if err := os.WriteFile(filepath.Join(d.clientDir, "index.html"), []byte("aterm client"), 0o644); err != nil {
		t.Fatal(err)
	}
	node := tailnetNode{FQDN: testFQDN, IPv4: "127.0.0.1", Owner: "kai@example.com"}
	server := httptest.NewTLSServer(d.handler(tailnetPolicy(node, []string{"tag:physical"}, []string{"https://coilyco.dev"}, whois)))
	t.Cleanup(server.Close)
	_, port, _ := net.SplitHostPort(server.Listener.Addr().String())
	// Trust the test server's own certificate. It names example.com, and the
	// dialer below routes the tailnet name to it.
	trusted := server.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	trusted.ServerName = "example.com"
	client := &http.Client{Transport: &http.Transport{
		TLSClientConfig: trusted,
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
		},
	}}
	return "https://" + testFQDN + ":" + port, client, d
}

func TestTailnetServesTheClientToAnAdmittedDeviceOnly(t *testing.T) {
	var calls atomic.Int32
	admitted := true
	base, client, _ := tailnetServer(t, func(string) (tailnetPeer, error) {
		calls.Add(1)
		if admitted {
			return peerWith("tagged-devices", "tag:physical"), nil
		}
		return peerWith("guest@example.com"), nil
	})
	response, err := client.Get(base + "/")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusOK || string(body) != "aterm client" {
		t.Fatalf("an admitted device should get the client: %d %q", response.StatusCode, body)
	}
	if response, err = client.Get(base + "/index.html"); err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("second load: %v %v", err, response)
	}
	response.Body.Close()
	if calls.Load() != 1 {
		t.Fatalf("a page load should cost one whois, got %d", calls.Load())
	}
}

func TestTailnetRefusesAnotherUserAndAnUnnamedHost(t *testing.T) {
	base, client, _ := tailnetServer(t, func(string) (tailnetPeer, error) { return peerWith("guest@example.com"), nil })
	if response, err := client.Get(base + "/"); err != nil || response.StatusCode != http.StatusForbidden {
		t.Fatalf("another tailnet user must get 403: %v %v", err, response)
	}
	// Reached by address rather than by name is not how the client is served.
	request, _ := http.NewRequest(http.MethodGet, base+"/", nil)
	request.Host = "127.0.0.1"
	if response, err := client.Do(request); err != nil || response.StatusCode != http.StatusForbidden {
		t.Fatalf("a request not addressed to the tailnet name must get 403: %v %v", err, response)
	}
	broken, brokenClient, _ := tailnetServer(t, func(string) (tailnetPeer, error) { return tailnetPeer{}, errors.New("tailscaled down") })
	if response, err := brokenClient.Get(broken + "/"); err != nil || response.StatusCode != http.StatusForbidden {
		t.Fatalf("an unanswered whois must refuse, not admit: %v %v", err, response)
	}
}

func TestTailnetWebsocketTakesOnlyThePageItServed(t *testing.T) {
	base, client, _ := tailnetServer(t, func(string) (tailnetPeer, error) { return peerWith("kai@example.com"), nil })
	address := "wss" + strings.TrimPrefix(base, "https") + "/"
	dial := func(origin string) (*websocket.Conn, *http.Response, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return websocket.Dial(ctx, address, &websocket.DialOptions{HTTPClient: client, HTTPHeader: http.Header{"Origin": {origin}}})
	}
	for _, origin := range []string{base, "https://coilyco.dev", "https://COILYCO.dev"} {
		ws, _, err := dial(origin)
		if err != nil {
			t.Fatalf("origin %q must open a socket: %v", origin, err)
		}
		_ = ws.CloseNow()
	}
	for _, origin := range []string{"https://evil.example", "http://" + strings.TrimPrefix(base, "https://"), "http://localhost:5173", "http://coilyco.dev", "https://coilyco.dev.evil.example", "https://www.coilyco.dev"} {
		if ws, response, err := dial(origin); err == nil {
			_ = ws.CloseNow()
			t.Fatalf("origin %q must be refused", origin)
		} else if response == nil || response.StatusCode != http.StatusForbidden {
			t.Fatalf("origin %q should get 403, got %v", origin, response)
		}
	}
}

func TestLoopbackServesTheClientWhenOneIsInstalled(t *testing.T) {
	d := newDaemon(func(string, ...any) {})
	server := httptest.NewServer(d.handler(loopbackPolicy()))
	t.Cleanup(server.Close)
	if response, err := http.Get(server.URL + "/"); err != nil || response.StatusCode != http.StatusNotFound {
		t.Fatalf("no client installed should be 404: %v %v", err, response)
	}
	d.clientDir = t.TempDir()
	if err := os.WriteFile(filepath.Join(d.clientDir, "index.html"), []byte("aterm client"), 0o644); err != nil {
		t.Fatal(err)
	}
	response, err := http.Get(server.URL + "/")
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("an installed client should be served: %v %v", err, response)
	}
	response.Body.Close()
}

func TestVPNGateOnlyOpensOnAConnectedTunnelOrNoMacVPN(t *testing.T) {
	for state, open := range map[string]bool{
		"Connected":     true,
		"No service":    true,
		"Disconnected":  false,
		"Connecting":    false,
		"Disconnecting": false,
		"":              false,
	} {
		if err := vpnGate(state); (err == nil) != open {
			t.Fatalf("%q: open=%v, want %v (%v)", state, err == nil, open, err)
		}
	}
}

// fakeTailscale installs a tailscale CLI that records each call in a file,
// and answers `status --json` with the given backend state.
func fakeTailscale(t *testing.T, backendState string) (calls string) {
	t.Helper()
	dir := t.TempDir()
	calls = filepath.Join(dir, "calls")
	script := "#!/bin/sh\necho \"$@\" >> '" + calls + "'\n" +
		`echo '{"BackendState":"` + backendState + `","Self":{"DNSName":"mac.example.ts.net.","TailscaleIPs":["100.64.0.1","fd7a::1"],"UserID":7},"User":{"7":{"LoginName":"kai@example.com"}}}'` + "\n"
	bin := filepath.Join(dir, "tailscale")
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	prevBin, prevGate := tailscaleBin, tailnetGate
	t.Cleanup(func() { tailscaleBin, tailnetGate = prevBin, prevGate })
	tailscaleBin = bin
	return calls
}

func TestTailscaleCLINeverRunsWhileTheGateIsClosed(t *testing.T) {
	calls := fakeTailscale(t, "Running")
	tailnetGate = func() error { return errors.New("the Tailscale VPN is \"Disconnected\"") }
	if _, err := readTailnetNode(); err == nil {
		t.Fatal("readTailnetNode succeeded through a closed gate")
	}
	if _, err := tailscaleWhois("100.64.0.2"); err == nil {
		t.Fatal("tailscaleWhois succeeded through a closed gate")
	}
	if _, err := os.Stat(calls); !os.IsNotExist(err) {
		t.Fatalf("the CLI ran while the gate was closed: %v", err)
	}
}

func TestReadTailnetNodeNeedsARunningBackend(t *testing.T) {
	fakeTailscale(t, "Running")
	tailnetGate = func() error { return nil }
	node, err := readTailnetNode()
	if err != nil || node.FQDN != testFQDN || node.IPv4 != "100.64.0.1" || node.Owner != "kai@example.com" {
		t.Fatalf("node = %+v, %v", node, err)
	}
	fakeTailscale(t, "Stopped")
	tailnetGate = func() error { return nil }
	if _, err := readTailnetNode(); err == nil || !strings.Contains(err.Error(), "Stopped") {
		t.Fatalf("a Stopped backend read as up: %v", err)
	}
}

func TestServeTailnetWhenUpRetriesUntilItBindsThenClosesOnDone(t *testing.T) {
	prev := tailnetBackoff
	t.Cleanup(func() { tailnetBackoff = prev })
	tailnetBackoff.first, tailnetBackoff.max = time.Millisecond, 4*time.Millisecond

	var attempts atomic.Int32
	server := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(server.Close)
	var logged atomic.Int32
	d := newDaemon(func(string, ...any) { logged.Add(1) })
	done, finished := make(chan struct{}), make(chan struct{})
	go func() {
		d.serveTailnetWhenUp(done, func() (*http.Server, error) {
			if attempts.Add(1) < 4 {
				return nil, errors.New("the Tailscale VPN is \"Disconnected\"")
			}
			return server.Config, nil
		})
		close(finished)
	}()
	for attempts.Load() < 4 {
		time.Sleep(time.Millisecond)
	}
	if got := logged.Load(); got != 1 {
		t.Fatalf("one standing reason logged %d times", got)
	}
	close(done)
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("serveTailnetWhenUp outlived done")
	}
	if conn, err := net.Dial("tcp", server.Listener.Addr().String()); err == nil {
		conn.Close()
		t.Fatal("the bound server kept accepting after done")
	}
}

func TestServeTailnetWhenUpStopsRetryingOnDone(t *testing.T) {
	prev := tailnetBackoff
	t.Cleanup(func() { tailnetBackoff = prev })
	tailnetBackoff.first, tailnetBackoff.max = time.Hour, time.Hour
	d := newDaemon(func(string, ...any) {})
	done, finished := make(chan struct{}), make(chan struct{})
	go func() {
		d.serveTailnetWhenUp(done, func() (*http.Server, error) { return nil, errors.New("down") })
		close(finished)
	}()
	close(done)
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("a pending retry held the daemon past done")
	}
}
