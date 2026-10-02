package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/urfave/cli/v3"
)

// replyView is a seat's latest reply as its harness wrote it down. `aterm status`
// shows only the visible rows, so a long reply that scrolled off is read here.
type replyView struct {
	Session string `json:"session"`
	Text    string `json:"text"`
	// Complete is whether the turn ended. Mid-turn the text is what exists so far.
	Complete bool `json:"complete"`
	// Source is the transcript path or session read, or "screen" for the visible rows.
	Source string `json:"source"`
}

// errNoTranscript is a seat whose transcript is missing or unreadable, which the
// verb answers with the screen rows. Its wrapper keeps the exit code.
var errNoTranscript = errors.New("no transcript")

// transcriptLine is the part of a claude transcript line the reader uses. The
// format is claude's and unversioned, so a field not named here is never read.
type transcriptLine struct {
	Type        string `json:"type"`
	IsSidechain bool   `json:"isSidechain"`
	Message     struct {
		Content    json.RawMessage `json:"content"`
		StopReason string          `json:"stop_reason"`
	} `json:"message"`
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// lastReply is the text claude wrote since the last user line, and whether the turn
// ended. A tool result is a user line, so narration before a tool call is dropped.
func lastReply(r io.Reader) (string, bool, error) {
	reader := bufio.NewReaderSize(r, 1<<20)
	var parts []string
	complete := false
	for {
		raw, err := reader.ReadBytes('\n')
		var line transcriptLine
		if len(bytes.TrimSpace(raw)) > 0 && json.Unmarshal(raw, &line) == nil && !line.IsSidechain {
			switch line.Type {
			case "user":
				parts, complete = nil, false
			case "assistant":
				var blocks []contentBlock
				_ = json.Unmarshal(line.Message.Content, &blocks)
				for _, block := range blocks {
					if text := strings.TrimSpace(block.Text); block.Type == "text" && text != "" {
						parts = append(parts, text)
					}
				}
				complete = line.Message.StopReason != "" && line.Message.StopReason != "tool_use"
			}
		}
		if err == io.EOF {
			return strings.Join(parts, "\n\n"), complete, nil
		}
		if err != nil {
			return "", false, err
		}
	}
}

// homeOf is the HOME a spawn asked for, since a session shadow moves it and the
// harness keeps its own files there. The last assignment wins, as in a process.
func homeOf(env []string) string {
	home := ""
	for _, entry := range env {
		if value, ok := strings.CutPrefix(entry, "HOME="); ok {
			home = value
		}
	}
	return home
}

// realHome is the user's home from the password database, not $HOME.
func realHome() string {
	if current, err := user.Current(); err == nil && current.HomeDir != "" {
		return current.HomeDir
	}
	home, _ := os.UserHomeDir()
	return home
}

// harnessDirVars are the variables that move where a harness keeps its files.
var harnessDirVars = []string{"CODEX_HOME", "CLAUDE_CONFIG_DIR", "XDG_DATA_HOME"}

// harnessDirsOf is the harness path variables a spawn set. A relative one is dropped,
// since nothing here knows what it was relative to. The last assignment wins.
func harnessDirsOf(env []string) map[string]string {
	dirs := map[string]string{}
	for _, entry := range env {
		for _, name := range harnessDirVars {
			if value, ok := strings.CutPrefix(entry, name+"="); ok && filepath.IsAbs(value) {
				dirs[name] = filepath.Clean(value)
			}
		}
	}
	if len(dirs) == 0 {
		return nil
	}
	return dirs
}

// dir is where a seat's harness keeps files: the variable it was spawned with, else
// a path under its recorded HOME, else under the real one.
func (e ledgerEntry) dir(variable string, rel ...string) string {
	if value := e.Dirs[variable]; value != "" {
		return value
	}
	home := e.Home
	if home == "" {
		home = realHome()
	}
	return filepath.Join(append([]string{home}, rel...)...)
}

// candidate is a session a store holds that could be a seat's, with when it began.
type candidate struct {
	ref     string
	started time.Time
}

// peersOf are the other seats on the same harness, cwd and store whose lifetime overlaps
// this one's, since their sessions are the ones this seat's could be mistaken for.
func peersOf(entry ledgerEntry, all []ledgerEntry, store func(ledgerEntry) string) []ledgerEntry {
	var peers []ledgerEntry
	for _, other := range all {
		if other.Name != entry.Name && other.Seat == entry.Seat && other.Cwd == entry.Cwd &&
			store(other) == store(entry) && overlaps(entry, other) {
			peers = append(peers, other)
		}
	}
	return peers
}

func overlaps(a, b ledgerEntry) bool {
	end := func(e ledgerEntry) time.Time {
		if e.Ended != nil {
			return *e.Ended
		}
		return time.Now().Add(24 * time.Hour)
	}
	return !a.Started.After(end(b)) && !b.Started.After(end(a))
}

// ownCandidate picks a seat's session. Alone the newest wins, which follows a new session
// in the seat. With a peer it must be the only one before the next peer began, or none.
func ownCandidate(entry ledgerEntry, peers []ledgerEntry, cands []candidate) (candidate, error) {
	if len(peers) == 0 {
		best := cands[0]
		for _, c := range cands {
			if c.started.After(best.started) {
				best = c
			}
		}
		return best, nil
	}
	var upper time.Time
	for _, peer := range peers {
		if peer.Started.After(entry.Started) && (upper.IsZero() || peer.Started.Before(upper)) {
			upper = peer.Started
		}
	}
	var own []candidate
	for _, c := range cands {
		if upper.IsZero() || c.started.Before(upper.Add(-5*time.Second)) {
			own = append(own, c)
		}
	}
	if len(own) != 1 {
		return candidate{}, withExit(exitOffRoster, fmt.Errorf(
			"%s shares %s with %d other seat(s), so which session is its own cannot be told: %w", entry.Name, entry.Cwd, len(peers), errNoTranscript))
	}
	return own[0], nil
}

// transcriptPath finds a claude seat's transcript under its recorded home. It lists
// the directory instead of globbing, so a bracket in the home cannot change a pattern.
func transcriptPath(entry ledgerEntry) (string, error) {
	if entry.Seat != "claude" || !conversationFlag.MatchString(entry.Conversation) {
		return "", withExit(exitUsage, fmt.Errorf("%s keeps no claude transcript aterm can read: %w", entry.Name, errNoTranscript))
	}
	projects := filepath.Join(entry.dir("CLAUDE_CONFIG_DIR", ".claude"), "projects")
	dirs, err := os.ReadDir(projects)
	if err != nil {
		return "", withExit(exitOffRoster, fmt.Errorf("no claude projects directory at %s: %w", projects, errNoTranscript))
	}
	for _, dir := range dirs {
		path := filepath.Join(projects, dir.Name(), entry.Conversation+".jsonl")
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			return path, nil
		}
	}
	return "", withExit(exitOffRoster, fmt.Errorf("no transcript for %s under %s: %w", entry.Name, projects, errNoTranscript))
}

