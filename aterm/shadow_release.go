package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// shadowReleaseCeiling bounds one aos call, which may wait on the native startup lock.
const shadowReleaseCeiling = 10 * time.Minute

// shadowReleaser has aos collect an ended session's native shadow, one call at a time.
// aos keeps dirty or unpushed worktrees, so close never costs work (COI-3427).
func shadowReleaser(logf func(string, ...any)) func(pid int) {
	var one sync.Mutex
	return func(pid int) {
		go func() {
			one.Lock()
			defer one.Unlock()
			binary := findAOS()
			if binary == "" {
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), shadowReleaseCeiling)
			defer cancel()
			out, err := exec.CommandContext(ctx, binary, "_native-shadow", "--finished", strconv.Itoa(pid)).CombinedOutput()
			text := strings.TrimSpace(string(out))
			if err != nil || !strings.HasPrefix(text, "0 dead") {
				logf("shadow of pid %d: %s (%v)", pid, text, err)
			}
		}()
	}
}

// findAOS is the aos binary, looked up on PATH and then where the installs put it, since
// a daemon started by a service manager has a thin PATH.
func findAOS() string {
	if path, err := exec.LookPath("aos"); err == nil {
		return path
	}
	for _, candidate := range []string{
		filepath.Join(realHome(), ".local", "bin", "aos"),
		"/opt/homebrew/bin/aos",
		"/usr/local/bin/aos",
	} {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}
