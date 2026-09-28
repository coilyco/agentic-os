package main

import (
	"bytes"
	"context"
	"errors"
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
	AgentID     string
	Agent       string
	Layout      string
	Delivery    string
	Warded      bool
	Composed    bool
	Guarded     bool
	NoSubstrate bool
	Auth        bool
	Kubeconfig  string
	HostNetwork bool
	DryRun      bool
	Arguments   []string
}

type wardLaunchPlan struct {
	Command     string
	Environment []string
	Args        []string
}

type standaloneWorkspace struct {
	CWD        string
	Source     string
	HomeSource string
}

func runIntegratedLaunch(
	ctx context.Context,
	cmd *cli.Command,
	defaults launchDefaults,
) error {
	if err := validateLegacyDensity(cmd.String("density")); err != nil {
		return err
	}
	opts := integratedLaunchOptions{
		Image:       strings.TrimSpace(cmd.String("image")),
		Role:        strings.TrimSpace(cmd.String("role")),
		AgentID:     strings.TrimSpace(cmd.String("agent-id")),
		Agent:       strings.TrimSpace(cmd.String("agent")),
		Layout:      strings.TrimSpace(cmd.String("layout")),
		Delivery:    strings.TrimSpace(cmd.String("delivery")),
		Warded:      defaults.Warded || cmd.Bool("warded"),
		Composed:    true,
		Guarded:     true,
		NoSubstrate: cmd.Bool("no-substrate"),
		Auth:        cmd.Bool("auth"),
		Kubeconfig:  cmd.String("kubeconfig"),
		HostNetwork: defaults.HostNetwork,
		DryRun:      cmd.Bool("dry-run"),
		Arguments:   cmd.Args().Slice(),
	}
	if defaults.RoleShortcut {
		if err := applyStandaloneRoleShortcut(&opts); err != nil {
			return err
		}
	}
	if opts.Role == "" && opts.Agent == "" && len(opts.Arguments) == 0 {
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

func applyStandaloneRoleShortcut(opts *integratedLaunchOptions) error {
	if opts.Role == "" && len(opts.Arguments) > 0 {
		candidate := strings.TrimSpace(opts.Arguments[0])
		if candidate != "" && !strings.HasPrefix(candidate, "-") {
			opts.Role = candidate
			opts.Arguments = append([]string(nil), opts.Arguments[1:]...)
		}
	}
	if opts.Agent == "" && len(opts.Arguments) > 0 {
		candidate := strings.TrimSpace(opts.Arguments[0])
		if isSupportedHarness(candidate) {
			opts.Agent = candidate
			opts.Arguments = append([]string(nil), opts.Arguments[1:]...)
		}
	}
	if opts.Agent == "" && opts.Role != "" {
		agent, err := standaloneDefaultAgentForRole(opts.Role)
		if err != nil {
			return err
		}
		opts.Agent = agent
	}
	return nil
}

func validateIntegratedLaunch(opts integratedLaunchOptions) error {
	if opts.Role == "" {
		return fmt.Errorf("integrated launch needs --role")
	}
	if !safeRoleSlug(opts.Role) {
		return fmt.Errorf("--role %q is not a safe shared role slug", opts.Role)
	}
	if opts.AgentID != "" && !safeAgentID(opts.AgentID) {
		return fmt.Errorf("--agent-id %q is not a safe peer id", opts.AgentID)
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
		if opts.Kubeconfig != "" {
			return fmt.Errorf(
				"--kubeconfig is available only for standalone launches because Ward owns warded runtime mounts",
			)
		}
		if len(opts.Arguments) == 0 {
			return fmt.Errorf("--warded role %s needs work text", opts.Role)
		}
		if forbidden := forbiddenWardArgument(opts.Arguments); forbidden != "" {
			return fmt.Errorf(
				"launch argument %q conflicts with AOS-owned Ward translation",
				forbidden,
			)
		}
	}
	if opts.AgentID != "" && !opts.Warded {
		return fmt.Errorf("--agent-id needs --warded")
	}
	return nil
}

// nativeHarnesses is every harness AOS starts natively, and the only list of
// them. The shell asks for it instead of restating it. See docs/aterm.md.
var nativeHarnesses = []string{"claude", "codex", "goose", "opencode"}

func isSupportedHarness(value string) bool {
	for _, harness := range nativeHarnesses {
		if harness == value {
			return true
		}
	}
	return false
}

// nativeHarnessList renders the set for an error message, so prose cannot drift
// from the set it describes.
func nativeHarnessList() string {
	if len(nativeHarnesses) < 2 {
		return strings.Join(nativeHarnesses, "")
	}
	head := nativeHarnesses[:len(nativeHarnesses)-1]
	return strings.Join(head, ", ") + ", or " + nativeHarnesses[len(nativeHarnesses)-1]
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

func safeAgentID(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for i, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			if i > 0 || (r != '-' && r != '_' && r != '.') {
				continue
			}
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
		"--role",
		"--agent-id",
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
) (returnErr error) {
	command, err := applyRoleModelProfile(
		ctx,
		append([]string{opts.Agent}, opts.Arguments...),
		opts.Role,
		opts.Agent,
		cmd.Root().ErrWriter,
		listHarnessModels,
	)
	if err != nil {
		return err
	}
	uid, gid := hostIdentity()
	auth, err := authForLaunch(ctx, opts.Auth, opts.Agent)
	if err != nil {
		if !opts.DryRun {
			return err
		}
		// A dry run starts nothing, so an unusable credential is a diagnostic
		// rather than a wall. The rendered plan carries no auth mount.
		fmt.Fprintf(cmd.Root().ErrWriter, "aos: %v; plan omits auth mounts\n", err)
	}
	defer func() {
		returnErr = errors.Join(returnErr, auth.Close())
	}()
	workspace, err := prepareStandaloneWorkspace(opts.Agent)
	if err != nil {
		return err
	}
	var issuePins issuePinLaunchContext
	if !opts.DryRun {
		issuePins, err = prepareIssuePinLaunchContext(ctx, opts.Role)
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
	if opts.HostNetwork {
		mcp.TailnetNetwork = ""
		mcp.Forwards = nil
	}
	plan, err := buildLaunchPlan(launchOptions{
		Image:           opts.Image,
		Role:            opts.Role,
		Layout:          opts.Agent,
		Delivery:        opts.Delivery,
		Composed:        opts.Composed,
		Guarded:         opts.Guarded,
		CWD:             workspace.CWD,
		WorkspaceSource: workspace.Source,
		HomeSource:      workspace.HomeSource,
		Command:         command,
		UID:             uid,
		GID:             gid,
		TTY:             isTerminal(os.Stdin),
		NoSubstrate:     opts.NoSubstrate,
		AuthMounts:      auth.Mounts,
		ForwardedEnvs:   forwardedEnvironment(opts.Auth),
		Kubeconfig:      opts.Kubeconfig,
		HostNetwork:     opts.HostNetwork,
		MCPInventory:    mcp.Inventory,
		TailnetNetwork:  mcp.TailnetNetwork,
		TailnetForwards: mcp.Forwards,
		IssuePinContext: issuePins,
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

func prepareStandaloneWorkspace(harness string) (standaloneWorkspace, error) {
	runtime, err := resolveNativeRuntime()
	if err != nil {
		return standaloneWorkspace{}, err
	}
	workspace, err := prepareNativeLaunchWorkspaceWithOptions(
		runtime,
		harness,
		nativeLaunchOptions{StandaloneHome: true},
	)
	if err != nil {
		return standaloneWorkspace{}, err
	}
	result := standaloneWorkspace{CWD: workspace.CWD}
	if workspace.SessionProjects != "" {
		if _, inside := relativeWithin(workspace.SessionProjects, workspace.CWD); inside {
			result.Source = workspace.SessionProjects
		}
	}
	result.HomeSource = workspace.SessionHome
	return result, nil
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
				Bundle:   "<AGENT_COMPOSE_BUNDLE>",
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
	plan, err := buildWardLaunchPlan(opts, bundlePath)
	if err != nil {
		return err
	}
	if opts.DryRun {
		fmt.Fprintln(
			cmd.Root().Writer,
			shellJoin(append(plan.Environment, append([]string{plan.Command}, plan.Args...)...)),
		)
		return nil
	}
	wardPath, err := hostLookPath("ward")
	if err != nil {
		return fmt.Errorf("--warded needs Ward on the host PATH: %w", err)
	}
	plan.Command = wardPath
	for _, entry := range plan.Environment {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			return fmt.Errorf("invalid Ward launch environment entry %q", entry)
		}
		launchEnvironment = replaceEnvironment(launchEnvironment, key, value)
	}
	return hostCommand(ctx, plan.Command, launchEnvironment, plan.Args...)
}

func buildWardLaunchPlan(opts integratedLaunchOptions, bundlePath string) (wardLaunchPlan, error) {
	args := []string{"agent", "run", "--role", opts.Role}
	if opts.AgentID != "" {
		args = append(args, "--agent-id", opts.AgentID)
	}
	args = append(args, opts.Arguments...)
	args = append(args, "--agent", opts.Agent, "--image", opts.Image)
	if bundlePath != "" {
		args = append(args, "--context-bundle", bundlePath)
	}
	return wardLaunchPlan{Command: "ward", Args: args}, nil
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
