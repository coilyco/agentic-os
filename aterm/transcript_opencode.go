package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var opencodePartID = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

// opencodeBuiltins are tools opencode ships, so any other name is an MCP server's.
var opencodeBuiltins = map[string]bool{
	"bash": true, "edit": true, "write": true, "read": true, "glob": true, "grep": true, "skill": true,
	"patch": true, "webfetch": true, "websearch": true, "todowrite": true, "todoread": true, "task": true, "list": true,
}

// opencodePartRow is one part of one message, with the message's own fields beside it.
type opencodePartRow struct {
	PartID    string `json:"pid"`
	PartTime  int64  `json:"ptime"`
	PartType  string `json:"ptype"`
	Text      string `json:"text"`
	Synthetic any    `json:"synthetic"`
	Tool      string `json:"tool"`
	Status    string `json:"status"`
	Command   string `json:"command"`
	FilePath  string `json:"filepath"`
	Pattern   string `json:"pattern"`
	Output    string `json:"out"`
	OutputLen int    `json:"outlen"`
	Error     string `json:"err"`
	Exit      *int   `json:"exit"`
	Diff      string `json:"diff"`
	Message   string `json:"mid"`
	Role      string `json:"role"`
	Model     string `json:"model"`
	Parent    string `json:"parent"`
	Finish    string `json:"finish"`
	Created   int64  `json:"mcreated"`
}

// opencodeAssistant is when an assistant message began and ended, to tell whether a user
// message was queued while one was still running.
type opencodeAssistant struct {
	Created int64  `json:"t"`
	Done    int64  `json:"done"`
	Finish  string `json:"finish"`
}

func opencodeCursor(row opencodePartRow) string {
	return fmt.Sprintf("%s%d:%s", transcriptCursorOpenSQL, row.PartTime, row.PartID)
}

func parseOpencodeCursor(cursor string) (int64, string, error) {
	rest := strings.TrimPrefix(cursor, transcriptCursorOpenSQL)
	at := strings.SplitN(rest, ":", 2)
	if !strings.HasPrefix(cursor, transcriptCursorOpenSQL) || len(at) != 2 || !opencodePartID.MatchString(at[1]) {
		return 0, "", withReason(reasonBadCursor, fmt.Errorf("%q is not a cursor this daemon issued: %w", cursor, errBadCursor))
	}
	millis, err := strconv.ParseInt(at[0], 10, 64)
	if err != nil || millis < 0 {
		return 0, "", withReason(reasonBadCursor, fmt.Errorf("%q is not a cursor this daemon issued: %w", cursor, errBadCursor))
	}
	return millis, at[1], nil
}

const opencodePartColumns = `p.id AS pid, p.time_created AS ptime, json_extract(p.data,'$.type') AS ptype,
  json_extract(p.data,'$.text') AS text, json_extract(p.data,'$.synthetic') AS synthetic,
  json_extract(p.data,'$.tool') AS tool, json_extract(p.data,'$.state.status') AS status,
  json_extract(p.data,'$.state.input.command') AS command, json_extract(p.data,'$.state.input.filePath') AS filepath,
  coalesce(json_extract(p.data,'$.state.input.pattern'), json_extract(p.data,'$.state.input.query')) AS pattern,
  substr(json_extract(p.data,'$.state.output'),1,70000) AS out, length(json_extract(p.data,'$.state.output')) AS outlen,
  json_extract(p.data,'$.state.error') AS err, json_extract(p.data,'$.state.metadata.exit') AS exit,
  substr(json_extract(p.data,'$.state.metadata.diff'),1,400000) AS diff,
  m.id AS mid, json_extract(m.data,'$.role') AS role, json_extract(m.data,'$.modelID') AS model,
  json_extract(m.data,'$.parentID') AS parent, json_extract(m.data,'$.finish') AS finish, m.time_created AS mcreated`

