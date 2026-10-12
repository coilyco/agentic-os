package main

import (
	"strings"
	"testing"
)

func TestFirstReplyLineSkipsBlankRowsCollapsesSpacesAndCuts(t *testing.T) {
	if got := firstReplyLine("\n  \n  Merged   the PR.\nSecond line"); got != "Merged the PR." {
		t.Fatalf("got %q", got)
	}
	if got := firstReplyLine(" \n\t"); got != "" {
		t.Fatalf("a reply with no text has no line, got %q", got)
	}
	long := firstReplyLine(strings.Repeat("word ", 200))
	if runes := []rune(long); len(runes) > replyLineChars+1 || !strings.HasSuffix(long, "…") {
		t.Fatalf("a long line is cut near %d runes with an ellipsis, got %d", replyLineChars, len(runes))
	}
}

// COI-2678: a roster view carries the first line of the last finished reply, keeps it
// while the next turn runs, and moves it when that turn finishes.
func TestRefreshCarriesTheFirstLineOfTheLastFinishedReply(t *testing.T) {
	c := newContextDaemon(t)
	c.d.refreshContexts()
	if got := c.claude.view().LastReply; got != "" {
		t.Fatalf("a transcript with no reply text has no line, got %q", got)
	}
	turn := []string{userLine("go"), assistantLine("end_turn", text("Merged the PR.\nSecond line."))}
	writeLines(t, c.claudeF, append([]string{claudeUsage("assistant", "claude-opus-5-5", false, 2, 10, 1000, 38)}, turn...)...)
	c.d.refreshContexts()
	if got := c.claude.view().LastReply; got != "Merged the PR." {
		t.Fatalf("finished reply: got %q", got)
	}
	running := append(turn, userLine("again"), assistantLine("tool_use", text("Looking now.")))
	writeLines(t, c.claudeF, append([]string{claudeUsage("assistant", "claude-opus-5-5", false, 2, 10, 1000, 38)}, running...)...)
	c.d.refreshContexts()
	if got := c.claude.view().LastReply; got != "Merged the PR." {
		t.Fatalf("a running turn keeps the last finished line, got %q", got)
	}
	done := append(running, toolResultLine(), assistantLine("end_turn", text("All green.")))
	writeLines(t, c.claudeF, append([]string{claudeUsage("assistant", "claude-opus-5-5", false, 2, 10, 1000, 38)}, done...)...)
	c.d.refreshContexts()
	if got := c.claude.view().LastReply; got != "All green." {
		t.Fatalf("next finished reply: got %q", got)
	}
	if got := c.goose.view().LastReply; got != "" {
		t.Fatalf("a seat with no transcript has none, got %q", got)
	}
}
