package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// pmsetShows reports whether pmset lists an assertion created for this test's pid.
func pmsetShows(t *testing.T) bool {
	t.Helper()
	out, err := exec.Command("/usr/bin/pmset", "-g", "assertions").Output()
	if err != nil {
		t.Fatalf("pmset: %v", err)
	}
	return strings.Contains(string(out), "Created for PID: "+strconv.Itoa(os.Getpid())+".")
}

func waitPmset(t *testing.T, want bool) {
	t.Helper()
	deadline := time.Now().Add(6 * time.Second)
	for pmsetShows(t) != want {
		if time.Now().After(deadline) {
			t.Fatalf("pmset assertion for this pid present=%v, want %v", !want, want)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestCaffeinateLockShowsInPmsetAndReleases(t *testing.T) {
	lock, err := acquireAwake(awakeWhy)
	if err != nil {
		t.Fatal(err)
	}
	waitPmset(t, true)
	if !lock.alive() || !strings.Contains(lock.describe(), "caffeinate pid") {
		t.Fatalf("lock should be alive and say so: %s", lock.describe())
	}
	lock.release()
	if lock.alive() {
		t.Fatal("a released lock should be dead")
	}
	waitPmset(t, false)
}

// awakeDaemon serves a daemon with keep-awake on and its own state dir, so a host.json
// written here reaches no other test.
func awakeDaemon(t *testing.T, kind hostKind) {
	t.Helper()
	t.Setenv(keepAwakeEnv, "on")
	root := t.TempDir()
	t.Setenv(stateDirEnv, filepath.Join(root, "state"))
	record := `{"format":"` + hostKindFormat + `","kind":"` + string(kind) + `"}`
	if err := os.WriteFile(filepath.Join(root, hostKindFile), []byte(record), 0o600); err != nil {
		t.Fatal(err)
	}
	testDaemon(t)
}

func awakeStatus(t *testing.T, session string) awakeState {
	t.Helper()
	status, err := sessionStatusOf(session, 0)
	if err != nil || status.Awake == nil {
		t.Fatalf("status: %v, host_awake %+v", err, status.Awake)
	}
	return *status.Awake
}

func TestDaemonKeepsAPersonalMacAwakeOnlyWhileASeatRuns(t *testing.T) {
	awakeDaemon(t, hostPersonal)
	waitPmset(t, false)
	tc := dialTest(t)
	tc.spawn("awake-test", "eng-platform", "Beetle-Ox", "sleep 3")
	waitPmset(t, true)
	if state := awakeStatus(t, "awake-test"); !state.Held || !strings.Contains(state.Detail, "caffeinate pid") {
		t.Fatalf("status should report the held lock, got %+v", state)
	}
	waitPmset(t, false)
}

func TestDaemonTakesNoAssertionOnADurableMac(t *testing.T) {
	awakeDaemon(t, hostDurable)
	tc := dialTest(t)
	tc.spawn("durable-test", "eng-platform", "Beetle-Ox", "sleep 3")
	if state := awakeStatus(t, "durable-test"); state.Held || state.Detail != "not a personal host" {
		t.Fatalf("a durable host should hold nothing, got %+v", state)
	}
	if pmsetShows(t) {
		t.Fatal("pmset lists an assertion for a durable host")
	}
}