// opencodeTranscript reads an opencode seat's session. Parts are ordered by when they
// were created and their id, and a cursor names one.
func opencodeTranscript(entry ledgerEntry, all []ledgerEntry, req transcriptRequest) (transcriptReply, error) {
	db := opencodeDB(entry)
	session, err := opencodeSession(db, entry, all)
	if err != nil {
		return transcriptReply{}, err
	}
	if !opencodeSessionID.MatchString(session) {
		return transcriptReply{}, withReason(reasonUnreadableFormat, fmt.Errorf("%q is not an opencode session id: %w", session, errUnreadableFormat))
	}
	limit := req.limit()
	where, order, rowLimit := "", "ASC", transcriptMaxLimit*3
	switch {
	case req.Since != "":
		millis, id, err := parseOpencodeCursor(req.Since)
		if err != nil {
			return transcriptReply{}, err
		}
		where = fmt.Sprintf(" AND (p.time_created, p.id) > (%d, '%s')", millis, id)
	case req.Before != "":
		millis, id, err := parseOpencodeCursor(req.Before)
		if err != nil {
			return transcriptReply{}, err
		}
		where = fmt.Sprintf(" AND (p.time_created, p.id) < (%d, '%s')", millis, id)
		order, rowLimit = "DESC", limit*3+1
	default:
		order, rowLimit = "DESC", limit*3+1
	}
	query := fmt.Sprintf(`SELECT %s FROM part p JOIN message m ON m.id = p.message_id
WHERE p.session_id = '%s' AND json_extract(p.data,'$.type') IN ('text','tool','compaction')%s
ORDER BY p.time_created %s, p.id %s LIMIT %d`, opencodePartColumns, session, where, order, order, rowLimit)
	var rows []opencodePartRow
	if err := sqliteJSON(db, query, &rows); err != nil {
		return transcriptReply{}, err
	}
	more := len(rows) == rowLimit && order == "DESC"
	if order == "DESC" {
		for a, b := 0, len(rows)-1; a < b; a, b = a+1, b-1 {
			rows[a], rows[b] = rows[b], rows[a]
		}
	}
	var assistants []opencodeAssistant
	if err := sqliteJSON(db, fmt.Sprintf(`SELECT time_created AS t, coalesce(json_extract(data,'$.time.completed'),0) AS done,
  coalesce(json_extract(data,'$.finish'),'') AS finish FROM message WHERE session_id = '%s' AND json_extract(data,'$.role') = 'assistant'`, session), &assistants); err != nil {
		return transcriptReply{}, err
	}
	items, keys, openKey := opencodeItems(rows, assistants)
	reply := transcriptReply{Reader: "opencode", Items: items, UpdatedAt: time.Now()}
	if req.Since != "" {
		if len(items) > transcriptMaxLimit {
			items, keys = items[:transcriptMaxLimit], keys[:transcriptMaxLimit]
			reply.Items, reply.More = items, true
		}
		reply.Cursor = firstNonEmpty(opencodeHeldCursor(keys, openKey), req.Since)
		reply.Items = boundItems(reply.Items)
		return reply, nil
	}
	if len(items) > limit {
		dropped := len(items) - limit
		items, keys = items[dropped:], keys[dropped:]
		more = true
	}
	reply.Items, reply.More = boundItems(items), more
	if more && len(keys) > 0 {
		reply.Older = keys[0]
	}
	reply.Cursor = opencodeHeldCursor(keys, openKey)
	if len(rows) > 0 {
		reply.UpdatedAt = time.UnixMilli(rows[len(rows)-1].PartTime)
	}
	return reply, nil
}

// opencodeHeldCursor is the cursor of the last item before the earliest running tool,
// so a since read returns the tool again when it finishes.
func opencodeHeldCursor(keys []string, openKey string) string {
	if len(keys) == 0 {
		return ""
	}
	if openKey == "" {
		return keys[len(keys)-1]
	}
	for index, key := range keys {
		if key == openKey {
			if index == 0 {
				return ""
			}
			return keys[index-1]
		}
	}
	return keys[len(keys)-1]
}

