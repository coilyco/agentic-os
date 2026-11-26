package main

import (
	"os"
	"os/exec"
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
	deadline := time.Now().Add(3 * time.Second)
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
