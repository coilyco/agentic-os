package main

import (
	"bytes"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// holdRig is a daemon the test runs as its own process, so it can be killed
// the way a crash or an upgrade kills one.
type holdRig struct {
	t      *testing.T
	dir    string
	socket string
	daemon *exec.Cmd
	log    *lockedBuffer
	// websocket is the address the daemon serves its HTTP surface on, none when empty.
	websocket string
}

// lockedBuffer is the daemon's log, written by exec's copier while the test reads it.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func newHoldRig(t *testing.T) *holdRig {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "aterm-hold-rig-")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	rig := &holdRig{t: t, dir: dir, socket: filepath.Join(dir, "d.sock")}
	t.Setenv(daemonSocketEnv, rig.socket)
	t.Cleanup(func() {
		rig.killDaemon(syscall.SIGKILL)
		rig.killServing()
		// A session this test left running would outlive it, so end each one.
		sockets, _ := filepath.Glob(filepath.Join(dir, "hold", "*.sock"))
		for _, socket := range sockets {
			rig.endHolder(socket)
		}
		_ = os.RemoveAll(dir)
	})
	return rig
}

// endHolder reads the child pid from a holder and ends its group, then lets
// the holder go.
func (r *holdRig) endHolder(socket string) {
	raw, err := net.DialTimeout("unix", socket, time.Second)
	if err != nil {
		return
	}
	c := newConn(raw)
	defer c.Close()
	_ = c.write(frame{Type: "hello", Format: holdFormat})
	if _, err := c.read(); err != nil {
		return
	}
	_ = c.write(frame{Type: "attach"})
	if reply, err := c.read(); err == nil && reply.Hold != nil {
		_ = syscall.Kill(-reply.Hold.PID, syscall.SIGKILL)
	}
	_ = c.write(frame{Type: "release"})
}

// killServing ends whatever daemon answers on the rig's socket, one a client
// started and the rig does not hold.
func (r *holdRig) killServing() {
	raw, err := net.DialTimeout("unix", r.socket, time.Second)
	if err != nil {
		return
	}
	c := newConn(raw)
	defer c.Close()
	_ = c.write(frame{Type: "hello", Format: daemonFormat})
	if welcome, err := c.read(); err == nil && welcome.PID > 0 {
		_ = syscall.Kill(welcome.PID, syscall.SIGKILL)
	}
}

