package main

import (
	"fmt"
	"io"
	"strings"
)

const (
	vibeTunnelBin = "vt"
	// The one name every aterm session carries, and the one launch clears.
	// See docs/aterm-bundles.md.
	vibeTunnelSessionName = "aterm"
	// vt refuses a nested session, so a window drops this marker.
	// See docs/aterm-bundles.md.
	vibeTunnelSessionEnv = "VIBETUNNEL_SESSION_ID"
	// Names the session from inside it, where vt knows its id, then becomes the
	// harness. A failed rename never costs the session.
	renameThenRun = `"$2" title "$1" >/dev/null 2>&1; shift 2; exec "$@"`
)

// -S skips vt's re-run through an interactive shell, and vt reads no `--`.
// See docs/aterm-bundles.md.
func vibeTunnelArgv(vt string, child []string) []string {
	trampoline := []string{vt, "-S", "/bin/sh", "-c", renameThenRun, "sh", vibeTunnelSessionName, vt}
	return append(trampoline, child...)
}

// wrapChild decides on the window's own PATH. A missing vt costs the browser
// view, never the session.
func wrapChild(
	argv []string,
	wanted bool,
	lookPath func(string) (string, error),
	notice io.Writer,
) ([]string, bool) {
	if !wanted {
		return argv, false
	}
	vt, err := lookPath(vibeTunnelBin)
	if err != nil {
		fmt.Fprintf(notice, "aterm: %s is not on PATH, so this session is not in VibeTunnel\n", vibeTunnelBin)
		return argv, false
	}
	return vibeTunnelArgv(vt, argv), true
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
