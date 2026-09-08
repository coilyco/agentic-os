package main

import (
	"context"
	"errors"
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
	Warded       bool
	RoleShortcut bool
	HostNetwork  bool
}

func main() {
	cmd := newCommandForInvocation(os.Args[0])
	args := normalizeRoleShortcutArgs(commandDefaultsForInvocation(os.Args[0]), os.Args)
	if err := cmd.Run(context.Background(), args); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", cmd.Name, err)
		os.Exit(1)
	}
}

func newCommand() *cli.Command {
	return newCommandWithDefaults("aos", launchDefaults{})
}

func newCommandForInvocation(executable string) *cli.Command {
	defaults := commandDefaultsForInvocation(executable)
	name := commandNameForInvocation(executable)
	return newCommandWithDefaults(name, defaults)
}

func commandDefaultsForInvocation(executable string) launchDefaults {
	name := strings.TrimSuffix(strings.ToLower(filepath.Base(executable)), ".exe")
	if name == "aosward" || strings.HasPrefix(name, "aosward-") {
		return launchDefaults{Warded: true}
	}
	for _, alias := range []string{"aoscompose", "aoscomposed"} {
		if name == alias || strings.HasPrefix(name, alias+"-") {
			return launchDefaults{RoleShortcut: true, HostNetwork: true}
		}
	}
	return launchDefaults{}
}

func commandNameForInvocation(executable string) string {
	name := strings.TrimSuffix(strings.ToLower(filepath.Base(executable)), ".exe")
	if name == "aosward" || strings.HasPrefix(name, "aosward-") {
		return "aosward"
	}
	for _, alias := range []string{"aoscompose", "aoscomposed"} {
		if name == alias || strings.HasPrefix(name, alias+"-") {
			return alias
		}
	}
	return "aos"
}

func normalizeRoleShortcutArgs(defaults launchDefaults, args []string) []string {
	if !defaults.RoleShortcut || hasDashTerminator(args) {
		return args
	}
	expectValue := false
	for index := 1; index < len(args); index++ {
		arg := args[index]
		if expectValue {
			expectValue = false
			continue
		}
		if strings.HasPrefix(arg, "--") {
			name, _, hasValue := strings.Cut(strings.TrimPrefix(arg, "--"), "=")
			if !hasValue && rootFlagTakesValue(name) {
				expectValue = true
			}
			continue
		}
		if strings.HasPrefix(arg, "-") {
			continue
		}
		if isRootSubcommand(arg) {
			return args
		}
		normalized := append([]string(nil), args[:index]...)
		normalized = append(normalized, "--")
		normalized = append(normalized, args[index:]...)
		return normalized
	}
	return args
}

func hasDashTerminator(args []string) bool {
	for _, arg := range args {
		if arg == "--" {
			return true
		}
	}
	return false
}

func rootFlagTakesValue(name string) bool {
	switch name {
	case "role", "agent-id", "agent", "image", "layout", "density", "delivery", "kubeconfig":
		return true
	default:
		return false
	}
}

