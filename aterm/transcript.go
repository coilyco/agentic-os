package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// The transcript read answers one frame, `transcript`, with a seat's conversation as
// the harness wrote it: prompts, replies, tool steps and notes, oldest first.

const (
	transcriptFeature      = "transcript"
	transcriptDefaultLimit = 50
	transcriptMaxLimit     = 200
	// transcriptTextCap bounds one item's text and one tool's output, so a single huge
	// result cannot fill a reply. A cut item says so and carries its full size.
	transcriptTextCap = 64 << 10
	// transcriptReplyCap bounds the text a whole reply carries.
	transcriptReplyCap = 1 << 20
	// transcriptWindow is the first slice of a file read from its end, doubled until a page
	// fills or the file is covered.
	transcriptMaxWindow = 128 << 20
)

// Reasons a transcript read is refused, named so a client can say why it fell back.
const (
	reasonNoTranscript      = "no_transcript"
	reasonAmbiguous         = "ambiguous"
	reasonUnreadableFormat  = "unreadable_format"
	reasonUnsupportedSeat   = "unsupported_harness"
	reasonBadCursor         = "bad_cursor"
	transcriptKindPrompt    = "prompt"
	transcriptKindReply     = "reply"
	transcriptKindTool      = "tool"
	transcriptKindNote      = "note"
	transcriptKindPeer      = "peer"
	transcriptClassShell    = "shell"
	transcriptClassEdit     = "edit"
	transcriptClassRead     = "read"
	transcriptClassSearch   = "search"
	transcriptClassMCP      = "mcp"
	transcriptClassOther    = "other"
	transcriptCursorFile    = "f:"
	transcriptCursorOpenSQL = "t:"
)

// transcriptWindow is the first slice of a file read from its end. A test narrows it.
var transcriptWindow int64 = 4 << 20

var (
	errAmbiguousSession   = errors.New("ambiguous session")
	errUnreadableFormat   = errors.New("unreadable transcript format")
	errUnsupportedHarness = errors.New("unsupported harness")
	errBadCursor          = errors.New("bad cursor")
)

// transcriptFrom is who sent a peer message.
type transcriptFrom struct {
	Role     string `json:"role"`
	Identity string `json:"identity"`
}

// transcriptDiff is the size of an edit, counts only. The patch is never sent.
type transcriptDiff struct {
	Files   int `json:"files"`
	Added   int `json:"added"`
	Removed int `json:"removed"`
}

// transcriptTool is a tool step. OK is absent while it runs or when the harness cannot
// say.
type transcriptTool struct {
	Name        string          `json:"name"`
	Class       string          `json:"class"`
	Summary     string          `json:"summary"`
	OK          *bool           `json:"ok,omitempty"`
	Output      string          `json:"output,omitempty"`
	OutputBytes int             `json:"output_bytes,omitempty"`
	Truncated   bool            `json:"truncated,omitempty"`
	Diff        *transcriptDiff `json:"diff,omitempty"`

	// verb and subject build an edit's summary once its diffstat is known.
	verb, subject string
}

// transcriptItem is one entry. For is the peer item this one answers, and Shared says
// that peer landed inside a running turn, so the item may also continue the earlier one.
type transcriptItem struct {
	ID        string          `json:"id"`
	Kind      string          `json:"kind"`
	Text      string          `json:"text,omitempty"`
	TextBytes int             `json:"text_bytes,omitempty"`
	Truncated bool            `json:"truncated,omitempty"`
	At        string          `json:"at,omitempty"`
	Model     string          `json:"model,omitempty"`
	Complete  *bool           `json:"complete,omitempty"`
	From      *transcriptFrom `json:"from,omitempty"`
	For       string          `json:"for,omitempty"`
	Shared    bool            `json:"shared,omitempty"`
	Tool      *transcriptTool `json:"tool,omitempty"`

	// pos orders a file reader's items: the byte offset of the line that began the item.
	pos int64
}

// transcriptRequest is the asking half of the frame. Since and Before are opaque cursors.
type transcriptRequest struct {
	Limit  int
	Before string
	Since  string
}

// transcriptReply is the answering half. Older is the cursor for the page before this
// one.
type transcriptReply struct {
	Reader    string
	Items     []transcriptItem
	Cursor    string
	Older     string
	More      bool
	UpdatedAt time.Time
}

func (r transcriptRequest) limit() int {
	switch {
	case r.Limit <= 0:
		return transcriptDefaultLimit
	case r.Limit > transcriptMaxLimit:
		return transcriptMaxLimit
	}
	return r.Limit
}

