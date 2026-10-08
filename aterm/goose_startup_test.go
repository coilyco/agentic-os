package main

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

// Real goose 1.52.0 startup output (COI-2003), base64 in JSON so text hooks skip it.
// Paste mode turns on right before the "Enter to send" hint, after the banner.
const gooseStartup = "testdata/goose-startup.json"

func recordedGoose(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile(gooseStartup)
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

// Goose is a watched seat: a quiet screen at a gate is not ready, but the DECSET 2004 it
// prints at its live prompt is, however the output is split into reads.
func TestRecordedGooseStartupIsReadyOnceBracketedPasteIsOn(t *testing.T) {
	recorded := recordedGoose(t)
	if !bytes.Contains(recorded, []byte("\x1b[?2004h")) {
		t.Fatal("the recording no longer holds goose turning bracketed paste on")
	}
	for _, size := range []int{1, 7, 512, 4096} {
		now := time.Now()
		seat := &ptySession{seat: "goose", started: now.Add(-time.Hour), lastOutput: now.Add(-time.Hour)}
		if seat.ready(now) {
			t.Fatalf("chunks of %d: a goose seat sitting quiet before any output is not ready", size)
		}
		for start := 0; start < len(recorded); start += size {
			end := min(start+size, len(recorded))
			seat.scanModes(recorded[start:end])
			seat.lastOutput = now.Add(-time.Hour)
		}
		if !seat.paste || !seat.pasteSeen {
			t.Fatalf("chunks of %d: paste=%v pasteSeen=%v after the recorded startup", size, seat.paste, seat.pasteSeen)
		}
		if !seat.ready(now) {
			t.Fatalf("chunks of %d: not ready though goose turned paste on", size)
		}
	}
}

// The recording stays free of anything but the seat's own screen.
func TestRecordedGooseStartupHoldsNoHostPath(t *testing.T) {
	recorded := recordedGoose(t)
	for _, private := range []string{"/Users/", "/private/var", "sk-", "Bearer "} {
		if strings.Contains(string(recorded), private) {
			t.Fatalf("the recording holds %q", private)
		}
	}
}

// TestLiveGoose sends a real goose seat a multi-line message and reads if it took it.
// ATERM_LIVE_GOOSE=<binary> runs it, ATERM_LIVE_RECORD=<file> refreshes the recording.
func TestLiveGoose(t *testing.T) {
	bin := os.Getenv("ATERM_LIVE_GOOSE")
	if bin == "" {
		t.Skip("set ATERM_LIVE_GOOSE to a goose binary")
	}
	testDaemon(t)
	sender := dialTest(t)
	sender.spawn("eng-platform-beetle-ox", "eng-platform", "Beetle-Ox", `echo TOKEN=$ATERM_SESSION_TOKEN; sleep 300`)
	token := sender.token()
	target := dialTest(t)
	cwd := t.TempDir()
	if record := os.Getenv("ATERM_LIVE_RECORD"); record != "" {
		cwd = "/tmp/aterm-goose-fixture"
		_ = os.MkdirAll(cwd, 0o700)
	}
	if _, err := target.c.request(frame{
		Type: "spawn", Session: "senior-sysadmin-beetle-ox", Role: "senior-sysadmin", Identity: "Beetle-Ox", Seat: "goose",
		Argv: []string{bin, "session"}, Env: os.Environ(), Cwd: cwd, Rows: 40, Cols: 120,
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
			"source": "goose seat, first 8s of output, COI-2003", "captured": time.Now().UTC().Format("2006-01-02"), "bytes": target.output.Bytes(),
		}, "", "  ")
		if err := os.WriteFile(record, append(saved, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	state := sendAs(t, token, "senior-sysadmin", "first line\nsecond line\nReply with the single word OK.")
	if state.State == "failed" {
		t.Fatalf("send failed: %s", state.Reason)
	}
	for deadline := time.Now().Add(40 * time.Second); ; time.Sleep(time.Second) {
		asker, err := dialDaemon(false)
		if err != nil {
			t.Fatal(err)
		}
		reply, err := asker.request(frame{Type: "status", Target: "senior-sysadmin-beetle-ox", Lines: 40})
		_ = asker.Close()
		if err != nil {
			t.Fatal(err)
		}
		screen := strings.Join(reply.Status.Screen, "\n")
		if reply.Status.Pending == 0 && strings.Contains(screen, "second line") {
			t.Logf("goose screen after the send:\n%s", screen)
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the message was never submitted (pending %d):\n%s", reply.Status.Pending, screen)
		}
	}
}
