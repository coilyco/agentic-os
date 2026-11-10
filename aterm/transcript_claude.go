package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// claudeLine is the part of a claude transcript line this reader uses. The format is
// claude's and unversioned, so a field not named here is never read.
type claudeLine struct {
	Type        string          `json:"type"`
	UUID        string          `json:"uuid"`
	ParentUUID  string          `json:"parentUuid"`
	Timestamp   string          `json:"timestamp"`
	IsMeta      bool            `json:"isMeta"`
	IsSidechain bool            `json:"isSidechain"`
	Origin      json.RawMessage `json:"origin"`
	Message     struct {
		ID         string          `json:"id"`
		Model      string          `json:"model"`
		StopReason string          `json:"stop_reason"`
		Content    json.RawMessage `json:"content"`
	} `json:"message"`
	ToolUseResult json.RawMessage `json:"toolUseResult"`
	Attachment    struct {
		Type   string          `json:"type"`
		Prompt string          `json:"prompt"`
		Origin json.RawMessage `json:"origin"`
	} `json:"attachment"`
}

// claudeOrigin is who a user line or queued message came from.
type claudeOrigin struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
	Body string `json:"body"`
}

func originOf(raw json.RawMessage) claudeOrigin {
	var origin claudeOrigin
	_ = json.Unmarshal(raw, &origin)
	return origin
}

// claudeBlock is one content block of a message.
type claudeBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
	IsError   bool            `json:"is_error"`
}

// blocksOf is a message's content as blocks. A bare string is one text block.
func blocksOf(raw json.RawMessage) []claudeBlock {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return []claudeBlock{{Type: "text", Text: text}}
	}
	var blocks []claudeBlock
	_ = json.Unmarshal(raw, &blocks)
	return blocks
}

// toolResultText is a tool_result's content, a string or a list of text blocks.
func toolResultText(raw json.RawMessage) string {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	var parts []claudeBlock
	if json.Unmarshal(raw, &parts) != nil {
		return ""
	}
	var texts []string
	for _, part := range parts {
		if part.Type == "text" || part.Text != "" {
			texts = append(texts, part.Text)
		}
	}
	return strings.Join(texts, "\n")
}

// claudeNode is a line's place in the parent chain, for working out which turn it
// belongs to.
type claudeNode struct {
	parent string
	// marker is the id of the prompt or peer item this line is, if it is one.
	marker string
	peer   bool
	shared bool
}

// claudeTranscript reads a claude seat's transcript file.
func claudeTranscript(path string, req transcriptRequest, all []ledgerEntry) (transcriptReply, error) {
	return fileTranscript("claude", path, req, func(lines []jsonLine) parsedLines {
		return parseClaude(lines, all)
	})
}

