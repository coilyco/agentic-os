package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/urfave/cli/v3"
)

// The launch card loses the window to the harness within a repaint and then to
// the terminal's scrollback cap. See docs/aterm.md. agentic-os#1456
func newCardCommand() *cli.Command {
	return &cli.Command{
		Name:  "card",
		Usage: "re-render this session's identity card, which the harness paints over at launch",
		Action: func(_ context.Context, cmd *cli.Command) error {
			return runCard(cmd.Root().Writer)
		},
	}
}

func runCard(writer io.Writer) error {
	payload := strings.TrimSpace(os.Getenv(cardEnv))
	if payload == "" {
		return withExit(exitMissing, fmt.Errorf(
			"%s is unset, so this shell is not inside a session aterm opened", cardEnv))
	}
	card, err := decodeSessionCard(payload)
	if err != nil {
		return err
	}
	_, err = fmt.Fprint(writer, renderCard(card, 1))
	return err
}
