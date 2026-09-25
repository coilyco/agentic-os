package main

import (
	"bytes"
	"os/exec"
	"slices"
	"syscall"
	"testing"
	"time"
)

func TestStopEscalatesToKillWhenTerminateIsIgnored(t *testing.T) {
	var sent []syscall.Signal
	reaper := sessionReaper{
		alive:  func(int) bool { return !slices.Contains(sent, syscall.SIGKILL) },
		signal: func(_ int, sig syscall.Signal) error { sent = append(sent, sig); return nil },
		wait:   func(time.Duration) {},
		notice: &bytes.Buffer{},
	}
	if !reaper.stop(4242) {
		t.Fatal("a process that ends on SIGKILL is gone")
	}
	if !slices.Equal(sent, []syscall.Signal{syscall.SIGTERM, syscall.SIGKILL}) {
		t.Fatalf("signals = %v, want SIGTERM then SIGKILL", sent)
	}
}

func TestStopEndsARealProcessThroughTheSystemReaper(t *testing.T) {
	child := exec.Command("sleep", "60")
	if err := child.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	// Reaping keeps a terminated child from reading as alive as a zombie.
	go func() { _ = child.Wait() }()
	t.Cleanup(func() { _ = child.Process.Kill() })
	if !systemReaper(&bytes.Buffer{}).stop(child.Process.Pid) {
		t.Fatal("the process should have been stopped")
	}
}
