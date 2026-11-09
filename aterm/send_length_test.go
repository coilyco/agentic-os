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

// COI-2619: a message at the limit arrives whole and one over is refused by name.
// Before the limit, about 3 MiB severed the target's holder link.
func TestSendRefusesAMessageOverTheLimitByNameAndCarriesOneAtIt(t *testing.T) {
	testDaemon(t)
	sender := dialTest(t)
	sender.spawn("eng-platform-beetle-ox", "eng-platform", "Beetle-Ox", `echo TOKEN=$ATERM_SESSION_TOKEN; sleep 60`)
	token := sender.token()
	body := longLine(maxSendBody)
	want := "\x1b[200~" + envelope("eng-platform", "Beetle-Ox", body) + "\x1b[201~" + "\r"
	got := filepath.Join(t.TempDir(), "read.bin")
	target := dialTest(t)
	target.spawn("frontend-eng-imp-dragonfly", "frontend-eng", "Imp-Dragonfly", fmt.Sprintf(
		`printf 'READY\033[?2004h\n'; stty raw -echo; sleep 1; head -c %d > %s; echo DONE`, len(want), got))
	target.until("READY")

	for _, size := range []int{maxSendBody + 1, 3 << 20} {
		c, err := dialDaemon(false)
		if err != nil {
			t.Fatal(err)
		}
		_, err = c.request(frame{Type: "send", Token: token, Target: "frontend-eng-imp-dragonfly", Body: longLine(size)})
		c.Close()
		if err == nil || !strings.Contains(err.Error(), "256 KiB") || !strings.Contains(err.Error(), "send the path") {
			t.Fatalf("a %d byte message read as %v, want a refusal naming 256 KiB", size, err)
		}
	}
	if state := sendAs(t, token, "frontend-eng-imp-dragonfly", body); state.State != "delivered" {
		t.Fatalf("a message at the limit read as %+v, want delivered", state)
	}
	target.until("DONE")
	read, err := os.ReadFile(got)
	if err != nil || string(read) != want {
		t.Fatalf("the target read %d bytes of %d, err %v", len(read), len(want), err)
	}
}

func TestSendMessageRefusesAnOverLongBodyBeforeDialing(t *testing.T) {
	t.Setenv(sessionTokenEnv, "unused")
	t.Setenv(daemonSocketEnv, filepath.Join(t.TempDir(), "absent.sock"))
	_, err := sendMessage("eng-platform", strings.Repeat("x", maxSendBody+1), sendOptions{})
	if err == nil || !strings.Contains(err.Error(), "256 KiB") {
		t.Fatalf("an over-long message read as %v, want the named limit before any dial", err)
	}
	// At the limit it is not refused for length, so it fails on the missing daemon.
	if _, err = sendMessage("eng-platform", strings.Repeat("x", maxSendBody), sendOptions{}); err == nil || strings.Contains(err.Error(), "256 KiB") {
		t.Fatalf("a message at the limit read as %v, want a dial failure", err)
	}
}
