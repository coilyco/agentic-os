package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/urfave/cli/v3"
)

var version = "dev"

const (
	defaultImage    = "forgejo.coilysiren.me/coilyco-flight-deck/agentic-os:release"
	defaultDelivery = "native-skills"
)

type launchDefaults struct {
	Warded bool
}

func main() {
	cmd := newCommandForInvocation(os.Args[0])
	if err := cmd.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", cmd.Name, err)
		os.Exit(1)
	}
}

func newCommand() *cli.Command {
	return newCommandWithDefaults("aos", launchDefaults{})
}

func newCommandForInvocation(executable string) *cli.Command {
	name := strings.TrimSuffix(strings.ToLower(filepath.Base(executable)), ".exe")
	if name == "aosward" || strings.HasPrefix(name, "aosward-") {
		return newCommandWithDefaults("aosward", launchDefaults{Warded: true})
	}
	for _, alias := range []string{"aoscompose", "aoscomposed"} {
		if name == alias || strings.HasPrefix(name, alias+"-") {
			return newCommandWithDefaults(alias, launchDefaults{})
		}
	}
	return newCommand()
}

func newCommandWithDefaults(name string, defaults launchDefaults) *cli.Command {
	return &cli.Command{
		Name:      name,
		Usage:     "launch composed agents with guarded tools",
		ArgsUsage: "[-- <launch arguments...>]",
		Version:   version,
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return runIntegratedLaunch(ctx, cmd, defaults)
		},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "role",
				Usage: "shared role slug selected across enabled capabilities",
			},
			&cli.StringFlag{
				Name:  "agent-id",
				Usage: "optional stable peer id for a generic warded role",
			},
			&cli.StringFlag{
				Name:  "agent",
				Usage: "agent harness selected for the launch",
			},
			&cli.BoolFlag{
				Name:  "warded",
				Usage: "delegate the runtime lifecycle and authority boundary to Ward",
			},
			&cli.BoolFlag{
				Name:  "composed",
				Usage: "compatibility flag; AOS always materializes agent-compose role context",
			},
			&cli.BoolFlag{
				Name:  "guarded",
				Usage: "compatibility flag; AOS always attaches aosguard and its generated skill",
			},
			&cli.StringFlag{
				Name:  "image",
				Value: defaultImage,
				Usage: "AOS dev-base image",
			},
			&cli.StringFlag{
				Name:  "layout",
				Usage: "agent-compose harness layout, inferred from the command when omitted",
			},
			&cli.StringFlag{
				Name:   "density",
				Usage:  "retired compatibility input, only full is accepted",
				Hidden: true,
			},
			&cli.StringFlag{
				Name:  "delivery",
				Value: defaultDelivery,
				Usage: "agent-compose delivery mode: native-skills or compiled",
			},
			&cli.BoolFlag{
				Name:  "dry-run",
				Usage: "print the Docker launch command without running it",
			},
			&cli.BoolFlag{
				Name:  "no-substrate",
				Usage: "skip materializing the baked read-only substrate",
			},
			&cli.BoolFlag{
				Name:  "auth",
				Value: true,
				Usage: "stage the selected harness's known host auth file when present",
			},
			&cli.StringFlag{
				Name:  "kubeconfig",
				Usage: "operator-selected host kubeconfig for an authorized standalone role",
			},
		},
		Commands: []*cli.Command{
			{
				Name:  "version",
				Usage: "print the build version",
				Action: func(_ context.Context, _ *cli.Command) error {
					fmt.Println(version)
					return nil
				},
			},
			{
				Name:  "converge",
				Usage: "converge AOS-owned catalogues and native tool configuration",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "config",
						Usage: "AOS convergence YAML (defaults to ~/.config/aos/converge.yaml)",
					},
					&cli.StringFlag{
						Name:  "home",
						Usage: "home receiving generated state and native projections",
					},
					&cli.BoolFlag{
						Name:  "check",
						Usage: "report drift without network or filesystem mutation",
					},
				},
				Action: runEnvironmentConverge,
			},
			{
				Name:      "acompose",
				Usage:     "launch a composed agent in the AOS image",
				ArgsUsage: "-- <harness> [args...]",
				Action:    runAcompose,
			},
			{
				Name:   "acompose-checkin",
				Usage:  "run an agent-specific composed-role check-in",
				Action: runAcomposeCheckin,
			},
			{
				Name:   "_native-shadow",
				Hidden: true,
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "harness"},
					&cli.BoolFlag{Name: "probe"},
					&cli.BoolFlag{Name: "assigned-role"},
				},
				Action: runNativeShadow,
			},
			{
				Name:      "_container-acompose",
				Hidden:    true,
				ArgsUsage: "-- <harness> [args...]",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "workspace",
						Required: true,
					},
					&cli.IntFlag{
						Name:     "uid",
						Required: true,
					},
					&cli.IntFlag{
						Name:     "gid",
						Required: true,
					},
					&cli.StringFlag{
						Name:   "mcp-inventory",
						Hidden: true,
					},
					&cli.StringSliceFlag{
						Name:   "tailnet-forward",
						Hidden: true,
					},
				},
				Action: runContainerAcompose,
			},
			{
				Name:   "_container-socks-forward",
				Hidden: true,
				Flags: []cli.Flag{
					&cli.IntFlag{
						Name:     "fd",
						Required: true,
					},
					&cli.StringFlag{
						Name:     "proxy",
						Required: true,
					},
					&cli.StringFlag{
						Name:     "target",
						Required: true,
					},
				},
				Action: func(_ context.Context, cmd *cli.Command) error {
					return runContainerSOCKSForward(
						cmd.Int("fd"),
						cmd.String("proxy"),
						cmd.String("target"),
					)
				},
			},
			{
				Name:   "_container-context-bundle",
				Hidden: true,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "output",
						Required: true,
					},
					&cli.IntFlag{
						Name:     "uid",
						Required: true,
					},
					&cli.IntFlag{
						Name:     "gid",
						Required: true,
					},
				},
				Action: runContainerContextBundle,
			},
		},
	}
}