// parseClaude turns lines into items. One content block is one line, so a reply is
// several lines sharing a message id.
func parseClaude(lines []jsonLine, all []ledgerEntry) parsedLines {
	var items []transcriptItem
	byTool := map[string]int{}
	nodes := map[string]*claudeNode{}
	root := map[string]string{} // uuid to the nearest marker's uuid
	replyAt := map[string]int{} // message id to the index of its latest reply item
	replyCount := map[string]int{}
	lastReply := -1

	rootOf := func(uuid string) *claudeNode {
		var chain []string
		for current := uuid; current != "" && len(chain) < 4096; {
			if known, ok := root[current]; ok {
				current = known
				if node := nodes[current]; node != nil && node.marker != "" {
					return node
				}
				break
			}
			node := nodes[current]
			if node == nil {
				break
			}
			if node.marker != "" {
				return node
			}
			chain = append(chain, current)
			current = node.parent
		}
		return nil
	}
	tag := func(item *transcriptItem, uuid string) {
		if node := rootOf(uuid); node != nil && node.peer {
			item.For, item.Shared = node.marker, node.shared
		}
	}

	for _, line := range lines {
		var entry claudeLine
		if json.Unmarshal(line.raw, &entry) != nil || entry.IsSidechain {
			continue
		}
		node := &claudeNode{parent: entry.ParentUUID}
		if entry.UUID != "" {
			nodes[entry.UUID] = node
		}
		switch entry.Type {
		case "user":
			origin := originOf(entry.Origin)
			if origin.Kind == "peer" {
				item := transcriptItem{ID: entry.UUID, Kind: transcriptKindPeer, At: entry.Timestamp, pos: line.pos}
				from := seatNamed(origin.Name, all)
				item.From = &from
				item.setText(origin.Body)
				items = append(items, item)
				node.marker, node.peer = entry.UUID, true
				lastReply = -1
				continue
			}
			var texts []string
			sawResult := false
			for _, block := range blocksOf(entry.Message.Content) {
				switch block.Type {
				case "tool_result":
					sawResult = true
					if index, ok := byTool[block.ToolUseID]; ok {
						attachClaudeResult(&items[index], block, entry.ToolUseResult)
					}
				case "text":
					texts = append(texts, block.Text)
				}
			}
			if sawResult || entry.IsMeta {
				continue
			}
			text := strings.TrimSpace(strings.Join(texts, "\n"))
			if text == "" || claudeSystemText(text) {
				continue
			}
			item := transcriptItem{ID: entry.UUID, Kind: transcriptKindPrompt, At: entry.Timestamp, pos: line.pos}
			if from, body, ok := peerOf(text); ok {
				item.Kind, item.From, text = transcriptKindPeer, &from, body
				node.peer = true
			}
			item.setText(text)
			node.marker = entry.UUID
			items = append(items, item)
			lastReply = -1
		case "attachment":
			if entry.Attachment.Type != "queued_command" {
				continue
			}
			origin := originOf(entry.Attachment.Origin)
			text := strings.TrimSpace(entry.Attachment.Prompt)
			item := transcriptItem{ID: entry.UUID, Kind: transcriptKindPrompt, At: entry.Timestamp, pos: line.pos}
			switch {
			case origin.Kind == "peer":
				from := seatNamed(origin.Name, all)
				item.Kind, item.From, text = transcriptKindPeer, &from, firstNonEmpty(origin.Body, text)
				node.peer = true
			case origin.Kind == "human":
				if from, body, ok := peerOf(text); ok {
					item.Kind, item.From, text = transcriptKindPeer, &from, body
					node.peer = true
				}
			default:
				continue
			}
			// A message queued while a turn ran lands in that turn, so what follows may also
			// continue the earlier task: the file cannot separate the two.
			node.shared = node.peer
			node.marker = entry.UUID
			item.Shared = false
			item.setText(text)
			items = append(items, item)
			lastReply = -1
		case "assistant":
			for _, block := range blocksOf(entry.Message.Content) {
				switch block.Type {
				case "text":
					text := strings.TrimSpace(block.Text)
					if text == "" {
						continue
					}
					if lastReply >= 0 && items[lastReply].ID == claudeReplyID(entry.Message.ID, replyCount[entry.Message.ID]) {
						items[lastReply].setText(items[lastReply].Text + "\n\n" + text)
					} else {
						replyCount[entry.Message.ID]++
						item := transcriptItem{ID: claudeReplyID(entry.Message.ID, replyCount[entry.Message.ID]), Kind: transcriptKindReply,
							At: entry.Timestamp, Model: entry.Message.Model, pos: line.pos}
						item.setText(text)
						tag(&item, entry.ParentUUID)
						items = append(items, item)
						lastReply = len(items) - 1
						replyAt[entry.Message.ID] = lastReply
					}
					if entry.Message.StopReason == "end_turn" {
						items[lastReply].Complete = boolPtr(true)
					}
				case "tool_use":
					item := claudeToolItem(block, entry, line.pos)
					tag(&item, entry.ParentUUID)
					byTool[block.ID] = len(items)
					items = append(items, item)
					lastReply = -1
				}
			}
		}
	}
	return parsedLines{items: items, openPos: openClaudeCall(items)}
}

