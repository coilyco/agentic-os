package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/urfave/cli/v3"
)

// resumeRun runs the launch a resume resolves to. A variable so a test reads the
// arguments without opening a window.
var resumeRun = func(self string, args []string, stdout, stderr io.Writer) error {
	command := exec.Command(self, args...)
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, stdout, stderr
	return command.Run()
}

// liveSessionNames is who the daemon holds now, or nobody while none answers.
func liveSessionNames() map[string]bool {
	live := map[string]bool{}
	c, err := dialDaemon(false)
	if err != nil {
		return live
	}
	defer c.Close()
	if reply, err := c.request(frame{Type: "list"}); err == nil {
		for _, view := range reply.Sessions {
			live[view.Name] = true
		}
	}
	return live
}

// findResumable picks the record a name or role slug means, among sessions that
// are not live. A role slug takes its most recent record.
func findResumable(entries []ledgerEntry, live map[string]bool, target string) (ledgerEntry, error) {
	var byRole []ledgerEntry
	for _, entry := range entries {
		if entry.Name == target {
			if live[entry.Name] {
				return ledgerEntry{}, withExit(exitUsage, fmt.Errorf("%s is live, so `aterm attach %s` reaches it", target, target))
			}
			return entry, nil
		}
		if entry.Role == target && !live[entry.Name] {
			byRole = append(byRole, entry)
		}
	}
	if len(byRole) > 0 {
		return byRole[len(byRole)-1], nil
	}
	return ledgerEntry{}, withExit(exitOffRoster, fmt.Errorf("no recorded session answers to %q. `aterm resume --list` shows them", target))
}

// resumeArgs is the launch that reopens an entry's conversation, in its old directory
// when that still exists. No --name, so the claim names claude. See docs/aterm.md.
func resumeArgs(entry ledgerEntry, headless, dryRun bool) ([]string, error) {
	if entry.Seat != "claude" || entry.Conversation == "" {
		return nil, withExit(exitUsage, fmt.Errorf(
			"%s has no conversation to resume: only a claude session minted with an id is recorded, and this one is %q",
			entry.Name, entry.Seat))
	}
	var args []string
	if headless {
		args = append(args, "--headless")
	}
	if dryRun {
		args = append(args, "--dry-run")
	}
	if info, err := os.Stat(entry.Cwd); err == nil && info.IsDir() {
		args = append(args, "--working-directory", entry.Cwd)
	}
	return append(args, entry.Role, entry.Seat, "--", "--resume", entry.Conversation), nil
}

func newResumeCommand() *cli.Command {
	return &cli.Command{
		Name:      "resume",
		Usage:     "reopen the conversation of a session that is no longer live, after a reboot or a lost holder",
		ArgsUsage: "[session or role]",
		Description: "The daemon records each session in " + stateDirEnv + " or ~/.local/state/aterm/sessions,\n" +
			"never its environment. Resume relaunches the role with the harness's own resume of the\n" +
			"recorded conversation, so the new session carries the old one's context.",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "list", Usage: "list the sessions that can be resumed"},
			&cli.BoolFlag{Name: "headless", Usage: "resume in the daemon with no window"},
			&cli.BoolFlag{Name: "dry-run", Usage: "print the launch plan without opening anything"},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			entries := readLedger(ledgerDir())
			live := liveSessionNames()
			if cmd.Bool("list") {
				return listResumable(cmd.Root().Writer, entries, live)
			}
			target := strings.TrimSpace(cmd.Args().First())
			if target == "" {
				return withExit(exitUsage, fmt.Errorf("name a session or role. `aterm resume --list` shows what can be resumed"))
			}
			entry, err := findResumable(entries, live, target)
			if err != nil {
				return err
			}
			args, err := resumeArgs(entry, cmd.Bool("headless"), cmd.Bool("dry-run"))
			if err != nil {
				return err
			}
			self, err := os.Executable()
			if err != nil {
				return err
			}
			return resumeRun(self, args, cmd.Root().Writer, cmd.Root().ErrWriter)
		},
	}
}

func listResumable(out io.Writer, entries []ledgerEntry, live map[string]bool) error {
	shown := 0
	for _, entry := range entries {
		if live[entry.Name] {
			continue
		}
		state, note := "running when last seen", ""
		if entry.Ended != nil {
			state = "ended " + entry.Ended.Local().Format(time.DateTime)
		}
		if entry.Seat != "claude" || entry.Conversation == "" {
			note = "  (not resumable: no conversation id)"
		}
		fmt.Fprintf(out, "%s  %s  %s%s\n", entry.Name, entry.Role, state, note)
		shown++
	}
	if shown == 0 {
		_, err := fmt.Fprintln(out, "nothing to resume")
		return err
	}
	return nil
}
