package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestRequireLoopbackRefusesAnyOtherAddress(t *testing.T) {
	for _, address := range []string{"127.0.0.1:7419", "[::1]:7419"} {
		if err := requireLoopback(address); err != nil {
			t.Fatalf("%s is loopback: %v", address, err)
		}
	}
	for _, address := range []string{"0.0.0.0:7419", ":7419", "100.64.0.1:7419", "localhost:7419"} {
		if requireLoopback(address) == nil {
			t.Fatalf("%s must be refused until tailnet auth exists", address)
		}
	}
}

// wsDaemon serves the handler alone, on a loopback test server, with a roster
// from the fixture rather than a live agent-compose.
func wsDaemon(t *testing.T) (*daemon, string) {
	t.Helper()
	d := newDaemon(func(string, ...any) {})
	d.holdDir = testHoldDir(t)
	d.roster = func(context.Context) (listedRoster, error) {
		document, err := parseRoster(fixture(t, "roster.json"))
		return listRoster(document), err
	}
	server := httptest.NewServer(d.websocketHandler())
	t.Cleanup(server.Close)
	return d, "ws" + strings.TrimPrefix(server.URL, "http")
}

func dialWS(t *testing.T, address, origin string) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	header := http.Header{}
	if origin != "" {
		header.Set("Origin", origin)
	}
	return websocket.Dial(ctx, address, &websocket.DialOptions{HTTPHeader: header})
}

func TestWebsocketRefusesAnythingButALoopbackBrowser(t *testing.T) {
	_, address := wsDaemon(t)
	for _, origin := range []string{"", "https://evil.example", "http://192.168.1.5:5173"} {
		ws, response, err := dialWS(t, address, origin)
		if err == nil {
			_ = ws.CloseNow()
			t.Fatalf("origin %q must be refused", origin)
		}
		if response == nil || response.StatusCode != http.StatusForbidden {
			t.Fatalf("origin %q should get 403, got %+v", origin, response)
		}
	}
}

func TestWebsocketRefusesAReboundHost(t *testing.T) {
	d := newDaemon(func(string, ...any) {})
	request := httptest.NewRequest(http.MethodGet, "http://attacker.example:7419/", nil)
	request.Header.Set("Origin", "http://localhost:5173")
	recorder := httptest.NewRecorder()
	d.websocketHandler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("a Host that is not loopback is DNS rebinding, got %d", recorder.Code)
	}
}

type wsClient struct {
	t  *testing.T
	ws *websocket.Conn
}

func (c wsClient) send(message frame) {
	c.t.Helper()
	encoded, _ := json.Marshal(message)
	if err := c.ws.Write(context.Background(), websocket.MessageText, encoded); err != nil {
		c.t.Fatalf("write: %v", err)
	}
}

// next reads until a frame of the wanted type, keeping output it passes.
func (c wsClient) next(want string, output *strings.Builder) frame {
	c.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for {
		_, data, err := c.ws.Read(ctx)
		if err != nil {
			c.t.Fatalf("read waiting for %s: %v (output %q)", want, err, output)
		}
		var message frame
		if err := json.Unmarshal(data, &message); err != nil {
			c.t.Fatalf("frame: %v", err)
		}
		if message.Type == "output" && output != nil {
			output.Write(message.Data)
		}
		if message.Type == want {
			return message
		}
		if message.Type == "error" {
			c.t.Fatalf("waiting for %s: %s", want, message.Error)
		}
	}
}

func (c wsClient) nextAny() frame {
	c.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, data, err := c.ws.Read(ctx)
	if err != nil {
		c.t.Fatalf("read: %v", err)
	}
	var message frame
	_ = json.Unmarshal(data, &message)
	return message
}

