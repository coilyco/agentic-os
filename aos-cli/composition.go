package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/urfave/cli/v3"
)

var hostLookPath = exec.LookPath

type integratedLaunchOptions struct {
	Image       string
	Role        string
	Agent       string
	Layout      string
	Delivery    string
	Composed    bool
	Guarded     bool
	NoSubstrate bool
	Auth        bool
	Kubeconfig  string
	HostNetwork bool
	DryRun      bool
	Arguments   []string
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
		Agent:       strings.TrimSpace(cmd.String("agent")),
		Layout:      strings.TrimSpace(cmd.String("layout")),
		Delivery:    strings.TrimSpace(cmd.String("delivery")),
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

func runStandaloneIntegratedLaunch(
	ctx context.Context,
	cmd *cli.Command,
	opts integratedLaunchOptions,
) (returnErr error) {
	command, pinnedEnv, err := applyRoleLaunchProfile(
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
	if command, err = applyConfiguredPermissionMode(command, opts.Agent); err != nil {
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
		PinnedEnvs:      pinnedEnv,
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

func replaceEnvironment(environment []string, key, value string) []string {
	updated := removeEnvironment(environment, key)
	return append(updated, key+"="+value)
}

func removeEnvironment(environment []string, key string) []string {
	prefix := key + "="
	updated := make([]string, 0, len(environment))
	for _, entry := range environment {
		if strings.HasPrefix(entry, prefix) {
			continue
		}
		updated = append(updated, entry)
	}
	return updated
}
