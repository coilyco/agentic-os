package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestLiveClaudeColdLaunch cold-launches real claude seats, a model turn each (COI-2300).
// ATERM_LIVE_CLAUDE=<binary> runs it. See docs/aterm-daemon.md.
func TestLiveClaudeColdLaunch(t *testing.T) {
	bin := os.Getenv("ATERM_LIVE_CLAUDE")
	if bin == "" {
		t.Skip("set ATERM_LIVE_CLAUDE to a claude binary")
	}
	launches, parallel := envInt("ATERM_LIVE_CLAUDE_N", 6), envInt("ATERM_LIVE_CLAUDE_PARALLEL", 3)
	testDaemon(t)
	boxGrace = 20 * time.Second // the real claude paints its box late on a loaded host
	sender := dialTest(t)
	sender.spawn("eng-platform-beetle-ox", "eng-platform", "Beetle-Ox", `echo TOKEN=$ATERM_SESSION_TOKEN; sleep 900`)
	token := sender.token()
	body := "Reply with the single word OK. " + strings.Repeat("Padding so the line is long enough to fold. ", 40)

	var mu sync.Mutex
	var bad []string
	slots := make(chan struct{}, parallel)
	var group sync.WaitGroup
	for index := range launches {
		group.Add(1)
		slots <- struct{}{}
		go func() {
			defer group.Done()
			defer func() { <-slots }()
			name := fmt.Sprintf("frontend-eng-imp-dragonfly-%d", index)
			verdict := coldLaunch(t, bin, token, name, body)
			t.Logf("%s: %s", name, verdict)
			if !strings.HasPrefix(verdict, "ok") {
				mu.Lock()
				bad = append(bad, name+": "+verdict)
				mu.Unlock()
			}
		}()
	}
	group.Wait()
	if len(bad) > 0 {
		t.Fatalf("%d of %d cold launches did not submit:\n%s", len(bad), launches, strings.Join(bad, "\n"))
	}
}

func envInt(name string, fallback int) int {
	if value, err := strconv.Atoi(os.Getenv(name)); err == nil && value > 0 {
		return value
	}
	return fallback
}

// coldLaunch spawns one claude seat, sends body, and judges the delivered state it got.
func coldLaunch(t *testing.T, bin, token, name, body string) string {
	cwd, err := os.MkdirTemp("/tmp", "aterm-live-claude-")
	if err != nil {
		return "temp dir: " + err.Error()
	}
	defer os.RemoveAll(cwd)
	target := dialTest(t)
	started := time.Now()
	if _, err := target.c.request(frame{
		Type: "spawn", Session: name, Role: "frontend-eng", Identity: "Imp-Dragonfly", Seat: "claude",
		Argv: []string{bin}, Env: seatEnv(), Cwd: cwd, Rows: 40, Cols: 120,
	}); err != nil {
		return "spawn: " + err.Error()
	}
	var raw bytes.Buffer
	var rawMu sync.Mutex
	go func() {
		for {
			message, err := target.c.read()
			if err != nil {
				return
			}
			if message.Type == "output" {
				rawMu.Lock()
				fmt.Fprintf(&raw, "\n@%s ", time.Since(started).Round(time.Millisecond))
				raw.Write(message.Data)
				rawMu.Unlock()
			}
		}
	}()
	defer func() {
		if dump := os.Getenv("ATERM_LIVE_CLAUDE_DUMP"); dump != "" {
			rawMu.Lock()
			_ = os.WriteFile(filepath.Join(dump, name+".raw"), raw.Bytes(), 0o600)
			rawMu.Unlock()
		}
	}()
	state := sendWaiting(t, token, name, body, 120)
	took := time.Since(started).Round(100 * time.Millisecond)
	screen := func() []string {
		asker, err := dialDaemon(false)
		if err != nil {
			return nil
		}
		defer asker.Close()
		reply, err := asker.request(frame{Type: "status", Target: name, Lines: 40})
		if err != nil {
			return nil
		}
		return reply.Status.Screen
	}
	// Delivered is a claim about this moment: the transcript must already show the message.
	atReport := screen()
	if state.State != "delivered" {
		return fmt.Sprintf("NOT DELIVERED, send said %s %q after %s. Screen:\n%s", state.State, state.Reason, took, strings.Join(atReport, "\n"))
	}
	if echoCount(atReport, body) == 0 {
		return fmt.Sprintf("FALSE DELIVERED after %s, the transcript did not show the message. Screen:\n%s", took, strings.Join(atReport, "\n"))
	}
	return fmt.Sprintf("ok, delivered %s after spawn", took)
}

// seatEnv is this environment without CLAUDE_* markers, bar the _KEEP list.
func seatEnv() []string {
	keep := strings.Split(os.Getenv("ATERM_LIVE_CLAUDE_KEEP"), ",")
	var env []string
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if !strings.HasPrefix(name, "CLAUDE") || slices.Contains(keep, name) || keep[0] == "ALL" {
			env = append(env, entry)
		}
	}
	return env
}
