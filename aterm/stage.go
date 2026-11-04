package main

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"

	"github.com/urfave/cli/v3"
)

func newStageCommand() *cli.Command {
	return &cli.Command{
		Name:      "stage",
		Usage:     "type a command into the plain terminal beside this session without running it, for Kai to run with Enter",
		ArgsUsage: "-- <command>",
		Description: "The daemon finds the terminal paired with the calling session from its token, and opens\n" +
			"one when none exists. The command lands on the shell's input line and is never submitted.\n" +
			"It refuses a newline, a carriage return and every other control byte, since that shell\n" +
			"is Kai's own, outside this session's sandbox. One argument is the whole command line.\n" +
			"Several are quoted into one. --note rides to the client beside the terminal and is\n" +
			"never typed. Exits 0 when staged, 2 when refused, 8 when no client is attached.",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "note", Usage: "one line shown beside the terminal, never typed into it"},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() == 0 {
				return withExit(exitUsage, fmt.Errorf("aterm stage needs a command: aterm stage --note \"why\" -- <command>"))
			}
			result, err := stageCommand(joinStaged(cmd.Args().Slice()), cmd.String("note"))
			if err != nil {
				return err
			}
			if err := stageExit(result); err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.Root().Writer, describeStage(result))
			return err
		},
	}
}

// shellSafe is a word a POSIX shell reads back unchanged.
var shellSafe = regexp.MustCompile(`^[A-Za-z0-9_@%+=:,./-]+$`)

// joinStaged is the command line a shell would read from these words: a lone
// argument is already a line, and several are single-quoted where they need it.
func joinStaged(words []string) string {
	if len(words) == 1 {
		return words[0]
	}
	quoted := make([]string, len(words))
	for i, word := range words {
		if shellSafe.MatchString(word) {
			quoted[i] = word
			continue
		}
		quoted[i] = "'" + strings.ReplaceAll(word, "'", `'\''`) + "'"
	}
	return strings.Join(quoted, " ")
}

type stageResult struct {
	State   string
	Session string
	Reason  string
	Opened  bool
	Viewers int
}

// stageCommand is the one path the CLI and the MCP tool share. A refusal comes back
// as a result, so each front door words it for its own reader.
func stageCommand(command, note string) (stageResult, error) {
	token := strings.TrimSpace(os.Getenv(sessionTokenEnv))
	if token == "" {
		return stageResult{}, withExit(exitUsage, fmt.Errorf("aterm stage speaks for a session aterm launched, "+
			"and %s is not set here", sessionTokenEnv))
	}
	c, err := dialDaemon(false)
	if err != nil {
		return stageResult{}, withExit(exitMissing, err)
	}
	defer c.Close()
	if !slices.Contains(c.features, stageFeature) {
		return stageResult{}, fmt.Errorf("the running aterm daemon predates stage. " +
			"It restarts on the upgraded binary after five idle minutes")
	}
	reply, err := c.request(frame{Type: "stage", Token: token, Body: command, Text: note})
	if err != nil {
		if reply.Code != 0 {
			return stageResult{}, withExit(reply.Code, err)
		}
		return stageResult{}, err
	}
	return stageResult{State: reply.State, Session: reply.Session, Reason: reply.Text, Opened: reply.Opened, Viewers: reply.Viewers}, nil
}

// describeStage words a result for a reader, and is where "nothing ran" is said.
func describeStage(r stageResult) string {
	switch r.State {
	case stageStaged:
		line := "staged in " + r.Session + ". Nothing ran: Kai presses Enter to run it"
		if r.Opened {
			line = "opened " + r.Session + " and " + line
		}
		if r.Reason != "" {
			line += ". " + r.Reason
		}
		return line
	case stageNoClient:
		return "no client attached: " + r.Reason
	default:
		return "refused: " + r.Reason
	}
}

// stageExit is the exit code a state earns, so a script can branch on it.
func stageExit(r stageResult) error {
	switch r.State {
	case stageStaged:
		return nil
	case stageNoClient:
		return withExit(exitNoClient, fmt.Errorf("%s", describeStage(r)))
	default:
		return withExit(exitUsage, fmt.Errorf("%s", describeStage(r)))
	}
}
