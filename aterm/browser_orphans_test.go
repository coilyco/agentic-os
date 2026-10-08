package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

const orphanDirEnv = "ATERM_TEST_ORPHAN_DIR"

// The child half of the orphan test: it starts a browser and waits to be killed.
func TestHelperStartsABrowserAndWaits(t *testing.T) {
	dir := os.Getenv(orphanDirEnv)
	if dir == "" {
		t.Skip("run by the orphan test")
	}
	if _, err := launchChromium(dir, "orphan", browserProfile{}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Minute)
}

func orphanDaemon(t *testing.T) (*daemon, string) {
	t.Helper()
	base := testHoldDir(t)
	d := &daemon{holdDir: filepath.Join(base, "hold"), logf: t.Logf}
	return d, d.browserProfileDir()
}

// killedDaemon runs the child half under a browser binary, kills it outright
// and returns the recorded pid. The browser is left to whatever it does alone.
func killedDaemon(t *testing.T, dir string) int {
	t.Helper()
	child := exec.Command(os.Args[0], "-test.run=^TestHelperStartsABrowserAndWaits$")
	child.Env = append(os.Environ(), orphanDirEnv+"="+dir)
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = child.Process.Kill(); _ = child.Wait() })
	var pid int
	waitFor(t, "the browser record", 15*time.Second, func() bool {
		records, _ := filepath.Glob(filepath.Join(dir, pidsDirName, "*.json"))
		if len(records) != 1 {
			return false
		}
		pid, _ = strconv.Atoi(strings.TrimSuffix(filepath.Base(records[0]), ".json"))
		return pid > 0
	})
	t.Cleanup(func() { _ = syscall.Kill(-pid, syscall.SIGKILL) })
	time.Sleep(2 * time.Second)
	_ = child.Process.Kill()
	_ = child.Wait()
	return pid
}

func nothingLeft(t *testing.T, dir string) {
	t.Helper()
	if left, _ := filepath.Glob(filepath.Join(dir, "*")); len(left) != 1 || filepath.Base(left[0]) != pidsDirName {
		t.Fatalf("only the records directory should remain: %v", left)
	}
	if records, _ := filepath.Glob(filepath.Join(dir, pidsDirName, "*")); len(records) != 0 {
		t.Fatalf("records should be removed: %v", records)
	}
}

// Chrome on macOS ends itself about a second after its CDP pipe closes, so what a
// killed daemon leaves behind there is the record and the throwaway profile.
func TestReapBrowsersLeavesNoChromiumNorProfileAfterADaemonIsKilled(t *testing.T) {
	if chromiumPath() == "" {
		t.Skip("no Chromium or Chrome on this host")
	}
	d, dir := orphanDaemon(t)
	if err := ensureSocketDir(dir); err != nil {
		t.Fatal(err)
	}
	pid := killedDaemon(t, dir)
	d.reapBrowsers()
	waitFor(t, "the browser to be gone", 15*time.Second, func() bool { return !groupAlive(pid) })
	nothingLeft(t, dir)
}

// A browser that ignores its pipe closing, as another build may.
func TestReapBrowsersEndsABrowserThatOutlivesItsDaemon(t *testing.T) {
	script := filepath.Join(t.TempDir(), "stubborn-chrome")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nsleep 300\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv(browserEnv, script)
	d, dir := orphanDaemon(t)
	if err := ensureSocketDir(dir); err != nil {
		t.Fatal(err)
	}
	pid := killedDaemon(t, dir)
	if !groupAlive(pid) {
		t.Fatal("the stand-in browser should outlive its daemon")
	}
	d.reapBrowsers()
	waitFor(t, "the orphan to end", 15*time.Second, func() bool { return !groupAlive(pid) })
	nothingLeft(t, dir)
}

func TestReapBrowsersLeavesAReusedPidAlone(t *testing.T) {
	d, dir := orphanDaemon(t)
	bystander := exec.Command("sleep", "60")
	bystander.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := bystander.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = bystander.Process.Kill(); _ = bystander.Wait() })
	if err := writeBrowserRecord(dir, browserRecord{PID: bystander.Process.Pid, Session: "gone", Profile: filepath.Join(dir, "p")}); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "stale-profile"), 0o700); err != nil {
		t.Fatal(err)
	}

	d.reapBrowsers()
	if !groupAlive(bystander.Process.Pid) {
		t.Fatal("a process that is not the recorded browser must not be ended")
	}
	if _, err := os.Stat(filepath.Join(dir, "stale-profile")); err == nil {
		t.Fatal("a profile no browser holds should be removed")
	}
	if _, err := os.Stat(browserRecordPath(dir, bystander.Process.Pid)); err == nil {
		t.Fatal("a record that names no browser should be dropped")
	}
}