func TestWebsocketCarriesTheSameFramesAndAnswersTheRoster(t *testing.T) {
	d, address := wsDaemon(t)
	ws, _, err := dialWS(t, address, "http://localhost:5173")
	if err != nil {
		t.Fatalf("a loopback browser origin must connect: %v", err)
	}
	t.Cleanup(func() { _ = ws.CloseNow() })
	client := wsClient{t, ws}
	client.send(frame{Type: "hello", Format: daemonFormat})
	if welcome := client.next("welcome", nil); welcome.Format != daemonFormat {
		t.Fatalf("welcome = %+v", welcome)
	}
	var launched []string
	d.launch = func(role, seat string) error { launched = append(launched, role+"/"+seat); return nil }
	client.send(frame{Type: "launch", ID: "l", Role: "scientist", Seat: "codex"})
	if reply := client.next("launched", nil); reply.Role != "scientist" || len(launched) != 1 || launched[0] != "scientist/codex" {
		t.Fatalf("launch should run the role's launch once: %+v, %v", reply, launched)
	}
	client.send(frame{Type: "launch", ID: "bad", Role: "../etc"})
	if reply := client.nextAny(); reply.Type != "error" || reply.Code != exitUsage {
		t.Fatalf("an unsafe role slug must be refused with exit 2: %+v", reply)
	}
	client.send(frame{Type: "roster", ID: "r"})
	roster := client.next("roster", nil)
	if roster.Roster == nil || roster.Roster.Format != rosterFormat || len(roster.Roster.Roles) == 0 {
		t.Fatalf("roster should be %s with roles: %+v", rosterFormat, roster.Roster)
	}
	// A session another connection started, as a kitty window's would be.
	s, err := startPTYSession(d, "scientist-evie", frame{
		Role: "scientist", Identity: "Evie", Seat: "codex",
		Argv: []string{"/bin/sh", "-c", "echo READY; cat"}, Env: os.Environ(), Cwd: "/",
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	d.mu.Lock()
	d.sessions[s.name] = s
	d.mu.Unlock()
	t.Cleanup(s.end)
	client.send(frame{Type: "attach", ID: "a", Session: "scientist-evie", Replay: true, Rows: 24, Cols: 80})
	var output strings.Builder
	client.next("attached", &output)
	// The browser is a person at a keyboard, so it may type into a session it
	// did not start.
	client.send(frame{Type: "input", Session: "scientist-evie", Data: []byte("typed in a browser\r")})
	deadline := time.Now().Add(10 * time.Second)
	for !strings.Contains(output.String(), "typed in a browser") && time.Now().Before(deadline) {
		client.next("output", &output)
	}
	if !strings.Contains(output.String(), "typed in a browser") {
		t.Fatalf("browser input never reached the session: %q", output.String())
	}
}

func TestIsLoopbackHost(t *testing.T) {
	for host, want := range map[string]bool{
		"localhost:5173": true, "127.0.0.1": true, "[::1]:80": true,
		"evil.example": false, "10.0.0.1:80": false, net.JoinHostPort("100.64.0.1", "1"): false,
	} {
		if got := isLoopbackHost(host); got != want {
			t.Fatalf("isLoopbackHost(%q) = %v, want %v", host, got, want)
		}
	}
}

// wsSession starts a session the websocket client did not spawn, whose process
// table puts the test's own pid under it when under is set.
func wsSession(t *testing.T, d *daemon, under bool) {
	t.Helper()
	s, err := startPTYSession(d, "scientist-evie", frame{
		Role: "scientist", Identity: "Evie", Seat: "codex",
		Argv: []string{"/bin/sh", "-c", "echo READY; cat -v"}, Env: os.Environ(), Cwd: "/",
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	d.mu.Lock()
	d.sessions[s.name] = s
	d.mu.Unlock()
	t.Cleanup(s.end)
	parent := 1
	if under {
		parent = s.pid
	}
	d.processes = func() ([]processEntry, error) {
		return []processEntry{{PID: s.pid, PPID: 1}, {PID: os.Getpid(), PPID: parent}}, nil
	}
}

func dialLoopbackClient(t *testing.T, address string) (wsClient, frame) {
	t.Helper()
	ws, _, err := dialWS(t, address, "http://localhost:5173")
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = ws.CloseNow() })
	client := wsClient{t, ws}
	client.send(frame{Type: "hello", Format: daemonFormat})
	return client, client.next("welcome", nil)
}

func TestWebsocketBrowserUnderASessionCannotType(t *testing.T) {
	d, address := wsDaemon(t)
	wsSession(t, d, true)
	d.peerLookup = func(net.Addr, net.Addr) ([]int, error) { return []int{os.Getpid()}, nil }
	var launched []string
	d.launch = func(role, seat string) error { launched = append(launched, role); return nil }
	client, welcome := dialLoopbackClient(t, address)
	if welcome.Typing == nil || welcome.Typing.Allowed || welcome.Typing.Reason != reasonSessionDescendant {
		t.Fatalf("welcome should say this peer is read-only: %+v", welcome.Typing)
	}
	if !slices.Contains(welcome.Features, typingGuardFeature) {
		t.Fatalf("welcome should carry %s: %v", typingGuardFeature, welcome.Features)
	}
	client.send(frame{Type: "attach", ID: "a", Session: "scientist-evie", Replay: true, Rows: 24, Cols: 80})
	client.next("attached", nil)
	client.send(frame{Type: "input", ID: "i", Session: "scientist-evie", Data: []byte("forged-by-agent-browser\r")})
	refusal := client.nextAny()
	for refusal.Type != "error" {
		refusal = client.nextAny()
	}
	if refusal.ID != "i" || refusal.Reason != reasonSessionDescendant || refusal.Code != exitFailure {
		t.Fatalf("input should be refused with %s: %+v", reasonSessionDescendant, refusal)
	}
	client.send(frame{Type: "launch", ID: "l", Role: "scientist"})
	for refusal = client.nextAny(); refusal.Type != "error"; refusal = client.nextAny() {
	}
	if refusal.ID != "l" || refusal.Reason != reasonSessionDescendant || len(launched) != 0 {
		t.Fatalf("launch should be refused too: %+v, launched %v", refusal, launched)
	}
	time.Sleep(300 * time.Millisecond)
	if status := d.session("scientist-evie").status(24); strings.Contains(strings.Join(status.Screen, "\n"), "forged-by-agent-browser") {
		t.Fatalf("the refused input reached the session: %q", status.Screen)
	}
}

func TestWebsocketBrowserOutsideEverySessionStillTypes(t *testing.T) {
	d, address := wsDaemon(t)
	wsSession(t, d, false)
	d.peerLookup = func(net.Addr, net.Addr) ([]int, error) { return []int{os.Getpid()}, nil }
	client, welcome := dialLoopbackClient(t, address)
	if welcome.Typing == nil || !welcome.Typing.Allowed {
		t.Fatalf("a browser outside every session may type: %+v", welcome.Typing)
	}
	client.send(frame{Type: "attach", ID: "a", Session: "scientist-evie", Replay: true, Rows: 24, Cols: 80})
	var output strings.Builder
	client.next("attached", &output)
	client.send(frame{Type: "input", Session: "scientist-evie", Data: []byte("typed-by-kai\r")})
	deadline := time.Now().Add(10 * time.Second)
	for !strings.Contains(output.String(), "typed-by-kai") && time.Now().Before(deadline) {
		client.next("output", &output)
	}
	if !strings.Contains(output.String(), "typed-by-kai") {
		t.Fatalf("a browser with no session ancestry never typed: %q", output.String())
	}
}

func TestWebsocketBrowserTheDaemonCannotNameCannotType(t *testing.T) {
	d, address := wsDaemon(t)
	wsSession(t, d, false)
	d.peerLookup = func(net.Addr, net.Addr) ([]int, error) { return nil, errors.New("lsof is not installed") }
	client, welcome := dialLoopbackClient(t, address)
	if welcome.Typing == nil || welcome.Typing.Allowed || welcome.Typing.Reason != reasonPeerUnread {
		t.Fatalf("an unnamed peer fails closed: %+v", welcome.Typing)
	}
	client.send(frame{Type: "attach", ID: "a", Session: "scientist-evie", Rows: 24, Cols: 80})
	client.next("attached", nil)
	client.send(frame{Type: "input", ID: "i", Session: "scientist-evie", Data: []byte("x")})
	refusal := client.nextAny()
	for refusal.Type != "error" {
		refusal = client.nextAny()
	}
	if refusal.Reason != reasonPeerUnread {
		t.Fatalf("want %s: %+v", reasonPeerUnread, refusal)
	}
}

func TestClientDescendsFromTreatsARemoteDeviceAsHavingNoPid(t *testing.T) {
	d := newDaemon(func(string, ...any) {})
	d.processes = func() ([]processEntry, error) { return []processEntry{{PID: 100, PPID: 1}}, nil }
	roots := map[int]bool{100: true}
	for name, testCase := range map[string]struct {
		peer peerStanding
		want bool
	}{
		"remote device":   {peerStanding{web: true, remote: true}, false},
		"unnamed local":   {peerStanding{web: true}, true},
		"one pid outside": {peerStanding{pids: []int{200, 100}}, true},
		"all outside":     {peerStanding{pids: []int{200, 300}}, false},
	} {
		if got := d.clientDescendsFrom(&client{peerStanding: testCase.peer}, roots); got != testCase.want {
			t.Fatalf("%s = %v, want %v", name, got, testCase.want)
		}
	}
}

func TestTCPPeerPIDsNamesThisProcessForItsOwnSocket(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		c, _ := listener.Accept()
		accepted <- c
	}()
	client, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	server := <-accepted
	defer server.Close()
	pids, err := tcpPeerPIDs(server.RemoteAddr(), server.LocalAddr())
	if err != nil {
		t.Skipf("this host cannot read its socket table: %v", err)
	}
	if !slices.Contains(pids, os.Getpid()) {
		t.Fatalf("the dialing side is this process %d, got %v", os.Getpid(), pids)
	}
	if !onThisHost(server.RemoteAddr()) {
		t.Fatal("a loopback peer is on this host")
	}
	if onThisHost(&net.TCPAddr{IP: net.ParseIP("203.0.113.9"), Port: 1}) {
		t.Fatal("a documentation address is another host")
	}
}

func TestParseLsofOwnersMatchesBothPorts(t *testing.T) {
	output := "p100\nf12\nn127.0.0.1:54321->127.0.0.1:7419\np200\nn127.0.0.1:7419->127.0.0.1:54321\np300\nn[::1]:54321->[::1]:7419\np400\nn127.0.0.1:54322->127.0.0.1:7419\n"
	if got := parseLsofOwners(output, 54321, 7419); !slices.Equal(got, []int{100, 300}) {
		t.Fatalf("owners = %v, want the clients 100 and 300, never the daemon's end 200", got)
	}
}
