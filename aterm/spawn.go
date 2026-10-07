package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

// earlyExitWindow bounds the wait for a startup failure. kitty stays alive for
// the session, so only a failing early exit is news.
const earlyExitWindow = 400 * time.Millisecond

// spawnWindow starts the terminal detached without walking away blind, so a
// terminal that refuses its own arguments reports why. See docs/aterm.md.
func spawnWindow(name string, args []string) error {
	return spawnWindowWithin(earlyExitWindow, name, args)
}

// spawnWindowWithin takes the window so a test of the failing path can allow a
// loaded machine far longer than 400ms, since the wait ends when the terminal does.
func spawnWindowWithin(window time.Duration, name string, args []string) error {
	log, err := os.CreateTemp("", "aterm-window-*.log")
	if err != nil {
		return fmt.Errorf("stage the terminal log: %w", err)
	}
	defer func() {
		_ = log.Close()
		_ = os.Remove(log.Name())
	}()
	command := exec.Command(name, args...)
	command.Stderr = log
	command.SysProcAttr = detachAttr()
	// A new window is a new session, so it opens on the canonical environment
	// rather than on this one's shadow. agentic-os#1460
	environ := canonicalEnviron(os.Environ(), readCanonicalLaunch())
	if environ == nil {
		environ = os.Environ()
	}
	command.Env = windowEnviron(environ)
	if err := command.Start(); err != nil {
		return err
	}
	// Wait keeps running past the timeout on purpose. The launcher exits right
	// after, and the detached session keeps the window alive without it.
	finished := make(chan error, 1)
	go func() { finished <- command.Wait() }()
	select {
	case err := <-finished:
		if err == nil {
			return nil
		}
		return fmt.Errorf("%s exited immediately: %w%s", name, err, terminalDetail(log))
	case <-time.After(window):
		return nil
	}
}

func terminalDetail(log *os.File) string {
	if _, err := log.Seek(0, io.SeekStart); err != nil {
		return ""
	}
	raw, err := io.ReadAll(log)
	if err != nil {
		return ""
	}
	detail := strings.TrimSpace(string(raw))
	if detail == "" {
		return ""
	}
	return ": " + detail
}

// runHeadlessStage runs the session stage on the environment a window would get,
// and waits, since the stage exits once the daemon holds the harness.
func runHeadlessStage(name string, args []string) error {
	command := exec.Command(name, args...)
	environ := canonicalEnviron(os.Environ(), readCanonicalLaunch())
	if environ == nil {
		environ = os.Environ()
	}
	command.Env = windowEnviron(environ)
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

// windowEnviron drops the terminal a bundle's launcher exported for its own
// window, so a launch from inside that session opens the target role's app.
func windowEnviron(environ []string) []string {
	kept := make([]string, 0, len(environ))
	for _, entry := range environ {
		if name, _, _ := strings.Cut(entry, "="); name == terminalBinEnv {
			continue
		}
		kept = append(kept, entry)
	}
	return kept
}
