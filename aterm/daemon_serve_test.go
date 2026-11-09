package main

import (
	"context"
	"errors"
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

const ownerLogin = "kai@example.com"

// taggedMac is this node as Tailscale reports a tagged host, with no owner.
func taggedMac() (tailnetNode, error) {
	return tailnetNode{FQDN: testFQDN, IPv4: "100.64.0.1"}, nil
}

// serveDaemon serves the loopback handler a Serve front would proxy to, with the
// forwarded policy a daemon builds from its options.
func serveDaemon(t *testing.T, read func() (tailnetNode, error), whois func(string) (tailnetPeer, error)) (*daemon, *httptest.Server) {
	t.Helper()
	d := newDaemon(func(string, ...any) {})
	d.holdDir = testHoldDir(t)
	d.clientDir = t.TempDir()
	if err := os.WriteFile(filepath.Join(d.clientDir, "index.html"), []byte("aterm client"), 0o644); err != nil {
		t.Fatal(err)
	}
	d.local().proxied = forwardedPolicy(&nodeReader{read: read}, []string{"tag:physical"}, []string{ownerLogin}, []string{"https://coilyco.dev"}, whois)
	server := httptest.NewServer(d.websocketHandler())
	t.Cleanup(server.Close)
	return d, server
}

func statusOf(t *testing.T, server *httptest.Server, host string, headers map[string]string) int {
	t.Helper()
	request, _ := http.NewRequest(http.MethodGet, server.URL+"/", nil)
	if host != "" {
		request.Host = host
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	response.Body.Close()
	return response.StatusCode
}

func TestForwardedRequestIsAdmittedByTheIdentityWhoisNamesNeverAsLoopback(t *testing.T) {
	peers := map[string]tailnetPeer{
		"100.64.0.7": peerWith(ownerLogin),
		"100.64.0.8": peerWith("tagged-devices", "tag:kai-tower-3026", "tag:physical"),
		"100.64.0.9": peerWith("guest@example.com"),
		"100.64.0.5": peerWith("tagged-devices", "tag:ci"),
	}
	var whoisCalls atomic.Int32
	_, server := serveDaemon(t, taggedMac, func(address string) (tailnetPeer, error) {
		whoisCalls.Add(1)
		if peer, ok := peers[address]; ok {
			return peer, nil
		}
		return tailnetPeer{}, errors.New("no such node")
	})
	serve := func(peer, login string) map[string]string {
		headers := map[string]string{"X-Forwarded-For": peer, "X-Forwarded-Host": testFQDN}
		if login != "" {
			headers["Tailscale-User-Login"] = login
		}
		return headers
	}
	for _, testCase := range []struct {
		name    string
		host    string
		headers map[string]string
		want    int
	}{
		{"the owner's untagged phone, which a tagged node alone would refuse", testFQDN, serve("100.64.0.7", ownerLogin), http.StatusOK},
		{"the same phone with the login in another case", testFQDN, serve("100.64.0.7", "Kai@Example.com"), http.StatusOK},
		{"a physical tower, which Serve sends no login for", testFQDN, serve("100.64.0.8", ""), http.StatusOK},
		{"another tailnet user", testFQDN, serve("100.64.0.9", "guest@example.com"), http.StatusForbidden},
		{"a CI runner", testFQDN, serve("100.64.0.5", ""), http.StatusForbidden},
		{"a login header that is not the peer's", testFQDN, serve("100.64.0.9", ownerLogin), http.StatusForbidden},
		{"the owner's phone with the header missing", testFQDN, serve("100.64.0.7", ""), http.StatusForbidden},
		{"a tower carrying a login header", testFQDN, serve("100.64.0.8", ownerLogin), http.StatusForbidden},
		{"a peer whois does not know", testFQDN, serve("100.64.0.99", ""), http.StatusForbidden},
		{"the proxy's own loopback address as the peer", testFQDN, serve("127.0.0.1", ownerLogin), http.StatusForbidden},
		{"two peers in one header", testFQDN, serve("100.64.0.7, 100.64.0.8", ownerLogin), http.StatusForbidden},
		{"no peer named at all", testFQDN, map[string]string{"X-Forwarded-Host": testFQDN}, http.StatusForbidden},
		{"a Host that is not this node's name", "127.0.0.1", serve("100.64.0.7", ownerLogin), http.StatusForbidden},
		{"a forwarded host that disagrees", testFQDN, map[string]string{"X-Forwarded-For": "100.64.0.7", "X-Forwarded-Host": "evil.example", "Tailscale-User-Login": ownerLogin}, http.StatusForbidden},
		{"a public Funnel request", testFQDN, map[string]string{"X-Forwarded-For": "203.0.113.4", "X-Forwarded-Host": testFQDN, "Tailscale-Funnel-Request": "?1"}, http.StatusForbidden},
	} {
		if got := statusOf(t, server, testCase.host, testCase.headers); got != testCase.want {
			t.Errorf("%s: status %d, want %d", testCase.name, got, testCase.want)
		}
	}
	if before := whoisCalls.Load(); statusOf(t, server, testFQDN, serve("127.0.0.1", ownerLogin)) != http.StatusForbidden || whoisCalls.Load() != before {
		t.Error("a loopback peer must be refused without asking whois")
	}
	if got := statusOf(t, server, "", nil); got != http.StatusOK {
		t.Errorf("a request through no proxy is still a plain loopback one and is served: %d", got)
	}
}

func TestForwardedRequestIsRefusedWhenTheNodeCannotBeRead(t *testing.T) {
	_, server := serveDaemon(t, func() (tailnetNode, error) { return tailnetNode{}, errors.New("backend is NeedsLogin") },
		func(string) (tailnetPeer, error) { return peerWith(ownerLogin), nil })
	headers := map[string]string{"X-Forwarded-For": "100.64.0.7", "X-Forwarded-Host": testFQDN, "Tailscale-User-Login": ownerLogin}
	if got := statusOf(t, server, testFQDN, headers); got != http.StatusForbidden {
		t.Fatalf("a daemon that cannot name its node admitted a proxied request: %d", got)
	}
}

func TestForwardedRequestIsRefusedWhenNoForwardedPolicyIsSet(t *testing.T) {
	d := newDaemon(func(string, ...any) {})
	server := httptest.NewServer(d.websocketHandler())
	t.Cleanup(server.Close)
	if got := statusOf(t, server, "", map[string]string{"X-Forwarded-For": "100.64.0.7"}); got != http.StatusForbidden {
		t.Fatalf("a proxied request on a daemon with no proxy policy must refuse: %d", got)
	}
}

func TestForwardedMarkersAreAnyProxyHeaderNotOnlyServes(t *testing.T) {
	for header, want := range map[string]bool{
		"X-Forwarded-For": true, "X-Forwarded-Host": true, "X-Forwarded-Proto": true, "Forwarded": true,
		"X-Real-Ip": true, "Tailscale-User-Login": true, "Tailscale-Funnel-Request": true,
		"Origin": false, "Upgrade": false, "User-Agent": false,
	} {
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.Header.Set(header, "x")
		if got := forwarded(request); got != want {
			t.Errorf("%s: forwarded=%v, want %v", header, got, want)
		}
	}
}

func TestForwardedWebsocketIsARemoteDeviceThatNeedsAPasskey(t *testing.T) {
	d, server := serveDaemon(t, taggedMac, func(string) (tailnetPeer, error) { return peerWith(ownerLogin), nil })
	wsSession(t, d, false)
	var walked atomic.Int32
	d.peerLookup = func(net.Addr, net.Addr) ([]int, error) {
		walked.Add(1)
		return []int{os.Getpid()}, nil
	}
	// The proxy's socket is loopback and its pid is outside every session, which
	// is the standing that types with no passkey. A device behind it must not get it.
	_, port, _ := net.SplitHostPort(server.Listener.Addr().String())
	host := testFQDN + ":" + port
	client := &http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ws, _, err := websocket.Dial(ctx, "ws://"+host+"/", &websocket.DialOptions{HTTPClient: client, HTTPHeader: http.Header{
		"Origin":               {"https://" + host},
		"X-Forwarded-For":      {"100.64.0.7"},
		"X-Forwarded-Host":     {host},
		"Tailscale-User-Login": {ownerLogin},
	}})
	if err != nil {
		t.Fatalf("an admitted proxied device should open a socket: %v", err)
	}
	t.Cleanup(func() { _ = ws.CloseNow() })
	device := wsClient{t, ws}
	device.send(frame{Type: "hello", Format: daemonFormat})
	welcome := device.next("welcome", nil)
	if welcome.Typing == nil || welcome.Typing.Allowed || welcome.Typing.Reason != reasonPasskeyRequired {
		t.Fatalf("a device behind a proxy starts locked: %+v", welcome.Typing)
	}
	device.send(frame{Type: "attach", ID: "a", Session: "scientist-evie", Rows: 24, Cols: 80})
	device.next("attached", nil)
	device.send(frame{Type: "input", ID: "i", Session: "scientist-evie", Data: []byte("typed-through-the-proxy\r")})
	refusal := device.nextAny()
	for refusal.Type != "error" {
		refusal = device.nextAny()
	}
	if refusal.Reason != reasonPasskeyRequired {
		t.Fatalf("typing through the proxy must need a passkey: %+v", refusal)
	}
	if walked.Load() != 0 {
		t.Fatalf("the proxy's socket was walked as a local browser %d times", walked.Load())
	}
}

func TestForwardedWebsocketTakesOnlyThePageTheProxyServes(t *testing.T) {
	_, server := serveDaemon(t, taggedMac, func(string) (tailnetPeer, error) { return peerWith(ownerLogin), nil })
	_, port, _ := net.SplitHostPort(server.Listener.Addr().String())
	host := testFQDN + ":" + port
	client := &http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}}}
	dial := func(origin string) (*http.Response, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		ws, response, err := websocket.Dial(ctx, "ws://"+host+"/", &websocket.DialOptions{HTTPClient: client, HTTPHeader: http.Header{
			"Origin": {origin}, "X-Forwarded-For": {"100.64.0.7"}, "X-Forwarded-Host": {host}, "Tailscale-User-Login": {ownerLogin},
		}})
		if err == nil {
			_ = ws.CloseNow()
		}
		return response, err
	}
	for _, origin := range []string{"https://" + host, "https://coilyco.dev"} {
		if _, err := dial(origin); err != nil {
			t.Errorf("origin %q must open a socket: %v", origin, err)
		}
	}
	for _, origin := range []string{"https://evil.example", "http://" + host, "http://localhost:5173"} {
		if response, err := dial(origin); err == nil || response == nil || response.StatusCode != http.StatusForbidden {
			t.Errorf("origin %q must be refused with 403: %v %v", origin, response, err)
		}
	}
}