func boolPtr(value bool) *bool { return &value }

// capText cuts text to the cap on a rune boundary and says whether it did.
func capText(text string) (string, int, bool) {
	size := len(text)
	if size <= transcriptTextCap {
		return text, size, false
	}
	cut := transcriptTextCap
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return text[:cut], size, true
}

// setText stores an item's text under the cap.
func (item *transcriptItem) setText(text string) {
	bounded, size, cut := capText(text)
	item.Text = bounded
	if cut {
		item.Truncated, item.TextBytes = true, size
	}
}

// setOutput stores a tool's output under the cap.
func (tool *transcriptTool) setOutput(output string) {
	bounded, size, cut := capText(output)
	tool.Output = bounded
	if cut {
		tool.Truncated, tool.OutputBytes = true, size
	}
}

// peerStamp reads the line aterm stamps on every message from another seat.
var peerStamp = regexp.MustCompile(`^\[from ([A-Za-z0-9._-]+) ([^\]\n]+)\] ?`)

// peerOf says whether typed text carries the peer stamp, and who from and what body.
func peerOf(text string) (transcriptFrom, string, bool) {
	match := peerStamp.FindStringSubmatch(text)
	if match == nil {
		return transcriptFrom{}, "", false
	}
	return transcriptFrom{Role: match[1], Identity: strings.TrimSpace(match[2])}, text[len(match[0]):], true
}

// seatNamed resolves a session name such as prod-director-griffin-goose to its role and
// identity from the ledger. An unknown name keeps no role.
func seatNamed(name string, all []ledgerEntry) transcriptFrom {
	for _, entry := range all {
		if entry.Name == name {
			return transcriptFrom{Role: entry.Role, Identity: entry.Identity}
		}
	}
	return transcriptFrom{Identity: name}
}

func baseName(path string) string {
	if path == "" {
		return ""
	}
	return filepath.Base(path)
}

// firstLine is the first non-empty line, spaces collapsed and cut for a one-line summary.
func summaryLine(text string, limit int) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.Join(strings.Fields(line), " ")
		if line == "" {
			continue
		}
		if utf8.RuneCountInString(line) > limit {
			runes := []rune(line)
			line = string(runes[:limit]) + "…"
		}
		return line
	}
	return ""
}

// diffSummary is "Edited name +a -r", the edit's one line.
func diffSummary(verb string, files []string, diff transcriptDiff) string {
	subject := baseName(firstOf(files))
	if len(files) > 1 {
		subject = fmt.Sprintf("%d files", len(files))
	}
	if subject == "" {
		subject = "a file"
	}
	return fmt.Sprintf("%s %s +%d -%d", verb, subject, diff.Added, diff.Removed)
}

func firstOf(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

// diffCounts counts the added and removed lines of a unified diff, not its headers.
func diffCounts(diff string) (added, removed int) {
	for _, line := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"):
		case strings.HasPrefix(line, "+"):
			added++
		case strings.HasPrefix(line, "-"):
			removed++
		}
	}
	return added, removed
}

// jsonLine is one complete line of a JSON-lines file and where it began.
type jsonLine struct {
	pos int64
	raw []byte
}

// readLines reads the complete lines of path from offset from, and says where the last
// one ended.
func readLines(path string, from, size int64, atLineStart bool) ([]jsonLine, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer file.Close()
	if _, err := file.Seek(from, io.SeekStart); err != nil {
		return nil, 0, err
	}
	reader := bufio.NewReaderSize(io.LimitReader(file, size-from), 1<<20)
	pos := from
	if from > 0 && !atLineStart {
		skipped, err := reader.ReadBytes('\n')
		pos += int64(len(skipped))
		if err != nil {
			return nil, pos, nil
		}
	}
	var lines []jsonLine
	end := pos
	for {
		raw, err := reader.ReadBytes('\n')
		if len(raw) > 0 && raw[len(raw)-1] == '\n' {
			if trimmed := bytes.TrimSpace(raw); len(trimmed) > 0 {
				lines = append(lines, jsonLine{pos: pos, raw: trimmed})
			}
			pos += int64(len(raw))
			end = pos
		}
		if err != nil {
			return lines, end, nil
		}
	}
}

// parsedLines is what a file reader makes of a slice of lines: the items, and the
// offset of the earliest tool call still waiting for its result (-1 if none), which a.
type parsedLines struct {
	items   []transcriptItem
	openPos int64
}

