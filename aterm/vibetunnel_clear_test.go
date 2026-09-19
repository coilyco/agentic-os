package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"
)

func writeSessionRecord(t *testing.T, control, id string, record vibeTunnelRecord) string {
	t.Helper()
	dir := filepath.Join(control, id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	raw, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "session.json"), raw, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	return dir
}

// fakeReaper answers every process question from the test, so no case needs a
// real process. The signals it was sent are what a case asserts on.
func fakeReaper(control string, alive func(sent []syscall.Signal) bool, began time.Time) (sessionReaper, *[]syscall.Signal, *bytes.Buffer) {
	var sent []syscall.Signal
	notice := &bytes.Buffer{}
	return sessionReaper{
		control:   control,
		alive:     func(int) bool { return alive(sent) },
		signal:    func(_ int, sig syscall.Signal) error { sent = append(sent, sig); return nil },
		startedAt: func(int) (time.Time, error) { return began, nil },
		wait:      func(time.Duration) {},
		notice:    notice,
	}, &sent, notice
}

func runningRecord(name string, began time.Time) vibeTunnelRecord {
	return vibeTunnelRecord{Name: name, Status: "running", PID: 4242, StartedAt: began.UTC().Format(time.RFC3339)}
}

func TestClearTerminatesARunningSessionAndRemovesItsRecord(t *testing.T) {
	control, began := t.TempDir(), time.Now()
	dir := writeSessionRecord(t, control, "fwd_1_1", runningRecord(vibeTunnelSessionName, began))
	reaper, sent, _ := fakeReaper(control, func(sent []syscall.Signal) bool { return len(sent) == 0 }, began)
	if cleared := reaper.clear(vibeTunnelSessionName); cleared != 1 {
		t.Fatalf("cleared = %d, want 1", cleared)
	}
	if !slices.Equal(*sent, []syscall.Signal{syscall.SIGTERM}) {
		t.Fatalf("signals = %v, want a single SIGTERM", *sent)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("the record should be gone: %v", err)
	}
}

func TestClearEscalatesToKillWhenTerminateIsIgnored(t *testing.T) {
	control, began := t.TempDir(), time.Now()
	writeSessionRecord(t, control, "fwd_1_1", runningRecord(vibeTunnelSessionName, began))
	killed := func(sent []syscall.Signal) bool { return !slices.Contains(sent, syscall.SIGKILL) }
	reaper, sent, _ := fakeReaper(control, killed, began)
	if reaper.clear(vibeTunnelSessionName) != 1 {
		t.Fatal("a killed session should be cleared")
	}
	if !slices.Equal(*sent, []syscall.Signal{syscall.SIGTERM, syscall.SIGKILL}) {
		t.Fatalf("signals = %v, want SIGTERM then SIGKILL", *sent)
	}
}

func TestClearKeepsARecordWhoseProcessWillNotEnd(t *testing.T) {
	control, began := t.TempDir(), time.Now()
	dir := writeSessionRecord(t, control, "fwd_1_1", runningRecord(vibeTunnelSessionName, began))
	reaper, _, notice := fakeReaper(control, func([]syscall.Signal) bool { return true }, began)
	if cleared := reaper.clear(vibeTunnelSessionName); cleared != 0 {
		t.Fatalf("cleared = %d, want 0", cleared)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("a live session must keep its record: %v", err)
	}
	if !strings.Contains(notice.String(), "would not end") {
		t.Fatalf("the survivor should be named: %q", notice.String())
	}
}

func TestClearLeavesEveryOtherNameAlone(t *testing.T) {
	control, began := t.TempDir(), time.Now()
	dir := writeSessionRecord(t, control, "fwd_1_1", runningRecord("claude (~/projects)", began))
	reaper, sent, _ := fakeReaper(control, func([]syscall.Signal) bool { return true }, began)
	if reaper.clear(vibeTunnelSessionName) != 0 || len(*sent) != 0 {
		t.Fatalf("a session under another name was touched: %v", *sent)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("its record should stay: %v", err)
	}
}

