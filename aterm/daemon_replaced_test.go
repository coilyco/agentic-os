package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// gone stands in for a binary an upgrade removed, and restores the real check.
func gone(t *testing.T) {
	t.Helper()
	previous := executableGone
	t.Cleanup(func() { executableGone = previous })
	executableGone = func() bool { return true }
}

func failingServing() (*tailnetServing, error) {
	return &tailnetServing{
		server: &http.Server{},
		probe:  func(context.Context) error { return errors.New("TLS handshake failed: EOF") },
	}, nil
}

func TestHoldTailnetStopsInsteadOfRebindingWhenItsBinaryIsGone(t *testing.T) {
	fastProbe(t)
	gone(t)
	var binds atomic.Int32
	var stopped atomic.Int32
	d := newDaemon(func(string, ...any) {})
	d.stopServing = func() { stopped.Add(1) }
	finished := make(chan struct{})
	go func() {
		d.serveTailnetWhenUp(make(chan struct{}), func() (*tailnetServing, error) {
			binds.Add(1)
			return failingServing()
		})
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("a daemon whose binary is gone kept rebinding")
	}
	if binds.Load() != 1 || stopped.Load() != 1 || !d.replaced.Load() {
		t.Fatalf("binds=%d stopped=%d replaced=%v, want 1, 1, true", binds.Load(), stopped.Load(), d.replaced.Load())
	}
}

func TestADaemonWhoseBinaryIsGoneExitsNonZeroAndLeavesItsSessionsToTheNextOne(t *testing.T) {
	fastProbe(t)
	gone(t)
	t.Setenv("SHELL", "/bin/sh")
	dir, err := os.MkdirTemp("/tmp", "aterm-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	first := newDaemon(func(string, ...any) {})
	first.holdDir = filepath.Join(dir, "hold")
	if err := os.MkdirAll(first.holdDir, 0o700); err != nil {
		t.Fatal(err)
	}
	shell, err := first.spawnTerminal(frame{Type: "spawn", Kind: kindTerminal})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	t.Cleanup(shell.end)
	first.letGoAll()

	result := make(chan error, 1)
	go func() {
		result <- runDaemon(daemonOptions{
			Socket: filepath.Join(dir, "d.sock"), Idle: time.Hour, TailnetPort: "7419",
			tailnetListen: failingServing,
		}, io.Discard)
	}()
	select {
	case err := <-result:
		if !errors.Is(err, errExecutableReplaced) {
			t.Fatalf("runDaemon returned %v, want the replacement error so the exit is non-zero", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the daemon did not stop")
	}

	second := newDaemon(func(string, ...any) {})
	second.holdDir = first.holdDir
	second.adoptHolders()
	t.Cleanup(func() {
		for _, adopted := range second.everySession() {
			adopted.end()
		}
	})
	views := second.terminalViews()
	if len(views) != 1 || views[0].PID != shell.pid {
		t.Fatalf("the session did not survive the stop for the next daemon to adopt: %+v", views)
	}
}

func TestExecutableGoneIsFalseForTheRunningTestBinary(t *testing.T) {
	if executableGone() {
		t.Fatal("the test binary exists, so it is not gone")
	}
}
