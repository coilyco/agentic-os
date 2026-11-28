package main

import (
	"fmt"
	"os"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// `transcript_search` finds a phrase in a seat's whole conversation (COI-3797). Each
// match's `before` cursor pages it in. Contract: the aterm-client transcript reference.

const (
	transcriptSearchFeature = "transcript_search"
	// transcriptSearchMax bounds the matches one reply carries, newest kept.
	transcriptSearchMax = 500
	// transcriptSearchContext is how many items after a match its page keeps, so the match
	// shows with what followed it rather than at the bottom edge.
	transcriptSearchContext = 8
	// transcriptSnippetRunes is how much text either side of a match a snippet keeps.
	transcriptSnippetRunes  = 60
	transcriptQueryMaxRunes = 200
)

// transcriptMatch is one item that holds the phrase. Field names where: the item's text,
// a tool's summary, or a tool's output.
type transcriptMatch struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	At      string `json:"at,omitempty"`
	Field   string `json:"field"`
	Snippet string `json:"snippet"`
	Before  string `json:"before,omitempty"`
}

// transcriptSearchReply: Total counts every match, the newest transcriptSearchMax sent,
// and Partial says the oldest part of a very long file was not searched.
type transcriptSearchReply struct {
	Reader  string
	Matches []transcriptMatch
	Total   int
	Partial bool
}

// keyedItems is every item a reader sees, oldest first, and the `before` cursor of each.
type keyedItems struct {
	reader  string
	items   []transcriptItem
	keys    []string
	partial bool
}

// fileItems reads a whole JSON-lines transcript, back as far as the largest read window.
func fileItems(reader, path string, parse func([]jsonLine) parsedLines) (keyedItems, error) {
	info, err := os.Stat(path)
	if err != nil {
		return keyedItems{}, withReason(reasonNoTranscript, withExit(exitOffRoster, fmt.Errorf("no transcript at %s: %w", path, errNoTranscript)))
	}
	start := info.Size() - transcriptMaxWindow
	if start < 0 {
		start = 0
	}
	lines, _, err := readLines(path, start, info.Size(), start == 0)
	if err != nil {
		return keyedItems{}, err
	}
	items := parse(lines).items
	sortItems(items)
	keys := make([]string, len(items))
	for index, item := range items {
		keys[index] = fileCursor(item.pos)
	}
	return keyedItems{reader: reader, items: items, keys: keys, partial: start > 0}, nil
}

// allTranscriptItems is transcriptOf without paging, for one harness at a time.
func allTranscriptItems(entry ledgerEntry, all []ledgerEntry) (keyedItems, error) {
	var keyed keyedItems
	var err error
	switch entry.Seat {
	case "claude":
		var path string
		if path, err = transcriptPath(entry); err == nil {
			keyed, err = fileItems("claude", path, func(lines []jsonLine) parsedLines { return parseClaude(lines, all) })
		}
	case "codex":
		var path string
		if path, err = codexRollout(entry, all); err == nil {
			keyed, err = fileItems("codex", path, parseCodex)
		}
	case "opencode":
		keyed, err = opencodeAllItems(entry, all)
	default:
		err = withExit(exitUsage, fmt.Errorf("%s runs %q, which keeps no transcript aterm reads: %w", entry.Name, entry.Seat, errUnsupportedHarness))
	}
	return keyed, transcriptRefusal(err)
}

// searchTranscript finds query in every item a client's views can show. A peer message
// and the replies that answer only it never show, so they never match either.
func searchTranscript(keyed keyedItems, query string) transcriptSearchReply {
	reply := transcriptSearchReply{Reader: keyed.reader, Partial: keyed.partial}
	needle := foldRunes(query)
	if len(needle) == 0 {
		return reply
	}
	for index, item := range keyed.items {
		if item.Kind == transcriptKindPeer || (item.For != "" && !item.Shared) {
			continue
		}
		field, snippet, ok := matchItem(item, needle)
		if !ok {
			continue
		}
		reply.Total++
		reply.Matches = append(reply.Matches, transcriptMatch{ID: item.ID, Kind: item.Kind, At: item.At, Field: field, Snippet: snippet, Before: pageAfter(keyed.keys, index)})
	}
	if len(reply.Matches) > transcriptSearchMax {
		reply.Matches = reply.Matches[len(reply.Matches)-transcriptSearchMax:]
	}
	return reply
}