func fileCursor(offset int64) string { return transcriptCursorFile + strconv.FormatInt(offset, 10) }

func parseFileCursor(cursor string) (int64, error) {
	if !strings.HasPrefix(cursor, transcriptCursorFile) {
		return 0, withReason(reasonBadCursor, fmt.Errorf("%q is not a cursor this daemon issued: %w", cursor, errBadCursor))
	}
	offset, err := strconv.ParseInt(strings.TrimPrefix(cursor, transcriptCursorFile), 10, 64)
	if err != nil || offset < 0 {
		return 0, withReason(reasonBadCursor, fmt.Errorf("%q is not a cursor this daemon issued: %w", cursor, errBadCursor))
	}
	return offset, nil
}

// fileTranscript pages a JSON-lines transcript. Newest asks start from the end and
// widen the window until a page fills, since a file can run to tens of megabytes.
func fileTranscript(reader, path string, req transcriptRequest, parse func([]jsonLine) parsedLines) (transcriptReply, error) {
	info, err := os.Stat(path)
	if err != nil {
		return transcriptReply{}, withReason(reasonNoTranscript, withExit(exitOffRoster, fmt.Errorf("no transcript at %s: %w", path, errNoTranscript)))
	}
	size := info.Size()
	reply := transcriptReply{Reader: reader, UpdatedAt: info.ModTime()}
	limit := req.limit()

	if req.Since != "" {
		from, err := parseFileCursor(req.Since)
		if err != nil {
			return transcriptReply{}, err
		}
		if from > size {
			return transcriptReply{}, withReason(reasonBadCursor, fmt.Errorf("the file is shorter than the cursor, so it was replaced: %w", errBadCursor))
		}
		if from == size {
			reply.Cursor = req.Since
			return reply, nil
		}
		lines, end, err := readLines(path, from, size, true)
		if err != nil {
			return transcriptReply{}, err
		}
		parsed := parse(lines)
		items := parsed.items
		if len(items) > transcriptMaxLimit {
			items, reply.More = items[:transcriptMaxLimit], true
			end = items[len(items)-1].pos + 1
		}
		reply.Items = boundItems(items)
		reply.Cursor = fileCursor(sinceCursorEnd(end, parsed.openPos, reply.More))
		return reply, nil
	}

	before := size
	if req.Before != "" {
		if before, err = parseFileCursor(req.Before); err != nil {
			return transcriptReply{}, err
		}
		if before > size {
			return transcriptReply{}, withReason(reasonBadCursor, fmt.Errorf("the file is shorter than the cursor, so it was replaced: %w", errBadCursor))
		}
	}
	window := transcriptWindow
	for {
		start := before - window
		if start < 0 {
			start = 0
		}
		lines, end, err := readLines(path, start, size, start == 0)
		if err != nil {
			return transcriptReply{}, err
		}
		parsed := parse(lines)
		var items []transcriptItem
		for _, item := range parsed.items {
			if item.pos < before {
				items = append(items, item)
			}
		}
		covered := start == 0 || window >= transcriptMaxWindow
		// An item whose line starts inside the first window may be missing its early part, so
		// only a page with limit+1 items, or the whole file, is final.
		if len(items) > limit || covered {
			if len(items) > limit {
				items, reply.More = items[len(items)-limit:], true
			} else {
				reply.More = start > 0
			}
			reply.Items = boundItems(items)
			if reply.More && len(items) > 0 {
				reply.Older = fileCursor(items[0].pos)
			}
			reply.Cursor = fileCursor(sinceCursorEnd(end, parsed.openPos, false))
			return reply, nil
		}
		window *= 2
	}
}

// sinceCursorEnd is where the next since read starts: the end of what was read, held back
// to the earliest tool call still waiting for its result.
func sinceCursorEnd(end, openPos int64, partial bool) int64 {
	if openPos >= 0 && openPos < end {
		return openPos
	}
	return end
}

// boundItems keeps a reply's total text under the cap, dropping the oldest items first.
func boundItems(items []transcriptItem) []transcriptItem {
	total := 0
	for index := len(items) - 1; index >= 0; index-- {
		size := len(items[index].Text)
		if items[index].Tool != nil {
			size += len(items[index].Tool.Output)
		}
		total += size
		if total > transcriptReplyCap {
			return items[index+1:]
		}
	}
	return items
}

// sortItems orders file items by where their line began.
func sortItems(items []transcriptItem) {
	sort.SliceStable(items, func(a, b int) bool { return items[a].pos < items[b].pos })
}
