package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/urfave/cli/v3"
)

const paneFormat = "aterm.pane.v1"

const (
	// kitty calls a pane a window, so `close-window` closes the preview and
	// `close-os-window` would close the session. See docs/aterm-pane.md.
	paneSplitLayout   = "tall"
	paneRestoreLayout = "fat"
	// Names the PANE's job, never the agent's role. Nothing inside the pane
	// can rewrite it, which is what makes it the handle. See docs/aterm-pane.md.
	paneVarName    = "role"
	defaultPaneTag = "preview"
	listenEnv      = "KITTY_LISTEN_ON"
	windowIDEnv    = "KITTY_WINDOW_ID"
)

type paneReport struct {
	Format string `json:"format"`
	Action string `json:"action"`
	Role   string `json:"role"`
	Tag    string `json:"tag"`
	Layout string `json:"layout"`
	// Share is the surviving pane's measured share of the window, which is what
	// the plate is recomposed against. 1 means the creature has it all.
	Share   float64 `json:"share"`
	Plate   string  `json:"plate"`
	Changed bool    `json:"changed"`
}

// paneRemote is one window's remote-control endpoint. Resolving it is the
// preflight, so a caller never reads a raw kitty refusal. See docs/aterm-pane.md.
type paneRemote struct {
	Kitty  string
	Socket string
}

type kittyWindow struct {
	ID       int               `json:"id"`
	Columns  int               `json:"columns"`
	Cmdline  []string          `json:"cmdline"`
	UserVars map[string]string `json:"user_vars"`
}

type kittyTab struct {
	ID       int           `json:"id"`
	IsActive bool          `json:"is_active"`
	Layout   string        `json:"layout"`
	Windows  []kittyWindow `json:"windows"`
}

type kittyOSWindow struct {
	ID       int        `json:"id"`
	IsActive bool       `json:"is_active"`
	Tabs     []kittyTab `json:"tabs"`
}

func newPaneCommand(deps commandDeps) *cli.Command {
	return &cli.Command{
		Name:  "pane",
		Usage: "split this session's window beside a command, and put it back",
		Description: "One verb each way, against the window aterm opened. `on` splits, launches,\n" +
			"and moves the role creature into the surviving pane. `off` puts the creature\n" +
			"back and closes the pane, deriving the plate from the role rather than from\n" +
			"anything `on` stashed, so it works after the agent that split the window died.",
		Commands: []*cli.Command{
			{
				Name:      "on",
				Usage:     "split the window and run a command beside the session",
				ArgsUsage: "-- <command> [arguments...]",
				Flags:     paneFlags(),
				Action: func(ctx context.Context, cmd *cli.Command) error {
					return runPaneOn(ctx, deps, cmd)
				},
			},
			{
				Name:  "off",
				Usage: "restore the creature and close the pane",
				Flags: paneFlags(),
				Action: func(ctx context.Context, cmd *cli.Command) error {
					return runPaneOff(ctx, deps, cmd)
				},
			},
		},
	}
}

func paneFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:  "tag",
			Value: defaultPaneTag,
			Usage: "kitty user variable identifying the pane, so later commands can match it",
		},
		&cli.StringFlag{
			Name:  "role",
			Usage: "role slug whose creature to recompose (read from the window's own session card by default)",
		},
		&cli.BoolFlag{
			Name:  "json",
			Usage: "machine-readable result",
		},
		&cli.StringFlag{
			Name:    "terminal-bin",
			Value:   defaultTerminalBin,
			Usage:   "terminal binary carrying the remote-control client",
			Sources: cli.EnvVars("ATERM_TERMINAL_BIN", "KITTY_BIN"),
		},
	}
}

// resolvePaneRemote is the whole preflight. An unset socket and a kitty that
// refuses the connection are different problems, so they get different lines.
func resolvePaneRemote(deps commandDeps, terminalBin string) (paneRemote, error) {
	socket := strings.TrimSpace(os.Getenv(listenEnv))
	if socket == "" {
		return paneRemote{}, withExit(exitUsage, fmt.Errorf(
			"%s is unset, so this shell is not inside a kitty window that accepts remote control. "+
				"kitty.conf needs `allow_remote_control socket-only` and `listen_on unix:/tmp/kitty`, "+
				"and the window has to be reopened after that", listenEnv))
	}
	kitty, err := requireBinary(deps.lookPath, terminalBin)
	if err != nil {
		return paneRemote{}, fmt.Errorf("terminal binary: %w", err)
	}
	return paneRemote{Kitty: kitty, Socket: socket}, nil
}

