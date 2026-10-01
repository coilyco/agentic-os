package main

import (
	"io"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"
)

// windowOn attaches a fake terminal to a session through the reconnecting loop,
// the way a kitty window does. Its output and log are what the test reads.
type windowOn struct {
	stdout *lockedBuffer
	log    *lockedBuffer
	pump   *inputPump
	done   chan attachResult
}

func attachWindow(t *testing.T, c *conn, session string) *windowOn {
	t.Helper()
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	w := &windowOn{stdout: &lockedBuffer{}, log: &lockedBuffer{}, pump: &inputPump{chunks: make(chan []byte, 8)}, done: make(chan attachResult, 1)}
	go func() { _, _ = io.Copy(w.stdout, read) }()
	go func() { w.done <- attachReconnecting(c, session, w.pump, write, false, w.log) }()
	t.Cleanup(func() {
		_ = write.Close()
		_ = read.Close()
	})
	return w
}

func (w *windowOn) waitFor(t *testing.T, label string, within time.Duration, holds func() bool) {
	t.Helper()
	waitFor(t, label, within, holds)
}

// A window outlives a daemon crash: the daemon comes back, the window redials
// and attaches again, and what it drew before is not drawn twice.
func TestWindowReconnectsAcrossADaemonCrashWithoutRedrawing(t *testing.T) {
	daemonStartArgs = []string{"--websocket", "", "--tailnet-port", ""}
	t.Cleanup(func() { daemonStartArgs = nil })
	rig := newHoldRig(t)
	rig.startDaemon()
	c, err := dialDaemon(false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.request(frame{
		Type: "spawn", Session: "eng-platform-win", Role: "eng-platform", Identity: "Beetle-Ox", Seat: "claude",
		Argv: []string{"/bin/sh", "-c", "echo READY; exec cat"}, Env: os.Environ(), Cwd: "/", Rows: 24, Cols: 80,
	}); err != nil {
		t.Fatal(err)
	}
	window := attachWindow(t, c, "eng-platform-win")
	window.waitFor(t, "READY on the window", 10*time.Second, func() bool { return strings.Contains(window.stdout.String(), "READY") })
	pid := rig.sessions()[0].PID

	rig.killDaemon(syscall.SIGKILL)
	window.waitFor(t, "the window to notice and reconnect", 30*time.Second, func() bool {
		return strings.Contains(window.log.String(), "reconnecting") && len(rig.sessionsQuiet()) == 1
	})
	if !alive(pid) {
		t.Fatalf("session pid %d died with the daemon", pid)
	}
	window.pump.chunks <- []byte("after\n")
	window.waitFor(t, "typing to reach the session after reconnecting", 15*time.Second, func() bool {
		return strings.Contains(window.stdout.String(), "after")
	})
	if got := strings.Count(window.stdout.String(), "READY"); got != 1 {
		t.Fatalf("READY was drawn %d times, a reconnect must replay only what the window missed:\n%q", got, window.stdout.String())
	}
	close(window.pump.chunks)
	if result := <-window.done; !result.detached || result.lost {
		t.Fatalf("the window should end by detaching: %+v", result)
	}
}

// A session that ended while the daemon was away hands the window its exit
// code, not an error.
func TestWindowLearnsTheExitCodeOfASessionThatEndedWhileTheDaemonWasAway(t *testing.T) {
	daemonStartArgs = []string{"--websocket", "", "--tailnet-port", ""}
	t.Cleanup(func() { daemonStartArgs = nil })
	rig := newHoldRig(t)
	rig.startDaemon()
	c, err := dialDaemon(false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.request(frame{
		Type: "spawn", Session: "scientist-win", Role: "scientist", Identity: "Evie", Seat: "codex",
		Argv: []string{"/bin/sh", "-c", "sleep 1; exit 7"}, Env: os.Environ(), Cwd: "/", Rows: 24, Cols: 80,
	}); err != nil {
		t.Fatal(err)
	}
	pid := rig.sessions()[0].PID
	window := attachWindow(t, c, "scientist-win")
	rig.killDaemon(syscall.SIGKILL)
	waitFor(t, "the session to end", 10*time.Second, func() bool { return !alive(pid) })
	select {
	case result := <-window.done:
		if result.lost || result.code != 7 {
			t.Fatalf("the window should learn exit 7: %+v\nlog: %s", result, window.log.String())
		}
	case <-time.After(40 * time.Second):
		t.Fatalf("the window never learned how the session ended\nlog: %s", window.log.String())
	}
}

func TestReplayFromAnOffsetSendsOnlyWhatWasMissed(t *testing.T) {
	s := &ptySession{clients: map[*conn]bool{}, scrollback: []byte("0123456789"), outputOffset: 110}
	for _, testCase := range []struct {
		from       int64
		want       string
		wantOffset int64
	}{
		{0, "0123456789", 100},
		{100, "0123456789", 100},
		{104, "456789", 104},
		{110, "", 110},
		{500, "", 110},
		{50, "0123456789", 100},
	} {
		got, offset := replayFrom(s, testCase.from)
		if string(got) != testCase.want || (len(got) > 0 && offset != testCase.wantOffset) {
			t.Fatalf("from %d replayed %q at %d, want %q at %d", testCase.from, got, offset, testCase.want, testCase.wantOffset)
		}
	}
}
