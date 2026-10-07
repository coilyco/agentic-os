package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestSpawnWindowReportsATerminalThatDiesOnStartup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fixture is a POSIX script")
	}
	script := filepath.Join(t.TempDir(), "terminal")
	body := "#!/bin/sh\necho 'unknown option' >&2\nexit 2\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatalf("write: %v", err)
	}
	err := spawnWindowWithin(time.Minute, script, []string{"--title", "x"})
	if err == nil {
		t.Fatal("a terminal that exits immediately should be reported")
	}
	// Without this the launcher prints a success line and no window appears,
	// which is the failure the old launcher had no way to surface.
	if !strings.Contains(err.Error(), "unknown option") {
		t.Fatalf("the report should carry the terminal's own words: %v", err)
	}
}

func TestSpawnWindowAcceptsATerminalThatKeepsRunning(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fixture is a POSIX script")
	}
	script := filepath.Join(t.TempDir(), "terminal")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nsleep 5\n"), 0o700); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := spawnWindow(script, nil); err != nil {
		t.Fatalf("a live terminal should not be an error: %v", err)
	}
}

// Alacritty hands off to an already-running instance and exits 0 at once on
// macOS, so a clean early exit is a normal launch rather than a failure.
func TestSpawnWindowAcceptsAnImmediateCleanExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fixture is a POSIX script")
	}
	script := filepath.Join(t.TempDir(), "terminal")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := spawnWindow(script, nil); err != nil {
		t.Fatalf("a hand-off exit should not be an error: %v", err)
	}
}
