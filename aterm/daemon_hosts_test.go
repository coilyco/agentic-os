package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const peerStatus = `{"BackendState":"Running","Peer":{
 "a":{"DNSName":"tower.example.ts.net.","OS":"linux","Online":true},
 "b":{"DNSName":"phone.example.ts.net.","OS":"iOS","Online":true},
 "c":{"DNSName":"laptop.example.ts.net.","OS":"macOS","Online":true},
 "d":{"DNSName":"asleep.example.ts.net.","OS":"macOS","Online":false},
 "e":{"DNSName":"win.example.ts.net.","OS":"windows","Online":true},
 "f":{"DNSName":"","OS":"linux","Online":true}
}}`

func TestParseTailnetPeersKeepsOnlyOnlineHostsThatCanRunADaemon(t *testing.T) {
	peers, err := parseTailnetPeers([]byte(peerStatus))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, peer := range peers {
		names = append(names, peer.FQDN)
	}
	if want := []string{"laptop.example.ts.net", "tower.example.ts.net"}; !slices.Equal(names, want) {
		t.Fatalf("peers = %v, want %v", names, want)
	}
	if _, err := parseTailnetPeers([]byte(`{"BackendState":"Stopped"}`)); err == nil {
		t.Fatal("a Stopped backend read as a tailnet with no peers")
	}
}

// fakeDiscovery answers from a fixed tailnet, and counts and records dials.
func fakeDiscovery(dialed *atomic.Int32, answers map[string]error) *hostDiscovery {
	return &hostDiscovery{
		peers: func() (string, []tailnetPeerNode, error) {
			return testFQDN, []tailnetPeerNode{
				{FQDN: testFQDN},
				{FQDN: "tower.example.ts.net"},
				{FQDN: "laptop.example.ts.net"},
				{FQDN: "kube.example.ts.net"},
			}, nil
		},
		probe: func(_ context.Context, _, host, _ string) (string, error) {
			dialed.Add(1)
			if err, ok := answers[host]; !ok || err != nil {
				return "", errors.Join(errors.New("no daemon"), err)
			}
			return "1.2.3", nil
		},
		port: func() string { return defaultTailnetPort },
	}
}