func isRootSubcommand(value string) bool {
	switch value {
	case "repositories", "version", "converge", "acompose", "acompose-checkin",
		"_native-shadow", "_launch-agent", "_container-acompose",
		"_container-socks-forward", "_container-context-bundle":
		return true
	default:
		return false
	}
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
				Usage: "require and stage the selected harness's supported host auth",
			},
			&cli.StringFlag{
				Name:  "kubeconfig",
				Usage: "operator-selected host kubeconfig for an authorized standalone role",
			},
		},
		Commands: []*cli.Command{
			{
				Name:  "repositories",
				Usage: "print the deterministic host-residency projection from Agent Compose",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "plan", Value: defaultRepositoryPlanPath(), Usage: "compiled Agent Compose repository plan"},
					&cli.StringFlag{Name: "format", Value: "json", Usage: "output format: json or lines"},
				},
				Action: runRepositories,
			},
			{
				Name:      "run",
				Usage:     "run a just verb from whichever resident repository declares it",
				ArgsUsage: "<verb> [args...]",
				Description: "There is no justfile at the projects root and an elevated working\n" +
					"directory is deliberate, so `just <verb>` typed there can only fail.\n" +
					"Candidates come from the residency set in Agent Compose's compiled\n" +
					"repository-plan.yaml, enumerated with `just --summary`. One match runs.\n" +
					"Several list every candidate, unless the working directory sits inside\n" +
					"one, which selects it the way bare `just` there would.\n\n" +
					"Before running it reports how far the resolved checkout trails its\n" +
					"upstream, stamped with the age of the last fetch. It never fetches and\n" +
					"mutates nothing, so the stamp is what makes the count trustworthy.\n\n" +
					"Inside a session shadow it still runs, because most verbs are ordinary\n" +
					"there. It names the hazard when the resolved repository is one the\n" +
					"shadow does not carry: that run reaches the canonical checkout while\n" +
					"HOME is still the shadow, so a verb rendering a ~-rooted host path\n" +
					"writes into a temporary directory rather than converging the host.\n" +
					"A per-verb guard stays where it is and says why. This says where.",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "plan", Value: defaultRepositoryPlanPath(), Usage: "compiled Agent Compose repository plan"},
					&cli.StringFlag{Name: "repo", Usage: "owner/name, when more than one resident repository declares the verb"},
					&cli.BoolFlag{Name: "handoff", Usage: "print the absolute-path command to run outside the agent session, and run nothing"},
				},
				Action: runRun,
			},
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
				Usage:     "launch a standalone composed agent in the AOS image (no Ward: use the root action for --warded)",
				ArgsUsage: "-- <harness> [args...]",
				Action:    runAcompose,
			},
			{
				Name:   "acompose-checkin",
				Usage:  "run an agent-specific composed-role check-in",
				Action: runAcomposeCheckin,
			},
			{
				Name:      "_launch-agent",
				Hidden:    true,
				ArgsUsage: "<role>",
				Action:    runLaunchAgent,
			},
			{
				Name:   "_native-shadow",
				Hidden: true,
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "harness"},
					&cli.StringFlag{Name: "role"},
					&cli.BoolFlag{Name: "probe"},
					&cli.BoolFlag{Name: "assigned-role"},
					&cli.BoolFlag{Name: "list"},
					&cli.BoolFlag{Name: "json"},
					&cli.BoolFlag{Name: "reap"},
					&cli.BoolFlag{Name: "dry-run"},
					&cli.StringFlag{Name: "release"},
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
						Name:   "bundle",
						Hidden: true,
					},
					&cli.StringFlag{
						Name:   "mcp-inventory",
						Hidden: true,
					},
					&cli.StringSliceFlag{
						Name:   "tailnet-forward",
						Hidden: true,
					},
					&cli.StringFlag{
						Name:   "issue-pin-context",
						Hidden: true,
					},
					&cli.StringFlag{
						Name:   "issue-pin-digest",
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
					&cli.StringFlag{
						Name:   "bundle",
						Hidden: true,
					},
					&cli.StringFlag{
						Name:   "issue-pin-context",
						Hidden: true,
					},
					&cli.StringFlag{
						Name:   "issue-pin-digest",
						Hidden: true,
					},
				},
				Action: runContainerContextBundle,
			},
		},
	}
}

// The legacy subcommand honors none of these. --composed is absent on purpose:
// it is a documented no-op rather than a dropped capability.
var acomposeRefusedFlags = []string{"warded", "guarded", "agent"}

func refuseIntegratedFlags(cmd *cli.Command) error {
	ignored := make([]string, 0, len(acomposeRefusedFlags))
	for _, name := range acomposeRefusedFlags {
		if cmd.IsSet(name) {
			ignored = append(ignored, "--"+name)
		}
	}
	if len(ignored) == 0 {
		return nil
	}
	return fmt.Errorf(
		"acompose does not honor %s: it launches a standalone AOS container "+
			"with no Ward lifecycle and no broker boundary.\n"+
			"For the integrated launch, use the root action:\n"+
			"  aos --warded --guarded --composed --role <role> --agent <agent>\n"+
			"For the standalone container, keep the documented form:\n"+
			"  aos --role <role> acompose -- <harness>",
		strings.Join(ignored, " "),
	)
}

func runAcompose(ctx context.Context, cmd *cli.Command) error {
	// Before validateLegacyDensity and before any fleet work: the point is to
	// stop ahead of materialization, not to report after it. agentic-os#810.
	if err := refuseIntegratedFlags(cmd); err != nil {
		return err
	}
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
) (returnErr error) {
	uid, gid := hostIdentity()
	auth, err := authForLaunch(ctx, cmd.Bool("auth"), layout)
	if err != nil {
		return err
	}
	defer func() {
		returnErr = errors.Join(returnErr, auth.Close())
	}()
	workspace, err := prepareStandaloneWorkspace(layout)
	if err != nil {
		return err
	}
	var issuePins issuePinLaunchContext
	if !cmd.Bool("dry-run") {
		issuePins, err = prepareIssuePinLaunchContext(ctx, role)
		if err != nil {
			return err
		}
		defer func() {
			returnErr = errors.Join(returnErr, issuePins.Close())
		}()
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
		CWD:             workspace.CWD,
		WorkspaceSource: workspace.Source,
		HomeSource:      workspace.HomeSource,
		Command:         command,
		UID:             uid,
		GID:             gid,
		TTY:             isTerminal(os.Stdin),
		NoSubstrate:     noSubstrate,
		AuthMounts:      auth.Mounts,
		ForwardedEnvs:   forwardedEnvironment(cmd.Bool("auth")),
		Guarded:         true,
		Kubeconfig:      cmd.String("kubeconfig"),
		MCPInventory:    mcp.Inventory,
		TailnetNetwork:  mcp.TailnetNetwork,
		TailnetForwards: mcp.Forwards,
		IssuePinContext: issuePins,
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
		IssuePinContext: strings.TrimSpace(cmd.String("issue-pin-context")),
		IssuePinDigest:  strings.TrimSpace(cmd.String("issue-pin-digest")),
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
