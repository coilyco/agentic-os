package main

import (
	"strings"
	"unicode/utf8"
)

// setShell names a shell step from the command after any leading cd, and keeps the
// whole command as FullSummary for Verbose (transcript.md, COI-3738).
func (t *transcriptTool) setShell(command string) {
	t.Class = transcriptClassShell
	t.Summary = "Ran " + summaryLine(dropLeadingCd(command), 100)
	if full := "Ran " + cutRunes(strings.Join(strings.Fields(command), " "), 400); full != t.Summary {
		t.FullSummary = full
	}
}

// dropLeadingCd is the command after its leading cd segments. A command that is only
// cds, or one whose cd it cannot read, comes back as it was.
func dropLeadingCd(command string) string {
	rest, dropped := command, false
	for {
		after, ok := afterCd(rest)
		if !ok {
			break
		}
		rest, dropped = after, true
	}
	if !dropped || strings.TrimSpace(rest) == "" {
		return command
	}
	return rest
}

// afterCd reads one `cd <dir>` and the &&, ; or newline after it. A bare cd at the end
// reads as nothing left. Anything else, `||` or `|` included, is not a plain cd.
func afterCd(command string) (string, bool) {
	s := strings.TrimLeft(command, " \t\n")
	if !strings.HasPrefix(s, "cd") || (len(s) > 2 && !strings.ContainsRune(" \t\n;&", rune(s[2]))) {
		return "", false
	}
	s = strings.TrimLeft(s[2:], " \t")
	if s != "" && (s[0] == '"' || s[0] == '\'') {
		end := strings.IndexByte(s[1:], s[0])
		if end < 0 {
			return "", false
		}
		s = s[end+2:]
	} else if end := strings.IndexAny(s, " \t\n;&|"); end >= 0 {
		s = s[end:]
	} else {
		s = ""
	}
	s = strings.TrimLeft(s, " \t")
	switch {
	case s == "":
		return "", true
	case strings.HasPrefix(s, "&&"):
		return s[2:], true
	case s[0] == ';' || s[0] == '\n':
		return s[1:], true
	}
	return "", false
}

func cutRunes(text string, limit int) string {
	if utf8.RuneCountInString(text) <= limit {
		return text
	}
	return string([]rune(text)[:limit]) + "…"
}
