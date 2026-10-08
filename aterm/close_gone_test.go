package main

import (
	"net"
	"os/exec"
	"testing"
	"time"
)

// A holder that drops the daemon from its clients never sends the exit frame
// (COI-2515), so the child's absence has to end the session.
func TestSessionEndsWhenItsChildIsGoneAndNoExitFrameCame(t *testing.T) {
	old := exitFrameGrace
	exitFrameGrace = 200 * time.Millisecond
	t.Cleanup(func() { exitFrameGrace = old })
	child := exec.Command("/usr/bin/true")
	if err := child.Run(); err != nil {
		t.Fatalf("run child: %v", err)
	}
	daemonEnd, holderEnd := net.Pipe()
	t.Cleanup(func() { _ = holderEnd.Close() })
	released := make(chan frame, 1)
	go func() {
		peer := newConn(holderEnd)
		if message, err := peer.read(); err == nil {
			released <- message
		}
	}()
	d := newDaemon(func(string, ...any) {})
	s := &ptySession{
		d: d, name: "gone-child", pid: child.Process.Pid, holder: newConn(daemonEnd),
		clients: map[*conn]bool{}, done: make(chan struct{}), forgotten: make(chan struct{}),
	}
	d.sessions[s.name] = s
	if !s.waitEnded(5*time.Second, 20*time.Millisecond) {
		t.Fatal("a session whose child is gone and whose holder was silent never ended")
	}
	select {
	case <-s.forgotten:
	case <-time.After(time.Second):
		t.Fatal("the daemon still holds the name of an ended session")
	}
	if message := <-released; message.Type != "release" {
		t.Fatalf("holder got %q, want release so it can go", message.Type)
	}
}

func TestSessionWithALiveChildDoesNotEndOnItsOwn(t *testing.T) {
	old := exitFrameGrace
	exitFrameGrace = 50 * time.Millisecond
	t.Cleanup(func() { exitFrameGrace = old })
	child := exec.Command("/bin/sleep", "30")
	if err := child.Start(); err != nil {
		t.Fatalf("start child: %v", err)
	}
	t.Cleanup(func() { _ = child.Process.Kill(); _ = child.Wait() })
	s := &ptySession{d: newDaemon(func(string, ...any) {}), name: "live-child", pid: child.Process.Pid, done: make(chan struct{})}
	if s.waitEnded(400*time.Millisecond, 20*time.Millisecond) {
		t.Fatal("a session with a live child was ended without an exit frame")
	}
}
