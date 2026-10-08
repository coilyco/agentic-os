package main

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// shellDaemon serves a unix-socket daemon whose login shell is /bin/sh, so a
// test does not run the host user's profile.
func shellDaemon(t *testing.T) *testClient {
	t.Helper()
	t.Setenv("SHELL", "/bin/sh")
	testDaemon(t)
	return dialTest(t)
}

func (tc *testClient) spawnTerminal(cwd string) frame {
	tc.t.Helper()
	reply, err := tc.c.request(frame{Type: "spawn", Kind: kindTerminal, Cwd: cwd, Rows: 24, Cols: 120})
	if err != nil {
		tc.t.Fatalf("spawn terminal: %v", err)
	}
	return reply
}

// frameOfType reads frames, keeping output, until one of the wanted type.
func (tc *testClient) frameOfType(want string) frame {
	tc.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		_ = tc.c.raw.SetReadDeadline(deadline)
		message, err := tc.c.read()
		if err != nil {
			tc.t.Fatalf("waiting for %s: %v (output %q)", want, err, tc.output.String())
		}
		if message.Type == "output" {
			tc.output.Write(message.Data)
		}
		if message.Type == want {
			return message
		}
	}
}

func (tc *testClient) list() frame {
	tc.t.Helper()
	reply, err := tc.c.request(frame{Type: "list"})
	if err != nil {
		tc.t.Fatalf("list: %v", err)
	}
	return reply
}

func (tc *testClient) type_(session, text string) {
	tc.t.Helper()
	if err := tc.c.write(frame{Type: "input", Session: session, Data: []byte(text)}); err != nil {
		tc.t.Fatalf("input: %v", err)
	}
}

func TestTerminalRunsTheLoginShellInTheCwdWithoutASeatToken(t *testing.T) {
	tc := shellDaemon(t)
	if !slices.Contains(tc.c.features, terminalsFeature) {
		t.Fatalf("welcome should offer %q: %v", terminalsFeature, tc.c.features)
	}
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	spawned := tc.spawnTerminal(dir)
	if spawned.Kind != kindTerminal || !strings.HasPrefix(spawned.Session, terminalName) || spawned.PID == 0 {
		t.Fatalf("spawned = %+v", spawned)
	}
	// Arithmetic, so the typed command's echo cannot satisfy the match.
	tc.type_(spawned.Session, "echo ok-$((6*7)) tok=${ATERM_SESSION_TOKEN:-none} $0; pwd\r")
	tc.until("ok-42 tok=none")
	tc.until(dir)
}

func TestTerminalIsListedApartFromSeatsAndIsNeverATarget(t *testing.T) {
	tc := shellDaemon(t)
	seat := dialTest(t)
	seat.spawn("eng-platform-beetle-ox", "eng-platform", "Beetle-Ox", `sleep 60`)
	spawned := tc.spawnTerminal("")
	listing := tc.list()
	if len(listing.Sessions) != 1 || listing.Sessions[0].Name != "eng-platform-beetle-ox" {
		t.Fatalf("sessions should hold the seat alone: %+v", listing.Sessions)
	}
	if len(listing.Terminals) != 1 || listing.Terminals[0].Name != spawned.Session || listing.Terminals[0].PID != spawned.PID {
		t.Fatalf("terminals should hold the shell alone: %+v", listing.Terminals)
	}
	if listing.Terminals[0].Cwd == "" || listing.Terminals[0].Started.IsZero() {
		t.Fatalf("a terminal reports where and when it started: %+v", listing.Terminals[0])
	}
	// Every way of addressing a seat refuses the shell's name.
	for _, verb := range []frame{
		{Type: "status", Target: spawned.Session},
		{Type: "clear", Target: spawned.Session},
		{Type: "send", Target: spawned.Session, Body: "hi", Token: "x"},
	} {
		other := dialTest(t)
		if reply, err := other.c.request(verb); err == nil {
			t.Fatalf("%s should not reach a terminal: %+v", verb.Type, reply)
		}
	}
	missing := dialTest(t)
	_, err := missing.c.request(frame{Type: "status", Target: spawned.Session})
	_, live, _ := strings.Cut(fmt.Sprint(err), "Live:")
	if err == nil || !strings.Contains(err.Error(), "no live session answers") || strings.Contains(live, "terminal-") {
		t.Fatalf("the refusal should be the seat one and list no terminal: %v", err)
	}
	// A shell holds no token, so nothing inside it can speak as a seat.
	tc.type_(spawned.Session, "echo TOKEN=${ATERM_SESSION_TOKEN:-none}\r")
	tc.until("TOKEN=none")
}

func TestTerminalEndsWhenTheShellExitsAndOnClose(t *testing.T) {
	tc := shellDaemon(t)
	first := tc.spawnTerminal("")
	tc.type_(first.Session, "exit 3\r")
	if exit := tc.frameOfType("exit"); exit.Code != 3 || exit.Session != first.Session {
		t.Fatalf("exit = %+v", exit)
	}
	deadline := time.Now().Add(5 * time.Second)
	for len(tc.list().Terminals) != 0 {
		if time.Now().After(deadline) {
			t.Fatalf("an exited shell stays listed: %+v", tc.list().Terminals)
		}
		time.Sleep(50 * time.Millisecond)
	}
	other := dialTest(t)
	if reply, err := other.c.request(frame{Type: "attach", Session: first.Session}); err != nil || reply.Type != "exited" || reply.Code != 3 {
		t.Fatalf("an ended terminal answers its code: %+v, %v", reply, err)
	}

	second := tc.spawnTerminal("")
	tc.type_(second.Session, "sleep 60\r")
	closer := dialTest(t)
	closed, err := closer.c.request(frame{Type: "close", Target: second.Session})
	if err != nil || closed.Type != "closed" {
		t.Fatalf("close = %+v, %v", closed, err)
	}
	if left := closer.list().Terminals; len(left) != 0 {
		t.Fatalf("a closed terminal stays listed: %+v", left)
	}
}

