package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// codexTranscriptLine is the part of a rollout line this reader uses. The typed
// item_completed events carry what the raw response items spread over several lines.
type codexTranscriptLine struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	Payload   struct {
		Type   string `json:"type"`
		TurnID string `json:"turn_id"`
		Model  string `json:"model"`
		Item   struct {
			Type             string          `json:"type"`
			ID               string          `json:"id"`
			Phase            string          `json:"phase"`
			Content          json.RawMessage `json:"content"`
			Command          json.RawMessage `json:"command"`
			Status           string          `json:"status"`
			ExitCode         *int            `json:"exit_code"`
			AggregatedOutput string          `json:"aggregated_output"`
			Stdout           string          `json:"stdout"`
			Stderr           string          `json:"stderr"`
			Server           string          `json:"server"`
			Tool             string          `json:"tool"`
			Result           json.RawMessage `json:"result"`
			Changes          map[string]struct {
				UnifiedDiff string `json:"unified_diff"`
			} `json:"changes"`
		} `json:"item"`
	} `json:"payload"`
}

// codexTranscript reads a codex seat's rollout.
func codexTranscript(entry ledgerEntry, all []ledgerEntry, req transcriptRequest) (transcriptReply, error) {
	path, err := codexRollout(entry, all)
	if err != nil {
		return transcriptReply{}, err
	}
	return fileTranscript("codex", path, req, parseCodex)
}

// codexTurn is what the reader knows of one turn: its current root, so an item answers
// it.
type codexTurn struct {
	items    int
	root     string
	rootPeer bool
	shared   bool
}

// parseCodex turns rollout lines into items. A turn's first user item is its prompt,
// and a user item after other items landed inside a running turn.
func parseCodex(lines []jsonLine) parsedLines {
	var items []transcriptItem
	turns := map[string]*codexTurn{}
	model := ""
	for _, line := range lines {
		var entry codexTranscriptLine
		if json.Unmarshal(line.raw, &entry) != nil {
			continue
		}
		payload := entry.Payload
		if entry.Type == "turn_context" && payload.Model != "" {
			model = payload.Model
		}
		if entry.Type != "event_msg" || payload.Type != "item_completed" {
			continue
		}
		item := payload.Item
		turn := turns[payload.TurnID]
		if turn == nil {
			turn = &codexTurn{}
			turns[payload.TurnID] = turn
		}
		out := transcriptItem{ID: item.ID, At: entry.Timestamp, pos: line.pos}
		switch item.Type {
		case "UserMessage":
			text := strings.TrimSpace(codexText(item.Content))
			if text == "" {
				continue
			}
			out.Kind = transcriptKindPrompt
			peer := false
			if from, body, ok := peerOf(text); ok {
				out.Kind, out.From, text, peer = transcriptKindPeer, &from, body, true
			}
			out.setText(text)
			turn.root, turn.rootPeer, turn.shared = item.ID, peer, peer && turn.items > 0
		case "AgentMessage":
			text := strings.TrimSpace(codexText(item.Content))
			if text == "" {
				continue
			}
			out.Kind, out.Model = transcriptKindReply, model
			if item.Phase == "final_answer" {
				out.Complete = boolPtr(true)
			}
			out.setText(text)
		case "CommandExecution":
			out.Kind, out.Model, out.Tool = transcriptKindTool, model, codexCommand(item.Command, item.Status, item.ExitCode, item.AggregatedOutput)
		case "FileChange":
			out.Kind, out.Model, out.Tool = transcriptKindTool, model, codexFileChange(item.Changes, item.Status, item.Stdout+item.Stderr)
		case "McpToolCall":
			out.Kind, out.Model, out.Tool = transcriptKindTool, model, codexMCP(item.Server, item.Tool, item.Status, item.Result)
		case "ContextCompaction":
			out.Kind = transcriptKindNote
			out.setText("Context compacted")
		default:
			continue
		}
		if out.Kind != transcriptKindPrompt && out.Kind != transcriptKindPeer && turn.rootPeer {
			out.For, out.Shared = turn.root, turn.shared
			// A mid-turn peer is answered by the next reply, and what follows is the task's.
			if turn.shared && out.Kind == transcriptKindReply {
				turn.rootPeer = false
			}
		}
		turn.items++
		items = append(items, out)
	}
	return parsedLines{items: items, openPos: -1}
}

// codexText joins the text of a content list.
func codexText(raw json.RawMessage) string {
	var blocks []struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) != nil {
		return ""
	}
	var parts []string
	for _, block := range blocks {
		parts = append(parts, block.Text)
	}
	return strings.Join(parts, "\n")
}

// codexOK is a step's outcome from its status and exit code. Codex has no error flag.
func codexOK(status string, exit *int) *bool {
	switch {
	case status == "failed" || status == "declined" || (exit != nil && *exit != 0):
		return boolPtr(false)
	case status == "completed":
		return boolPtr(true)
	}
	return nil
}

func codexCommand(raw json.RawMessage, status string, exit *int, output string) *transcriptTool {
	var argv []string
	_ = json.Unmarshal(raw, &argv)
	command := ""
	if len(argv) > 0 {
		command = argv[len(argv)-1]
	}
	tool := &transcriptTool{Name: "exec", Class: transcriptClassShell, Summary: "Ran " + summaryLine(command, 100), OK: codexOK(status, exit)}
	tool.setOutput(output)
	return tool
}

func codexFileChange(changes map[string]struct {
	UnifiedDiff string `json:"unified_diff"`
}, status, output string) *transcriptTool {
	var files []string
	for path := range changes {
		files = append(files, path)
	}
	sort.Strings(files)
	diff := transcriptDiff{Files: len(files)}
	for _, path := range files {
		added, removed := diffCounts(changes[path].UnifiedDiff)
		diff.Added, diff.Removed = diff.Added+added, diff.Removed+removed
	}
	tool := &transcriptTool{Name: "apply_patch", Class: transcriptClassEdit, Summary: diffSummary("Edited", files, diff), OK: codexOK(status, nil), Diff: &diff}
	tool.setOutput(output)
	return tool
}

func codexMCP(server, name, status string, result json.RawMessage) *transcriptTool {
	tool := &transcriptTool{Name: server + "." + name, Class: transcriptClassMCP, Summary: fmt.Sprintf("Called %s.%s", server, name), OK: codexOK(status, nil)}
	var body struct {
		Content json.RawMessage `json:"content"`
	}
	if json.Unmarshal(result, &body) == nil {
		tool.setOutput(codexText(body.Content))
	}
	return tool
}
