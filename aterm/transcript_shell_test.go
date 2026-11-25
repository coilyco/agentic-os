package main

import (
	"strings"
	"testing"
)

func TestShellSummaryDropsLeadingCd(t *testing.T) {
	long := "/private/tmp/claude-502/" + strings.Repeat("x", 120) + "/scratchpad"
	cases := []struct{ name, command, summary, full string }{
		{"cd then ;", "cd /a/b; python3 -c 'print(1)'", "Ran python3 -c 'print(1)'", "Ran cd /a/b; python3 -c 'print(1)'"},
		{"cd then &&", "cd /a/b && just test", "Ran just test", "Ran cd /a/b && just test"},
		{"quoted path, chained cd", `cd "/a b" && cd c && go test ./...`, "Ran go test ./...", `Ran cd "/a b" && cd c && go test ./...`},
		{"cd on its own line", "cd /a/b\ngit status", "Ran git status", "Ran cd /a/b git status"},
		{"a path longer than the line", "cd " + long + "; ls -la", "Ran ls -la", ""},
		{"bare cd stays", "cd /a/b", "Ran cd /a/b", ""},
		{"no cd", "just aterm-client-gate", "Ran just aterm-client-gate", ""},
		{"cd || is not a plain cd", "cd /a/b || exit 1", "Ran cd /a/b || exit 1", ""},
		{"cdk is not cd", "cdk deploy", "Ran cdk deploy", ""},
		{"unclosed quote stays", `cd "/a b && ls`, `Ran cd "/a b && ls`, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tool := &transcriptTool{}
			tool.setShell(c.command)
			if tool.Class != transcriptClassShell || tool.Summary != c.summary {
				t.Fatalf("summary %q, want %q", tool.Summary, c.summary)
			}
			if c.full != "" && tool.FullSummary != c.full {
				t.Fatalf("full %q, want %q", tool.FullSummary, c.full)
			}
			if c.summary == "Ran "+strings.Join(strings.Fields(c.command), " ") && tool.FullSummary != "" {
				t.Fatalf("full %q sent for a summary that dropped nothing", tool.FullSummary)
			}
		})
	}
}

func TestShellSummaryFullKeepsTheLongCd(t *testing.T) {
	tool := &transcriptTool{}
	tool.setShell("cd /private/tmp/" + strings.Repeat("y", 200) + "; ls")
	if tool.Summary != "Ran ls" || !strings.HasPrefix(tool.FullSummary, "Ran cd /private/tmp/") || !strings.HasSuffix(tool.FullSummary, "; ls") {
		t.Fatalf("summary %q, full %q", tool.Summary, tool.FullSummary)
	}
}
