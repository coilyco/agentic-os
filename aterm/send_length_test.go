package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// longLine is one line of n characters in numbered words, so a cut shows which
// word it fell in, ending in a marker no cut keeps.
func longLine(n int) string {
	var line strings.Builder
	for word := 1; line.Len() < n; word++ {
		fmt.Fprintf(&line, "w%04d ", word)
	}
	return strings.TrimRight(line.String()[:n-len("END")], " ") + "END"
}

// COI-2383: daemon, holder and PTY write a long line whole to a target that reads late.
// The harness's own paste handling is outside this test.
func TestDaemonDeliversALongSingleLineWhole(t *testing.T) {
	testDaemon(t)
	sender := dialTest(t)
	sender.spawn("eng-platform-beetle-ox", "eng-platform", "Beetle-Ox", `echo TOKEN=$ATERM_SESSION_TOKEN; sleep 60`)
	token := sender.token()
	for _, size := range []int{2000, 64000} {
		t.Run(fmt.Sprintf("%d chars", size), func(t *testing.T) {
			body := longLine(size)
			if len(body) != size {
				t.Fatalf("fixture is %d chars, want %d", len(body), size)
			}
			// What a paste-capable seat reads: one bracketed paste, then Enter.
			want := "\x1b[200~" + envelope("eng-platform", "Beetle-Ox", body) + "\x1b[201~" + "\r"
			got := filepath.Join(t.TempDir(), "read.bin")
			name := fmt.Sprintf("frontend-eng-imp-dragonfly-%d", size)
			target := dialTest(t)
			target.spawn(name, "frontend-eng", "Imp-Dragonfly", fmt.Sprintf(
				`printf 'READY\033[?2004h\n'; stty raw -echo; sleep 1; head -c %d > %s; echo DONE`, len(want), got))
			target.until("READY")
			if state := sendAs(t, token, name, body); state.State != "delivered" {
				t.Fatalf("state = %+v, want delivered", state)
			}
			target.until("DONE")
			read, err := os.ReadFile(got)
			if err != nil {
				t.Fatalf("read back: %v", err)
			}
			if string(read) != want {
				t.Fatalf("the target read %d bytes, want %d, ending %q", len(read), len(want), string(read[max(0, len(read)-40):]))
			}
		})
	}
}
