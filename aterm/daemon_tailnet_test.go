package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
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
		err := admitPeer(testCase.peer, []string{"kai@example.com"}, allow)
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
	return tailnetServerWith(t, siblingPages{}, whois)
}

func tailnetServerWith(t *testing.T, siblings siblingPages, whois func(string) (tailnetPeer, error)) (string, *http.Client, *daemon) {
	t.Helper()
	d := newDaemon(func(string, ...any) {})
	d.clientDir = t.TempDir()
	if err := os.WriteFile(filepath.Join(d.clientDir, "index.html"), []byte("aterm client"), 0o644); err != nil {
		t.Fatal(err)
	}
	node := tailnetNode{FQDN: testFQDN, IPv4: "127.0.0.1", Owner: "kai@example.com"}
	server := httptest.NewTLSServer(d.handler(tailnetPolicy(node, []string{"tag:physical"}, nil, []string{"https://coilyco.dev"}, siblings, whois)))
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
	d.clientFS = nil
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
	return fakeTailscaleAt(t, backendState, "100.64.0.1")
}

// fakeTailscaleAt reports this node at ip, so a test can aim the node at a
// listener on loopback.
func fakeTailscaleAt(t *testing.T, backendState, ip string) (calls string) {
	t.Helper()
	dir := t.TempDir()
	calls = filepath.Join(dir, "calls")
	script := "#!/bin/sh\necho \"$@\" >> '" + calls + "'\n" +
		`echo '{"BackendState":"` + backendState + `","Self":{"DNSName":"mac.example.ts.net.","TailscaleIPs":["` + ip + `","fd7a::1"],"UserID":7},"User":{"7":{"LoginName":"kai@example.com"}}}'` + "\n"
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
		d.serveTailnetWhenUp(done, func() (*tailnetServing, error) {
			if attempts.Add(1) < 4 {
				return nil, errors.New("the Tailscale VPN is \"Disconnected\"")
			}
			return &tailnetServing{server: server.Config}, nil
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
		d.serveTailnetWhenUp(done, func() (*tailnetServing, error) { return nil, errors.New("down") })
		close(finished)
	}()
	close(done)
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("a pending retry held the daemon past done")
	}
}

// fastProbe paces the self-probe in milliseconds for the test's length.
func fastProbe(t *testing.T) {
	t.Helper()
	prevBackoff, prevProbe := tailnetBackoff, tailnetProbe
	t.Cleanup(func() { tailnetBackoff, tailnetProbe = prevBackoff, prevProbe })
	tailnetBackoff.first, tailnetBackoff.max = time.Millisecond, 4*time.Millisecond
	tailnetProbe.every, tailnetProbe.timeout, tailnetProbe.fails = time.Millisecond, time.Second, 2
}

