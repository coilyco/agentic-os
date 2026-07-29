package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/urfave/cli/v3"
)

var (
	hostLookPath = exec.LookPath
	hostCommand  = runHostCommand
)

type integratedLaunchOptions struct {
	Image       string
	Role        string
	Agent       string
	Layout      string
	Delivery    string
	Warded      bool
	Composed    bool
	Guarded     bool
	NoSubstrate bool
	Auth        bool
	DryRun      bool
	Arguments   []string
}

type wardLaunchPlan struct {
	Command string
	Args    []string
}

func runIntegratedLaunch(ctx context.Context, cmd *cli.Command) error {
	if err := validateLegacyDensity(cmd.String("density")); err != nil {
		return err
	}
	opts := integratedLaunchOptions{
		Image:       strings.TrimSpace(cmd.String("image")),
		Role:        strings.TrimSpace(cmd.String("role")),
		Agent:       strings.TrimSpace(cmd.String("agent")),
		Layout:      strings.TrimSpace(cmd.String("layout")),
		Delivery:    strings.TrimSpace(cmd.String("delivery")),
		Warded:      cmd.Bool("warded"),
		Composed:    cmd.Bool("composed"),
		Guarded:     cmd.Bool("guarded"),
		NoSubstrate: cmd.Bool("no-substrate"),
		Auth:        cmd.Bool("auth"),
		DryRun:      cmd.Bool("dry-run"),
		Arguments:   cmd.Args().Slice(),
	}
	if !opts.Warded && !opts.Composed && !opts.Guarded {
		return cli.ShowAppHelp(cmd)
	}
	if err := validateIntegratedLaunch(opts); err != nil {
		return err
	}
	opts.Layout = opts.Agent

	if opts.Warded {
		return runWardedLaunch(ctx, cmd, opts)
	}
	return runStandaloneIntegratedLaunch(ctx, cmd, opts)
}

func validateIntegratedLaunch(opts integratedLaunchOptions) error {
	if opts.Role == "" {
		return fmt.Errorf("integrated launch needs --role")
	}
	if !safeRoleSlug(opts.Role) {
		return fmt.Errorf("--role %q is not a safe shared role slug", opts.Role)
	}
	if opts.Agent == "" {
		return fmt.Errorf("integrated launch needs --agent")
	}
	layout, err := resolveLayout("", opts.Agent)
	if err != nil || layout != opts.Agent {
		return fmt.Errorf("unsupported --agent %q: want claude, codex, goose, or opencode", opts.Agent)
	}
	if opts.Layout != "" && opts.Layout != opts.Agent {
		return fmt.Errorf("--agent %s conflicts with --layout %s", opts.Agent, opts.Layout)
	}
	if opts.Image == "" {
		return fmt.Errorf("--image must not be empty")
	}
	if opts.Composed && opts.Delivery != "native-skills" && opts.Delivery != "compiled" {
		return fmt.Errorf("unsupported --delivery %q: want native-skills or compiled", opts.Delivery)
	}
	if !opts.Composed && opts.NoSubstrate {
		return fmt.Errorf("--no-substrate needs --composed")
	}
	if opts.Warded {
		switch opts.Role {
		case "director", "qa", "engineer":
		default:
			return fmt.Errorf(
				"--warded role %q is unsupported: Ward ships director, qa, and engineer",
				opts.Role,
			)
		}
		if (opts.Role == "engineer" || opts.Role == "qa") && len(opts.Arguments) == 0 {
			return fmt.Errorf("--warded %s needs an issue reference or freeform work", opts.Role)
		}
		if forbidden := forbiddenWardArgument(opts.Arguments); forbidden != "" {
			return fmt.Errorf(
				"launch argument %q conflicts with AOS-owned Ward translation",
				forbidden,
			)
		}
	}
	return nil
}

func safeRoleSlug(value string) bool {
	if value == "" || value[0] < 'a' || value[0] > 'z' {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			continue
		}
		return false
	}
	return true
}

func forbiddenWardArgument(arguments []string) string {
	owned := []string{
		"--agent",
		"--harness",
		"--image",
		"--tag",
		"--context-bundle",
		"--ward-source",
		"--ward-version",
	}
	for _, argument := range arguments {
		for _, flag := range owned {
			if argument == flag || strings.HasPrefix(argument, flag+"=") {
				return argument
			}
		}
	}
	return ""
}

