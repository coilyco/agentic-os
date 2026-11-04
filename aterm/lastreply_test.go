package main

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

const stampedTurn = `{"type":"user","timestamp":"2026-11-04T17:00:00.000Z","message":{"content":"go"}}
{"type":"assistant","timestamp":"2026-11-04T17:00:05.000Z","message":{"content":[{"type":"text","text":"Working."},{"type":"tool_use"}],"stop_reason":"tool_use"}}
{"type":"user","timestamp":"2026-11-04T17:00:06.000Z","message":{"content":"result"}}
{"type":"assistant","timestamp":"2026-11-04T17:01:30.250Z","message":{"content":[{"type":"text","text":"All done.\n\nSecond paragraph."}],"stop_reason":"end_turn"}}
`

func TestLastReplyAtReadsTheWholeReplyAndWhenTheTurnEnded(t *testing.T) {
	text, complete, at, err := lastReplyAt(strings.NewReader(stampedTurn))
	if err != nil || !complete || text != "All done.\n\nSecond paragraph." {
		t.Fatalf("reply = %q complete=%v err=%v, want the whole final reply", text, complete, err)
	}
	if want := time.Date(2026, 11, 4, 17, 1, 30, 250_000_000, time.UTC); !at.Equal(want) {
		t.Fatalf("at = %v, want the last line's stamp %v", at, want)
	}
	next := stampedTurn + `{"type":"user","timestamp":"2026-11-04T17:02:00.000Z","message":{"content":"more"}}` + "\n"
	if _, complete, at, _ := lastReplyAt(strings.NewReader(next)); complete || !at.IsZero() {
		t.Fatalf("a new turn should hold no finished reply yet: complete=%v at=%v", complete, at)
	}
}

func TestBoundReplyCutsOnARuneBoundaryAndSaysSo(t *testing.T) {
	if text, cut := boundReply("short"); text != "short" || cut {
		t.Fatalf("a short reply should pass whole: %q %v", text, cut)
	}
	long := strings.Repeat("é", maxLastReplyBytes)
	text, cut := boundReply(long)
	if !cut || len(text) > maxLastReplyBytes || !utf8.ValidString(text) || len(text) < maxLastReplyBytes-3 {
		t.Fatalf("a long reply should be cut on a rune boundary near the bound: cut=%v len=%d valid=%v", cut, len(text), utf8.ValidString(text))
	}
}

func TestKeepReplyStoresTheFullTextAndWhenItEndedAndPushesOnlyOnAChange(t *testing.T) {
	s := &ptySession{name: "scientist-evie"}
	ended := time.Date(2026, 11, 4, 17, 1, 30, 0, time.UTC)
	sig := fileSig{size: 10, mod: ended.Add(time.Minute)}
	first := replyView{Text: "All done.\n\nSecond paragraph.", Complete: true, At: ended}
	if !s.keepReply(first, sig) {
		t.Fatal("a new finished reply should change what clients see")
	}
	view := s.lastReplyView()
	if view.Text != first.Text || view.At == nil || !view.At.Equal(ended) || view.Truncated {
		t.Fatalf("view = %+v, want the full reply and its end", view)
	}
	if s.lastReply != "All done." {
		t.Fatalf("the first line should stay for the hover preview, got %q", s.lastReply)
	}
	if s.keepReply(first, sig) {
		t.Fatal("the same reply again should change nothing")
	}
	if s.keepReply(replyView{Text: "Half a sentence that is still being written", Complete: false}, sig) {
		t.Fatal("a running turn should keep the earlier reply")
	}
	if got := s.lastReplyView().Text; got != first.Text {
		t.Fatalf("the earlier reply should survive a running turn, got %q", got)
	}
	later := ended.Add(time.Hour)
	if !s.keepReply(replyView{Text: first.Text, Complete: true, At: later}, sig) {
		t.Fatal("the same words in a newer turn should move the time")
	}
	if got := s.lastReplyView(); got.At == nil || !got.At.Equal(later) {
		t.Fatalf("at = %v, want %v", got.At, later)
	}
}

func TestKeepReplyFallsBackToTheFileTimeWhenTheHarnessStampsNothing(t *testing.T) {
	s := &ptySession{name: "goose-evie"}
	mod := time.Date(2026, 11, 4, 18, 0, 0, 0, time.UTC)
	if !s.keepReply(replyView{Text: "done", Complete: true}, fileSig{size: 1, mod: mod}) {
		t.Fatal("a finished reply should be kept")
	}
	if got := s.lastReplyView(); got.At == nil || !got.At.Equal(mod) {
		t.Fatalf("at = %v, want the transcript's modification time %v", got.At, mod)
	}
}

func TestALastReplyRequestAnswersASeatThatHasNoneYetAndRefusesAnUnknownTarget(t *testing.T) {
	testDaemon(t)
	seat := dialTest(t)
	seat.spawn("scientist-evie", "scientist", "Evie", `printf 'READY\033[?2004h\n'; sleep 30`)
	seat.until("READY")
	c := dialTest(t).c
	reply, err := c.request(frame{Type: "last_reply", Target: "scientist-evie"})
	if err != nil || reply.Type != "last_reply" || reply.Reply == nil || reply.Reply.Session != "scientist-evie" || reply.Reply.Text != "" || reply.Reply.At != nil {
		t.Fatalf("a seat with no reply yet should answer an empty reply: %+v %v", reply, err)
	}
	if _, err := c.request(frame{Type: "last_reply", Target: "nobody-here"}); err == nil {
		t.Fatal("an unknown target should refuse")
	}
}
