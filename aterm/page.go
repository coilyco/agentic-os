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

func newPageCommand() *cli.Command {
	return &cli.Command{
		Name:  "page",
		Usage: "publish a page to the Treehouse pages host, where only the owning account opens it",
		Description: "The daemon reads the page, one .html file or a directory holding index.html and its assets,\n" +
			"only from this seat's working directory or scratchpad, at most 8 MiB and 64 files, and sends\n" +
			"it with its own Treehouse identity. The seat never holds a token. The daemon needs --treehouse-pages.",
		Commands: []*cli.Command{
			{
				Name:  "publish",
				Usage: "publish a page, or replace one with --page",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "title", Required: true, Usage: "name shown in the page list"},
					&cli.StringFlag{Name: "path", Required: true, Usage: "an .html file, or a directory holding index.html"},
					&cli.StringFlag{Name: "page", Usage: "the id of a page to replace, kept across republishes"},
				},
				Action: func(_ context.Context, cmd *cli.Command) error {
					reply, err := publishPage(cmd.String("title"), cmd.String("path"), cmd.String("page"))
					if err != nil {
						return err
					}
					_, err = fmt.Fprintf(cmd.Root().Writer, "published %s, %d files, %d bytes\n", reply.Page.Page, reply.Page.Files, reply.Page.Bytes)
					return err
				},
			},
			{
				Name:  "list",
				Usage: "list the account's pages",
				Action: func(_ context.Context, cmd *cli.Command) error {
					reply, err := pageCall(frame{Type: "page_list"})
					if err != nil {
						return err
					}
					for _, p := range reply.Pages {
						if _, err := fmt.Fprintf(cmd.Root().Writer, "%s  %s  %s\n", p.Page, p.Updated, p.Title); err != nil {
							return err
						}
					}
					return nil
				},
			},
			{
				Name:      "remove",
				Usage:     "remove a page",
				ArgsUsage: "<page id>",
				Action: func(_ context.Context, cmd *cli.Command) error {
					if cmd.Args().Len() != 1 {
						return withExit(exitUsage, fmt.Errorf("aterm page remove needs one page id"))
					}
					if _, err := pageCall(frame{Type: "page_remove", PageID: cmd.Args().First()}); err != nil {
						return err
					}
					_, err := fmt.Fprintln(cmd.Root().Writer, "removed")
					return err
				},
			},
		},
	}
}

// publishPage is the one path the CLI and the MCP tool share. The daemon reads the files,
// so the request carries an absolute path and never the content.
func publishPage(title, path, page string) (frame, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return frame{}, withExit(exitUsage, err)
	}
	return pageCall(frame{Type: "page_publish", Title: title, Path: abs, PageID: page})
}

// pageCall sends one page request as the seat the token names.
func pageCall(request frame) (frame, error) {
	token := strings.TrimSpace(os.Getenv(sessionTokenEnv))
	if token == "" {
		return frame{}, withExit(exitUsage, fmt.Errorf("aterm page speaks for a session aterm launched, "+
			"and %s is not set here", sessionTokenEnv))
	}
	c, err := dialDaemon(false)
	if err != nil {
		return frame{}, withExit(exitMissing, err)
	}
	defer c.Close()
	if !slices.Contains(c.features, pagesFeature) {
		return frame{}, withReason(reasonNoPages, fmt.Errorf("the running aterm daemon publishes no pages. "+
			"It needs --treehouse-pages, and a daemon older than pages restarts on the upgraded binary after five idle minutes"))
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
