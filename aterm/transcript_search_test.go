package main

import (
	"fmt"
	"strings"
	"testing"
)

// Every item a search can return is paged in by its `before` cursor, for each harness.
func TestTranscriptSearchPagesEveryMatchInByItsCursor(t *testing.T) {
	type harness struct {
		keyed func(t *testing.T) keyedItems
		read  func(t *testing.T, req transcriptRequest) transcriptReply
	}
	claudePath := transcriptTempFile(t, transcriptFixture(t, "claude.jsonl"))
	codexPath := transcriptTempFile(t, transcriptFixture(t, "codex.jsonl"))
	harnesses := map[string]harness{
		"claude": {
			keyed: func(t *testing.T) keyedItems {
				keyed, err := fileItems("claude", claudePath, func(lines []jsonLine) parsedLines { return parseClaude(lines, transcriptLedger) })
				if err != nil {
					t.Fatal(err)
				}
				return keyed
			},
			read: func(t *testing.T, req transcriptRequest) transcriptReply {
				reply, err := claudeTranscript(claudePath, req, transcriptLedger)
				if err != nil {
					t.Fatal(err)
				}
				return reply
			},
		},
		"codex": {
			keyed: func(t *testing.T) keyedItems {
				keyed, err := fileItems("codex", codexPath, parseCodex)
				if err != nil {
					t.Fatal(err)
				}
				return keyed
			},
			read: func(t *testing.T, req transcriptRequest) transcriptReply {
				reply, err := fileTranscript("codex", codexPath, req, parseCodex)
				if err != nil {
					t.Fatal(err)
				}
				return reply
			},
		},
		"opencode": {
			keyed: func(t *testing.T) keyedItems {
				keyed, err := opencodeAllItems(opencodeFixtureDB(t), nil)
				if err != nil {
					t.Fatal(err)
				}
				return keyed
			},
			read: func(t *testing.T, req transcriptRequest) transcriptReply {
				reply, err := opencodeTranscript(opencodeFixtureDB(t), nil, req)
				if err != nil {
					t.Fatal(err)
				}
				return reply
			},
		},
	}
	for name, h := range harnesses {
		t.Run(name, func(t *testing.T) {
			keyed := h.keyed(t)
			full := h.read(t, transcriptRequest{Limit: 200})
			if got, want := itemKinds(keyed.items), itemKinds(full.Items); got != want {
				t.Fatalf("a search sees the items a read does:\n%s\n%s", got, want)
			}
			paged := 0
			for index, item := range keyed.items {
				before := pageAfter(keyed.keys, index)
				page := h.read(t, transcriptRequest{Limit: transcriptSearchContext + 2, Before: before})
				if _, ok := itemsByID(page.Items)[item.ID]; !ok {
					t.Fatalf("item %d (%s) is not on the page before %q: %s", index, item.ID, before, itemKinds(page.Items))
				}
				if before != "" {
					paged++
				}
			}
			if paged == 0 {
				t.Fatal("the fixture is long enough that some match pages back")
			}
		})
	}
}

func TestTranscriptSearchLeavesOutPeerMessagesAndWhatAnswersOnlyThem(t *testing.T) {
	keyed, err := fileItems("claude", transcriptTempFile(t, transcriptFixture(t, "claude.jsonl")), func(lines []jsonLine) parsedLines { return parseClaude(lines, transcriptLedger) })
	if err != nil {
		t.Fatal(err)
	}
	reply := searchTranscript(keyed, "MERGE on green")
	for _, match := range reply.Matches {
		if match.Kind == transcriptKindPeer {
			t.Fatalf("a peer message never shows in a seat's view, so it never matches: %+v", match)
		}
	}
	if reply.Total != 1 || reply.Matches[0].Kind != transcriptKindReply || !strings.Contains(reply.Matches[0].Snippet, "merge on green") {
		t.Fatalf("the shared reply matches, whatever the case: %+v", reply)
	}
	if none := searchTranscript(keyed, "nothing says this"); none.Total != 0 || len(none.Matches) != 0 {
		t.Fatalf("no match = %+v", none)
	}
	if output := searchTranscript(keyed, "847 passed"); output.Total != 1 || output.Matches[0].Field != "output" {
		t.Fatalf("a tool's output matches and says so: %+v", output)
	}
	if summary := searchTranscript(keyed, "aterm-client-gate"); summary.Total == 0 || summary.Matches[0].Field != "summary" {
		t.Fatalf("a tool's line matches: %+v", summary)
	}
}

