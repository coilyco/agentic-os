package main

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/urfave/cli/v3"
)

func newClearCommand() *cli.Command {
	return &cli.Command{
		Name:          "clear",
		ShellComplete: completeSessionName,
		Usage:         "start a live session over by typing its harness's clear command, for Kai and the director",
		ArgsUsage:     "<role|identity|seat|session>",
		Description: "Targets resolve as `aterm close`'s do. The command is typed unstamped, so it lands as\n" +
			"input rather than text. It refuses an idle session's opposite: one on a prompt always,\n" +
			"and one busy, holding Kai's draft, or with undelivered messages unless --force. It never\n" +
			"clears the caller's own session, and only the director role or Kai's client may ask.",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "force", Usage: "clear even a busy session, or one holding a draft or undelivered messages"},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() != 1 {
				return withExit(exitUsage, fmt.Errorf("aterm clear needs one target. `aterm agents` lists them"))
			}
			name, command, err := clearTarget(cmd.Args().First(), cmd.Bool("force"))
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.Root().Writer, "typed %s into %s\n", command, name)
			return err
		},
	}
}

// clearTarget is the one path the CLI and the MCP tool share. The token, when the
// caller runs in a session, is how the daemon learns whether its role may clear.
func clearTarget(target string, force bool) (string, string, error) {
	c, err := dialDaemon(false)
	if err != nil {
		return "", "", withExit(exitMissing, err)
	}
	defer c.Close()
	if !slices.Contains(c.features, clearFeature) {
		return "", "", fmt.Errorf("the running aterm daemon predates clear. " +
			"It restarts on the upgraded binary after five idle minutes")
	}
	token := strings.TrimSpace(os.Getenv(sessionTokenEnv))
	reply, err := c.request(frame{Type: "clear", Token: token, Target: target, Force: force})
	if err != nil {
		if reply.Code != 0 {
			return "", "", withExit(reply.Code, err)
		}
		return "", "", err
	}
	return reply.Session, reply.Text, nil
}
