package main

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/urfave/cli/v3"
)

func newReloadCommand() *cli.Command {
	return &cli.Command{
		Name:          "reload",
		ShellComplete: completeSessionName,
		Usage:         "restart an idle seat on its own conversation, so it reads its MCP servers and connectors again",
		ArgsUsage:     "<role|identity|seat|session>",
		Description: "Targets resolve as `aterm close`'s do. The seat is closed and resumed headless under the\n" +
			"identity it had, in its directory, and the verb waits for it to read idle. It refuses a seat\n" +
			"that is not idle, holds a draft or undelivered messages, or has no conversation to resume,\n" +
			"unless --force, and never reloads the caller. A connector added on claude.ai after the seat\n" +
			"started is read at that restart. See docs/aterm-daemon.md.",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "force", Usage: "reload a seat that is not idle or holds a draft or undelivered messages"},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() != 1 {
				return withExit(exitUsage, fmt.Errorf("aterm reload needs one target. `aterm agents` lists them"))
			}
			c, err := dialDaemon(false)
			if err != nil {
				return withExit(exitMissing, err)
			}
			defer c.Close()
			if !slices.Contains(c.features, reloadFeature) {
				return errNoReload
			}
			token := strings.TrimSpace(os.Getenv(sessionTokenEnv))
			reply, err := c.request(frame{Type: "reload", Token: token, Target: cmd.Args().First(), Force: cmd.Bool("force")})
			if err != nil {
				if reply.Code != 0 {
					return withExit(reply.Code, err)
				}
				return err
			}
			_, err = fmt.Fprintln(cmd.Root().Writer, reply.Text)
			return err
		},
	}
}