func TestClearRemovesAnEndedRecordWithoutSignalling(t *testing.T) {
	control, began := t.TempDir(), time.Now()
	ended := runningRecord(vibeTunnelSessionName, began)
	ended.Status = "exited"
	dir := writeSessionRecord(t, control, "fwd_1_1", ended)
	reaper, sent, _ := fakeReaper(control, func([]syscall.Signal) bool { return true }, began)
	if reaper.clear(vibeTunnelSessionName) != 1 || len(*sent) != 0 {
		t.Fatalf("an ended session needs removal and no signal: %v", *sent)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("the record should be gone: %v", err)
	}
}

func TestClearNeverSignalsAPidThatWasReused(t *testing.T) {
	control, began := t.TempDir(), time.Now()
	dir := writeSessionRecord(t, control, "fwd_1_1", runningRecord(vibeTunnelSessionName, began))
	// The pid is alive, but its process began an hour after the session did.
	reaper, sent, _ := fakeReaper(control, func([]syscall.Signal) bool { return true }, began.Add(time.Hour))
	if reaper.clear(vibeTunnelSessionName) != 1 {
		t.Fatal("a stale record should still be removed")
	}
	if len(*sent) != 0 {
		t.Fatalf("an unrelated process was signalled: %v", *sent)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("the stale record should be gone: %v", err)
	}
}

func TestClearLeavesASessionItCannotVerify(t *testing.T) {
	control, began := t.TempDir(), time.Now()
	dir := writeSessionRecord(t, control, "fwd_1_1", runningRecord(vibeTunnelSessionName, began))
	reaper, sent, notice := fakeReaper(control, func([]syscall.Signal) bool { return true }, began)
	reaper.startedAt = func(int) (time.Time, error) { return time.Time{}, os.ErrNotExist }
	if reaper.clear(vibeTunnelSessionName) != 0 || len(*sent) != 0 {
		t.Fatalf("an unverifiable process was signalled: %v", *sent)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("its record should stay: %v", err)
	}
	if !strings.Contains(notice.String(), "could not be verified") {
		t.Fatalf("the skip should be named: %q", notice.String())
	}
}

func TestClearIgnoresADirectoryTheServerWouldNeverName(t *testing.T) {
	control, began := t.TempDir(), time.Now()
	dir := writeSessionRecord(t, control, "not a session id", runningRecord(vibeTunnelSessionName, began))
	reaper, _, _ := fakeReaper(control, func([]syscall.Signal) bool { return false }, began)
	if reaper.clear(vibeTunnelSessionName) != 0 {
		t.Fatal("a directory outside the server's id pattern was cleared")
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("it should stay: %v", err)
	}
}

func TestClearIsQuietWhenNoServerEverRan(t *testing.T) {
	reaper, _, _ := fakeReaper(filepath.Join(t.TempDir(), "absent"), func([]syscall.Signal) bool { return false }, time.Now())
	if reaper.clear(vibeTunnelSessionName) != 0 {
		t.Fatal("a missing control directory has nothing to clear")
	}
}

func TestClearEndsARealProcessThroughTheSystemReaper(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fixture is a POSIX process")
	}
	child := exec.Command("sleep", "60")
	if err := child.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	// Reaping keeps a terminated child from reading as alive as a zombie.
	go func() { _ = child.Wait() }()
	t.Cleanup(func() { _ = child.Process.Kill() })
	began, err := processStart(child.Process.Pid)
	if err != nil {
		t.Skipf("this host's ps cannot report a start time: %v", err)
	}
	control := t.TempDir()
	record := runningRecord(vibeTunnelSessionName, began)
	record.PID = child.Process.Pid
	dir := writeSessionRecord(t, control, "fwd_1_1", record)
	reaper := systemReaper(&bytes.Buffer{})
	reaper.control = control
	if reaper.clear(vibeTunnelSessionName) != 1 {
		t.Fatal("the running session should be cleared")
	}
	if reaper.alive(child.Process.Pid) {
		t.Fatal("the process should have been terminated")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("the record should be gone: %v", err)
	}
}