func runStandaloneIntegratedLaunch(
	ctx context.Context,
	cmd *cli.Command,
	opts integratedLaunchOptions,
) error {
	cwd, err := filepath.Abs(".")
	if err != nil {
		return fmt.Errorf("resolve current working directory: %w", err)
	}
	command := append([]string{opts.Agent}, opts.Arguments...)
	uid, gid := hostIdentity()
	authMounts := []authMount{}
	if opts.Auth {
		authMounts = discoverAuthMounts(opts.Agent)
	}
	mcp, err := discoverMCPLaunch(ctx)
	if err != nil {
		return err
	}
	plan, err := buildLaunchPlan(launchOptions{
		Image:           opts.Image,
		Role:            opts.Role,
		Layout:          opts.Agent,
		Delivery:        opts.Delivery,
		Composed:        opts.Composed,
		Guarded:         opts.Guarded,
		CWD:             cwd,
		Command:         command,
		UID:             uid,
		GID:             gid,
		TTY:             isTerminal(os.Stdin),
		NoSubstrate:     opts.NoSubstrate,
		AuthMounts:      authMounts,
		ForwardedEnvs:   forwardedEnvironment(),
		MCPInventory:    mcp.Inventory,
		TailnetNetwork:  mcp.TailnetNetwork,
		TailnetForwards: mcp.Forwards,
	})
	if err != nil {
		return err
	}
	if opts.DryRun {
		fmt.Fprintln(cmd.Root().Writer, shellJoin(append([]string{"docker"}, plan.DockerArgs...)))
		return nil
	}
	return runDocker(ctx, plan.DockerArgs)
}

func runWardedLaunch(
	ctx context.Context,
	cmd *cli.Command,
	opts integratedLaunchOptions,
) error {
	var launchEnvironment []string
	if !opts.DryRun {
		var err error
		launchEnvironment, err = wardLaunchEnvironment(ctx)
		if err != nil {
			return err
		}
	}
	bundlePath := ""
	if opts.Composed || opts.Guarded {
		if opts.DryRun {
			uid, gid := hostIdentity()
			plan, err := buildContextBundlePlan(contextBundlePlanOptions{
				Image:    opts.Image,
				Role:     opts.Role,
				Agent:    opts.Agent,
				Delivery: opts.Delivery,
				Composed: opts.Composed,
				Guarded:  opts.Guarded,
				Output:   "<AOS_CONTEXT_BUNDLE_STAGING>",
				UID:      uid,
				GID:      gid,
			})
			if err != nil {
				return err
			}
			fmt.Fprintln(
				cmd.Root().Writer,
				shellJoin(append([]string{"docker"}, plan.DockerArgs...)),
			)
			bundlePath = "<AOS_CONTEXT_BUNDLE>"
		} else {
			wardPath, err := resolveWardWithContextContract(ctx)
			if err != nil {
				return err
			}
			_ = wardPath
			bundlePath, err = materializeContextBundle(ctx, contextBundleMaterializeOptions{
				Image:    opts.Image,
				Role:     opts.Role,
				Agent:    opts.Agent,
				Delivery: opts.Delivery,
				Composed: opts.Composed,
				Guarded:  opts.Guarded,
			})
			if err != nil {
				return err
			}
		}
	}
	plan := buildWardLaunchPlan(opts, bundlePath)
	if opts.DryRun {
		fmt.Fprintln(
			cmd.Root().Writer,
			shellJoin(append([]string{plan.Command}, plan.Args...)),
		)
		return nil
	}
	wardPath, err := hostLookPath("ward")
	if err != nil {
		return fmt.Errorf("--warded needs Ward on the host PATH: %w", err)
	}
	plan.Command = wardPath
	return hostCommand(ctx, plan.Command, launchEnvironment, plan.Args...)
}

func buildWardLaunchPlan(opts integratedLaunchOptions, bundlePath string) wardLaunchPlan {
	args := []string{"agent", opts.Role}
	args = append(args, opts.Arguments...)
	args = append(args, "--agent", opts.Agent, "--image", opts.Image)
	if bundlePath != "" {
		args = append(args, "--context-bundle", bundlePath)
	}
	return wardLaunchPlan{Command: "ward", Args: args}
}

func resolveWardWithContextContract(ctx context.Context) (string, error) {
	wardPath, err := hostLookPath("ward")
	if err != nil {
		return "", fmt.Errorf("--warded needs Ward on the host PATH: %w", err)
	}
	command := exec.CommandContext(ctx, wardPath, "agent", "flags")
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	if err := command.Run(); err != nil {
		return "", fmt.Errorf("inspect Ward context-bundle contract: %w", err)
	}
	if !strings.Contains(output.String(), "--context-bundle") {
		return "", fmt.Errorf(
			"installed Ward does not advertise --context-bundle; update Ward before using --composed or --guarded with --warded",
		)
	}
	return wardPath, nil
}

func runHostCommand(
	ctx context.Context,
	name string,
	environment []string,
	args ...string,
) error {
	command := exec.CommandContext(ctx, name, args...)
	command.Env = environment
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("%s %s: %w", filepath.Base(name), args[0], err)
	}
	return nil
}
