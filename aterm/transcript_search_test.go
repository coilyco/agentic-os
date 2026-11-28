package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

// COI-3797 slice 2: one search covers every live seat, and one without a transcript says
// why without hiding the others.
func TestTranscriptSearchCoversEveryLiveSeat(t *testing.T) {
	d, state, seat := searchRig(t)
	seat("eng-platform-b", "The Lighthouse migration is done.")
	seat("eng-platform-a", "I checked the lighthouse logs.")
	seat("eng-platform-c", "Nothing to see.")
	d.sessions["shell-z"] = &ptySession{d: d, name: "shell-z", seat: "goose", done: make(chan struct{})}
	results := everySeat(t, d, readLedger(state), "lighthouse")
	var got []string
	for _, result := range results {
		got = append(got, fmt.Sprintf("%s:%d:%s", result.Session, result.Total, result.Reason))
	}
	if want := "eng-platform-a:1: eng-platform-b:1: eng-platform-c:0: shell-z:0:no_transcript"; strings.Join(got, " ") != want {
		t.Fatalf("results = %s, want %s", strings.Join(got, " "), want)
	}
	if match := results[1].Matches[0]; match.Kind != transcriptKindReply || !strings.Contains(match.Snippet, "Lighthouse migration") {
		t.Fatalf("a seat's match carries its snippet: %+v", match)
	}
}

// searchRig is a daemon with live claude seats recorded in a ledger. seat adds one and
// returns its transcript file.
func searchRig(t *testing.T) (*daemon, string, func(name, phrase string) string) {
	t.Helper()
	state := t.TempDir()
	d := newDaemon(func(string, ...any) {})
	d.ledgerDir = state
	return d, state, func(name, phrase string) string {
		home, id := filepath.Join(t.TempDir(), "home"), newConversationID()
		seatTranscript(t, home, id, userLine("go"), assistantLine("end_turn", text(phrase)))
		if err := writeLedger(state, ledgerEntry{Name: name, Role: "eng-platform", Seat: "claude", Conversation: id, Home: home, Started: time.Now().UTC()}); err != nil {
			t.Fatal(err)
		}
		d.sessions[name] = &ptySession{d: d, name: name, seat: "claude", done: make(chan struct{})}
		return filepath.Join(home, ".claude", "projects", "-some-project", id+".jsonl")
	}
}

func everySeat(t *testing.T, d *daemon, recorded []ledgerEntry, query string) []transcriptSeatResult {
	t.Helper()
	results, err := d.searchEverySeat(context.Background(), recorded, query)
	if err != nil {
		t.Fatal(err)
	}
	return results
}

func readFrame(t *testing.T, c *conn) frame {
	t.Helper()
	got := make(chan frame, 1)
	go func() {
		message, _ := c.read()
		got <- message
	}()
	select {
	case message := <-got:
		return message
	case <-time.After(5 * time.Second):
		t.Fatal("no frame came back")
		return frame{}
	}
}

// A search waits off the read loop, so the connection keeps taking frames, and a newer
// search from the same connection cancels the one still waiting.
func TestASearchNeverBlocksItsConnectionAndANewerOneCancelsIt(t *testing.T) {
	d, _, seat := searchRig(t)
	seat("eng-platform-a", "The lighthouse logs.")
	server, peer := net.Pipe()
	defer server.Close()
	defer peer.Close()
	cl := &client{c: newConn(server), owned: map[string]bool{}, attached: map[string]*ptySession{}}
	device := newConn(peer)
	// Both slots are taken, as by two other devices' searches.
	d.searching.init()
	for range transcriptSearchSlots {
		d.searching.slots <- struct{}{}
	}
	returned := make(chan error, 1)
	go func() {
		returned <- d.handle(cl, frame{Type: "transcript_search", ID: "1", All: true, Query: "lighthouse"})
	}()
	select {
	case err := <-returned:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("handle waited for the search, so the connection's polls and typing would wait too")
	}
	if err := d.handle(cl, frame{Type: "transcript_search", ID: "2", Session: "eng-platform-a", Query: "lighthouse"}); err != nil {
		t.Fatal(err)
	}
	if first := readFrame(t, device); first.ID != "1" || first.Type != "error" || first.Reason != reasonSearchCancelled {
		t.Fatalf("the replaced search ends with its own reason: %+v", first)
	}
	for range transcriptSearchSlots {
		<-d.searching.slots
	}
	if second := readFrame(t, device); second.ID != "2" || second.Type != "transcript_search" || second.Total != 1 {
		t.Fatalf("the newer search answers once a slot frees: %+v", second)
	}
}

