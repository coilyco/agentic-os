package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Real opencode 1.18.35 output (COI-2532), base64 in JSON so text hooks skip it. Each
// screen is the raw output from spawn to one state and extends the one before it.
const opencodeBoxScreens = "testdata/opencode-box.json"

const (
	boxRows, boxCols = 40, 120
	// opencodeMultiline is what the recordings' multi-line screens were typed from.
	opencodeMultiline = "[from eng-platform Beetle-Ox] first line\nsecond line\nReply with the single word OK."
)

func loadOpencodeBox(path string) (map[string][]byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var recording struct {
		Screens map[string][]byte `json:"screens"`
	}
	if err := json.Unmarshal(raw, &recording); err != nil {
		return nil, err
	}
	return recording.Screens, nil
}

func opencodeBoxLines(t *testing.T, name string) []string {
	t.Helper()
	screens, err := loadOpencodeBox(opencodeBoxScreens)
	if err != nil {
		t.Fatal(err)
	}
	recorded, ok := screens[name]
	if !ok {
		t.Fatalf("no %q screen in the recording", name)
	}
	scr := newScreen(boxRows, boxCols)
	scr.write(recorded)
	return scr.text()
}

func TestOpencodeBoxScreensHoldNoHostPath(t *testing.T) {
	screens, err := loadOpencodeBox(opencodeBoxScreens)
	if err != nil {
		t.Fatal(err)
	}
	for name, recorded := range screens {
		for _, private := range []string{"/Users/", "/private/var", "sk-", "Bearer "} {
			if bytes.Contains(recorded, []byte(private)) {
				t.Fatalf("%s holds %q", name, private)
			}
		}
	}
}

func TestBoxHoldsReadsOpencodesInputBox(t *testing.T) {
	short := "[from eng-platform Beetle-Ox] do the thing"
	cases := []struct {
		screen, text    string
		readable, holds bool
		why             string
	}{
		{"empty", opencodeMultiline, true, false, "a fresh box holds nothing"},
		{"unsent_short", short, true, true, "short text typed and not submitted is still in the box"},
		{"unsent_folded_multiline", opencodeMultiline, true, true, "a multi-line paste folds to a placeholder that is still in the box"},
		{"unsent_folded_long_line", short + " and a good deal more words", true, true, "one long line folds too"},
		{"sent", opencodeMultiline, true, false, "the same words in the transcript above an empty box are a message sent"},
	}
	for _, c := range cases {
		readable, holds := boxHolds("opencode", opencodeBoxLines(t, c.screen), c.text)
		if readable != c.readable || holds != c.holds {
			t.Errorf("%s: readable=%v holds=%v, want %v %v: %s", c.screen, readable, holds, c.readable, c.holds, c.why)
		}
	}
	// A screen with no input box on it, as when the window is too short, is unknown.
	if readable, holds := boxHolds("opencode", opencodeBoxLines(t, "sent")[:12], opencodeMultiline); readable || holds {
		t.Error("the transcript alone is unknown, not unsent and not sent")
	}
}

// fakeOpencodeCommand replays the recording. Args: its path, then Enters to drop.
const fakeOpencodeCommand = "fake-opencode-box"

// runFakeOpencode paints the startup screen, the folded paste once a paste ends, and
// the sent screen on the first Enter it does not drop.
func runFakeOpencode() {
	screens, err := loadOpencodeBox(os.Args[2])
	if err != nil {
		fmt.Println(err)
		return
	}
	drops, _ := strconv.Atoi(os.Args[3])
	empty, typed, sent := screens["empty"], screens["unsent_folded_multiline"], screens["sent"]
	os.Stdout.Write(empty)
	buf := make([]byte, 4096)
	var seen []byte
	pasted, submitted := false, false
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil {
			return
		}
		seen = append(seen, buf[:n]...)
		if !pasted && bytes.Contains(seen, []byte("\x1b[201~")) {
			pasted = true
			os.Stdout.Write(typed[len(empty):])
			seen = seen[bytes.Index(seen, []byte("\x1b[201~"))+6:]
		}
		for pasted && !submitted && bytes.IndexByte(seen, '\r') >= 0 {
			seen = seen[bytes.IndexByte(seen, '\r')+1:]
			if drops > 0 {
				drops--
				continue
			}
			submitted = true
			os.Stdout.Write(sent[len(typed):])
		}
	}
}

func fakeOpencodeSeat(t *testing.T, drops int) string {
	t.Helper()
	recording, err := filepath.Abs(opencodeBoxScreens)
	if err != nil {
		t.Fatal(err)
	}
	testDaemon(t)
	sender := dialTest(t)
	sender.spawn("eng-platform-beetle-ox", "eng-platform", "Beetle-Ox", `echo TOKEN=$ATERM_SESSION_TOKEN; sleep 60`)
	token := sender.token()
	target := dialTest(t)
	_, err = target.c.request(frame{
		Type: "spawn", Session: "eng-junior-beetle-ox", Role: "eng-junior", Identity: "Beetle-Ox", Seat: "opencode",
		Argv: []string{"/bin/sh", "-c", fmt.Sprintf("stty raw -echo; exec %s %s %s %d", os.Args[0], fakeOpencodeCommand, recording, drops)},
		Env:  os.Environ(), Cwd: "/", Rows: boxRows, Cols: boxCols,
	})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	return token
}

// The recording's turn footer paints only once opencode took the message as a prompt.
const opencodeTookIt = "▣"

// seatScreen is what the daemon's screen for the seat shows. The attached client goes
// unread, as one idle through the send falls behind a screen this size.
func seatScreen(t *testing.T) string {
	t.Helper()
	asker, err := dialDaemon(false)
	if err != nil {
		t.Fatal(err)
	}
	defer asker.Close()
	reply, err := asker.request(frame{Type: "status", Target: "eng-junior-beetle-ox", Lines: boxRows})
	if err != nil {
		t.Fatal(err)
	}
	return strings.Join(reply.Status.Screen, "\n")
}

// An opencode seat that drops the first Enter gets it again, and delivered follows.
func TestOpencodeMessageWhoseFirstEnterIsDroppedIsSubmitted(t *testing.T) {
	token := fakeOpencodeSeat(t, 1)
	body := strings.TrimPrefix(opencodeMultiline, "[from eng-platform Beetle-Ox] ")
	state := sendWaiting(t, token, "eng-junior", body, 30)
	if state.State != "delivered" {
		t.Fatalf("state = %+v, want delivered", state)
	}
	if !strings.Contains(seatScreen(t), opencodeTookIt) {
		t.Fatal("delivered, but the seat never took the message")
	}
}

// An opencode seat that never takes Enter must not be reported as delivered.
func TestOpencodeMessageTypedButNeverSubmittedIsNotDelivered(t *testing.T) {
	defer func(w time.Duration, r int) { submitWindow, enterRetries = w, r }(submitWindow, enterRetries)
	submitWindow, enterRetries = 300*time.Millisecond, 2
	token := fakeOpencodeSeat(t, 100)
	body := strings.TrimPrefix(opencodeMultiline, "[from eng-platform Beetle-Ox] ")
	state := sendWaiting(t, token, "eng-junior", body, 30)
	if state.State != "failed" || !strings.Contains(state.Reason, "not Enter") {
		t.Fatalf("state = %+v, want failed naming the unsent text", state)
	}
	if strings.Contains(seatScreen(t), opencodeTookIt) {
		t.Fatal("the fake took an Enter it should have dropped")
	}
}