func claudeReplyID(message string, n int) string {
	if n <= 1 {
		return message
	}
	return fmt.Sprintf("%s:%d", message, n)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// claudeSystemText is text claude writes into a user line that no person typed.
func claudeSystemText(text string) bool {
	for _, prefix := range []string{"<local-command", "<command-name>", "<command-message>", "<system-reminder>", "<task-notification>", "Caveat: The messages below"} {
		if strings.HasPrefix(text, prefix) {
			return true
		}
	}
	return false
}

// openClaudeCall is the offset of the earliest tool call with no result yet, or -1.
func openClaudeCall(items []transcriptItem) int64 {
	open := int64(-1)
	for _, item := range items {
		if item.Tool != nil && item.Tool.OK == nil && (open < 0 || item.pos < open) {
			open = item.pos
		}
	}
	return open
}

func claudeToolItem(block claudeBlock, entry claudeLine, pos int64) transcriptItem {
	var input map[string]any
	_ = json.Unmarshal(block.Input, &input)
	str := func(key string) string {
		value, _ := input[key].(string)
		return value
	}
	tool := &transcriptTool{Name: block.Name, Class: transcriptClassOther, Summary: block.Name}
	switch name := block.Name; {
	case name == "Bash":
		tool.Class, tool.Summary = transcriptClassShell, "Ran "+summaryLine(str("command"), 100)
	case name == "Edit" || name == "MultiEdit" || name == "NotebookEdit":
		tool.Class, tool.verb, tool.subject = transcriptClassEdit, "Edited", firstNonEmpty(str("file_path"), str("notebook_path"))
		tool.Summary = "Edited " + baseName(tool.subject)
	case name == "Write":
		tool.Class, tool.verb, tool.subject = transcriptClassEdit, "Wrote", str("file_path")
		tool.Summary = "Wrote " + baseName(tool.subject)
	case name == "Read":
		tool.Class, tool.Summary = transcriptClassRead, "Read "+baseName(str("file_path"))
	case name == "Grep" || name == "Glob" || name == "WebSearch":
		tool.Class, tool.Summary = transcriptClassSearch, "Searched "+summaryLine(firstNonEmpty(str("pattern"), str("query")), 80)
	case strings.HasPrefix(name, "mcp__"):
		tool.Class, tool.Summary = transcriptClassMCP, "Called "+strings.Replace(strings.TrimPrefix(name, "mcp__"), "__", ".", 1)
	}
	return transcriptItem{ID: block.ID, Kind: transcriptKindTool, At: entry.Timestamp, Model: entry.Message.Model, Tool: tool, pos: pos}
}

// attachClaudeResult fills a tool item from its result block and the line's own richer
// record.
func attachClaudeResult(item *transcriptItem, block claudeBlock, rich json.RawMessage) {
	tool := item.Tool
	tool.OK = boolPtr(!block.IsError)
	output := toolResultText(block.Content)
	var result struct {
		Stdout          string `json:"stdout"`
		Stderr          string `json:"stderr"`
		StructuredPatch []struct {
			Lines []string `json:"lines"`
		} `json:"structuredPatch"`
		Type    string `json:"type"`
		Content string `json:"content"`
	}
	if json.Unmarshal(rich, &result) == nil {
		if result.Stdout != "" || result.Stderr != "" {
			output = strings.TrimRight(result.Stdout+result.Stderr, "\n")
			if block.IsError {
				output = toolResultText(block.Content)
			}
		}
		if tool.Class == transcriptClassEdit {
			diff := transcriptDiff{Files: 1}
			for _, hunk := range result.StructuredPatch {
				added, removed := diffCounts(strings.Join(hunk.Lines, "\n"))
				diff.Added, diff.Removed = diff.Added+added, diff.Removed+removed
			}
			if len(result.StructuredPatch) == 0 && result.Type == "create" {
				diff.Added = strings.Count(result.Content, "\n") + 1
			}
			tool.Diff = &diff
			tool.Summary = diffSummary(tool.verb, []string{tool.subject}, diff)
			output = ""
		}
	}
	tool.setOutput(output)
}