// The cache answers an unchanged file, reads a grown one again, stays in its budget, and
// lets a seat's items go when its session ends.
func TestTheItemCacheFollowsTheFilesStaysInBudgetAndLeavesWithTheSeat(t *testing.T) {
	d, state, seat := searchRig(t)
	pathA := seat("eng-platform-a", "The lighthouse logs.")
	seat("eng-platform-b", "More lighthouse notes.")
	seat("eng-platform-c", "Nothing here.")
	recorded := readLedger(state)
	everySeat(t, d, recorded, "lighthouse")
	if d.searching.misses != 3 || d.searching.cachedBytes() == 0 {
		t.Fatalf("a first search reads every seat and keeps them: misses %d, bytes %d", d.searching.misses, d.searching.cachedBytes())
	}
	everySeat(t, d, recorded, "notes")
	if d.searching.misses != 3 {
		t.Fatalf("unchanged files are answered from the cache: misses %d", d.searching.misses)
	}
	file, _ := os.OpenFile(pathA, os.O_APPEND|os.O_WRONLY, 0)
	file.WriteString(assistantLine("end_turn", text("A later lighthouse line.")) + "\n")
	file.Close()
	if results := everySeat(t, d, recorded, "lighthouse"); d.searching.misses != 4 || !strings.Contains(results[0].Matches[0].Snippet, "A later lighthouse line") {
		t.Fatalf("a grown file is read again and its new line found: misses %d, %+v", d.searching.misses, results[0])
	}
	one := d.searching.cachedBytes() / 3
	saved := transcriptCacheBudget
	transcriptCacheBudget = 2*one + one/2
	defer func() { transcriptCacheBudget = saved }()
	d.searching.drop("eng-platform-a")
	d.searching.drop("eng-platform-b")
	d.searching.drop("eng-platform-c")
	everySeat(t, d, recorded, "lighthouse")
	if held := d.searching.cachedBytes(); held > transcriptCacheBudget || len(d.searching.cache) != 2 {
		t.Fatalf("the cache keeps within its budget: %d of %d bytes, %d seats", held, transcriptCacheBudget, len(d.searching.cache))
	}
	s := d.sessions["eng-platform-c"]
	d.forget(s)
	if _, held := d.searching.cache["eng-platform-c"]; held {
		t.Fatal("a seat that ended keeps no cached items")
	}
}

// The ASCII fast path finds what folding rune by rune finds, at the same place.
func TestTheASCIIPathAgreesWithTheRunePath(t *testing.T) {
	runePath := func(text string, needle []rune) (string, bool) {
		runes, folded := []rune(text), foldRunes(text)
		at := runeIndex(folded, needle)
		if at < 0 {
			return "", false
		}
		start, end := max(at-transcriptSnippetRunes, 0), min(at+len(needle)+transcriptSnippetRunes, len(runes))
		prefix, suffix := "…", "…"
		if start == 0 {
			prefix = ""
		}
		if end == len(runes) {
			suffix = ""
		}
		return prefix + strings.Join(strings.Fields(string(runes[start:end])), " ") + suffix, true
	}
	texts := []string{
		"Ran the Gate → it passed, then the GATE again",
		strings.Repeat("é", 70) + " the gate " + strings.Repeat("→", 70),
		"no match here at all",
		"gat",
		"Kelvin is a temperature",
	}
	for _, text := range texts {
		for _, query := range []string{"gate", "GATE", "the gate", "kelvin", "→"} {
			want, wantOK := runePath(text, foldRunes(query))
			got, gotOK := snippetOf(text, foldRunes(query))
			if got != want || gotOK != wantOK {
				t.Errorf("%q in %q: got %q %v, the rune path %q %v", query, text, got, gotOK, want, wantOK)
			}
		}
	}
}