func TestProbeTailnetTLSNamesTheHalfThatFailed(t *testing.T) {
	healthy := httptest.NewTLSServer(http.NotFoundHandler())
	t.Cleanup(healthy.Close)
	trusted := healthy.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	trusted.ServerName = "example.com"
	ctx := context.Background()
	if err := probeTailnetTLS(ctx, healthy.Listener.Addr().String(), trusted); err != nil {
		t.Fatalf("a serving listener failed its probe: %v", err)
	}

	// The 2026-10 failure: the socket accepts and every handshake dies after it.
	wedged, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { wedged.Close() })
	go func() {
		for {
			conn, err := wedged.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()
	if err := probeTailnetTLS(ctx, wedged.Addr().String(), trusted); err == nil || !strings.Contains(err.Error(), "TLS handshake failed") {
		t.Fatalf("a listener that drops every handshake read as %v", err)
	}

	gone := wedged.Addr().String()
	wedged.Close()
	if err := probeTailnetTLS(ctx, gone, trusted); err == nil || !strings.Contains(err.Error(), "no TCP connection") {
		t.Fatalf("a closed port read as %v", err)
	}
}

func TestServeTailnetWhenUpBindsAgainWhenHandshakesFail(t *testing.T) {
	fastProbe(t)
	var binds atomic.Int32
	first := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(first.Close)
	second := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(second.Close)
	d := newDaemon(func(string, ...any) {})
	done, finished := make(chan struct{}), make(chan struct{})
	go func() {
		d.serveTailnetWhenUp(done, func() (*tailnetServing, error) {
			if binds.Add(1) == 1 {
				return &tailnetServing{server: first.Config, probe: func(context.Context) error { return errors.New("handshake failed") }}, nil
			}
			return &tailnetServing{server: second.Config, probe: func(context.Context) error { return nil }}, nil
		})
		close(finished)
	}()
	deadline := time.Now().Add(5 * time.Second)
	for binds.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if binds.Load() != 2 {
		t.Fatalf("a listener failing every handshake was bound %d times, want a second", binds.Load())
	}
	if conn, err := net.Dial("tcp", first.Listener.Addr().String()); err == nil {
		conn.Close()
		t.Fatal("the stale listener kept its port after the rebind")
	}
	time.Sleep(20 * time.Millisecond)
	if binds.Load() != 2 {
		t.Fatalf("a healthy listener was rebound, %d binds", binds.Load())
	}
	close(done)
	<-finished
}

func TestServeTailnetWhenUpBindsAgainWhenServeEnds(t *testing.T) {
	fastProbe(t)
	var binds atomic.Int32
	d := newDaemon(func(string, ...any) {})
	ended := make(chan error, 1)
	ended <- errors.New("accept: socket is not connected")
	done, finished := make(chan struct{}), make(chan struct{})
	go func() {
		d.serveTailnetWhenUp(done, func() (*tailnetServing, error) {
			if binds.Add(1) == 1 {
				return &tailnetServing{server: &http.Server{}, ended: ended}, nil
			}
			return &tailnetServing{server: &http.Server{}}, nil
		})
		close(finished)
	}()
	deadline := time.Now().Add(5 * time.Second)
	for binds.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if binds.Load() != 2 {
		t.Fatalf("a Serve that returned was not replaced, %d binds", binds.Load())
	}
	close(done)
	<-finished
}

// tailnetTestCert is a self-signed certificate for the fake tailnet name, so a
// handshake verifies the name the way one against tailscaled's certificate does.
func tailnetTestCert(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: testFQDN},
		DNSNames:              []string{testFQDN},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

// spawnAnswer dials the tailnet handler as a browser would, sends hello and then a spawn
// of a seat that writes marker, and returns the daemon's answer to the spawn.
func spawnAnswer(t *testing.T, base string, client *http.Client, marker string) frame {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ws, _, err := websocket.Dial(ctx, "wss"+strings.TrimPrefix(base, "https")+"/", &websocket.DialOptions{
		HTTPClient: client, HTTPHeader: http.Header{"Origin": {"https://coilyco.dev"}},
	})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer ws.CloseNow()
	ws.SetReadLimit(maxFrame)
	send := func(message frame) {
		raw, _ := json.Marshal(message)
		if err := ws.Write(ctx, websocket.MessageText, raw); err != nil {
			t.Fatalf("write %s: %v", message.Type, err)
		}
	}
	send(frame{Type: "hello", Format: daemonFormat, Version: "test", ID: "h"})
	send(frame{Type: "spawn", ID: "s", Session: "probe-seat", Role: "probe", Argv: []string{"/bin/sh", "-c", "touch " + marker}, Rows: 24, Cols: 80})
	for {
		_, raw, err := ws.Read(ctx)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		var reply frame
		if err := json.Unmarshal(raw, &reply); err == nil && reply.ID == "s" {
			return reply
		}
	}
}

// A peer on this host that dials the node's own tailnet name has a local source address,
// which once made it count as a browser on this host and skip the passkey (COI-3498).
func TestTailnetPeerOnThisHostCannotSpawnWithoutUnlocking(t *testing.T) {
	base, client, _ := tailnetServer(t, func(string) (tailnetPeer, error) { return peerWith("kai@example.com"), nil })
	marker := filepath.Join(t.TempDir(), "ran")
	reply := spawnAnswer(t, base, client, marker)
	if reply.Type != "error" || reply.Reason != reasonPasskeyRequired {
		t.Fatalf("an admitted but locked peer must get passkey_required, got %+v", reply)
	}
	time.Sleep(300 * time.Millisecond)
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("the refused spawn still ran its argv")
	}
}

// Without the policy's remote flag the same peer gets past the lock, so the test above
// fails if that flag is ever dropped.
func TestTailnetPolicyWithoutRemoteLetsALocalPeerPastTheLock(t *testing.T) {
	d := newDaemon(func(string, ...any) {})
	node := tailnetNode{FQDN: testFQDN, IPv4: "127.0.0.1", Owner: "kai@example.com"}
	policy := tailnetPolicy(node, []string{"tag:physical"}, nil, []string{"https://coilyco.dev"}, siblingPages{}, func(string) (tailnetPeer, error) { return peerWith("kai@example.com"), nil })
	policy.remote = false
	server := httptest.NewTLSServer(d.handler(policy))
	t.Cleanup(server.Close)
	_, port, _ := net.SplitHostPort(server.Listener.Addr().String())
	trusted := server.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	trusted.ServerName = "example.com"
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: trusted, DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}}}
	reply := spawnAnswer(t, "https://"+testFQDN+":"+port, client, filepath.Join(t.TempDir(), "ran"))
	if reply.Reason == reasonPasskeyRequired {
		t.Fatalf("a local peer with remote off should not meet the passkey lock, got %+v", reply)
	}
}

