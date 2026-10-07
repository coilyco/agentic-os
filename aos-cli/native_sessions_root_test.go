//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestEnsureNativePrivateSessionsRootCreatesPrivateChain(t *testing.T) {
	base := t.TempDir()
	uid := os.Getuid()
	want := filepath.Join(base, "u"+strconv.Itoa(uid), "aos", "native")
	for attempt := 0; attempt < 2; attempt++ {
		got, err := ensureNativePrivateSessionsRoot(base, uid)
		if err != nil {
			t.Fatalf("attempt %d: %v", attempt, err)
		}
		if got != want {
			t.Fatalf("root = %q, want %q", got, want)
		}
	}
	for path := want; path != base; path = filepath.Dir(path) {
		info, err := os.Lstat(path)
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm != 0o700 {
			t.Errorf("%s mode = %04o, want 0700", path, perm)
		}
	}
}

func TestEnsureNativePrivateSessionsRootRefusesSymlink(t *testing.T) {
	base := t.TempDir()
	elsewhere := t.TempDir()
	uid := os.Getuid()
	if err := os.Symlink(elsewhere, filepath.Join(base, "u"+strconv.Itoa(uid))); err != nil {
		t.Fatal(err)
	}
	if _, err := ensureNativePrivateSessionsRoot(base, uid); err == nil ||
		!strings.Contains(err.Error(), "not a real directory") {
		t.Fatalf("error = %v, want a refusal naming a non-directory", err)
	}
	entries, _ := os.ReadDir(elsewhere)
	if len(entries) != 0 {
		t.Errorf("the symlink target gained %d entries", len(entries))
	}
}

func TestEnsureNativePrivateSessionsRootRefusesForeignOwner(t *testing.T) {
	base := t.TempDir()
	// The directory is ours, so asking for another uid is the foreign case.
	if _, err := ensureNativePrivateSessionsRoot(base, os.Getuid()+1); err == nil ||
		!strings.Contains(err.Error(), "owned by uid") {
		t.Fatalf("error = %v, want an ownership refusal", err)
	}
}

func TestEnsureNativePrivateSessionsRootRefusesLooseMode(t *testing.T) {
	base := t.TempDir()
	uid := os.Getuid()
	loose := filepath.Join(base, "u"+strconv.Itoa(uid))
	if err := os.Mkdir(loose, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(loose, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := ensureNativePrivateSessionsRoot(base, uid); err == nil ||
		!strings.Contains(err.Error(), "want 0700") {
		t.Fatalf("error = %v, want a mode refusal", err)
	}
}

func TestEnsureNativePrivateSessionsRootNeedsBase(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent")
	if _, err := ensureNativePrivateSessionsRoot(missing, os.Getuid()); err == nil {
		t.Fatal("a missing base was accepted")
	}
}

// The shipped macOS root must leave room for the Codex socket, or the move
// fixed nothing. The uid is the variable part, so test a wide one too.
func TestNativeSessionsRootFitsCodexSocket(t *testing.T) {
	for _, uid := range []string{"502", "1234567"} {
		root := filepath.Join(nativeSharedTempRoot, "u"+uid, "aos", "native", "ab12")
		if err := checkNativeCodexSocketBudget(root); err != nil {
			t.Errorf("uid %s: %v", uid, err)
		}
	}
}

func TestCheckNativeCodexSocketBudgetNamesTheOverrun(t *testing.T) {
	root := filepath.Join("/", strings.Repeat("a", 120), "ab12")
	err := checkNativeCodexSocketBudget(root)
	if err == nil {
		t.Fatal("an over-long socket path was accepted")
	}
	for _, want := range []string{"bytes", "AOS_NATIVE_SESSIONS_DIR", "app-server-control.sock"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q lacks %q", err, want)
		}
	}
}

func TestCheckNativeCodexSocketBudgetMeasuresResolvedPath(t *testing.T) {
	real := filepath.Join(t.TempDir(), strings.Repeat("r", 100))
	if err := os.Mkdir(real, 0o700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "s")
	if err := os.Symlink(real, alias); err != nil {
		t.Fatal(err)
	}
	if err := checkNativeCodexSocketBudget(filepath.Join(alias, "ab12")); err == nil {
		t.Fatal("a short alias hid a long real path")
	}
}

func TestReserveNativeSessionChecksSocketBudgetForCodexOnly(t *testing.T) {
	runtime := nativeTestRuntime(t, t.TempDir())
	runtime.SessionsRoot = filepath.Join(t.TempDir(), strings.Repeat("s", 110))
	runtime.RequestedID = "ab12"
	if _, _, err := reserveNativeSession(runtime, "codex", nil); err == nil ||
		!strings.Contains(err.Error(), "unix socket limit") {
		t.Fatalf("codex error = %v, want the socket budget refusal", err)
	}
	if _, err := os.Stat(filepath.Join(runtime.SessionsRoot, "ab12")); err == nil {
		t.Error("the refused launch left a session directory")
	}
	runtime.RequestedID = "ab12"
	if _, _, err := reserveNativeSession(runtime, "claude", nil); err != nil {
		t.Fatalf("claude has no socket and must launch: %v", err)
	}
}
