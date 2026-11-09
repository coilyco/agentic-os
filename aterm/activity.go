package main

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/urfave/cli/v3"
)

func newActivityCommand() *cli.Command {
	return &cli.Command{
		Name:      "activity",
		Usage:     "set the one line watchers see for what this seat is doing",
		ArgsUsage: "<text>",
		Description: "Run it from inside a seat, at each change of task. The daemon keeps one line per\n" +
			"session, cut to 57 characters with control characters dropped and line breaks read\n" +
			"as a space, and pushes it to watchers with the time it was set. It ends with the\n" +
			"session. `aterm activity --clear` empties it. Only the session's own token sets it,\n" +
			"so it names no target and nothing on the screen can write it.",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "clear", Usage: "remove the line instead of setting one"},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			text := strings.Join(cmd.Args().Slice(), " ")
			if cmd.Bool("clear") == (cmd.Args().Len() > 0) {
				return withExit(exitUsage, fmt.Errorf("aterm activity needs text to set, or --clear and no text"))
			}
			stored, err := setActivityLine(text)
			if err != nil {
				return err
			}
			if stored == "" {
				_, err = fmt.Fprintln(cmd.Root().Writer, "activity cleared")
				return err
			}
			_, err = fmt.Fprintf(cmd.Root().Writer, "activity set: %s\n", stored)
			return err
		},
	}
}

// setActivityLine sends the line under the caller's own session token and returns what
// the daemon stored, which is the text after cutting.
func setActivityLine(text string) (string, error) {
	token := strings.TrimSpace(os.Getenv(sessionTokenEnv))
	if token == "" {
		return "", withExit(exitUsage, fmt.Errorf("aterm activity runs inside a seat, and %s is not set here", sessionTokenEnv))
	}
	c, err := dialDaemon(false)
	if err != nil {
		return "", withExit(exitMissing, err)
	}
	defer c.Close()
	if !slices.Contains(c.features, activityFeature) {
		return "", fmt.Errorf("the running aterm daemon predates activity. " +
			"It restarts on the upgraded binary after five idle minutes")
	}
	reply, err := c.request(frame{Type: "activity", Token: token, Text: text})
	if err != nil {
		if reply.Code != 0 {
			return "", withExit(reply.Code, err)
		}
		return "", err
	}
	return reply.Text, nil
}