// siblingFixture is a tailnet of this node's owner's laptop, a physical tower, a
// guest's laptop and a CI runner, with the names tailscaled holds for them.
func siblingFixture() (siblingPages, func(string) (tailnetPeer, error)) {
	book := map[string]string{
		"laptop.example.ts.net": "100.64.0.7",
		"tower.example.ts.net":  "100.64.0.8",
		"guest.example.ts.net":  "100.64.0.9",
		"runner.example.ts.net": "100.64.0.5",
	}
	peers := map[string]tailnetPeer{
		"127.0.0.1":  peerWith("kai@example.com"),
		"100.64.0.7": peerWith("kai@example.com"),
		"100.64.0.8": peerWith("tagged-devices", "tag:physical"),
		"100.64.0.9": peerWith("guest@example.com"),
		"100.64.0.5": peerWith("tagged-devices", "tag:ci"),
	}
	whois := func(address string) (tailnetPeer, error) {
		if host, _, err := net.SplitHostPort(address); err == nil {
			address = host
		}
		if peer, ok := peers[address]; ok {
			return peer, nil
		}
		return tailnetPeer{}, errors.New("no such node")
	}
	return siblingPages{addresses: func() (map[string]string, error) { return book, nil }, ports: []string{"7419", "7500"}}, whois
}

func TestTailnetWebsocketTakesAPageServedByAnAdmittedSiblingNode(t *testing.T) {
	siblings, whois := siblingFixture()
	base, client, _ := tailnetServerWith(t, siblings, whois)
	address := "wss" + strings.TrimPrefix(base, "https") + "/"
	dial := func(origin string) (*websocket.Conn, *http.Response, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return websocket.Dial(ctx, address, &websocket.DialOptions{HTTPClient: client, HTTPHeader: http.Header{"Origin": {origin}}})
	}
	for _, origin := range []string{
		"https://laptop.example.ts.net:7419", "https://LAPTOP.example.ts.net:7419", "https://laptop.example.ts.net",
		"https://laptop.example.ts.net:443", "https://laptop.example.ts.net:7500", "https://tower.example.ts.net:7419",
	} {
		ws, _, err := dial(origin)
		if err != nil {
			t.Fatalf("a page from an admitted sibling, origin %q, must open a socket: %v", origin, err)
		}
		_ = ws.CloseNow()
	}
	for name, origin := range map[string]string{
		"another tailnet user's node":       "https://guest.example.ts.net:7419",
		"a tagged node without the tag":     "https://runner.example.ts.net:7419",
		"a peer tailscaled does not name":   "https://stranger.example.ts.net:7419",
		"a port that is not a daemon's":     "https://laptop.example.ts.net:8443",
		"a bare address":                    "https://100.64.0.7:7419",
		"a name that only ends like a peer": "https://laptop.example.ts.net.evil.example:7419",
		"a sibling over http":               "http://laptop.example.ts.net:7419",
		"a short name":                      "https://laptop:7419",
	} {
		if ws, response, err := dial(origin); err == nil {
			_ = ws.CloseNow()
			t.Fatalf("%s, origin %q, must be refused", name, origin)
		} else if response == nil || response.StatusCode != http.StatusForbidden {
			t.Fatalf("%s should get 403, got %v", name, response)
		}
	}
}

