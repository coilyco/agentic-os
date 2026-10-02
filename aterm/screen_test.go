package main

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func screenOf(rows, cols int, writes ...string) *screen {
	s := newScreen(rows, cols)
	for _, w := range writes {
		s.write([]byte(w))
	}
	return s
}

func TestScreenPlacesTextAndLineBreaks(t *testing.T) {
	got := screenOf(5, 20, "hello\r\nworld").text()
	if want := []string{"hello", "world"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// A TUI repaints by moving up and erasing down, so the old frame must not
// survive under the new one. This is what stripping the escapes cannot do.
func TestScreenRepaintReplacesTheOldFrame(t *testing.T) {
	s := screenOf(6, 30, "Do you want to proceed?\r\n1. Yes\r\n2. No", "\x1b[2A\r\x1b[J", "working...")
	if got, want := s.text(), []string{"working..."}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestScreenEraseLineAndCells(t *testing.T) {
	s := screenOf(3, 20, "abcdef", "\r\x1b[2K", "xy")
	if got := s.text(); !reflect.DeepEqual(got, []string{"xy"}) {
		t.Fatalf("erase line: %q", got)
	}
	s = screenOf(3, 20, "abcdef", "\r\x1b[3C\x1b[K")
	if got := s.text(); !reflect.DeepEqual(got, []string{"abc"}) {
		t.Fatalf("erase to end: %q", got)
	}
}

func TestScreenCursorAddressing(t *testing.T) {
	s := screenOf(5, 20, "\x1b[3;5Hhere")
	got := s.text()
	if len(got) != 3 || got[2] != "    here" {
		t.Fatalf("got %q", got)
	}
}

func TestScreenScrollsOffTheTop(t *testing.T) {
	s := screenOf(3, 10, "1\r\n2\r\n3\r\n4\r\n5")
	if got, want := s.text(), []string{"3", "4", "5"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestScreenWrapsAtTheRightEdge(t *testing.T) {
	s := screenOf(4, 5, "abcdefgh")
	if got, want := s.text(), []string{"abcde", "fgh"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestScreenWideRunesTakeTwoCells(t *testing.T) {
	s := screenOf(2, 10, "a世b")
	if got := s.text(); !reflect.DeepEqual(got, []string{"a世b"}) {
		t.Fatalf("got %q", got)
	}
	if s.col != 4 {
		t.Fatalf("cursor at %d, want 4", s.col)
	}
}

func TestScreenAlternateScreenRestoresTheMainOne(t *testing.T) {
	s := screenOf(4, 20, "main text", "\x1b[?1049h", "\x1b[Hfull screen app")
	if got := s.text(); !reflect.DeepEqual(got, []string{"full screen app"}) {
		t.Fatalf("on the alternate screen: %q", got)
	}
	s.write([]byte("\x1b[?1049l"))
	if got := s.text(); !reflect.DeepEqual(got, []string{"main text"}) {
		t.Fatalf("after leaving it: %q", got)
	}
}

func TestScreenInsertAndDeleteLines(t *testing.T) {
	s := screenOf(4, 10, "a\r\nb\r\nc", "\x1b[2;1H\x1b[L", "X")
	if got, want := s.text(), []string{"a", "X", "b", "c"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("insert: got %q, want %q", got, want)
	}
	s.write([]byte("\x1b[2;1H\x1b[M"))
	if got, want := s.text(), []string{"a", "b", "c"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("delete: got %q, want %q", got, want)
	}
}

func TestScreenIgnoresColorAndOtherSequences(t *testing.T) {
	s := screenOf(3, 40, "\x1b[1;31mred\x1b[0m \x1b]0;a title\x07ok\x1b[?25l")
	if got := s.text(); !reflect.DeepEqual(got, []string{"red ok"}) {
		t.Fatalf("got %q", got)
	}
}

func TestScreenSequenceSplitAcrossWrites(t *testing.T) {
	s := screenOf(4, 20, "abc\x1b[", "2Dxy")
	if got := s.text(); !reflect.DeepEqual(got, []string{"axy"}) {
		t.Fatalf("got %q", got)
	}
}

func TestScreenResizeKeepsWhatFits(t *testing.T) {
	s := screenOf(4, 10, "keep\r\nthis")
	s.resize(2, 6)
	if got, want := s.text(), []string{"keep", "this"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	s.write([]byte("\r\n\r\nmore"))
	if len(s.text()) > 2 {
		t.Fatalf("screen grew past its rows: %q", s.text())
	}
}

func TestClassifyReadsThePromptBeforeTheClock(t *testing.T) {
	prompt := []string{"Bash command", "  git push", "Do you want to proceed?", "❯ 1. Yes", "  2. No", "Esc to cancel"}
	cases := []struct {
		name  string
		seat  string
		lines []string
		ready bool
		quiet time.Duration
		want  string
	}{
		{"not ready is starting, whatever is drawn", "claude", prompt, false, time.Hour, stateStarting},
		{"a permission prompt holds even while output is fresh", "claude", prompt, true, time.Second, statePrompt},
		{"a choice card is a prompt", "claude", []string{"Which one?", "Enter to select · ↑/↓ to navigate · Esc to cancel"}, true, time.Minute, statePrompt},
		{"codex approval", "codex", []string{"Would you like to run the following command?", "Yes, proceed (y)"}, true, time.Minute, statePrompt},
		{"recent output is busy", "claude", []string{"✻ Thinking…"}, true, time.Second, stateBusy},
		{"a silent tool with the interrupt hint is still busy", "claude", []string{"✻ Running… (45s · esc to interrupt)"}, true, time.Minute, stateBusy},
		{"the spinner line a live claude shows while a tool runs", "claude", []string{"✽ Grounding… (12m 25s · ↓ 44.5k tokens)", "❯"}, true, time.Minute, stateBusy},
		{"quiet with an empty box is idle", "claude", []string{"╭───╮", "│ > │", "╰───╯", "  ? for shortcuts"}, true, time.Minute, stateIdle},
		{"an unknown seat has no marks, only the clock", "other", prompt, true, time.Minute, stateIdle},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := classify(c.seat, c.lines, c.ready, c.quiet); got != c.want {
				t.Fatalf("classify = %q, want %q", got, c.want)
			}
		})
	}
}

// A prompt that was answered must stop counting: the screen no longer shows it.
func TestClassifyStopsSeeingAnAnsweredPrompt(t *testing.T) {
	s := screenOf(8, 40, "Do you want to proceed?\r\n1. Yes\r\nEsc to cancel")
	if got := classify("claude", s.text(), true, time.Minute); got != statePrompt {
		t.Fatalf("before the answer: %q", got)
	}
	s.write([]byte("\x1b[3A\r\x1b[J"))
	s.write([]byte(strings.Repeat("done\r\n", 2)))
	if got := classify("claude", s.text(), true, time.Minute); got != stateIdle {
		t.Fatalf("after the answer: %q", got)
	}
}

// The state a client lists is read off a real session's screen: held on a
// prompt while it shows, and no longer once the program repaints over it.
func TestListedStateFollowsTheScreen(t *testing.T) {
	testDaemon(t)
	tc := dialTest(t)
	script := `printf '\033[?2004h'; printf 'Do you want to proceed?\r\n1. Yes\r\nEsc to cancel\r\n'; ` +
		`sleep 3; printf '\033[3A\033[Jall done\r\n'; exec cat`
	tc.spawn("state-test", "eng-platform", "Beetle-Ox", script)
	stateOf := func() string {
		reply, err := tc.c.request(frame{Type: "list"})
		if err != nil || len(reply.Sessions) != 1 {
			t.Fatalf("list: %+v %v", reply, err)
		}
		return reply.Sessions[0].State
	}
	waitFor(t, "a prompt state", 8*time.Second, func() bool { return stateOf() == statePrompt })
	waitFor(t, "the prompt to clear", 8*time.Second, func() bool { return stateOf() != statePrompt })
	if got := stateOf(); got != stateBusy {
		t.Fatalf("just after a repaint the session is %q, want %q", got, stateBusy)
	}
}

func TestScreenScrollRegionScrollsOnlyItsRows(t *testing.T) {
	s := screenOf(6, 12, "head\r\nrow1\r\nrow2\r\nrow3\r\nrow4\r\nfoot", "\x1b[2;5r", "\x1b[1S")
	if got, want := s.text(), []string{"head", "row2", "row3", "row4", "", "foot"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	s.write([]byte("\x1b[1T"))
	if got, want := s.text(), []string{"head", "", "row2", "row3", "row4", "foot"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("scroll down: got %q, want %q", got, want)
	}
}

// A line feed at the bottom margin scrolls the region, not the whole screen.
func TestScreenLineFeedAtTheBottomMarginScrollsTheRegion(t *testing.T) {
	s := screenOf(5, 10, "top\x1b[5;1Hfoot", "\x1b[2;4r", "\x1b[4;1Hold\r\nnew")
	if got, want := s.text(), []string{"top", "", "old", "new", "foot"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestScreenResetMarginsRestoreFullScreenScrolling(t *testing.T) {
	s := screenOf(3, 10, "\x1b[2;3r", "\x1b[r", "a\r\nb\r\nc\r\nd")
	if got, want := s.text(), []string{"b", "c", "d"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// The repaint a current claude does: synchronized output, home, big downward
// moves, a column jump, an erase to the end of the line.
func TestScreenClaudeStyleRepaint(t *testing.T) {
	s := screenOf(10, 40, "\x1b[?2026h\x1b[?25l\x1b[H\r\x1b[5Bold status\x1b[?25h\x1b[?2026l")
	s.write([]byte("\x1b[?2026h\x1b[H\r\x1b[5B\x1b[2Knew\x1b[8Gstatus\x1b[K\x1b[?2026l"))
	got := s.text()
	if len(got) != 6 || got[5] != "new    status" {
		t.Fatalf("got %q", got)
	}
}