func runAcompose(ctx context.Context, cmd *cli.Command) error {
	if err := validateLegacyDensity(cmd.String("density")); err != nil {
		return err
	}
	role := strings.TrimSpace(cmd.String("role"))
	if role == "" {
		return fmt.Errorf("acompose needs --role")
	}
	command := argvAfterDash(os.Args)
	if len(command) == 0 {
		return fmt.Errorf("acompose needs a command after `--`")
	}
	layout, err := resolveLayout(cmd.String("layout"), command[0])
	if err != nil {
		return err
	}
	return runComposedLaunch(ctx, cmd, role, layout, command, cmd.Bool("no-substrate"))
}

func runAcomposeCheckin(ctx context.Context, cmd *cli.Command) error {
	if err := validateLegacyDensity(cmd.String("density")); err != nil {
		return err
	}
	role := strings.TrimSpace(cmd.String("role"))
	if role == "" {
		return fmt.Errorf("acompose-checkin needs --role")
	}
	spec, err := resolveAcomposeCheckin(cmd.String("agent"))
	if err != nil {
		return err
	}
	if layout := strings.TrimSpace(cmd.String("layout")); layout != "" && layout != spec.Layout {
		return fmt.Errorf("--agent %s conflicts with --layout %s", spec.Agent, layout)
	}
	return runComposedLaunch(ctx, cmd, role, spec.Layout, spec.Command, true)
}

func runComposedLaunch(
	ctx context.Context,
	cmd *cli.Command,
	role string,
	layout string,
	command []string,
	noSubstrate bool,
) error {
	cwd, err := filepath.Abs(".")
	if err != nil {
		return fmt.Errorf("resolve current working directory: %w", err)
	}
	uid, gid := hostIdentity()
	authMounts := []authMount{}
	if cmd.Bool("auth") {
		authMounts = discoverAuthMounts(layout)
	}
	mcp, err := discoverMCPLaunch(ctx)
	if err != nil {
		return err
	}
	plan, err := buildLaunchPlan(launchOptions{
		Image:           cmd.String("image"),
		Role:            role,
		Layout:          layout,
		Delivery:        cmd.String("delivery"),
		Composed:        true,
		CWD:             cwd,
		Command:         command,
		UID:             uid,
		GID:             gid,
		TTY:             isTerminal(os.Stdin),
		NoSubstrate:     noSubstrate,
		AuthMounts:      authMounts,
		ForwardedEnvs:   forwardedEnvironment(),
		Guarded:         true,
		Kubeconfig:      cmd.String("kubeconfig"),
		MCPInventory:    mcp.Inventory,
		TailnetNetwork:  mcp.TailnetNetwork,
		TailnetForwards: mcp.Forwards,
	})
	if err != nil {
		return err
	}
	if cmd.Bool("dry-run") {
		fmt.Fprintln(cmd.Root().Writer, shellJoin(append([]string{"docker"}, plan.DockerArgs...)))
		return nil
	}
	return runDocker(ctx, plan.DockerArgs)
}

func runContainerAcompose(ctx context.Context, cmd *cli.Command) error {
	if os.Getenv("AOS_CONTAINER") != "1" {
		return fmt.Errorf("_container-acompose is internal to an AOS container")
	}
	if err := validateLegacyDensity(cmd.String("density")); err != nil {
		return err
	}
	command := argvAfterDash(os.Args)
	if len(command) == 0 {
		return fmt.Errorf("_container-acompose needs a command after `--`")
	}
	role := strings.TrimSpace(cmd.String("role"))
	if role == "" {
		return fmt.Errorf("_container-acompose needs --role")
	}
	layout, err := resolveLayout(cmd.String("layout"), command[0])
	if err != nil {
		return err
	}
	forwards, err := decodeTailnetForwards(cmd.StringSlice("tailnet-forward"))
	if err != nil {
		return err
	}
	spec, err := prepareContainer(ctx, bootstrapOptions{
		Role:            role,
		Layout:          layout,
		Delivery:        cmd.String("delivery"),
		Composed:        cmd.Bool("composed"),
		Guarded:         cmd.Bool("guarded"),
		Workspace:       cmd.String("workspace"),
		UID:             cmd.Int("uid"),
		GID:             cmd.Int("gid"),
		Command:         command,
		NoSubstrate:     cmd.Bool("no-substrate"),
		MCPInventory:    cmd.String("mcp-inventory"),
		TailnetForwards: forwards,
	}, osCommandRunner{})
	if err != nil {
		return err
	}
	if err := startSOCKSForwarders(cmd.Int("uid"), cmd.Int("gid"), spec); err != nil {
		return err
	}
	return execAs(cmd.Int("uid"), cmd.Int("gid"), spec)
}

func validateLegacyDensity(value string) error {
	value = strings.TrimSpace(value)
	if value == "" || value == "full" {
		return nil
	}
	return fmt.Errorf("personality density %q was removed; omit --density", value)
}

func argvAfterDash(argv []string) []string {
	for i, arg := range argv {
		if arg == "--" {
			return append([]string(nil), argv[i+1:]...)
		}
	}
	return nil
}
