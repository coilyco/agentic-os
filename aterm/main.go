// Command aterm launches one composed agent session in its own branded window.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/urfave/cli/v3"
)

var version = "dev"

const (
	defaultExpression  = "acting"
	defaultOverlayBin  = "agent-compose"
	defaultAOSBin      = "aos"
	defaultTerminalBin = "kitty"
	defaultATermBin    = "aterm"
	// An agent session is the window's whole job, so it opens at the size that
	// job needs. kitty's own default font is 11.0. See docs/aterm.md.
	defaultStartAs       = "fullscreen"
	defaultFontSize      = "14.5"
	defaultWorkingEnvVar = "PROJECTS_ROOT"
)

type commandDeps struct {
	lookPath func(string) (string, error)
	output   func(context.Context, string, ...string) ([]byte, error)
	run      func(context.Context, string, ...string) error
	spawn    func(context.Context, string, ...string) error
	self     func() (string, error)
	pick     func(rosterDocument) (string, string, error)
	tty      func() bool
	// notice carries the slow-call line. A nil writer stays silent, which is
	// what a test wants unless it is asserting on the notice itself.
	notice io.Writer
}

func systemDeps() commandDeps {
	return commandDeps{
		lookPath: exec.LookPath,
		output: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			raw, err := exec.CommandContext(ctx, name, args...).Output()
			if err == nil {
				return raw, nil
			}
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				if detail := strings.TrimSpace(string(exitErr.Stderr)); detail != "" {
					return nil, fmt.Errorf("%s", detail)
				}
			}
			return nil, err
		},
		run: func(ctx context.Context, name string, args ...string) error {
			return exec.CommandContext(ctx, name, args...).Run()
		},
		spawn: func(_ context.Context, name string, args ...string) error {
			return spawnWindow(name, args)
		},
		self:   os.Executable,
		pick:   pickRoleAndSeat,
		tty:    func() bool { return interactiveTTY(os.Stdin, os.Stdout) },
		notice: os.Stderr,
	}
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == sessionCommand {
		options, err := parseSessionArgs(os.Args[2:])
		if err != nil {
			fmt.Fprintln(os.Stderr, "aterm:", err)
			os.Exit(exitUsage)
		}
		options.Motion = options.Motion && cardMotionWanted(os.Stdout, false)
		options.Audible = soundWanted(os.Stdout, options.Audible)
		os.Exit(runSession(options, os.Stdin, os.Stdout, os.Stderr))
	}
	if err := newCommand(systemDeps()).Run(context.Background(), os.Args); err != nil {
		fmt.Fprintln(os.Stderr, "aterm:", err)
		os.Exit(exitCodeFor(err))
	}
}

func defaultWorkingDirectory() string {
	if projectsRoot := strings.TrimSpace(os.Getenv(defaultWorkingEnvVar)); projectsRoot != "" {
		return projectsRoot
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, "projects")
}

func newCommand(deps commandDeps) *cli.Command {
	return &cli.Command{
		Name:      "aterm",
		Usage:     "launch one composed agent session in its own branded window",
		ArgsUsage: "[role] [seat] [-- harness arguments...]",
		Description: "With no role, aterm asks. It validates the role and seat against the\n" +
			"live Agent Compose roster before opening anything, and the window it\n" +
			"opens runs the same native session `acompose` runs in place.",
		Version:               version,
		EnableShellCompletion: true,
		ShellComplete: func(ctx context.Context, cmd *cli.Command) {
			completeInvocation(ctx, deps, cmd)
		},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "expression",
				Value: defaultExpression,
				Usage: "caller-supplied renderer expression",
			},
			&cli.StringFlag{Name: "task-title", Usage: "repository or issue label for the window title"},
			&cli.StringFlag{
				Name:  "start-as",
				Value: defaultStartAs,
				Usage: "kitty window state at open: normal, maximized, or fullscreen",
			},
			&cli.StringFlag{
				Name:  "font-size",
				Value: defaultFontSize,
				Usage: "kitty font size for the session window",
			},
			&cli.StringFlag{
				Name:  "working-directory",
				Value: defaultWorkingDirectory(),
				Usage: "agent working directory (defaults to the projects root)",
			},
			&cli.BoolFlag{
				Name:  "list",
				Usage: "print the live roster and exit",
			},
			&cli.BoolFlag{
				Name:  "no-motion",
				Usage: "skip the identity card animation, for a recording or a log",
			},
			&cli.BoolFlag{
				Name:  "sound",
				Usage: "play the role's sound mark at launch, which is otherwise silent",
			},
			&cli.BoolFlag{
				Name:  "no-creature",
				Usage: "leave the window background flat, without the role's creature behind the session",
			},
			&cli.BoolFlag{
				Name:  "json",
				Usage: "machine-readable output, with --list or --dry-run",
			},
			&cli.BoolFlag{
				Name:  "hold",
				Usage: "keep the window open after a clean exit (a failure always holds)",
			},
			&cli.BoolFlag{
				Name:  "dry-run",
				Usage: "print the resolved identity and launch plan without opening a window",
			},
			&cli.StringFlag{
				Name:    "agent-compose-bin",
				Value:   defaultOverlayBin,
				Sources: cli.EnvVars("AGENT_COMPOSE_BIN"),
			},
			&cli.StringFlag{
				Name:    "aos-bin",
				Value:   defaultAOSBin,
				Sources: cli.EnvVars("AOS_BIN"),
			},
			&cli.StringFlag{
				Name:    "terminal-bin",
				Value:   defaultTerminalBin,
				Usage:   "terminal to open the window with (kitty's flag dialect)",
				Sources: cli.EnvVars("ATERM_TERMINAL_BIN", "KITTY_BIN"),
			},
		},
		Commands: []*cli.Command{
			{
				Name:  "doctor",
				Usage: "preflight the whole launch chain and say what would stop a window",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					return runDoctor(ctx, deps, cmd)
				},
			},
			newBundlesCommand(deps),
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return runLaunch(ctx, deps, cmd)
		},
	}
}

