package main

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"text/template"

	"github.com/urfave/cli/v3"
)

// launchdLabel is the agent that runs the daemon at login and brings it back
// after a crash. See docs/aterm-daemon.md.
const launchdLabel = "dev.coilyco.aterm-daemon"

//go:embed launchd.plist.tmpl
var launchdTemplate string

// launchdOptions is what the plist varies on. Extra is a test's way to point
// the job at its own socket without touching the real daemon's.
type launchdOptions struct {
	Label string
	Bin   string
	Home  string
	Extra []string
}

// daemonEnvLoader reads daemon.env as a launcher does: literal KEY=VALUE text,
// ATERM_ keys only, never sourced. See docs/aterm-bundles.md.
const daemonEnvLoader = `f="${XDG_CONFIG_HOME:-$HOME/.config}/aterm/daemon.env"; ` +
	`if [ -f "$f" ]; then while IFS= read -r line || [ -n "$line" ]; do ` +
	`case "$line" in ATERM_*=*) k="${line%%=*}"; case "$k" in *[!A-Za-z0-9_]*) ;; *) export "$k=${line#*=}";; esac;; esac; ` +
	`done < "$f"; fi`

// renderLaunchdPlist is the agent's plist for one user. The job sources the
// host's daemon.env itself, since no app launcher exports it for a launchd job.
func renderLaunchdPlist(options launchdOptions) (string, error) {
	if options.Label == "" {
		options.Label = launchdLabel
	}
	if options.Bin == "" || options.Home == "" {
		return "", errors.New("the plist needs the aterm binary and the home directory")
	}
	command := shellQuote(options.Bin) + " daemon --idle 0"
	for _, arg := range options.Extra {
		command += " " + shellQuote(arg)
	}
	script := daemonEnvLoader + "; exec " + command
	fields := map[string]string{
		"Label":  xmlEscape(options.Label),
		"Script": xmlEscape(script),
		"Log":    xmlEscape(filepath.Join(options.Home, "Library", "Logs", "aterm-daemon.log")),
	}
	parsed, err := template.New("plist").Option("missingkey=error").Parse(launchdTemplate)
	if err != nil {
		return "", err
	}
	var out bytes.Buffer
	if err := parsed.Execute(&out, fields); err != nil {
		return "", err
	}
	return out.String(), nil
}

// launchctl runs the macOS service manager. A variable so a test can watch the
// calls without touching the host's.
var launchctl = func(args ...string) error {
	if runtime.GOOS != "darwin" {
		return errors.New("launchd is macOS only")
	}
	return exec.Command("launchctl", args...).Run()
}

func launchdTarget() string {
	return "gui/" + strconv.Itoa(os.Getuid()) + "/" + launchdLabel
}

// launchdLoaded is whether the agent is loaded in this user's login domain.
func launchdLoaded() bool { return launchctl("print", launchdTarget()) == nil }

// startViaLaunchd asks the loaded agent to start the daemon, but only for the
// socket that agent serves. A false answer leaves the caller to start one itself.
func startViaLaunchd(socket string) bool {
	if socket != defaultDaemonSocket() || !launchdLoaded() {
		return false
	}
	return launchctl("kickstart", launchdTarget()) == nil
}

// stableBinary is the path for a plist. An upgrade removes the Cellar path
// os.Executable reports, so prefer the PATH entry resolving to the same file.
func stableBinary() (string, error) {
	self, err := os.Executable()
	if err != nil {
		return "", err
	}
	if onPath, err := exec.LookPath(filepath.Base(self)); err == nil {
		a, errA := filepath.EvalSymlinks(onPath)
		b, errB := filepath.EvalSymlinks(self)
		if errA == nil && errB == nil && a == b {
			return onPath, nil
		}
	}
	return self, nil
}

func newDaemonLaunchdCommand() *cli.Command {
	return &cli.Command{
		Name:  "launchd",
		Usage: "print the launchd agent that runs the daemon at login and revives it after a crash",
		Description: "Write it to ~/Library/LaunchAgents/" + launchdLabel + ".plist and load it with\n" +
			"launchctl bootstrap. Host rollout belongs to ansible, which stops any daemon already\n" +
			"running first, since a daemon from before session holders ends every session when it stops.",
		Flags: []cli.Flag{&cli.StringFlag{Name: "bin", Usage: "aterm binary the job runs, default the one on PATH"}},
		Action: func(_ context.Context, cmd *cli.Command) error {
			bin := cmd.String("bin")
			if bin == "" {
				var err error
				if bin, err = stableBinary(); err != nil {
					return err
				}
			}
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			plist, err := renderLaunchdPlist(launchdOptions{Bin: bin, Home: home})
			if err != nil {
				return err
			}
			_, err = fmt.Fprint(cmd.Root().Writer, plist)
			return err
		},
	}
}