func TestFindListsDaemonsThatAnsweredAndNeverThisNode(t *testing.T) {
	var dialed atomic.Int32
	// kube refuses, tower and laptop answer. This node is in the status too.
	discovery := fakeDiscovery(&dialed, map[string]error{
		"tower.example.ts.net": nil, "laptop.example.ts.net": nil, "kube.example.ts.net": errors.New("403"),
	})
	found, err := discovery.find(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []hostView{
		{Name: "laptop", Host: "laptop.example.ts.net", Port: "7419", Version: "1.2.3"},
		{Name: "tower", Host: "tower.example.ts.net", Port: "7419", Version: "1.2.3"},
	}
	if !slices.Equal(found, want) {
		t.Fatalf("found = %+v, want %+v", found, want)
	}
	if dialed.Load() != 3 {
		t.Fatalf("dialed %d peers, want 3 and never this node", dialed.Load())
	}
}

func TestFindReusesAnAnswerForItsTTLAndSweepsAgainAfter(t *testing.T) {
	var dialed atomic.Int32
	discovery := fakeDiscovery(&dialed, map[string]error{"tower.example.ts.net": nil})
	for range 3 {
		if _, err := discovery.find(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if dialed.Load() != 3 {
		t.Fatalf("three requests dialed %d times, want one sweep of 3", dialed.Load())
	}
	discovery.expires = time.Now().Add(-time.Second)
	if _, err := discovery.find(context.Background()); err != nil || dialed.Load() != 6 {
		t.Fatalf("an expired answer should sweep again: %d dials, %v", dialed.Load(), err)
	}
}

func TestFindOffWithoutAPortAndReportsAnUnreadableTailnet(t *testing.T) {
	var dialed atomic.Int32
	off := fakeDiscovery(&dialed, nil)
	off.port = func() string { return "" }
	if found, err := off.find(context.Background()); err != nil || len(found) != 0 || dialed.Load() != 0 {
		t.Fatalf("an empty port should turn discovery off: %v %v %d", found, err, dialed.Load())
	}
	down := fakeDiscovery(&dialed, nil)
	down.peers = func() (string, []tailnetPeerNode, error) { return "", nil, errors.New("backend Stopped") }
	if _, err := down.find(context.Background()); err == nil || !strings.Contains(err.Error(), "Stopped") {
		t.Fatalf("an unreadable tailnet should say why: %v", err)
	}
}

func TestProbeSeesADaemonOnlyWhenItAdmitsThisNode(t *testing.T) {
	admitting, admittingClient, _ := tailnetServer(t, func(string) (tailnetPeer, error) { return peerWith("kai@example.com"), nil })
	admittingURL, _ := url.Parse(admitting)
	got, err := daemonProber(admittingClient)(context.Background(), testFQDN, testFQDN, admittingURL.Port())
	if err != nil || got != version {
		t.Fatalf("an admitting daemon should answer with its version %q: %q, %v", version, got, err)
	}

	refusing, refusingClient, _ := tailnetServer(t, func(string) (tailnetPeer, error) { return peerWith("guest@example.com"), nil })
	refusingURL, _ := url.Parse(refusing)
	if _, err := daemonProber(refusingClient)(context.Background(), testFQDN, testFQDN, refusingURL.Port()); err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("a daemon that refuses this node should read as 403: %v", err)
	}
}

func TestProbeSendsThisNodesPageAsTheOriginAndAPeerThatRefusesItIsAbsent(t *testing.T) {
	base, client, _ := tailnetServer(t, func(string) (tailnetPeer, error) { return peerWith("kai@example.com"), nil })
	parsed, _ := url.Parse(base)
	// The peer admits this device, but this node's page is not one it serves or allows.
	_, err := daemonProber(client)(context.Background(), "other.example.ts.net", testFQDN, parsed.Port())
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("a peer that does not allow this node's page should read as 403: %v", err)
	}
}

func TestProbeIgnoresAServiceThatIsNotADaemonAndAPortNobodyHolds(t *testing.T) {
	plain := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("hello")) }))
	t.Cleanup(plain.Close)
	_, port, _ := net.SplitHostPort(plain.Listener.Addr().String())
	trusted := plain.Client()
	trusted.Transport.(*http.Transport).DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, plain.Listener.Addr().String())
	}
	trusted.Transport.(*http.Transport).TLSClientConfig.InsecureSkipVerify = true
	if _, err := daemonProber(trusted)(context.Background(), testFQDN, testFQDN, port); err == nil {
		t.Fatal("a web server that is not a daemon read as one")
	}

	idle, _ := net.Listen("tcp", "127.0.0.1:0")
	_, idlePort, _ := net.SplitHostPort(idle.Addr().String())
	idle.Close()
	closedClient := &http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, "127.0.0.1:"+idlePort)
	}}}
	if _, err := daemonProber(closedClient)(context.Background(), testFQDN, testFQDN, idlePort); err == nil {
		t.Fatal("a closed port read as a daemon")
	}
}

func TestHostsFrameAnswersOverTheWebsocketAndAnUnreadableTailnetIsNotFatal(t *testing.T) {
	d, address := wsDaemon(t)
	var dialed atomic.Int32
	d.discovery = fakeDiscovery(&dialed, map[string]error{"tower.example.ts.net": nil})
	ws, _, err := dialWS(t, address, "http://localhost:5173")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ws.CloseNow() })
	client := wsClient{t, ws}
	client.send(frame{Type: "hello", Format: daemonFormat})
	client.next("welcome", nil)

	client.send(frame{Type: "hosts", ID: "h1"})
	reply := client.next("hosts", nil)
	if reply.ID != "h1" || len(reply.Hosts) != 1 || reply.Hosts[0].Host != "tower.example.ts.net" {
		t.Fatalf("hosts reply = %+v", reply)
	}

	d.discovery.expires = time.Time{}
	d.discovery.peers = func() (string, []tailnetPeerNode, error) { return "", nil, errors.New("backend Stopped") }
	client.send(frame{Type: "hosts", ID: "h2"})
	failed := client.nextAny()
	if failed.Type != "error" || failed.ID != "h2" || !strings.Contains(failed.Error, "Stopped") {
		t.Fatalf("an unreadable tailnet should be an error frame: %+v", failed)
	}
	client.send(frame{Type: "list", ID: "after"})
	if after := client.next("sessions", nil); after.ID != "after" {
		t.Fatalf("the connection should survive a failed discovery: %+v", after)
	}
}
