package main

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"
)

// listener is a subscriber that records the lines the daemon writes to it.
type listener struct {
	mu    sync.Mutex
	lines []string
}

func (l *listener) conn() *conn {
	return &conn{
		writeLine: func(line []byte) error {
			l.mu.Lock()
			defer l.mu.Unlock()
			l.lines = append(l.lines, string(line))
			return nil
		},
		closer: func() error { return nil },
	}
}

// messages waits for the writer goroutine, then returns the raw `message` lines.
func (l *listener) messages(t *testing.T, count int) []string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		l.mu.Lock()
		got := append([]string(nil), l.lines...)
		l.mu.Unlock()
		if len(got) >= count {
			return got
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("wanted %d frames, got %d", count, len(l.lines))
	return nil
}

func textOf(t *testing.T, line string) (string, bool) {
	t.Helper()
	var decoded frame
	if err := json.Unmarshal([]byte(line), &decoded); err != nil || decoded.Type != "message" || decoded.Message == nil {
		t.Fatalf("not a message frame: %q (%v)", line, err)
	}
	return decoded.Message.Text, decoded.Message.TextCut
}

// COI-3418: the body rides the `message` event only to a subscriber the typing
// guard lets type. The four standings below are the four ways it answers.
func TestMessageEventCarriesTheBodyOnlyToAPeerThatMayType(t *testing.T) {
	d := newDaemon(func(string, ...any) {})
	reads := 0
	d.processes = func() ([]processEntry, error) {
		reads++
		return []processEntry{{PID: 101, PPID: 100}, {PID: 900, PPID: 1}}, nil
	}
	d.sessions["seat"] = &ptySession{name: "seat", pid: 100}
	standings := []struct {
		name     string
		peer     peerStanding
		asserted bool
		sees     bool
	}{
		{"local outside every session", peerStanding{pids: []int{900}}, false, true},
		{"local inside a session", peerStanding{pids: []int{101}}, false, false},
		{"remote device after its unlock", peerStanding{web: true, remote: true}, true, true},
		{"remote device before its unlock", peerStanding{web: true, remote: true}, false, false},
	}
	listeners := make([]*listener, len(standings))
	for i, standing := range standings {
		listeners[i] = &listener{}
		c := listeners[i].conn()
		d.subscribers[c] = &client{c: c, peerStanding: standing.peer, asserted: standing.asserted}
	}
	body := "please read\nthe plan: ship it"
	p := &pendingSend{msg: peerMessage{ID: "m1", From: "eng-platform Beetle-Ox", Target: "seat"}, d: d, done: make(chan struct{})}
	p.shown, p.cut = bodyHead(body)
	p.setState("queued", "")
	for i, standing := range standings {
		line := listeners[i].messages(t, 1)[0]
		text, cut := textOf(t, line)
		want := ""
		if standing.sees {
			want = body
		}
		if text != want || cut {
			t.Errorf("%s: text %q cut %v, want %q", standing.name, text, cut, want)
		}
		if !standing.sees && strings.Contains(line, `"text"`) {
			t.Errorf("%s: the frame names a text field at all: %s", standing.name, line)
		}
	}
	if reads != 1 {
		t.Errorf("the process table was read %d times for one state change, want 1", reads)
	}
	// The sender's `sent` reply and the receipts are built from the snapshot.
	if got := p.snapshot(); got.Text != "" || got.TextCut {
		t.Errorf("the pending message's own state carries a body: %+v", got)
	}
}

func TestBodyHeadCutsALongBodyAtARuneAndSaysSo(t *testing.T) {
	if text, cut := bodyHead("short"); text != "short" || cut {
		t.Fatalf("a short body came back %q cut %v", text, cut)
	}
	exact := strings.Repeat("a", messageTextHead)
	if text, cut := bodyHead(exact); text != exact || cut {
		t.Fatalf("a body at the head size was cut: %d bytes cut %v", len(text), cut)
	}
	// A three-byte rune straddling the limit must not be left half written.
	long := strings.Repeat("a", messageTextHead-1) + "€" + strings.Repeat("b", 100)
	text, cut := bodyHead(long)
	if !cut || !utf8.ValidString(text) || len(text) > messageTextHead || !strings.HasPrefix(long, text) {
		t.Fatalf("cut %v valid %v len %d", cut, utf8.ValidString(text), len(text))
	}
	if text != strings.Repeat("a", messageTextHead-1) {
		t.Fatalf("the straddling rune should drop whole, ends %q", text[len(text)-3:])
	}
}

// The acceptance case on a real daemon: a subscriber that may type hears the
// body it was sent, and a body past the head arrives cut and flagged.
func TestDaemonMessageEventCarriesTheBodySent(t *testing.T) {
	testDaemon(t)
	sender := dialTest(t)
	sender.spawn("eng-platform-beetle-ox", "eng-platform", "Beetle-Ox", `echo TOKEN=$ATERM_SESSION_TOKEN; sleep 60`)
	token := sender.token()
	dialTest(t).spawn("frontend-eng-imp-dragonfly", "frontend-eng", "Imp-Dragonfly",
		`printf 'READY\033[?2004h\n'; stty raw -echo; cat >/dev/null`)
	watch := dialTest(t)
	if err := watch.c.write(frame{Type: "subscribe", ID: "s", Channel: "sessions"}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	long := longLine(messageTextHead + 500)
	for _, body := range []string{"line one\nline two", long} {
		wantText, wantCut := bodyHead(body)
		id := sendAs(t, token, "frontend-eng", body).ID
		deadline := time.Now().Add(10 * time.Second)
		for done := false; !done; {
			_ = watch.c.raw.SetReadDeadline(deadline)
			message, err := watch.c.read()
			if err != nil {
				t.Fatalf("no message event for %s: %v", id, err)
			}
			if message.Type != "message" || message.Message.ID != id {
				continue
			}
			if message.Message.Text != wantText || message.Message.TextCut != wantCut {
				t.Fatalf("event %s state %s: text %d bytes cut %v, want %d bytes cut %v",
					id, message.Message.State, len(message.Message.Text), message.Message.TextCut, len(wantText), wantCut)
			}
			done = finalState(message.Message.State)
		}
	}
}