// opencodeItems groups rows into items, with each item's cursor and the cursor of the
// earliest tool still running.
func opencodeItems(rows []opencodePartRow, assistants []opencodeAssistant) ([]transcriptItem, []string, string) {
	var items []transcriptItem
	var keys []string
	peers := map[string]bool{}  // user message id to whether it is a peer message
	shared := map[string]bool{} // user message id to whether it landed mid-turn
	userAt := map[string]int{}  // user message id to its item index
	openKey := ""
	for _, row := range rows {
		key := opencodeCursor(row)
		at := time.UnixMilli(row.PartTime).UTC().Format(time.RFC3339Nano)
		switch {
		case row.Role == "user" && row.PartType == "text":
			text := strings.TrimSpace(row.Text)
			if text == "" || row.Synthetic == true || row.Synthetic == float64(1) {
				continue
			}
			if index, ok := userAt[row.Message]; ok {
				items[index].setText(items[index].Text + "\n\n" + text)
				continue
			}
			item := transcriptItem{ID: row.Message, Kind: transcriptKindPrompt, At: at}
			if from, body, ok := peerOf(text); ok {
				item.Kind, item.From, text = transcriptKindPeer, &from, body
				peers[row.Message] = true
				shared[row.Message] = opencodeQueued(row.Created, assistants)
			}
			item.setText(text)
			userAt[row.Message] = len(items)
			items = append(items, item)
			keys = append(keys, key)
		case row.Role == "assistant" && row.PartType == "text":
			text := strings.TrimSpace(row.Text)
			if text == "" || row.Synthetic == true || row.Synthetic == float64(1) {
				continue
			}
			item := transcriptItem{ID: row.PartID, Kind: transcriptKindReply, At: at, Model: row.Model}
			if row.Finish != "" && row.Finish != "tool-calls" {
				item.Complete = boolPtr(true)
			}
			item.setText(text)
			opencodeTag(&item, row, peers, shared)
			items = append(items, item)
			keys = append(keys, key)
		case row.PartType == "tool":
			item := transcriptItem{ID: row.PartID, Kind: transcriptKindTool, At: at, Model: row.Model, Tool: opencodeTool(row)}
			opencodeTag(&item, row, peers, shared)
			if item.Tool.OK == nil && openKey == "" {
				openKey = key
			}
			items = append(items, item)
			keys = append(keys, key)
		case row.PartType == "compaction":
			item := transcriptItem{ID: row.PartID, Kind: transcriptKindNote, At: at}
			item.setText("Context compacted")
			items = append(items, item)
			keys = append(keys, key)
		}
	}
	return items, keys, openKey
}

// opencodeTag says an answer is for a peer message, by the user message it answers.
func opencodeTag(item *transcriptItem, row opencodePartRow, peers, shared map[string]bool) {
	if row.Parent != "" && peers[row.Parent] {
		item.For, item.Shared = row.Parent, shared[row.Parent]
	}
}

// opencodeQueued says whether an assistant message was still running when a user message
// was created, which is how a message lands inside a turn.
func opencodeQueued(created int64, assistants []opencodeAssistant) bool {
	for _, assistant := range assistants {
		if assistant.Created < created && (assistant.Finish == "" || assistant.Done > created) {
			return true
		}
	}
	return false
}

func opencodeTool(row opencodePartRow) *transcriptTool {
	tool := &transcriptTool{Name: row.Tool, Class: transcriptClassOther, Summary: row.Tool}
	switch name := row.Tool; {
	case name == "bash":
		tool.Class, tool.Summary = transcriptClassShell, "Ran "+summaryLine(row.Command, 100)
	case name == "edit" || name == "write" || name == "patch":
		tool.Class = transcriptClassEdit
		verb := map[string]string{"edit": "Edited", "write": "Wrote", "patch": "Patched"}[name]
		diff := transcriptDiff{Files: 1}
		diff.Added, diff.Removed = diffCounts(row.Diff)
		if row.Diff == "" && name == "write" {
			diff.Added = 0
		}
		tool.Diff, tool.Summary = &diff, diffSummary(verb, []string{row.FilePath}, diff)
	case name == "read" || name == "list":
		tool.Class, tool.Summary = transcriptClassRead, "Read "+baseName(row.FilePath)
	case name == "grep" || name == "glob" || name == "websearch":
		tool.Class, tool.Summary = transcriptClassSearch, "Searched "+summaryLine(row.Pattern, 80)
	case !opencodeBuiltins[name]:
		tool.Class, tool.Summary = transcriptClassMCP, "Called "+name
	}
	switch row.Status {
	case "completed":
		tool.OK = boolPtr(row.Exit == nil || *row.Exit == 0)
	case "error":
		tool.OK = boolPtr(false)
	}
	output := row.Output
	if row.Status == "error" && output == "" {
		output = row.Error
	}
	if tool.Class != transcriptClassEdit {
		tool.setOutput(output)
		if row.OutputLen > len(output) {
			tool.Truncated, tool.OutputBytes = true, row.OutputLen
		}
	}
	return tool
}