func TestASiblingNodesSocketStillMeetsThePasskey(t *testing.T) {
	siblings, whois := siblingFixture()
	base, client, _ := tailnetServerWith(t, siblings, whois)
	marker := filepath.Join(t.TempDir(), "ran")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ws, _, err := websocket.Dial(ctx, "wss"+strings.TrimPrefix(base, "https")+"/", &websocket.DialOptions{
		HTTPClient: client, HTTPHeader: http.Header{"Origin": {"https://laptop.example.ts.net:7419"}},
	})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer ws.CloseNow()
	ws.SetReadLimit(maxFrame)
	for _, message := range []frame{
		{Type: "hello", Format: daemonFormat, Version: "test", ID: "h"},
		{Type: "spawn", ID: "s", Session: "probe-seat", Role: "probe", Argv: []string{"/bin/sh", "-c", "touch " + marker}, Rows: 24, Cols: 80},
	} {
		raw, _ := json.Marshal(message)
		if err := ws.Write(ctx, websocket.MessageText, raw); err != nil {
			t.Fatalf("write %s: %v", message.Type, err)
		}
	}
	for {
		_, raw, err := ws.Read(ctx)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		var reply frame
		if json.Unmarshal(raw, &reply) == nil && reply.ID == "s" {
			if reply.Type != "error" || reply.Reason != reasonPasskeyRequired {
				t.Fatalf("a sibling page's socket must still get passkey_required, got %+v", reply)
			}
			break
		}
	}
	time.Sleep(300 * time.Millisecond)
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("the refused spawn still ran its argv")
	}
}

func TestASiblingIsJudgedByTheNameTailscaledHoldsAndNotByAnAddressTheOriginSupplies(t *testing.T) {
	siblings, whois := siblingFixture()
	var asked []string
	judged := func(address string) (tailnetPeer, error) {
		asked = append(asked, address)
		return whois(address)
	}
	policy := tailnetPolicy(tailnetNode{FQDN: testFQDN, IPv4: "100.64.0.1", Owner: "kai@example.com"}, []string{"tag:physical"}, nil, nil, siblings, judged)
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Host = testFQDN + ":7419"
	origin, _ := url.Parse("https://laptop.example.ts.net:7419")
	if !policy.origin(request, origin) || len(asked) != 1 || asked[0] != "100.64.0.7" {
		t.Fatalf("the sibling should be judged at the address tailscaled names for it, asked %v", asked)
	}
	asked = nil
	if bare, _ := url.Parse("https://100.64.0.7:7419"); policy.origin(request, bare) || len(asked) != 0 {
		t.Fatalf("an origin that is an address must be refused without a whois, asked %v", asked)
	}
}

func TestASiblingIsRefusedWhenTailscaledCannotBeRead(t *testing.T) {
	siblings, whois := siblingFixture()
	siblings.addresses = func() (map[string]string, error) { return nil, errors.New("the backend is down") }
	policy := tailnetPolicy(tailnetNode{FQDN: testFQDN, IPv4: "100.64.0.1", Owner: "kai@example.com"}, []string{"tag:physical"}, nil, []string{"https://coilyco.dev"}, siblings, whois)
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Host = testFQDN + ":7419"
	origin, _ := url.Parse("https://laptop.example.ts.net:7419")
	if policy.origin(request, origin) {
		t.Fatal("an unreadable tailnet must admit no sibling")
	}
	if !policy.origin(request, &url.URL{Scheme: "https", Host: "coilyco.dev"}) {
		t.Fatal("the configured page must still be admitted when the tailnet cannot be read")
	}
}

func TestParseTailnetAddressesKeepsPeerNamesAndTheirIPv4(t *testing.T) {
	raw := []byte(`{"Peer":{"a":{"DNSName":"Laptop.example.ts.net.","TailscaleIPs":["fd7a:115c::7","100.64.0.7"]},"b":{"DNSName":"","TailscaleIPs":["100.64.0.8"]},"c":{"DNSName":"v6only.example.ts.net.","TailscaleIPs":["fd7a:115c::9"]}}}`)
	book, err := parseTailnetAddresses(raw)
	if err != nil || len(book) != 1 || book["laptop.example.ts.net"] != "100.64.0.7" {
		t.Fatalf("want only the laptop at its IPv4, got %v (%v)", book, err)
	}
	if _, err := parseTailnetAddresses([]byte("not json")); err == nil {
		t.Fatal("a malformed status must be an error")
	}
}

func TestAddressBookKeepsAnAnswerBrieflyAndNeverAFailure(t *testing.T) {
	var reads int
	fail := true
	book := &addressBook{read: func() (map[string]string, error) {
		reads++
		if fail {
			return nil, errors.New("down")
		}
		return map[string]string{"laptop.example.ts.net": "100.64.0.7"}, nil
	}}
	if _, err := book.get(); err == nil {
		t.Fatal("a failed read must surface")
	}
	fail = false
	for range 3 {
		if got, err := book.get(); err != nil || got["laptop.example.ts.net"] != "100.64.0.7" {
			t.Fatalf("book %v (%v)", got, err)
		}
	}
	if reads != 2 {
		t.Fatalf("a failure is retried and a success reused, got %d reads", reads)
	}
}