// ask runs one remote-control verb. Every kitty call in this file goes through
// here, so the socket is never spelled twice and stderr always reaches the user.
func (remote paneRemote) ask(
	ctx context.Context, deps commandDeps, verb string, arguments ...string,
) ([]byte, error) {
	argv := append([]string{"@", "--to", remote.Socket, verb}, arguments...)
	raw, err := deps.output(ctx, remote.Kitty, argv...)
	if err != nil {
		return nil, fmt.Errorf("kitty %s: %w", verb, err)
	}
	return raw, nil
}

func (remote paneRemote) windows(ctx context.Context, deps commandDeps) ([]kittyOSWindow, error) {
	raw, err := remote.ask(ctx, deps, "ls")
	if err != nil {
		return nil, withExit(exitFailure, fmt.Errorf(
			"%w. The window was reachable at %s but refused remote control, which "+
				"`allow_remote_control` in kitty.conf governs", err, remote.Socket))
	}
	var document []kittyOSWindow
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, withExit(exitFailure, fmt.Errorf("decode the kitty window list: %w", err))
	}
	return document, nil
}

// selectTab finds the tab this command runs inside. KITTY_WINDOW_ID is exact,
// and the active tab is the fallback for a caller outside any pane.
func selectTab(document []kittyOSWindow) (kittyTab, error) {
	self, err := strconv.Atoi(strings.TrimSpace(os.Getenv(windowIDEnv)))
	if err == nil {
		for _, osWindow := range document {
			for _, tab := range osWindow.Tabs {
				for _, window := range tab.Windows {
					if window.ID == self {
						return tab, nil
					}
				}
			}
		}
	}
	for _, osWindow := range document {
		if !osWindow.IsActive {
			continue
		}
		for _, tab := range osWindow.Tabs {
			if tab.IsActive {
				return tab, nil
			}
		}
	}
	for _, osWindow := range document {
		if len(osWindow.Tabs) > 0 {
			return osWindow.Tabs[0], nil
		}
	}
	return kittyTab{}, withExit(exitFailure, fmt.Errorf("kitty reported no tab to act on"))
}

// resolveRole reads the slug out of the session card aterm put in the window's
// argv, which is why `off` needs nothing from `on`. See docs/aterm-pane.md.
func resolveRole(tab kittyTab, requested string) (string, error) {
	if requested = strings.TrimSpace(requested); requested != "" {
		if !safeRoleSlug(requested) {
			return "", withExit(exitUsage, fmt.Errorf("role %q is not a safe role slug", requested))
		}
		return requested, nil
	}
	for _, window := range tab.Windows {
		if role := roleFromCmdline(window.Cmdline); role != "" {
			return role, nil
		}
	}
	return "", withExit(exitUsage, fmt.Errorf(
		"no window in this tab carries an aterm session card, so the role is unknown. "+
			"Pass --role <slug>, which `aterm --list` names"))
}

func roleFromCmdline(cmdline []string) string {
	for index, value := range cmdline {
		if value != "--card" || index+1 >= len(cmdline) {
			continue
		}
		card, err := decodeSessionCard(cmdline[index+1])
		if err != nil {
			continue
		}
		if role := strings.TrimSpace(card.Role); safeRoleSlug(role) {
			return role
		}
	}
	return ""
}

func taggedWindow(tab kittyTab, tag string) (kittyWindow, bool) {
	for _, window := range tab.Windows {
		if window.UserVars[paneVarName] == tag {
			return window, true
		}
	}
	return kittyWindow{}, false
}

// survivingShare divides the columns kitty reports, since layout bias counts
// the divider it does not own. Only `tall` is measurable. See docs/aterm-pane.md.
func survivingShare(tab kittyTab, tag string) float64 {
	if tab.Layout != paneSplitLayout {
		return creatureWholeWindow
	}
	total, surviving := 0, 0
	for _, window := range tab.Windows {
		total += window.Columns
		if window.UserVars[paneVarName] != tag {
			surviving += window.Columns
		}
	}
	if total == 0 || surviving == 0 || surviving == total {
		return creatureWholeWindow
	}
	return float64(surviving) / float64(total)
}

// recomposeCreature re-derives the plate at the measured share. Never
// --configured, which would escape this window. See docs/aterm-pane.md.
func recomposeCreature(
	ctx context.Context, deps commandDeps, remote paneRemote, role string, occupancy float64,
) (string, error) {
	plate := bakeCreaturePlate(role, creaturePresence, occupancy)
	if plate.Path == "" {
		// A role with no art launched without a creature too, so there is
		// nothing to move and nothing to put back.
		return "", nil
	}
	if _, err := remote.ask(
		ctx, deps, "set-background-image", "--layout", "cscaled", plate.Path,
	); err != nil {
		return "", withExit(exitFailure, err)
	}
	return plate.Path, nil
}

