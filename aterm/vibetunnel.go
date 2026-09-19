package main

import (
	"fmt"
	"io"
	"strings"
)

const (
	vibeTunnelBin = "vt"
	// vt refuses a nested session, so a window drops this marker.
	// See docs/aterm-bundles.md.
	vibeTunnelSessionEnv = "VIBETUNNEL_SESSION_ID"
)

// -S skips vt's re-run through an interactive shell, and vt reads no `--`.
// See docs/aterm-bundles.md.
func vibeTunnelArgv(vt string, child []string) []string {
	return append([]string{vt, "-S"}, child...)
}

// wrapChild decides on the window's own PATH. A missing vt costs the browser
// view, never the session.
func wrapChild(
	argv []string,
	wanted bool,
	lookPath func(string) (string, error),
	notice io.Writer,
) []string {
	if !wanted {
		return argv
	}
	vt, err := lookPath(vibeTunnelBin)
	if err != nil {
		fmt.Fprintf(notice, "aterm: %s is not on PATH, so this session is not in VibeTunnel\n", vibeTunnelBin)
		return argv
	}
	return vibeTunnelArgv(vt, argv)
}

func withoutVibeTunnelSession(environ []string) []string {
	kept := make([]string, 0, len(environ))
	for _, entry := range environ {
		if name, _, _ := strings.Cut(entry, "="); name == vibeTunnelSessionEnv {
			continue
		}
		kept = append(kept, entry)
	}
	return kept
}
