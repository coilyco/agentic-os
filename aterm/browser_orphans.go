package main

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// pidsDirName holds one record per running browser, beside the throwaway profiles.
const pidsDirName = "pids"

// browserRecord is what a later daemon needs to end a browser this one left behind.
type browserRecord struct {
	PID     int    `json:"pid"`
	Session string `json:"session"`
	Profile string `json:"profile"`
}

func browserRecordPath(dir string, pid int) string {
	return filepath.Join(dir, pidsDirName, strconv.Itoa(pid)+".json")
}

func writeBrowserRecord(dir string, record browserRecord) error {
	if err := os.MkdirAll(filepath.Join(dir, pidsDirName), 0o700); err != nil {
		return err
	}
	raw, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return os.WriteFile(browserRecordPath(dir, record.PID), raw, 0o600)
}

// groupAlive is whether a non-zombie is left in process group pgid, since kill(2)
// succeeds on a zombie and a container whose pid 1 does not reap keeps them.
func groupAlive(pgid int) bool {
	err := syscall.Kill(-pgid, 0)
	if err != nil && !errors.Is(err, syscall.EPERM) {
		return false
	}
	return !onlyZombies(pgid)
}

// onlyZombies reads procfs, and is false where there is none.
func onlyZombies(pgid int) bool {
	stats, _ := filepath.Glob("/proc/[0-9]*/stat")
	if len(stats) == 0 {
		return false
	}
	found := false
	for _, path := range stats {
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		// "pid (comm) state ppid pgrp ...", and comm may hold spaces and parentheses.
		rest := string(raw[strings.LastIndexByte(string(raw), ')')+1:])
		fields := strings.Fields(rest)
		if len(fields) < 3 || fields[2] != strconv.Itoa(pgid) {
			continue
		}
		if fields[0] != "Z" {
			return false
		}
		found = true
	}
	return found
}

// browserProcessMatches is whether pid is still the Chromium the record names,
// since a pid can be reused once its process is gone.
func browserProcessMatches(record browserRecord) bool {
	out, err := exec.Command("ps", "-o", "command=", "-p", strconv.Itoa(record.PID)).Output()
	if err != nil {
		return false
	}
	command := string(out)
	return strings.Contains(command, "--remote-debugging-pipe") && strings.Contains(command, "--user-data-dir="+record.Profile)
}

// endGroup ends a process group the way a browser's own stop does.
func endGroup(pgid int) {
	_ = syscall.Kill(-pgid, syscall.SIGTERM)
	for deadline := time.Now().Add(3 * time.Second); groupAlive(pgid) && time.Now().Before(deadline); {
		time.Sleep(50 * time.Millisecond)
	}
	if groupAlive(pgid) {
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
	}
}

// reapBrowsers ends the Chromium an earlier daemon left and removes its profiles.
// It cannot adopt one, since the CDP pipe died with that daemon.
func (d *daemon) reapBrowsers() {
	dir := d.browserProfileDir()
	if dir == "" {
		return
	}
	records, _ := filepath.Glob(filepath.Join(dir, pidsDirName, "*.json"))
	for _, path := range records {
		var record browserRecord
		raw, err := os.ReadFile(path)
		if err == nil {
			err = json.Unmarshal(raw, &record)
		}
		_ = os.Remove(path)
		if err != nil || record.PID <= 1 {
			d.logf("dropped unreadable browser record %s", filepath.Base(path))
			continue
		}
		// A group with no leader left is the orphan's own, since a pid cannot be
		// handed out while a process group still carries it.
		leader := syscall.Kill(record.PID, 0) == nil || errors.Is(syscall.Kill(record.PID, 0), syscall.EPERM)
		if (leader && browserProcessMatches(record)) || (!leader && groupAlive(record.PID)) {
			d.logf("ending the browser of %s that an earlier daemon left running (pid %d)", record.Session, record.PID)
			endGroup(record.PID)
		}
	}
	entries, _ := os.ReadDir(dir)
	for _, entry := range entries {
		if entry.IsDir() && entry.Name() != pidsDirName {
			_ = os.RemoveAll(filepath.Join(dir, entry.Name()))
		}
	}
}
