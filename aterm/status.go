package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/urfave/cli/v3"
)

func newStatusCommand() *cli.Command {
	return &cli.Command{
		Name:      "status",
		Usage:     "read a live session's state and screen without typing into it",
		ArgsUsage: "<role|identity|seat|session>",
		Description: "Targets resolve as `aterm close`'s do. It reports whether the session sits on a\n" +
			"permission or choice prompt (with the text), how long since it wrote and since\n" +
			"anyone typed, whether Kai has a draft, and the last rows of the screen. It is\n" +
			"read-only: it never types and never focuses a window.",
		Flags: []cli.Flag{
			&cli.IntFlag{Name: "lines", Value: defaultStatusLines, Usage: "screen rows to show, at most 200"},
			&cli.BoolFlag{Name: "json", Usage: "machine-readable, the daemon's status object"},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() != 1 {
				return withExit(exitUsage, fmt.Errorf("aterm status needs one target. `aterm agents` lists them"))
			}
			status, err := sessionStatusOf(cmd.Args().First(), cmd.Int("lines"))
			if err != nil {
				return err
			}
			writer := cmd.Root().Writer
			if cmd.Bool("json") {
				encoded, _ := json.MarshalIndent(status, "", "  ")
				_, err := fmt.Fprintf(writer, "%s\n", encoded)
				return err
			}
			return writeStatus(writer, status)
		},
	}
}

// sessionStatusOf is the one path the CLI and the MCP tool share.
func sessionStatusOf(target string, lines int) (sessionStatus, error) {
	c, err := dialDaemon(false)
	if err != nil {
		return sessionStatus{}, withExit(exitMissing, err)
	}
	defer c.Close()
	if !slices.Contains(c.features, statusFeature) {
		return sessionStatus{}, fmt.Errorf("the running aterm daemon predates status. " +
			"It restarts on the upgraded binary after five idle minutes")
	}
	reply, err := c.request(frame{Type: "status", Target: target, Lines: lines})
	if err != nil {
		if reply.Code != 0 {
			return sessionStatus{}, withExit(reply.Code, err)
		}
		return sessionStatus{}, err
	}
	if reply.Status == nil {
		return sessionStatus{}, fmt.Errorf("the daemon answered a status with no status")
	}
	return *reply.Status, nil
}

func writeStatus(w io.Writer, status sessionStatus) error {
	held := []string{}
	if status.Drafted {
		held = append(held, "Kai drafting")
	}
	if status.Pending > 0 {
		held = append(held, fmt.Sprintf("%d pending", status.Pending))
	}
	input := "no input yet"
	if status.InputSeconds >= 0 {
		input = "last input " + (time.Duration(status.InputSeconds) * time.Second).String() + " ago"
	}
	head := fmt.Sprintf("%s  %s  quiet %s, %s", status.Name, status.State,
		(time.Duration(status.QuietSeconds) * time.Second).String(), input)
	if len(held) > 0 {
		head += ", " + strings.Join(held, ", ")
	}
	if _, err := fmt.Fprintln(w, head); err != nil {
		return err
	}
	if len(status.Prompt) > 0 {
		fmt.Fprintln(w, "-- prompt --")
		fmt.Fprintln(w, strings.Join(status.Prompt, "\n"))
	}
	_, err := fmt.Fprintf(w, "-- screen, last %d of %dx%d --\n%s\n", len(status.Screen), status.Rows, status.Cols,
		strings.Join(status.Screen, "\n"))
	return err
}
