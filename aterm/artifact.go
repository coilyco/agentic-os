package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/urfave/cli/v3"
)

func newArtifactCommand() *cli.Command {
	return &cli.Command{
		Name:  "artifact",
		Usage: "show a claude.ai artifact's source file in aterm's client, labeled with this seat",
		Description: "The artifact itself is private to claude.ai, which refuses to be framed, so the client\n" +
			"shows the seat's own source file and links out to the page. The daemon reads the file, only\n" +
			"from this seat's working directory or scratchpad, at most 2 MiB. The address must be exactly\n" +
			"https://claude.ai/artifact/<id>. A second add of the same address replaces the page.",
		Commands: []*cli.Command{
			{
				Name:  "add",
				Usage: "register an artifact and push its source to clients",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "url", Required: true, Usage: "https://claude.ai/artifact/<id>"},
					&cli.StringFlag{Name: "title", Required: true, Usage: "name shown above the page"},
					&cli.StringFlag{Name: "source", Required: true, Usage: "the artifact's HTML source file"},
					&cli.BoolFlag{Name: "waiting", Usage: "mark it as waiting on Kai"},
					&cli.BoolFlag{Name: "standing", Usage: "keep it after this seat ends, and across a daemon restart"},
				},
				Action: func(_ context.Context, cmd *cli.Command) error {
					reply, err := addArtifact(cmd.String("url"), cmd.String("title"), cmd.String("source"), cmd.Bool("waiting"), cmd.Bool("standing"))
					if err != nil {
						return err
					}
					_, err = fmt.Fprintf(cmd.Root().Writer, "added %s, shown to %d clients\n", reply.ArtifactID, reply.Delivered)
					return err
				},
			},
			{
				Name:      "waiting",
				Usage:     "mark an artifact as waiting on Kai, or not with --off",
				ArgsUsage: "<url|id>",
				Flags:     []cli.Flag{&cli.BoolFlag{Name: "off", Usage: "no longer waiting"}},
				Action: func(_ context.Context, cmd *cli.Command) error {
					if cmd.Args().Len() != 1 {
						return withExit(exitUsage, fmt.Errorf("aterm artifact waiting needs one address or id"))
					}
					waiting := !cmd.Bool("off")
					if _, err := artifactCall(frame{Type: "artifact_waiting", ArtifactID: cmd.Args().First(), Waiting: &waiting}); err != nil {
						return err
					}
					_, err := fmt.Fprintf(cmd.Root().Writer, "waiting %t\n", waiting)
					return err
				},
			},
			{
				Name:      "remove",
				Usage:     "close an artifact in the client",
				ArgsUsage: "<url|id>",
				Action: func(_ context.Context, cmd *cli.Command) error {
					if cmd.Args().Len() != 1 {
						return withExit(exitUsage, fmt.Errorf("aterm artifact remove needs one address or id"))
					}
					if _, err := artifactCall(frame{Type: "artifact_remove", ArtifactID: cmd.Args().First()}); err != nil {
						return err
					}
					_, err := fmt.Fprintln(cmd.Root().Writer, "removed")
					return err
				},
			},
		},
	}
}

// addArtifact is the one path the CLI and the MCP tool share. The daemon reads the
// file, so the request carries an absolute path and never the source.
func addArtifact(address, title, source string, waiting, standing bool) (frame, error) {
	path, err := filepath.Abs(source)
	if err != nil {
		return frame{}, withExit(exitUsage, err)
	}
	return artifactCall(frame{Type: "artifact_add", URL: address, Title: title, Path: path, Waiting: &waiting, Standing: standing})
}

// artifactCall sends one artifact request as the seat the token names.
func artifactCall(request frame) (frame, error) {
	token := strings.TrimSpace(os.Getenv(sessionTokenEnv))
	if token == "" {
		return frame{}, withExit(exitUsage, fmt.Errorf("aterm artifact speaks for a session aterm launched, "+
			"and %s is not set here", sessionTokenEnv))
	}
	c, err := dialDaemon(false)
	if err != nil {
		return frame{}, withExit(exitMissing, err)
	}
	defer c.Close()
	if !slices.Contains(c.features, artifactsFeature) {
		return frame{}, fmt.Errorf("the running aterm daemon predates artifacts. " +
			"It restarts on the upgraded binary after five idle minutes")
	}
	request.Token = token
	reply, err := c.request(request)
	if err != nil {
		if reply.Code != 0 {
			return frame{}, withExit(reply.Code, withReason(reply.Reason, err))
		}
		return frame{}, withReason(reply.Reason, err)
	}
	return reply, nil
}