func TestAdmitPeerTakesAConfiguredOwnerOnATaggedNode(t *testing.T) {
	node, _ := taggedMac()
	owners := admittedOwners(node, []string{ownerLogin})
	allow := []string{"tag:physical"}
	for _, testCase := range []struct {
		name  string
		peer  tailnetPeer
		owner []string
		admit bool
	}{
		{"the owner's phone", peerWith(ownerLogin), owners, true},
		{"the owner's phone in another case", peerWith("KAI@example.com"), owners, true},
		{"another user", peerWith("guest@example.com"), owners, false},
		{"a tagged device claiming the owner's login", peerWith(ownerLogin, "tag:proxy"), owners, false},
		{"the owner's phone with none configured", peerWith(ownerLogin), admittedOwners(node, nil), false},
	} {
		if err := admitPeer(testCase.peer, testCase.owner, allow); (err == nil) != testCase.admit {
			t.Errorf("%s: admitted=%v, want %v (%v)", testCase.name, err == nil, testCase.admit, err)
		}
	}
}

func TestTailnetListenerAdmitsAConfiguredOwnersPhoneOnATaggedNode(t *testing.T) {
	node, _ := taggedMac()
	policy := tailnetPolicy(node, []string{"tag:physical"}, []string{ownerLogin}, nil, func(string) (tailnetPeer, error) { return peerWith(ownerLogin), nil })
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Host, request.RemoteAddr = testFQDN+":7419", "100.64.0.7:50000"
	if err := policy.admit(request); err != nil {
		t.Fatalf("the owner's phone was refused on the direct listener: %v", err)
	}
	strict := tailnetPolicy(node, []string{"tag:physical"}, nil, nil, func(string) (tailnetPeer, error) { return peerWith(ownerLogin), nil })
	if err := strict.admit(request); err == nil {
		t.Fatal("with no owner configured a tagged node must keep refusing untagged devices")
	}
}

