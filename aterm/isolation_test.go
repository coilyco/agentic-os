package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTestsStartWithAThrowawayStateDirAndNoLiveDaemon(t *testing.T) {
	if err := realLedgerGuard(defaultLedgerDir(), func(string) {})(ledgerDir()); err != nil {
		t.Fatalf("the state dir a test inherits is the real one: %v", err)
	}
	if got := ledgerDir(); !strings.HasPrefix(got, "/tmp/aterm-tests-") {
		t.Fatalf("a test that sets nothing would write to %s", got)
	}
	if got := os.Getenv(daemonSocketEnv); !strings.HasPrefix(got, "/tmp/aterm-tests-") {
		t.Fatalf("a test that sets nothing would dial %q", got)
	}
}

func TestTheInstalledGuardRefusesTheRealLedgerAndEverythingBelowIt(t *testing.T) {
	if ledgerGuard == nil {
		t.Fatal("the tests must run with the ledger guard installed")
	}
	real := defaultLedgerDir()
	for _, dir := range []string{real, filepath.Join(real, "sub"), real + "/../sessions"} {
		// Only the guard function is called, so a broken guard cannot write anything here.
		if err := ledgerGuard(dir); err == nil {
			t.Fatalf("the guard let %s through", dir)
		}
	}
	// Those calls were recorded as violations, which is the point, so clear them.
	realLedgerMu.Lock()
	realLedgerViolations = nil
	realLedgerMu.Unlock()
	if err := ledgerGuard(t.TempDir()); err != nil {
		t.Fatalf("an ordinary directory must pass: %v", err)
	}
}

func TestGuardedWriteAndPruneLeaveAProtectedDirectoryAlone(t *testing.T) {
	protected := t.TempDir()
	old := time.Now().Add(-ledgerKeep - time.Hour).UTC()
	if err := writeLedger(protected, ledgerEntry{Name: "old-ended", Started: old, Ended: &old}); err != nil {
		t.Fatal(err)
	}
	var seen []string
	guard := realLedgerGuard(protected, func(dir string) { seen = append(seen, dir) })
	if err := writeLedgerGuarded(guard, protected, ledgerEntry{Name: "new", Started: time.Now().UTC()}); err == nil {
		t.Fatal("a write into the protected directory must be refused")
	}
	pruneLedgerGuarded(guard, protected, time.Now())
	entries := readLedger(protected)
	if len(entries) != 1 || entries[0].Name != "old-ended" {
		t.Fatalf("the protected directory changed: %+v", entries)
	}
	if len(seen) != 2 {
		t.Fatalf("both the write and the prune should have been recorded, got %v", seen)
	}
}
