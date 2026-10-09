package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestLiveClaudeBusySeat sends three messages into a real seat's long Bash call, then
// checks its transcript for the bytes (COI-2619). ATERM_LIVE_CLAUDE=<binary> runs it.
func TestLiveClaudeBusySeat(t *testing.T) {
	bin, config := os.Getenv("ATERM_LIVE_CLAUDE"), os.Getenv("CLAUDE_CONFIG_DIR")
	if bin == "" || config == "" {
		t.Skip("set ATERM_LIVE_CLAUDE to a claude binary, and CLAUDE_CONFIG_DIR, kept by ATERM_LIVE_CLAUDE_KEEP")
	}
	testDaemon(t)
	sender := dialTest(t)
	sender.spawn("eng-platform-beetle-ox", "eng-platform", "Beetle-Ox", `echo TOKEN=$ATERM_SESSION_TOKEN; sleep 900`)
	token := sender.token()

	cwd, err := os.MkdirTemp("/tmp", "aterm-live-busy-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(cwd)
	name := "frontend-eng-imp-dragonfly"
	target := dialTest(t)
	if _, err := target.c.request(frame{
		Type: "spawn", Session: name, Role: "frontend-eng", Identity: "Imp-Dragonfly", Seat: "claude",
		Argv: []string{bin}, Env: seatEnv(), Cwd: cwd, Rows: 40, Cols: 120,
	}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	go func() {
		for {
			if _, err := target.c.read(); err != nil {
				return
			}
		}
	}()

	task := "Wait about two minutes inside ONE foreground Bash tool call, for example with a long ping, " +
		"so you stay inside a single long-running tool call. When it returns, reply with the single word waited."
	if state := sendWaiting(t, token, name, task, 120); state.State != "delivered" {
		t.Fatalf("the task: %+v", state)
	}
	if !waitScreen(t, name, "Bash(", 90*time.Second) {
		t.Fatal("the seat never started its Bash call")
	}

	filler := func(count int) string {
		words := make([]string, count)
		for index := range words {
			words[index] = fmt.Sprintf("w%05d", index)
		}
		return strings.Join(words, " ")
	}
	bodies := []string{
		"Busy probe one, ignore it and reply with nothing. " + filler(20) + " END-ONE-4417",
		"Busy probe two, ignore it and reply with nothing.\n" + strings.Repeat(filler(10)+"\n", 90) + "END-TWO-8826",
		"Busy probe three, ignore it and reply with nothing. " + filler(2900) + " END-THREE-2093",
	}
	for index, body := range bodies {
		state := sendWaiting(t, token, name, body, 60)
		t.Logf("probe %d: %d bytes, sent while the seat was in its Bash call, state %s %q",
			index+1, len(body), state.State, state.Reason)
		if state.State == "failed" {
			t.Errorf("probe %d failed: %q", index+1, state.Reason)
		}
	}

	deadline := time.Now().Add(6 * time.Minute)
	for ; time.Now().Before(deadline); time.Sleep(5 * time.Second) {
		raw := liveTranscript(t, config)
		missing := 0
		for _, body := range bodies {
			if !bytes.Contains(raw, jsonText("[from eng-platform Beetle-Ox] "+body)) {
				missing++
			}
		}
		if missing == 0 {
			for index, body := range bodies {
				sum := sha256.Sum256([]byte("[from eng-platform Beetle-Ox] " + body))
				t.Logf("probe %d byte-identical in the seat's transcript, sha256 %x", index+1, sum[:8])
			}
			return
		}
	}
	t.Fatal("the seat's transcript never held every message byte for byte")
}

// waitScreen polls a session's screen for text.
func waitScreen(t *testing.T, name, text string, within time.Duration) bool {
	t.Helper()
	for deadline := time.Now().Add(within); time.Now().Before(deadline); time.Sleep(time.Second) {
		asker, err := dialDaemon(false)
		if err != nil {
			t.Fatal(err)
		}
		reply, err := asker.request(frame{Type: "status", Target: name, Lines: 60})
		_ = asker.Close()
		if err == nil && strings.Contains(strings.Join(reply.Status.Screen, "\n"), text) {
			return true
		}
	}
	return false
}

// liveTranscript is the newest transcript of a seat this test started.
func liveTranscript(t *testing.T, config string) []byte {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join(config, "projects", "*aterm-live-busy-*", "*.jsonl"))
	var newest string
	var when time.Time
	for _, file := range files {
		if info, err := os.Stat(file); err == nil && info.ModTime().After(when) {
			newest, when = file, info.ModTime()
		}
	}
	if newest == "" {
		return nil
	}
	raw, _ := os.ReadFile(newest)
	return raw
}

// jsonText is text as it is written inside a JSON string, quotes dropped.
func jsonText(text string) []byte {
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(text)
	encoded := bytes.TrimSpace(out.Bytes())
	return encoded[1 : len(encoded)-1]
}