func TestTerminalSpawnRefusesWhatOnlyASeatTakes(t *testing.T) {
	tc := shellDaemon(t)
	for name, spawn := range map[string]frame{
		"argv":     {Argv: []string{"/bin/sh"}},
		"role":     {Role: "scientist"},
		"identity": {Identity: "Evie"},
		"seat":     {Seat: "claude"},
		"env":      {Env: []string{"A=1"}},
		"session":  {Session: "scientist-evie"},
		"cwd":      {Cwd: "/no/such/directory"},
		"relative": {Cwd: "tmp"},
	} {
		spawn.Type, spawn.Kind = "spawn", kindTerminal
		if _, err := tc.c.request(spawn); err == nil {
			t.Fatalf("%s should refuse a terminal spawn", name)
		}
	}
	if _, err := tc.c.request(frame{Type: "spawn", Kind: "kiosk", Argv: []string{"/bin/sh"}}); err == nil {
		t.Fatal("an unknown kind should be refused rather than started as a seat")
	}
	if listing := tc.list(); len(listing.Terminals) != 0 || len(listing.Sessions) != 0 {
		t.Fatalf("a refused spawn leaves nothing behind: %+v", listing)
	}
}

func TestTerminalSpawnFromARemoteDeviceWaitsOnGating(t *testing.T) {
	d := newDaemon(func(string, ...any) {})
	d.holdDir = testHoldDir(t)
	err := d.handle(&client{peerStanding: peerStanding{remote: true}, owned: map[string]bool{}, attached: map[string]*ptySession{}}, frame{Type: "spawn", Kind: kindTerminal})
	if reasonFor(err) != reasonRemoteTerminal {
		t.Fatalf("a remote device gets %q, got %v", reasonRemoteTerminal, err)
	}
	if len(d.terminals) != 0 {
		t.Fatal("nothing starts for a refused remote spawn")
	}
}

func TestTerminalSurvivesADaemonRestart(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")
	first := newDaemon(func(string, ...any) {})
	first.holdDir = testHoldDir(t)
	s, err := first.spawnTerminal(frame{Type: "spawn", Kind: kindTerminal})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	t.Cleanup(s.end)
	first.letGoAll()

	second := newDaemon(func(string, ...any) {})
	second.holdDir = first.holdDir
	second.adoptHolders()
	t.Cleanup(func() {
		for _, adopted := range second.everySession() {
			adopted.end()
		}
	})
	views := second.terminalViews()
	if len(views) != 1 || views[0].Name != s.name || views[0].PID != s.pid {
		t.Fatalf("the adopted shell should keep its name: %+v", views)
	}
	if len(second.views()) != 0 || len(second.sessions) != 0 || len(second.tokens) != 0 {
		t.Fatalf("an adopted shell is never a seat: %+v", second.views())
	}
}

// The acceptance path: a browser's websocket opens a terminal, types into it,
// and reads the answer back, and the roster it subscribes to lists the shell apart.
func TestTerminalOverAWebsocketEchoesWhatIsTyped(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")
	_, address := wsDaemon(t)
	ws, _, err := dialWS(t, address, "http://localhost:5173")
	if err != nil {
		t.Fatalf("a loopback browser origin must connect: %v", err)
	}
	t.Cleanup(func() { _ = ws.CloseNow() })
	client := wsClient{t, ws}
	client.send(frame{Type: "hello", Format: daemonFormat})
	client.next("welcome", nil)
	client.send(frame{Type: "subscribe", ID: "sub", Channel: "sessions"})
	if roster := client.next("sessions", nil); len(roster.Terminals) != 0 {
		t.Fatalf("no terminal yet: %+v", roster.Terminals)
	}
	client.send(frame{Type: "spawn", ID: "s", Kind: kindTerminal, Rows: 24, Cols: 80})
	var output strings.Builder
	spawned := client.next("spawned", &output)
	t.Cleanup(func() {
		client.send(frame{Type: "close", ID: "c", Target: spawned.Session})
		client.next("closed", nil)
	})
	client.send(frame{Type: "input", Session: spawned.Session, Data: []byte("echo ok-$((6*7))\r")})
	deadline := time.Now().Add(10 * time.Second)
	for !strings.Contains(output.String(), "ok-42") && time.Now().Before(deadline) {
		client.next("output", &output)
	}
	if !strings.Contains(output.String(), "ok-42") {
		t.Fatalf("the shell never answered: %q", output.String())
	}
	client.send(frame{Type: "list", ID: "l"})
	listing := client.next("sessions", nil)
	for listing.ID != "l" {
		listing = client.next("sessions", nil)
	}
	if len(listing.Sessions) != 0 || len(listing.Terminals) != 1 || listing.Terminals[0].Name != spawned.Session {
		t.Fatalf("listing = %+v", listing)
	}
}