// replyOf reads one recorded session's latest reply from its harness's own files.
// The Slack sidecar calls it in-process, since it runs as the same user.
func replyOf(entry ledgerEntry, all []ledgerEntry) (replyView, error) {
	switch entry.Seat {
	case "claude":
		return claudeReply(entry)
	case "codex":
		return codexReply(entry, all)
	case "opencode":
		return opencodeReply(entry, all)
	}
	return replyView{}, withExit(exitUsage, fmt.Errorf("%s runs %q, which keeps no transcript aterm reads: %w", entry.Name, entry.Seat, errNoTranscript))
}

func claudeReply(entry ledgerEntry) (replyView, error) {
	path, err := transcriptPath(entry)
	if err != nil {
		return replyView{}, err
	}
	file, err := os.Open(path)
	if err != nil {
		return replyView{}, err
	}
	defer file.Close()
	text, complete, err := lastReply(file)
	if err != nil {
		return replyView{}, err
	}
	return replyView{Session: entry.Name, Text: text, Complete: complete, Source: path}, nil
}

// screenReply is the fallback: the rows the session shows now, from the daemon.
func screenReply(entry ledgerEntry, cause error) (replyView, error) {
	status, err := sessionStatusOf(entry.Name, maxStatusLines)
	if err != nil {
		return replyView{}, withExit(exitCodeFor(err), fmt.Errorf("%w, and no live screen to read instead: %v", cause, err))
	}
	return replyView{Session: entry.Name, Text: strings.Join(status.Screen, "\n"), Complete: status.State == stateIdle, Source: "screen"}, nil
}

// findRecorded picks the record a name or role slug means, live or ended. A role
// slug takes its most recent record.
func findRecorded(entries []ledgerEntry, target string) (ledgerEntry, error) {
	var byRole *ledgerEntry
	for index, entry := range entries {
		if entry.Name == target {
			return entry, nil
		}
		if entry.Role == target {
			byRole = &entries[index]
		}
	}
	if byRole != nil {
		return *byRole, nil
	}
	return ledgerEntry{}, withExit(exitOffRoster, fmt.Errorf("no recorded session answers to %q. `aterm agents` lists the live ones", target))
}

func newReplyCommand() *cli.Command {
	return &cli.Command{
		Name:      "reply",
		Usage:     "print a seat's latest reply in full, from its harness's transcript",
		ArgsUsage: "<session or role>",
		Description: "Reads the transcript the seat's harness keeps (claude, codex, opencode), so a reply\n" +
			"longer than the screen comes back whole where `aterm status` shows only visible rows. With\n" +
			"no transcript found it prints the visible screen and says so. It never types into the\n" +
			"session. A turn still running prints the text so far and says so.",
		Flags: []cli.Flag{&cli.BoolFlag{Name: "json", Usage: "machine-readable, the reply object"}},
		Action: func(_ context.Context, cmd *cli.Command) error {
			target := strings.TrimSpace(cmd.Args().First())
			if target == "" {
				return withExit(exitUsage, errors.New("aterm reply needs one target. `aterm agents` lists them"))
			}
			recorded := readLedger(ledgerDir())
			entry, err := findRecorded(recorded, target)
			if err != nil {
				return err
			}
			reply, err := replyOf(entry, recorded)
			if errors.Is(err, errNoTranscript) {
				fmt.Fprintf(cmd.Root().ErrWriter, "%v, so this is the visible screen\n", err)
				reply, err = screenReply(entry, err)
			}
			if err != nil {
				return err
			}
			writer := cmd.Root().Writer
			if cmd.Bool("json") {
				encoded, _ := json.MarshalIndent(reply, "", "  ")
				_, err := fmt.Fprintf(writer, "%s\n", encoded)
				return err
			}
			if !reply.Complete {
				fmt.Fprintf(cmd.Root().ErrWriter, "%s is mid-turn, so this is the text so far\n", reply.Session)
			}
			_, err = fmt.Fprintln(writer, reply.Text)
			return err
		},
	}
}