func TestExecutableGoneSeesTheOldVersionRemovedBehindARepointedSymlink(t *testing.T) {
	dir := t.TempDir()
	for _, version := range []string{"0.481.0", "0.491.0"} {
		if err := os.MkdirAll(filepath.Join(dir, "Cellar", version, "bin"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "Cellar", version, "bin", "aterm"), []byte(version), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(dir, "aterm")
	if err := os.Symlink(filepath.Join(dir, "Cellar", "0.481.0", "bin", "aterm"), link); err != nil {
		t.Fatal(err)
	}
	resolvedAtStart := resolveExecutableAt(link)
	if fileGone(resolvedAtStart) {
		t.Fatal("the version the daemon runs exists, so it is not gone")
	}
	// brew upgrade: the new version is linked and the old one removed.
	if err := os.RemoveAll(filepath.Join(dir, "Cellar", "0.481.0")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "Cellar", "0.491.0", "bin", "aterm"), link); err != nil {
		t.Fatal(err)
	}
	if fileGone(link) {
		t.Fatal("the launch path now resolves to the new version, which is why checking it never fired")
	}
	if !fileGone(resolvedAtStart) {
		t.Fatalf("the removed version %s was not seen as gone", strings.TrimPrefix(resolvedAtStart, dir))
	}
}
