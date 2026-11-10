package main

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// liveCwd is where a running process works now, or "" when it cannot be read.
func liveCwd(pid int) string {
	if pid <= 0 {
		return ""
	}
	if runtime.GOOS == "linux" {
		dir, err := os.Readlink("/proc/" + strconv.Itoa(pid) + "/cwd")
		if err != nil {
			return ""
		}
		return dir
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "/usr/sbin/lsof", "-a", "-d", "cwd", "-p", strconv.Itoa(pid), "-Fn").Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "n/") {
			return line[1:]
		}
	}
	return ""
}

// withLiveCwd points a codex or opencode seat's record at the folder its process works
// in, so the finders match the harness's own files.
func withLiveCwd(entry ledgerEntry, pid int) ledgerEntry {
	if entry.Seat != "codex" && entry.Seat != "opencode" {
		return entry
	}
	if cwd := liveCwd(pid); cwd != "" {
		entry.Cwd = cwd
	}
	return entry
}
