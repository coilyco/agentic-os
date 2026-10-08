package main

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

// Real opencode 1.18 startup output (COI-2129), base64 in JSON so text hooks skip it.
// Paste mode is on at byte 231, "Ask anything" paints at 8655.
const opencodeStartup = "testdata/opencode-startup.json"

func recordedOpencode(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile(opencodeStartup)
	if err != nil {
		t.Fatal(err)
	}
	var recording struct {
		Bytes []byte `json:"bytes"`
	}
	if err := json.Unmarshal(raw, &recording); err != nil {
		t.Fatal(err)
	}
	return recording.Bytes
}

// replay feeds the recording to a fresh opencode seat in size-byte chunks, and reports
// whether it read as ready with paste on and no prompt yet.
func replay(t *testing.T, size int, now time.Time) (readyWithoutPrompt, pasteWithoutPrompt bool, seat *ptySession) {
	t.Helper()
	recorded := recordedOpencode(t)
	if !bytes.Contains(recorded, promptMarks["opencode"]) {
		t.Fatal("the recording no longer holds opencode's prompt mark")
	}
	seat = &ptySession{seat: "opencode", started: now.Add(-10 * time.Second), lastOutput: now}
	for start := 0; start < len(recorded); start += size {
		end := min(start+size, len(recorded))
		seat.scanModes(recorded[start:end])
		seat.scanPrompt(recorded[start:end], now)
		seat.pasteSeen = seat.pasteSeen || seat.paste
		if seat.pasteSeen && seat.promptSeen.IsZero() {
			pasteWithoutPrompt = true
			readyWithoutPrompt = readyWithoutPrompt || seat.ready(now)
		}
	}
	return readyWithoutPrompt, pasteWithoutPrompt, seat
}

// Paste is on about 8KB of repaint before opencode paints its prompt and drops a paste
// typed in between, so the paste rule alone is the message typed but never submitted.
func TestRecordedOpencodeStartupIsNotReadyUntilThePromptPaints(t *testing.T) {
	for _, size := range []int{1, 7, 512, 4096} {
		now := time.Now()
		readyWithoutPrompt, pasteWithoutPrompt, seat := replay(t, size, now)
		if !pasteWithoutPrompt {
			t.Fatalf("chunks of %d: paste mode should be on before the mark", size)
		}
		if readyWithoutPrompt {
			t.Fatalf("chunks of %d: ready with paste on and no prompt yet, so a message would be dropped", size)
		}
		if seat.promptSeen.IsZero() {
			t.Fatalf("chunks of %d: the mark in the recording was not found", size)
		}
		if seat.ready(now.Add(promptSettle / 2)) {
			t.Fatalf("chunks of %d: ready before the input had its settle beat", size)
		}
		if !seat.ready(now.Add(promptSettle + time.Millisecond)) {
			t.Fatalf("chunks of %d: not ready after the prompt painted and settled", size)
		}
	}
}

// The recording stays free of anything but the seat's own screen.
func TestRecordedOpencodeStartupHoldsNoHostPath(t *testing.T) {
	recorded := recordedOpencode(t)
	for _, private := range []string{"/Users/", "/private/var", "sk-", "Bearer "} {
		if strings.Contains(string(recorded), private) {
			t.Fatalf("the recording holds %q", private)
		}
	}
}

// TestLiveOpencode sends a real opencode seat a multi-line message (one model turn).
// ATERM_LIVE_OPENCODE=<binary> runs it, ATERM_LIVE_RECORD=<file> refreshes the recording.
func TestLiveOpencode(t *testing.T) {
	bin := os.Getenv("ATERM_LIVE_OPENCODE")
	if bin == "" {
		t.Skip("set ATERM_LIVE_OPENCODE to an opencode binary")
	}
	testDaemon(t)
	sender := dialTest(t)
	sender.spawn("eng-platform-beetle-ox", "eng-platform", "Beetle-Ox", `echo TOKEN=$ATERM_SESSION_TOKEN; sleep 300`)
	token := sender.token()
	target := dialTest(t)
	cwd := t.TempDir()
	if record := os.Getenv("ATERM_LIVE_RECORD"); record != "" {
		cwd = "/tmp/aterm-opencode-fixture"
		_ = os.MkdirAll(cwd, 0o700)
	}
	if _, err := target.c.request(frame{
		Type: "spawn", Session: "eng-junior-beetle-ox", Role: "eng-junior", Identity: "Beetle-Ox", Seat: "opencode",
		Argv: []string{bin}, Env: os.Environ(), Cwd: cwd, Rows: 40, Cols: 120,
	}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if record := os.Getenv("ATERM_LIVE_RECORD"); record != "" {
		deadline := time.Now().Add(8 * time.Second)
		for time.Now().Before(deadline) {
			_ = target.c.raw.SetReadDeadline(deadline)
			message, err := target.c.read()
			if err != nil {
				break
			}
			if message.Type == "output" {
				target.output.Write(message.Data)
			}
		}
		saved, _ := json.MarshalIndent(map[string]any{
			"source": "opencode seat, first 8s of output, COI-2129", "captured": time.Now().UTC().Format("2006-01-02"), "bytes": target.output.Bytes(),
		}, "", "  ")
		if err := os.WriteFile(record, append(saved, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	state := sendAs(t, token, "eng-junior", "first line\nsecond line\nReply with the single word OK.")
	if state.State == "failed" {
		t.Fatalf("send failed: %s", state.Reason)
	}
	for deadline := time.Now().Add(40 * time.Second); ; time.Sleep(time.Second) {
		asker, err := dialDaemon(false)
		if err != nil {
			t.Fatal(err)
		}
		reply, err := asker.request(frame{Type: "status", Target: "eng-junior-beetle-ox", Lines: 40})
		_ = asker.Close()
		if err != nil {
			t.Fatal(err)
		}
		screen := strings.Join(reply.Status.Screen, "\n")
		// The turn footer appears only once opencode took the message as a prompt.
		if reply.Status.Pending == 0 && strings.Contains(screen, "second line") && strings.Contains(screen, "▣") {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the message was never submitted (pending %d):\n%s", reply.Status.Pending, screen)
		}
	}
}
