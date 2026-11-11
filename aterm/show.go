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

func newShowCommand() *cli.Command {
	return &cli.Command{
		Name:      "show",
		Usage:     "show an image, a video or a file in aterm's client, labeled with this seat",
		ArgsUsage: "<path>",
		Description: "The daemon copies the file, so a file you overwrite later is a new item, and reads its type from\n" +
			"the bytes: image (PNG, JPEG, GIF, WebP), video (MP4, WebM), or file for anything else. The path must\n" +
			"be under this seat's working directory or scratchpad, at most 64 MiB. A seat keeps its last 32\n" +
			"items within 256 MiB, and the oldest go first. Run it again for each new frame.",
		Flags: []cli.Flag{&cli.StringFlag{Name: "title", Usage: "name shown with it, the file's own name when unset"}},
		Action: func(_ context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() != 1 {
				return withExit(exitUsage, fmt.Errorf("aterm show needs one path"))
			}
			reply, err := showMedia(cmd.Args().First(), cmd.String("title"))
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.Root().Writer, describeShown(reply))
			return err
		},
	}
}

// showMedia is the one path the CLI and the MCP tool share. The daemon reads the file,
// so the request carries an absolute path and never the bytes.
func showMedia(path, title string) (frame, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return frame{}, withExit(exitUsage, err)
	}
	token := strings.TrimSpace(os.Getenv(sessionTokenEnv))
	if token == "" {
		return frame{}, withExit(exitUsage, fmt.Errorf("aterm show speaks for a session aterm launched, "+
			"and %s is not set here", sessionTokenEnv))
	}
	c, err := dialDaemon(false)
	if err != nil {
		return frame{}, withExit(exitMissing, err)
	}
	defer c.Close()
	if !slices.Contains(c.features, mediaFeature) {
		return frame{}, fmt.Errorf("the running aterm daemon predates media. " +
			"It restarts on the upgraded binary after five idle minutes")
	}
	reply, err := c.request(frame{Type: "media_show", Token: token, Path: absolute, Title: title})
	if err != nil {
		if reply.Code != 0 {
			return frame{}, withExit(reply.Code, withReason(reply.Reason, err))
		}
		return frame{}, withReason(reply.Reason, err)
	}
	return reply, nil
}

// describeShown is what the verb and the tool say back: the kind, size and who took it.
func describeShown(reply frame) string {
	if reply.Media == nil {
		return "shown"
	}
	taken := fmt.Sprintf("shown to %d clients", reply.Delivered)
	if reply.Delivered == 0 {
		taken = "kept, no client is showing media now"
	}
	return fmt.Sprintf("%s %q, %d bytes, %s", reply.Media.Kind, reply.Media.Name, reply.Media.Size, taken)
}
