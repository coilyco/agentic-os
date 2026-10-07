package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// A holder or daemon the tests start is this test binary run again with the
// verb, since os.Executable is what the daemon launches.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && (os.Args[1] == holdCommand || os.Args[1] == "daemon") {
		main()
		os.Exit(0)
	}
	if len(os.Args) > 3 && os.Args[1] == fakeBoxCommand {
		runFakeBox()
		os.Exit(0)
	}
	os.Exit(runIsolated(m))
}

// realLedgerViolations are the writes the guard refused, which fail the whole run.
var (
	realLedgerMu         sync.Mutex
	realLedgerViolations []string
)

// realLedgerGuard refuses any ledger directory at or under real and records it.
func realLedgerGuard(real string, record func(string)) func(string) error {
	return func(dir string) error {
		if relative, err := filepath.Rel(filepath.Clean(real), filepath.Clean(dir)); err == nil && !strings.HasPrefix(relative, "..") {
			record(dir)
			return fmt.Errorf("a test touched the real ledger directory %s, so the write was refused", dir)
		}
		return nil
	}
}

// runIsolated gives every test a throwaway state dir and a socket nobody listens on, so
// none can reach the real ledger or a live daemon, and fails the run if one tries.
func runIsolated(m *testing.M) int {
	real := defaultLedgerDir()
	root, err := os.MkdirTemp("/tmp", "aterm-tests-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "aterm tests: no temp directory:", err)
		return 1
	}
	defer os.RemoveAll(root)
	_ = os.Setenv(stateDirEnv, filepath.Join(root, "state"))
	_ = os.Setenv(daemonSocketEnv, filepath.Join(root, "no-daemon.sock"))
	ledgerGuard = realLedgerGuard(real, func(dir string) {
		realLedgerMu.Lock()
		defer realLedgerMu.Unlock()
		realLedgerViolations = append(realLedgerViolations, dir)
	})
	code := m.Run()
	realLedgerMu.Lock()
	defer realLedgerMu.Unlock()
	if len(realLedgerViolations) > 0 {
		fmt.Fprintf(os.Stderr, "FAIL: tests touched the real ledger %s (%d refused writes)\n", real, len(realLedgerViolations))
		return 1
	}
	return code
}
