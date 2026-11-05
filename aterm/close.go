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
		ArgsUsage:     "<role|identity|seat|session|self>",
		Description: "Targets resolve as `aterm send`'s do, and one that matches several sessions\n" +
			"refuses. An idle claude gets its own /exit first, then SIGTERM, then SIGKILL after\n" +
			"a grace period. A session holding Kai's unsent draft or undelivered messages\n" +
			"stays open unless --force. A seat closes a name only after listing it, since names are reused.\n" +
			"Use `name@ref`, the ref from `aterm agents`, to name one session exactly.\n" +
			"`self` ends the caller's own session, found from its session token and never from a\n" +
			"name, with the same draft and undelivered-message guards. Outside a seat it refuses.\n" +
			"`self` is reserved, so `self@ref` is how a session actually named self is closed.",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "force", Usage: "close even a session holding a draft or undelivered messages"},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() != 1 {
				return withExit(exitUsage, fmt.Errorf("aterm close needs one target. `aterm agents` lists them"))
			}
			if cmd.Args().First() == closeSelfTarget {
				name, err := closeSelf(cmd.Bool("force"))
				if err != nil {
					return err
				}
				_, err = fmt.Fprintf(cmd.Root().Writer, "closing %s\n", name)
				return err
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
	return closeRequest(closeFeature, frame{Target: target, Force: force})
}

// closeSelfTarget is the CLI word for the caller's own session. The MCP tool never
// sends it, so close_session still refuses the caller's own session.
const closeSelfTarget = "self"

// closeSelf ends the caller's own session. The daemon resolves it from the token
// and the reply comes before the end, so the caller reads it while still alive.
func closeSelf(force bool) (string, error) {
	if strings.TrimSpace(os.Getenv(sessionTokenEnv)) == "" {
		return "", withExit(exitUsage, fmt.Errorf("aterm close self needs a seat: %s is not set, "+
			"so this process is outside every session", sessionTokenEnv))
	}
	name, _, err := closeRequest(closeSelfFeature, frame{Self: true, Force: force})
	return name, err
}

func closeRequest(feature string, message frame) (string, int, error) {
	c, err := dialDaemon(false)
	if err != nil {
		return "", 0, withExit(exitMissing, err)
	}
	defer c.Close()
	if !slices.Contains(c.features, feature) {
		return "", 0, fmt.Errorf("the running aterm daemon predates %s. "+
			"It restarts on the upgraded binary after five idle minutes", feature)
	}
	message.Type = "close"
	message.Token = strings.TrimSpace(os.Getenv(sessionTokenEnv))
	reply, err := c.request(message)
	if err != nil {
		if reply.Code != 0 {
			return "", 0, withExit(reply.Code, err)
		}
		return "", 0, err
	}
	return reply.Session, reply.Code, nil
}
