package main

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/urfave/cli/v3"
)

func newCloseCommand() *cli.Command {
	return &cli.Command{
		Name:          "close",
		ShellComplete: completeSessionName,
		Usage:         "end a live session and drop it, which closing its window does not",
		ArgsUsage:     "<role|identity|seat|session>",
		Description: "Targets resolve as `aterm send`'s do, and one that matches several sessions\n" +
			"refuses. The harness gets SIGTERM, then SIGKILL after a grace period. A session\n" +
			"holding Kai's unsent draft or undelivered messages stays open unless --force.",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "force", Usage: "close even a session holding a draft or undelivered messages"},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() != 1 {
				return withExit(exitUsage, fmt.Errorf("aterm close needs one target. `aterm agents` lists them"))
			}
			name, code, err := closeSession(cmd.Args().First(), cmd.Bool("force"))
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.Root().Writer, "closed %s (exit %d)\n", name, code)
			return err
		},
	}
}

// closeSession is the one path the CLI and the MCP tool share. The token, when
// the caller runs in a session, lets the daemon refuse closing that session.
func closeSession(target string, force bool) (string, int, error) {
	c, err := dialDaemon(false)
	if err != nil {
		return "", 0, withExit(exitMissing, err)
	}
	defer c.Close()
	if !slices.Contains(c.features, closeFeature) {
		return "", 0, fmt.Errorf("the running aterm daemon predates close. " +
			"It restarts on the upgraded binary after five idle minutes")
	}
	token := strings.TrimSpace(os.Getenv(sessionTokenEnv))
	reply, err := c.request(frame{Type: "close", Token: token, Target: target, Force: force})
	if err != nil {
		if reply.Code != 0 {
			return "", 0, withExit(reply.Code, err)
		}
		return "", 0, err
	}
	return reply.Session, reply.Code, nil
}