// COI-3797: a phrase early in a 1,000-plus line transcript is found and paged in.
func TestTranscriptSearchFindsAnEarlyPhraseInALongTranscript(t *testing.T) {
	var lines []string
	lines = append(lines, `{"type":"user","uuid":"early","parentUuid":"","timestamp":"2026-11-27T08:00:00.000Z","message":{"role":"user","content":"check the lighthouse migration before lunch"},"origin":{"kind":"human"}}`)
	for n := 0; n < 700; n++ {
		lines = append(lines,
			fmt.Sprintf(`{"type":"assistant","uuid":"a%d","parentUuid":"p","timestamp":"2026-11-27T09:00:00.000Z","message":{"id":"msg_%d","model":"m","stop_reason":"tool_use","content":[{"type":"tool_use","id":"toolu_%d","name":"Bash","input":{"command":"just test %d"}}]}}`, n, n, n, n),
			fmt.Sprintf(`{"type":"user","uuid":"u%d","parentUuid":"a%d","timestamp":"2026-11-27T09:00:01.000Z","message":{"role":"user","content":[{"tool_use_id":"toolu_%d","type":"tool_result","content":"ok %d"}]}}`, n, n, n, n))
	}
	path := transcriptTempFile(t, strings.Join(lines, "\n")+"\n")
	keyed, err := fileItems("claude", path, func(lines []jsonLine) parsedLines { return parseClaude(lines, nil) })
	if err != nil {
		t.Fatal(err)
	}
	reply := searchTranscript(keyed, "Lighthouse")
	if reply.Total != 1 || reply.Matches[0].ID != "early" || reply.Matches[0].Before == "" || reply.Partial {
		t.Fatalf("the early prompt is found and pages back: %+v", reply)
	}
	page, err := claudeTranscript(path, transcriptRequest{Limit: 50, Before: reply.Matches[0].Before}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := itemsByID(page.Items)["early"]; !ok {
		t.Fatalf("the match's page holds it: %s", itemKinds(page.Items))
	}
	// Many matches: every tool call ran `just test`, and the newest are the ones sent.
	many := searchTranscript(keyed, "just test")
	if many.Total != 700 || len(many.Matches) != transcriptSearchMax || many.Matches[len(many.Matches)-1].ID != "toolu_699" {
		t.Fatalf("many matches = total %d, sent %d", many.Total, len(many.Matches))
	}
}

func TestSnippetKeepsTheMatchOnOneLineWithItsNeighbours(t *testing.T) {
	text := strings.Repeat("a", 100) + "\n\nFind ME here\n" + strings.Repeat("b", 100)
	snippet, ok := snippetOf(text, foldRunes("find me"))
	if !ok || !strings.HasPrefix(snippet, "…") || !strings.HasSuffix(snippet, "…") || !strings.Contains(snippet, "Find ME here") || strings.Contains(snippet, "\n") {
		t.Fatalf("snippet = %q", snippet)
	}
	if _, ok := snippetOf("Größe", foldRunes("GRÖSSE")); ok {
		t.Fatal("folding is rune for rune, so ß never becomes ss")
	}
	if snippet, ok := snippetOf("ÉCOLE", foldRunes("école")); !ok || snippet != "ÉCOLE" {
		t.Fatalf("a non-ASCII rune folds: %q %v", snippet, ok)
	}
}