func runPaneOn(ctx context.Context, deps commandDeps, cmd *cli.Command) error {
	command := cmd.Args().Slice()
	if len(command) == 0 {
		return withExit(exitUsage, fmt.Errorf("pane on needs a command after `--`"))
	}
	remote, err := resolvePaneRemote(deps, cmd.String("terminal-bin"))
	if err != nil {
		return err
	}
	document, err := remote.windows(ctx, deps)
	if err != nil {
		return err
	}
	tab, err := selectTab(document)
	if err != nil {
		return err
	}
	tag := strings.TrimSpace(cmd.String("tag"))
	if tag == "" {
		return withExit(exitUsage, fmt.Errorf("--tag is empty, and the pane needs a handle to be closed by"))
	}
	if _, found := taggedWindow(tab, tag); found {
		return withExit(exitUsage, fmt.Errorf(
			"a pane tagged %q is already open in this tab. `aterm pane off --tag %s` closes it first", tag, tag))
	}
	role, err := resolveRole(tab, cmd.String("role"))
	if err != nil {
		return err
	}
	// The layout decides the split direction, not a launch flag, so it goes
	// first or the pane lands as a band underneath. See docs/aterm-pane.md.
	if _, err := remote.ask(ctx, deps, "goto-layout", paneSplitLayout); err != nil {
		return withExit(exitFailure, err)
	}
	launch := append([]string{
		"--dont-take-focus",
		"--var", fmt.Sprintf("%s=%s", paneVarName, tag),
	}, command...)
	if _, err := remote.ask(ctx, deps, "launch", launch...); err != nil {
		return withExit(exitSpawn, err)
	}
	// Measure after the split, because the surviving pane's share is a fact
	// about the window kitty just rearranged rather than one about the request.
	split, err := remote.windows(ctx, deps)
	if err != nil {
		return err
	}
	measured, err := selectTab(split)
	if err != nil {
		return err
	}
	share := survivingShare(measured, tag)
	plate, err := recomposeCreature(ctx, deps, remote, role, share)
	if err != nil {
		return err
	}
	return writePaneReport(cmd.Root().Writer, paneReport{
		Format: paneFormat, Action: "on", Role: role, Tag: tag,
		Layout: paneSplitLayout, Share: share, Plate: plate, Changed: true,
	}, cmd.Bool("json"))
}

func runPaneOff(ctx context.Context, deps commandDeps, cmd *cli.Command) error {
	remote, err := resolvePaneRemote(deps, cmd.String("terminal-bin"))
	if err != nil {
		return err
	}
	document, err := remote.windows(ctx, deps)
	if err != nil {
		return err
	}
	tab, err := selectTab(document)
	if err != nil {
		return err
	}
	role, err := resolveRole(tab, cmd.String("role"))
	if err != nil {
		return err
	}
	tag := strings.TrimSpace(cmd.String("tag"))
	// The plate goes back before the pane closes, so a failure here leaves a
	// window that is merely split rather than one wearing the wrong creature.
	plate, err := recomposeCreature(ctx, deps, remote, role, creatureWholeWindow)
	if err != nil {
		return err
	}
	// Asking kitty to close nothing is an error, so the match is checked here
	// where "no pane" is the ordinary second call rather than a failure.
	_, found := taggedWindow(tab, tag)
	if found {
		if _, err := remote.ask(ctx, deps, "close-window", "--match", "var:"+paneVarName+"="+tag); err != nil {
			return withExit(exitFailure, err)
		}
	}
	if _, err := remote.ask(ctx, deps, "goto-layout", paneRestoreLayout); err != nil {
		return withExit(exitFailure, err)
	}
	return writePaneReport(cmd.Root().Writer, paneReport{
		Format: paneFormat, Action: "off", Role: role, Tag: tag,
		Layout: paneRestoreLayout, Share: creatureWholeWindow, Plate: plate, Changed: found,
	}, cmd.Bool("json"))
}

func writePaneReport(writer io.Writer, report paneReport, asJSON bool) error {
	if asJSON {
		encoded, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal the pane report: %w", err)
		}
		_, err = fmt.Fprintf(writer, "%s\n", encoded)
		return err
	}
	if report.Action == "on" {
		_, err := fmt.Fprintf(writer, "%s // pane %q open, %s, creature at %.0f%% of the window\n",
			report.Role, report.Tag, report.Layout, report.Share*100)
		return err
	}
	if !report.Changed {
		_, err := fmt.Fprintf(writer, "%s // no pane %q was open, creature restored, %s\n",
			report.Role, report.Tag, report.Layout)
		return err
	}
	_, err := fmt.Fprintf(writer, "%s // pane %q closed, creature restored, %s\n",
		report.Role, report.Tag, report.Layout)
	return err
}