// pageAfter is the `before` cursor of a page ending a few items past index, past any item
// sharing its line. Empty means the newest page holds it.
func pageAfter(keys []string, index int) string {
	end := index + transcriptSearchContext
	for end < len(keys) && keys[end] == keys[index] {
		end++
	}
	if end >= len(keys) {
		return ""
	}
	return keys[end]
}

// matchItem looks in the text a view draws for an item: its words, then a tool's line,
// then a tool's output.
func matchItem(item transcriptItem, needle []rune) (string, string, bool) {
	fields := []struct{ name, text string }{{"text", item.Text}}
	if item.Tool != nil {
		fields = append(fields, struct{ name, text string }{"summary", firstNonEmpty(item.Tool.FullSummary, item.Tool.Summary)}, struct{ name, text string }{"output", item.Tool.Output})
	}
	for _, field := range fields {
		if snippet, ok := snippetOf(field.text, needle); ok {
			return field.name, snippet, true
		}
	}
	return "", "", false
}

// foldRunes lowers text rune by rune, so an index into it is an index into the original.
func foldRunes(text string) []rune {
	runes := []rune(text)
	for index, r := range runes {
		runes[index] = unicode.ToLower(r)
	}
	return runes
}

// snippetOf is the first match of needle in text with a little either side, on one line.
func snippetOf(text string, needle []rune) (string, bool) {
	if text == "" || !utf8.ValidString(text) {
		return "", false
	}
	runes := []rune(text)
	folded := foldRunes(text)
	at := runeIndex(folded, needle)
	if at < 0 {
		return "", false
	}
	start, end := at-transcriptSnippetRunes, at+len(needle)+transcriptSnippetRunes
	prefix, suffix := "…", "…"
	if start <= 0 {
		start, prefix = 0, ""
	}
	if end >= len(runes) {
		end, suffix = len(runes), ""
	}
	return prefix + strings.Join(strings.Fields(string(runes[start:end])), " ") + suffix, true
}

func runeIndex(haystack, needle []rune) int {
	for start := 0; start+len(needle) <= len(haystack); start++ {
		found := true
		for offset, r := range needle {
			if haystack[start+offset] != r {
				found = false
				break
			}
		}
		if found {
			return start
		}
	}
	return -1
}

// transcriptSearch answers a transcript_search request. It is gated like the read itself,
// since a snippet of tool output can hold a secret (remoteLocked).
func (d *daemon) transcriptSearch(cl *client, message frame) error {
	query := strings.TrimSpace(message.Query)
	if query == "" || utf8.RuneCountInString(query) > transcriptQueryMaxRunes {
		return withExit(exitUsage, fmt.Errorf("a search needs a phrase of 1 to %d characters", transcriptQueryMaxRunes))
	}
	s, err := d.oneTarget(firstNonEmpty(message.Session, message.Target))
	if err != nil {
		return err
	}
	d.mu.Lock()
	ledger := d.ledgerDir
	d.mu.Unlock()
	var recorded []ledgerEntry
	if ledger != "" {
		recorded = readLedger(ledger)
	}
	entry, err := findRecorded(recorded, s.name)
	if err != nil {
		return withReason(reasonNoTranscript, fmt.Errorf("%s has no ledger record to find its transcript by: %w", s.name, errNoTranscript))
	}
	keyed, err := allTranscriptItems(withLiveCwd(entry, s.pid), recorded)
	if err != nil {
		return err
	}
	reply := searchTranscript(keyed, query)
	return cl.c.write(frame{
		Type: "transcript_search", ID: message.ID, Session: s.name, Reader: reply.Reader, Query: query,
		Matches: reply.Matches, Total: reply.Total, More: reply.Partial, UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	})
}
