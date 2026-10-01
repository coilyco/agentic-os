package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
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
