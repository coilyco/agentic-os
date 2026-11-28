package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// `transcript_search` finds a phrase in a seat's whole conversation (COI-3797). Each
// match's `before` cursor pages it in. Contract: the aterm-client transcript reference.

const (
	transcriptSearchFeature = "transcript_search"
	// transcriptSearchAllFeature says a search with `all` covers every live seat.
	transcriptSearchAllFeature = "transcript_search_all"
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
	if snippet, ok := snippetOf(item.Text, needle); ok {
		return "text", snippet, true
	}
	if item.Tool == nil {
		return "", "", false
	}
	if snippet, ok := snippetOf(firstNonEmpty(item.Tool.FullSummary, item.Tool.Summary), needle); ok {
		return "summary", snippet, true
	}
	if snippet, ok := snippetOf(item.Tool.Output, needle); ok {
		return "output", snippet, true
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
	if text == "" {
		return "", false
	}
	// An ASCII phrase is found in the bytes, with no copy. Only K (U+212A) and I (U+0130)
	// fold to ASCII from outside it, so a text holding either takes the rune path.
	if ascii, ok := asciiNeedle(needle); ok && !strings.Contains(text, "\u212a") && !strings.Contains(text, "\u0130") {
		at := indexFoldASCII(text, ascii)
		if at < 0 || !utf8.ValidString(text) {
			return "", false
		}
		return snippetAround(text, at, len(ascii)), true
	}
	if !utf8.ValidString(text) {
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

// transcriptSeatResult is one seat's part of a search of every live seat. Reason names
// why a seat could not be searched, and such a seat carries no matches.
type transcriptSeatResult struct {
	Session string            `json:"session"`
	Reader  string            `json:"reader,omitempty"`
	Matches []transcriptMatch `json:"matches,omitempty"`
	Total   int               `json:"total"`
	Partial bool              `json:"more,omitempty"`
	Reason  string            `json:"reason,omitempty"`
}

// transcriptSeatMax bounds one seat's matches in a search across every seat, newest kept.
const transcriptSeatMax = 50

// searchSeat searches one live seat's transcript, found by its ledger record.
func (d *daemon) searchSeat(ctx context.Context, s *ptySession, recorded []ledgerEntry, query string) (transcriptSearchReply, error) {
	if ctx.Err() != nil {
		return transcriptSearchReply{}, searchCancelled()
	}
	entry, err := findRecorded(recorded, s.name)
	if err != nil {
		return transcriptSearchReply{}, withReason(reasonNoTranscript, fmt.Errorf("%s has no ledger record to find its transcript by: %w", s.name, errNoTranscript))
	}
	keyed, err := d.searching.items(s.name, withLiveCwd(entry, s.pid), recorded)
	if err != nil {
		return transcriptSearchReply{}, err
	}
	return searchTranscript(keyed, query), nil
}

// transcriptSearch answers a transcript_search request, for one seat or, with All, every
// live seat. It is gated like the read itself, since a snippet can hold a secret.
func (d *daemon) transcriptSearch(ctx context.Context, cl *client, message frame) error {
	query := strings.TrimSpace(message.Query)
	if query == "" || utf8.RuneCountInString(query) > transcriptQueryMaxRunes {
		return withExit(exitUsage, fmt.Errorf("a search needs a phrase of 1 to %d characters", transcriptQueryMaxRunes))
	}
	release, err := d.searching.acquire(ctx)
	if err != nil {
		return err
	}
	defer release()
	d.mu.Lock()
	ledger := d.ledgerDir
	d.mu.Unlock()
	var recorded []ledgerEntry
	if ledger != "" {
		recorded = readLedger(ledger)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if message.All {
		seats, err := d.searchEverySeat(ctx, recorded, query)
		if err != nil {
			return err
		}
		return cl.c.write(frame{Type: "transcript_search", ID: message.ID, Query: query, Seats: seats, UpdatedAt: now})
	}
	s, err := d.oneTarget(firstNonEmpty(message.Session, message.Target))
	if err != nil {
		return err
	}
	reply, err := d.searchSeat(ctx, s, recorded, query)
	if err != nil {
		return err
	}
	return cl.c.write(frame{
		Type: "transcript_search", ID: message.ID, Session: s.name, Reader: reply.Reader, Query: query,
		Matches: reply.Matches, Total: reply.Total, More: reply.Partial, UpdatedAt: now,
	})
}

// searchEverySeat searches each live seat by name. A seat that cannot be read says
// why, so one harness without a transcript never hides the rest.
func (d *daemon) searchEverySeat(ctx context.Context, recorded []ledgerEntry, query string) ([]transcriptSeatResult, error) {
	d.mu.Lock()
	seats := make([]*ptySession, 0, len(d.sessions))
	for _, s := range d.sessions {
		seats = append(seats, s)
	}
	d.mu.Unlock()
	sort.Slice(seats, func(a, b int) bool { return seats[a].name < seats[b].name })
	results := make([]transcriptSeatResult, 0, len(seats))
	for _, s := range seats {
		result := transcriptSeatResult{Session: s.name}
		reply, err := d.searchSeat(ctx, s, recorded, query)
		if errors.Is(err, errSearchCancelled) {
			return nil, err
		}
		if err != nil {
			result.Reason = firstNonEmpty(reasonFor(transcriptRefusal(err)), reasonNoTranscript)
		} else {
			result.Reader, result.Total, result.Partial = reply.Reader, reply.Total, reply.Partial
			result.Matches = reply.Matches
			if len(result.Matches) > transcriptSeatMax {
				result.Matches = result.Matches[len(result.Matches)-transcriptSeatMax:]
			}
		}
		results = append(results, result)
	}
	return results, nil
}

// asciiNeedle is a folded needle as bytes, when every rune of it is ASCII.
func asciiNeedle(needle []rune) (string, bool) {
	out := make([]byte, len(needle))
	for index, r := range needle {
		if r >= utf8.RuneSelf {
			return "", false
		}
		out[index] = byte(r)
	}
	return string(out), true
}

// indexFoldASCII is the byte offset of needle (lower case ASCII) in text, case folded.
func indexFoldASCII(text, needle string) int {
	first, upper := needle[0], needle[0]
	if first >= 'a' && first <= 'z' {
		upper = first - 'a' + 'A'
	}
	for start := 0; start+len(needle) <= len(text); start++ {
		if text[start] != first && text[start] != upper {
			continue
		}
		matched := true
		for offset := 1; offset < len(needle); offset++ {
			c := text[start+offset]
			if c >= 'A' && c <= 'Z' {
				c += 'a' - 'A'
			}
			if c != needle[offset] {
				matched = false
				break
			}
		}
		if matched {
			return start
		}
	}
	return -1
}

// snippetAround is text at a byte match, transcriptSnippetRunes either side, on one line.
func snippetAround(text string, at, size int) string {
	start, end := at, at+size
	for count := 0; count < transcriptSnippetRunes && start > 0; count++ {
		_, width := utf8.DecodeLastRuneInString(text[:start])
		start -= width
	}
	for count := 0; count < transcriptSnippetRunes && end < len(text); count++ {
		_, width := utf8.DecodeRuneInString(text[end:])
		end += width
	}
	prefix, suffix := "…", "…"
	if start == 0 {
		prefix = ""
	}
	if end == len(text) {
		suffix = ""
	}
	return prefix + strings.Join(strings.Fields(text[start:end]), " ") + suffix
}
