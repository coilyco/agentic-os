package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// fakeBoxCommand plays a claude seat that boots late and drops the first N Enters
// (COI-2474). Args: boot milliseconds, Enters to drop.
const fakeBoxCommand = "fake-claude-box"

func runFakeBox() {
	boot, _ := strconv.Atoi(os.Args[2])
	drops, _ := strconv.Atoi(os.Args[3])
	fmt.Print("\x1b[?2004h")
	time.Sleep(time.Duration(boot) * time.Millisecond)
	var sent []string
	var input strings.Builder
	draw := func(placeholder string) {
		fmt.Print("\x1b[2J\x1b[H")
		for _, line := range sent {
			fmt.Printf("> %s\r\n", line)
		}
		rule := strings.Repeat("─", 60)
		fmt.Printf("%s\r\n❯ %s%s\r\n%s\r\n  auto mode on\r\n", rule, input.String(), placeholder, rule)
	}
	draw(` Try "how do I log an error?"`)
	reader := bufio.NewReader(os.Stdin)
	inPaste := false
	for {
		b, err := reader.ReadByte()
		if err != nil {
			return
		}
		switch {
		case b == 0x1b:
			seq := make([]byte, 5)
			n := 0
			for ; n < len(seq); n++ {
				if seq[n], err = reader.ReadByte(); err != nil {
					return
				}
				if seq[n] == '~' {
					n++
					break
				}
			}
			inPaste = string(seq[:n]) == "[200~"
			if !inPaste {
				draw("")
			}
		case b == '\r' && !inPaste:
			if drops > 0 {
				drops--
				continue
			}
			sent = append(sent, input.String())
			fmt.Printf("SUBMITTED %s\r\n", input.String())
			input.Reset()
			draw("")
		default:
			input.WriteByte(b)
			if !inPaste {
				draw("")
			}
		}
	}
}

func fakeBoxSeat(t *testing.T, bootMillis, drops int) (token string, target *testClient) {
	t.Helper()
	testDaemon(t)
	sender := dialTest(t)
	sender.spawn("eng-platform-beetle-ox", "eng-platform", "Beetle-Ox", `echo TOKEN=$ATERM_SESSION_TOKEN; sleep 60`)
	token = sender.token()
	target = dialTest(t)
	_, err := target.c.request(frame{
		Type: "spawn", Session: "eng-junior-beetle-ox", Role: "eng-junior", Identity: "Beetle-Ox", Seat: "claude",
		Argv: []string{"/bin/sh", "-c", fmt.Sprintf("stty raw -echo; exec %s %s %d %d", os.Args[0], fakeBoxCommand, bootMillis, drops)},
		Env:  os.Environ(), Cwd: "/", Rows: 24, Cols: 200,
	})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	return token, target
}

// The seat is not up when the message is sent and drops the first Enter after it
// comes up. The daemon must still get the text submitted, and report delivered only then.
func TestMessageQueuedForAStartingSeatIsSubmittedOnceItsPromptIsUp(t *testing.T) {
	token, target := fakeBoxSeat(t, 2500, 1)
	state := sendWaiting(t, token, "eng-junior", "do the thing", 1)
	if state.State != "queued" {
		t.Fatalf("state = %+v, want queued while the seat is starting", state)
	}
	// Sends to one seat deliver in order, so the second one's receipt follows the first's.
	sendWaiting(t, token, "eng-junior", "second thing", 30)
	// Alone: a dropped Enter left unretried would submit it glued to the second message.
	target.until("SUBMITTED [from eng-platform Beetle-Ox] do the thing\r\n")
}

// A seat that never takes Enter must not be reported as delivered.
func TestMessageTypedButNeverSubmittedIsNotDelivered(t *testing.T) {
	defer func(w time.Duration, r int) { submitWindow, enterRetries = w, r }(submitWindow, enterRetries)
	submitWindow, enterRetries = 300*time.Millisecond, 2
	token, target := fakeBoxSeat(t, 200, 100)
	state := sendWaiting(t, token, "eng-junior", "do the thing", 30)
	if state.State != "failed" || !strings.Contains(state.Reason, "not Enter") {
		t.Fatalf("state = %+v, want failed naming the unsent text", state)
	}
	if strings.Contains(target.output.String(), "SUBMITTED") {
		t.Fatal("the fake took an Enter it should have dropped")
	}
}

func TestBoxHoldsReadsOnlyClaudesInputBox(t *testing.T) {
	rule := strings.Repeat("─", 40)
	typed := []string{"> earlier", rule, "❯ [from eng-platform Beetle-Ox] do the", "  thing now please", rule, "  auto mode on"}
	text := "[from eng-platform Beetle-Ox] do the thing now please"
	if readable, holds := boxHolds("claude", typed, text); !readable || !holds {
		t.Fatal("text wrapped inside the box is still waiting")
	}
	sent := []string{"> [from eng-platform Beetle-Ox] do the thing now please", "", rule, "❯", rule, "  esc to interrupt"}
	if readable, holds := boxHolds("claude", sent, text); !readable || holds {
		t.Fatal("the same words in the transcript above an empty box are a message sent")
	}
	folded := []string{rule, "❯ [Pasted text #1 +20 lines]", rule}
	if _, holds := boxHolds("claude", folded, text); !holds {
		t.Fatal("a folded paste is still waiting")
	}
	if readable, holds := boxHolds("claude", []string{rule, "❯ " + text}, text); readable || holds {
		t.Fatal("a screen caught with half its box is unknown, not unsent and not sent")
	}
	if readable, holds := boxHolds("codex", typed, text); readable || holds {
		t.Fatal("only claude's box is read")
	}
}
