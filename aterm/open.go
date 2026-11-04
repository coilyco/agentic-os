package main

import (
	"context"
	"errors"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/urfave/cli/v3"
)

// nativeOpenEnv marks a process aterm started to run the native opener, so a
// shim that process reaches goes straight to the native one and never loops.
const nativeOpenEnv = "ATERM_OPEN_NATIVE"

// openDeps are the two things `aterm open` does, so a test needs neither a
// daemon nor a browser.
type openDeps struct {
	// ask offers an address to the daemon and says how many clients took it.
	ask func(address string) (int, error)
	// native runs the host's own opener with the arguments untouched.
	native func(args []string) error
}

func newOpenCommand() *cli.Command {
	return &cli.Command{
		Name:  "open",
		Usage: "open a page in aterm's browser when a client is attached, else in the native one",
		// The seat's `open` and `xdg-open` shims pass whatever they were given, flags included.
		SkipFlagParsing: true,
		ArgsUsage:       "<url>",
		Description: "One http or https address goes to the clients showing aterm's browser, labeled with\n" +
			"the seat that asked. With no such client, or outside a seat, it opens the host's own\n" +
			"browser. Any other arguments run the host's opener unchanged.",
		Action: func(_ context.Context, cmd *cli.Command) error {
			return runOpen(cmd.Args().Slice(), openDeps{ask: askDaemonToOpen, native: nativeOpen})
		},
	}
}

// isWebAddress is the one shape the daemon takes, so `open -a Safari`, a file and a
// folder never reach it.
func isWebAddress(arg string) bool {
	parsed, err := url.Parse(arg)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}

func runOpen(args []string, deps openDeps) error {
	if os.Getenv(nativeOpenEnv) != "" || len(args) != 1 || !isWebAddress(args[0]) {
		return deps.native(args)
	}
	taken, err := deps.ask(args[0])
	if err != nil {
		// A seat that asks too often is refused. Opening the native browser instead
		// would be the flood the limit exists to stop.
		var refused reasonError
		if errors.As(err, &refused) && refused.reason == reasonTooMany {
			return withExit(exitFailure, err)
		}
		return deps.native(args)
	}
	if taken == 0 {
		return deps.native(args)
	}
	return nil
}

// askDaemonToOpen sends open_url as the seat the token names. Any failure to ask
// returns an error and the caller opens the native browser, so nothing is lost.
func askDaemonToOpen(address string) (int, error) {
	token := strings.TrimSpace(os.Getenv(sessionTokenEnv))
	if token == "" {
		return 0, errors.New("not in an aterm seat")
	}
	c, err := dialDaemon(false)
	if err != nil {
		return 0, err
	}
	defer c.Close()
	if !slices.Contains(c.features, openURLFeature) {
		return 0, errors.New("the running aterm daemon predates open_url")
	}
	reply, err := c.request(frame{Type: "open_url", Token: token, URL: address})
	if err != nil {
		return 0, withReason(reply.Reason, err)
	}
	return reply.Delivered, nil
}

// nativeOpen runs the host's own opener. On macOS that is /usr/bin/open by absolute
// path. Elsewhere it is the first xdg-open on PATH outside the shim directory.
func nativeOpen(args []string) error {
	opener, err := nativeOpener(os.Getenv("PATH"))
	if err != nil {
		return withExit(exitMissing, err)
	}
	command := exec.Command(opener, args...)
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	command.Env = withoutShims(os.Environ(), shimDirectory())
	if err := command.Run(); err != nil {
		var exited *exec.ExitError
		if errors.As(err, &exited) {
			return withExit(exited.ExitCode(), err)
		}
		return err
	}
	return nil
}

// shimDirectory is where the daemon put the shims, found the way the daemon's
// socket directory is, so the verb needs nothing from the environment.
func shimDirectory() string {
	return filepath.Join(filepath.Dir(daemonSocket()), shimDirName)
}

func nativeOpener(path string) (string, error) {
	if runtime.GOOS == "darwin" {
		return "/usr/bin/open", nil
	}
	shims := shimDirectory()
	for _, dir := range filepath.SplitList(path) {
		if filepath.Clean(dir) == filepath.Clean(shims) || dir == "" {
			continue
		}
		candidate := filepath.Join(dir, "xdg-open")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() && info.Mode().Perm()&0o111 != 0 {
			return candidate, nil
		}
	}
	return "", errors.New("no xdg-open outside the aterm shims, so there is no native browser to fall back to")
}

// withoutShims is the environment for the native opener: PATH without the shim
// directory, BROWSER cleared when it names a shim, and the loop guard set.
func withoutShims(environ []string, shims string) []string {
	kept := make([]string, 0, len(environ)+1)
	for _, entry := range environ {
		key, value, _ := strings.Cut(entry, "=")
		switch {
		case key == "BROWSER" && strings.HasPrefix(value, shims+string(os.PathSeparator)):
			continue
		case key == "PATH":
			dirs := slices.DeleteFunc(filepath.SplitList(value), func(dir string) bool {
				return filepath.Clean(dir) == filepath.Clean(shims)
			})
			entry = "PATH=" + strings.Join(dirs, string(os.PathListSeparator))
		}
		kept = append(kept, entry)
	}
	return append(kept, nativeOpenEnv+"=1")
}