func (r *holdRig) startDaemon() {
	r.t.Helper()
	r.log = &lockedBuffer{}
	command := exec.Command(os.Args[0], "daemon", "--socket", r.socket, "--websocket", r.websocket, "--tailnet-port", "", "--idle", "1h")
	command.Stderr = r.log
	command.Stdout = r.log
	if err := command.Start(); err != nil {
		r.t.Fatalf("start daemon: %v", err)
	}
	r.daemon = command
	for waited := 0; waited < 200; waited++ {
		if raw, err := net.Dial("unix", r.socket); err == nil {
			_ = raw.Close()
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	r.t.Fatalf("daemon never answered:\n%s", r.log.String())
}

func (r *holdRig) killDaemon(signal syscall.Signal) {
	if r.daemon == nil || r.daemon.Process == nil {
		return
	}
	_ = r.daemon.Process.Signal(signal)
	_, _ = r.daemon.Process.Wait()
	r.daemon = nil
}

func (r *holdRig) sessions() []sessionView {
	r.t.Helper()
	c, err := dialDaemon(false)
	if err != nil {
		r.t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	reply, err := c.request(frame{Type: "list"})
	if err != nil {
		r.t.Fatalf("list: %v", err)
	}
	return reply.Sessions
}

// sessionsQuiet lists sessions, or nothing while no daemon answers.
func (r *holdRig) sessionsQuiet() []sessionView {
	c, err := dialDaemon(false)
	if err != nil {
		return nil
	}
	defer c.Close()
	reply, err := c.request(frame{Type: "list"})
	if err != nil {
		return nil
	}
	return reply.Sessions
}

func alive(pid int) bool { return syscall.Kill(pid, 0) == nil }

func waitFor(t *testing.T, what string, within time.Duration, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(within)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("never saw %s", what)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

// A daemon that dies, by a crash or by being asked, leaves the session running,
// and the next one adopts it with its history, token, and a terminal that types.
func TestSessionOutlivesItsDaemonAndTheNextOneAdoptsIt(t *testing.T) {
	for _, how := range []struct {
		name   string
		signal syscall.Signal
	}{{"SIGKILL", syscall.SIGKILL}, {"SIGTERM", syscall.SIGTERM}} {
		t.Run(how.name, func(t *testing.T) {
			rig := newHoldRig(t)
			rig.startDaemon()
			first := dialTest(t)
			first.spawn("eng-platform-test", "eng-platform", "Beetle-Ox", "echo TOKEN=$ATERM_SESSION_TOKEN; echo READY; exec cat")
			first.until("READY")
			views := rig.sessions()
			if len(views) != 1 {
				t.Fatalf("one session should be live: %+v", views)
			}
			pid := views[0].PID
			if !alive(pid) {
				t.Fatalf("session pid %d is not running", pid)
			}
			token := first.token()

			rig.killDaemon(how.signal)
			time.Sleep(300 * time.Millisecond)
			if !alive(pid) {
				t.Fatalf("session pid %d ended with its daemon (%s)", pid, how.name)
			}

			rig.startDaemon()
			views = rig.sessions()
			if len(views) != 1 || views[0].Name != "eng-platform-test" || views[0].PID != pid {
				t.Fatalf("the new daemon should adopt the same session and pid %d: %+v\n%s", pid, views, rig.log.String())
			}
			again := dialTest(t)
			if _, err := again.c.request(frame{Type: "attach", Session: "eng-platform-test", Replay: true, Rows: 24, Cols: 80}); err != nil {
				t.Fatalf("attach: %v", err)
			}
			again.until("READY")
			if err := again.c.write(frame{Type: "input", Session: "eng-platform-test", Data: []byte("still here\n")}); err != nil {
				t.Fatalf("input: %v", err)
			}
			again.until("still here")
			who, err := again.c.request(frame{Type: "whoami", Token: token})
			if err != nil || len(who.Sessions) != 1 || who.Sessions[0].Name != "eng-platform-test" {
				t.Fatalf("the token should still name the session: %+v %v", who, err)
			}
		})
	}
}

// The session ends while no daemon is looking. The holder keeps its exit code
// for the next daemon, which reports it and lets the holder go.
func TestSessionThatEndedWhileTheDaemonWasAwayIsReportedAndCleared(t *testing.T) {
	rig := newHoldRig(t)
	rig.startDaemon()
	first := dialTest(t)
	first.spawn("scientist-test", "scientist", "Evie", "sleep 1; exit 7")
	pid := rig.sessions()[0].PID
	rig.killDaemon(syscall.SIGKILL)
	waitFor(t, "the session to end", 10*time.Second, func() bool { return !alive(pid) })

	rig.startDaemon()
	if views := rig.sessions(); len(views) != 0 {
		t.Fatalf("an ended session should not list: %+v", views)
	}
	if !strings.Contains(rig.log.String(), "session scientist-test") || !strings.Contains(rig.log.String(), "exited 7") {
		t.Fatalf("the daemon should log the exit code it collected:\n%s", rig.log.String())
	}
	waitFor(t, "the holder to go", 10*time.Second, func() bool {
		left, _ := filepath.Glob(filepath.Join(rig.dir, "hold", "*.sock"))
		return len(left) == 0
	})
}

// A socket nobody listens on is a holder that already ended.
func TestDaemonDropsAHolderSocketNobodyAnswers(t *testing.T) {
	rig := newHoldRig(t)
	holdDir := filepath.Join(rig.dir, "hold")
	if err := os.MkdirAll(holdDir, 0o700); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(holdDir, "gone-00000000.sock")
	listener, err := net.Listen("unix", stale)
	if err != nil {
		t.Fatal(err)
	}
	listener.(*net.UnixListener).SetUnlinkOnClose(false)
	_ = listener.Close()
	rig.startDaemon()
	waitFor(t, "the stale socket to go", 5*time.Second, func() bool {
		_, err := os.Stat(stale)
		return os.IsNotExist(err)
	})
}

func TestHolderSocketNamesStaySafeAndDistinct(t *testing.T) {
	dir := "/tmp/aterm-1/hold"
	for _, name := range []string{"../../etc/passwd", "a/b", ".hidden", strings.Repeat("x", 300)} {
		got := holdSocketPath(dir, name)
		if filepath.Dir(got) != dir {
			t.Fatalf("holdSocketPath(%q) = %q escapes the hold directory", name, got)
		}
		if strings.HasPrefix(filepath.Base(got), ".") {
			t.Fatalf("holdSocketPath(%q) = %q starts with a dot", name, got)
		}
		if len(got) > 100 {
			t.Fatalf("holdSocketPath(%q) is %d long, over a socket path's room", name, len(got))
		}
	}
	if holdSocketPath(dir, "a/b") == holdSocketPath(dir, "a_b") {
		t.Fatal("two names that reduce alike must stay apart")
	}
}

// A daemon that adopts a session rebuilds its screen from the holder's replay,
// at the size the holder reports, so a prompt held across a restart still shows.
func TestAdoptedSessionStillShowsItsPrompt(t *testing.T) {
	rig := newHoldRig(t)
	rig.startDaemon()
	first := dialTest(t)
	first.spawn("adopt-state", "eng-platform", "Beetle-Ox",
		`printf '\033[?2004h'; printf 'Do you want to proceed?\r\n1. Yes\r\nEsc to cancel\r\n'; exec cat`)
	first.until("Esc to cancel")
	rig.killDaemon(syscall.SIGKILL)
	rig.startDaemon()
	waitFor(t, "the adopted session to read as a prompt", 8*time.Second, func() bool {
		views := rig.sessionsQuiet()
		return len(views) == 1 && views[0].State == statePrompt
	})
}