func runLaunch(ctx context.Context, deps commandDeps, cmd *cli.Command) error {
	stdout := cmd.Root().Writer
	agentCompose, err := requireBinary(deps.lookPath, cmd.String("agent-compose-bin"))
	if err != nil {
		return fmt.Errorf("agent-compose binary: %w", err)
	}
	roster, err := loadRoster(ctx, deps, agentCompose)
	if err != nil {
		return err
	}
	if cmd.Bool("json") && !cmd.Bool("list") && !cmd.Bool("dry-run") {
		return withExit(exitUsage, fmt.Errorf("--json applies to --list and --dry-run"))
	}
	if cmd.Bool("list") {
		if cmd.Bool("json") {
			return writeRosterJSON(stdout, roster)
		}
		return writeRoster(stdout, roster)
	}
	cwd, err := validateWorkingDirectory(cmd.String("working-directory"))
	if err != nil {
		return withExit(exitUsage, err)
	}
	role, seat, extra, err := resolveInvocation(
		ctx, deps, cmd.String("aos-bin"), roster, cmd.Args().Slice(),
	)
	if err != nil {
		return err
	}
	if err := validateWindow(cmd.String("start-as"), cmd.String("font-size")); err != nil {
		return err
	}
	request := launchRequest{
		Role:             role,
		Seat:             seat,
		Expression:       strings.TrimSpace(cmd.String("expression")),
		TaskTitle:        cmd.String("task-title"),
		WorkingDirectory: cwd,
		StartAs:          strings.TrimSpace(cmd.String("start-as")),
		FontSize:         strings.TrimSpace(cmd.String("font-size")),
		AgentComposeBin:  cmd.String("agent-compose-bin"),
		AOSBin:           cmd.String("aos-bin"),
		TerminalBin:      cmd.String("terminal-bin"),
		Workspace:        workspaceLabel(ctx, deps, cwd, cmd.IsSet("working-directory")),
		NoMotion:         cmd.Bool("no-motion"),
		Sound:            cmd.Bool("sound"),
		Extra:            extra,
		Hold:             cmd.Bool("hold"),
	}
	if creatureWanted(cmd.Bool("no-creature")) {
		request.Creature = bakeCreaturePlate(role, creaturePresence)
	}
	aos, err := requireBinary(deps.lookPath, request.AOSBin)
	if err != nil {
		return fmt.Errorf("aos binary: %w", err)
	}
	self, err := deps.self()
	if err != nil {
		return fmt.Errorf("resolve the aterm executable: %w", err)
	}
	document, err := loadOverlay(ctx, deps, agentCompose, role, seat, request.Expression)
	if err != nil {
		return err
	}
	shadowed := nativeShadowAvailable(ctx, deps, aos)
	plan, err := buildLaunchPlan(document, request, cwd, self, agentCompose, aos, shadowed)
	if err != nil {
		return err
	}
	if cmd.Bool("dry-run") {
		if !cmd.Bool("json") {
			return renderPlan(stdout, document, plan)
		}
		encoded, err := json.MarshalIndent(plan, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal the launch plan: %w", err)
		}
		_, err = fmt.Fprintf(stdout, "%s\n", encoded)
		return err
	}
	terminal, err := requireBinary(deps.lookPath, request.TerminalBin)
	if err != nil {
		return fmt.Errorf("terminal binary: %w", err)
	}
	if installed := bundleTerminal(request.Role); installed != "" {
		terminal = installed
	}
	if err := deps.spawn(ctx, terminal, plan.Arguments...); err != nil {
		return withExit(exitSpawn, fmt.Errorf("open the window: %w", err))
	}
	return announce(stdout, plan)
}

