package main

import (
	"errors"
	"os"
	"testing"
	"time"
)

func TestListProcessesSeesThisProcessAndItsParent(t *testing.T) {
	entries, err := listProcesses()
	if err != nil {
		t.Skipf("this host's ps cannot list processes: %v", err)
	}
	for _, entry := range entries {
		if entry.PID == os.Getpid() {
			if entry.PPID != os.Getppid() || entry.Command == "" {
				t.Fatalf("entry = %+v, want ppid %d and a command", entry, os.Getppid())
			}
			return
		}
	}
	t.Fatalf("this process %d is missing from the listing", os.Getpid())
}

func TestListProcessesRetriesOnceAndNoMore(t *testing.T) {
	// ps failing once under load must not refuse a person's answer. See COI-2736.
	earlier := processRetryPause
	processRetryPause = time.Millisecond
	t.Cleanup(func() { processRetryPause = earlier })
	calls := 0
	flaky := func() ([]processEntry, error) {
		if calls++; calls == 1 {
			return nil, errors.New("resource temporarily unavailable")
		}
		return []processEntry{{PID: 7, PPID: 1}}, nil
	}
	if entries, err := listWithRetry(flaky); err != nil || len(entries) != 1 || calls != 2 {
		t.Fatalf("one failure should be absorbed: entries %v, err %v, calls %d", entries, err, calls)
	}
	calls = 0
	broken := func() ([]processEntry, error) { calls++; return nil, errors.New("no ps") }
	if _, err := listWithRetry(broken); err == nil || calls != 2 {
		t.Fatalf("a second failure must surface after exactly one retry: err %v, calls %d", err, calls)
	}
}
