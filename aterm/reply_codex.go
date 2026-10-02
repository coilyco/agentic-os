package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// codexLine is the part of a codex rollout line the reader uses. Content stays
// raw because only a message carries blocks, and other lines put other shapes there.
type codexLine struct {
	Type    string `json:"type"`
	Payload struct {
		Type             string          `json:"type"`
		Role             string          `json:"role"`
		Phase            string          `json:"phase"`
		Content          json.RawMessage `json:"content"`
		LastAgentMessage string          `json:"last_agent_message"`
		Cwd              string          `json:"cwd"`
		Timestamp        string          `json:"timestamp"`
	} `json:"payload"`
}

// lastCodexReply is the latest turn's final_answer, or the commentary since the last tool
// output while it runs. A turn runs from task_started to task_complete or turn_aborted.
func lastCodexReply(r io.Reader) (string, bool, error) {
	reader := bufio.NewReaderSize(r, 1<<20)
	var final, commentary []string
	complete := false
	for {
		raw, err := reader.ReadBytes('\n')
		var line codexLine
		if len(bytes.TrimSpace(raw)) > 0 && json.Unmarshal(raw, &line) == nil {
			payload := line.Payload
			switch {
			case line.Type == "event_msg" && payload.Type == "task_started":
				final, commentary, complete = nil, nil, false
			case line.Type == "event_msg" && payload.Type == "task_complete":
				complete = true
				if len(final) == 0 && strings.TrimSpace(payload.LastAgentMessage) != "" {
					final = []string{strings.TrimSpace(payload.LastAgentMessage)}
				}
			case line.Type == "event_msg" && payload.Type == "turn_aborted":
				complete = true
			case line.Type == "response_item" && strings.HasSuffix(payload.Type, "_output"):
				commentary = nil
			case line.Type == "response_item" && payload.Type == "message" && payload.Role == "assistant":
				var blocks []contentBlock
				_ = json.Unmarshal(payload.Content, &blocks)
				for _, block := range blocks {
					text := strings.TrimSpace(block.Text)
					if block.Type != "output_text" || text == "" {
						continue
					}
					if payload.Phase == "final_answer" {
						final = append(final, text)
					} else {
						commentary = append(commentary, text)
					}
				}
			}
		}
		if err == io.EOF {
			if len(final) > 0 {
				return strings.Join(final, "\n\n"), complete, nil
			}
			return strings.Join(commentary, "\n\n"), complete, nil
		}
		if err != nil {
			return "", false, err
		}
	}
}

// codexMeta is the cwd and start of the rollout in a file, from its first line.
func codexMeta(path string) (cwd string, started time.Time, ok bool) {
	file, err := os.Open(path)
	if err != nil {
		return "", time.Time{}, false
	}
	defer file.Close()
	raw, err := bufio.NewReaderSize(file, 1<<16).ReadBytes('\n')
	var line codexLine
	if (err != nil && err != io.EOF) || json.Unmarshal(raw, &line) != nil || line.Type != "session_meta" {
		return "", time.Time{}, false
	}
	started, err = time.Parse(time.RFC3339Nano, line.Payload.Timestamp)
	return line.Payload.Cwd, started, err == nil && line.Payload.Cwd != ""
}

// codexRollout finds a seat's rollout by cwd and a start no earlier than its own, newest
// first. The scan starts a day early, since a date directory is local and a start is UTC.
func codexRollout(home string, entry ledgerEntry) (string, error) {
	root := filepath.Join(home, ".codex", "sessions")
	firstDay := entry.Started.AddDate(0, 0, -1).Format("2006/01/02")
	best, bestStart := "", time.Time{}
	for _, year := range subdirs(root) {
		for _, month := range subdirs(filepath.Join(root, year)) {
			for _, day := range subdirs(filepath.Join(root, year, month)) {
				if year+"/"+month+"/"+day < firstDay {
					continue
				}
				files, _ := filepath.Glob(filepath.Join(root, year, month, day, "rollout-*.jsonl"))
				for _, path := range files {
					cwd, started, ok := codexMeta(path)
					if !ok || cwd != entry.Cwd || started.Before(entry.Started.Add(-5*time.Second)) {
						continue
					}
					if entry.Ended != nil && started.After(entry.Ended.Add(5*time.Second)) {
						continue
					}
					if started.After(bestStart) {
						best, bestStart = path, started
					}
				}
			}
		}
	}
	if best == "" {
		return "", withExit(exitOffRoster, fmt.Errorf("no codex rollout for %s under %s: %w", entry.Name, root, errNoTranscript))
	}
	return best, nil
}

// subdirs lists a directory's subdirectories in order, or none when it is missing.
func subdirs(path string) []string {
	entries, _ := os.ReadDir(path)
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	slices.Sort(names)
	return names
}

func codexReply(entry ledgerEntry) (replyView, error) {
	home := entry.Home
	if home == "" {
		home = realHome()
	}
	path, err := codexRollout(home, entry)
	if err != nil {
		return replyView{}, err
	}
	file, err := os.Open(path)
	if err != nil {
		return replyView{}, err
	}
	defer file.Close()
	text, complete, err := lastCodexReply(file)
	if err != nil {
		return replyView{}, err
	}
	return replyView{Session: entry.Name, Text: text, Complete: complete, Source: path}, nil
}