func announce(writer io.Writer, plan launchPlan) error {
	_, err := fmt.Fprintf(writer, "%s // %s\n", plan.Identity.Annotation, plan.Identity.Seat)
	return err
}

// resolveInvocation turns the positional tail into a validated role and seat.
// A bare invocation asks, since the roster is what the caller usually lacks.
func resolveInvocation(
	ctx context.Context,
	deps commandDeps,
	aosBin string,
	document rosterDocument,
	args []string,
) (string, string, []string, error) {
	args = append([]string(nil), args...)
	role := ""
	seat := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		role = strings.TrimSpace(args[0])
		args = args[1:]
		if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
			seat = strings.TrimSpace(args[0])
			args = args[1:]
		}
	}
	if role == "" {
		if !deps.tty() {
			return "", "", nil, withExit(exitUsage, fmt.Errorf(
				"a role is required when aterm is not attached to a terminal. `aterm --list` names them",
			))
		}
		picked, pickedSeat, err := deps.pick(document)
		if err != nil {
			return "", "", nil, err
		}
		return picked, pickedSeat, args, nil
	}
	if !safeRoleSlug(role) {
		return "", "", nil, withExit(exitUsage, fmt.Errorf("role %q is not a safe role slug", role))
	}
	selected, ok := document.role(role)
	if !ok {
		return "", "", nil, unknownRoleError(role, document)
	}
	native := selected.nativeSeats()
	if len(native) == 0 {
		return "", "", nil, withExit(exitOffRoster, fmt.Errorf("role %s has no launchable native seat", role))
	}
	if seat == "" {
		return role, defaultSeat(ctx, deps, aosBin, selected), args, nil
	}
	if !isNativeHarness(seat) || !seatInRole(seat, selected) {
		return "", "", nil, unknownSeatError(seat, selected)
	}
	return role, seat, args, nil
}

// defaultSeat asks aos, which owns the launch profiles, so this binary carries
// no second parser of them. See docs/aterm.md.
func defaultSeat(ctx context.Context, deps commandDeps, aosBin string, role rosterRole) string {
	if aos, err := requireBinary(deps.lookPath, aosBin); err == nil {
		command := []string{aos, "_launch-agent", role.Slug}
		raw, err := whileWaiting2(deps.notice, command, func() ([]byte, error) {
			return deps.output(ctx, command[0], command[1:]...)
		})
		if err == nil {
			seat := strings.TrimSpace(string(raw))
			if isNativeHarness(seat) && seatInRole(seat, role) {
				return seat
			}
		}
	}
	// An aos too old for the verb still launches, on the catalogue's own order
	// whose first native entry is the frontier seat.
	return role.nativeSeats()[0].Harness
}

// validateWindow refuses a bad window option here, where the message can name
// the flag, rather than letting kitty refuse an argv it did not ask for.
func validateWindow(startAs, fontSize string) error {
	states := []string{"normal", "maximized", "fullscreen", "minimized"}
	if !slices.Contains(states, strings.TrimSpace(startAs)) {
		return withExit(exitUsage, fmt.Errorf(
			"--start-as %q is not one of %s", startAs, strings.Join(states, ", ")))
	}
	size, err := strconv.ParseFloat(strings.TrimSpace(fontSize), 64)
	if err != nil || size <= 0 {
		return withExit(exitUsage, fmt.Errorf("--font-size %q is not a positive number", fontSize))
	}
	return nil
}

func requireBinary(lookPath func(string) (string, error), name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", withExit(exitUsage, fmt.Errorf("binary name is empty"))
	}
	resolved, err := lookPath(name)
	if err != nil {
		return "", withExit(exitMissing, fmt.Errorf("%q was not found on PATH: %w", name, err))
	}
	return resolved, nil
}

func validateWorkingDirectory(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("working directory is empty")
	}
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", fmt.Errorf("resolve working directory: %w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", fmt.Errorf("inspect working directory %q: %w", absolute, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("working directory %q is not a directory", absolute)
	}
	return absolute, nil
}
