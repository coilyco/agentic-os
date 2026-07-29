package main

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	defaultSubstrateManifest = "/opt/agentic-os/substrate-repos.txt"
	defaultSubstrateSeed     = "/opt/substrate-seed"
	defaultSubstrateRoot     = "/substrate"
	defaultAgentHome         = "/home/aos"
	noSubstrateProviderRoot  = "/tmp/aos-provider"
	aosProviderRef           = "coilyco-flight-deck/agentic-os"
	defaultAOSGuardBinary    = "/usr/local/bin/aosguard"
	defaultAOSGuardSkill     = "/opt/agentic-os/aosguard-skill/aosguard"
)

type commandRunner interface {
	Run(context.Context, string, ...string) error
}

type osCommandRunner struct{}

func (osCommandRunner) Run(ctx context.Context, name string, args ...string) error {
	command := exec.CommandContext(ctx, name, args...)
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

type bootstrapOptions struct {
	Role              string
	Layout            string
	Delivery          string
	Composed          bool
	Guarded           bool
	Workspace         string
	UID               int
	GID               int
	Command           []string
	NoSubstrate       bool
	SubstrateManifest string
	SubstrateSeed     string
	SubstrateCache    string
	SubstrateRoot     string
	AgentHome         string
	AgentComposeBin   string
	AOSGuardBinary    string
	AOSGuardSkill     string
	MCPInventory      string
	TailnetForwards   []tailnetForward
}

type execSpec struct {
	Command         []string
	Environment     []string
	TailnetForwards []tailnetForward
}

type substrateRepo struct {
	Owner string
	Name  string
}

func (r substrateRepo) Ref() string {
	return r.Owner + "/" + r.Name
}

func (r substrateRepo) MirrorName() string {
	return r.Owner + "__" + r.Name + ".git"
}

func prepareContainer(
	ctx context.Context,
	opts bootstrapOptions,
	runner commandRunner,
) (execSpec, error) {
	opts = bootstrapDefaults(opts)
	if len(opts.Command) == 0 {
		return execSpec{}, fmt.Errorf("container command must not be empty")
	}
	if !opts.Composed && !opts.Guarded {
		return execSpec{}, fmt.Errorf("container launch needs composed context, guarded tools, or both")
	}
	if opts.UID < 0 || opts.GID < 0 {
		return execSpec{}, fmt.Errorf("uid and gid must be non-negative")
	}
	if err := os.MkdirAll(opts.AgentHome, 0o755); err != nil {
		return execSpec{}, fmt.Errorf("create agent HOME: %w", err)
	}
	if opts.Composed {
		repos, err := loadSubstrateRepos(opts.SubstrateManifest)
		if err != nil {
			return execSpec{}, err
		}
		provider, err := prepareSubstrate(ctx, opts, repos, runner)
		if err != nil {
			return execSpec{}, err
		}
		if err := composeHome(ctx, opts, provider, runner); err != nil {
			return execSpec{}, err
		}
	}
	if opts.Guarded {
		if err := stageAOSGuardContext(opts.Layout, opts.Role, opts.AgentHome, opts.AOSGuardSkill); err != nil {
			return execSpec{}, err
		}
	}
	if err := stageHarnessDefaults(opts.Role, opts.Layout, opts.AgentHome, opts.Workspace); err != nil {
		return execSpec{}, err
	}
	if err := stageMCPProjection(ctx, opts, runner); err != nil {
		return execSpec{}, err
	}
	if err := stageHarnessAuth(opts.Layout, opts.AgentHome); err != nil {
		return execSpec{}, err
	}
	if err := chownTree(opts.AgentHome, opts.UID, opts.GID); err != nil {
		return execSpec{}, fmt.Errorf("hand off agent HOME: %w", err)
	}
	if opts.Composed && !opts.NoSubstrate {
		if err := makeTreeReadOnly(opts.SubstrateRoot); err != nil {
			return execSpec{}, fmt.Errorf("make substrate read-only: %w", err)
		}
	}
	return execSpec{
		Command:         opts.Command,
		TailnetForwards: append([]tailnetForward(nil), opts.TailnetForwards...),
		Environment: environmentWith(map[string]string{
			"HOME":            opts.AgentHome,
			"USER":            "aos",
			"LOGNAME":         "aos",
			"CODEX_HOME":      filepath.Join(opts.AgentHome, ".codex"),
			"XDG_CONFIG_HOME": filepath.Join(opts.AgentHome, ".config"),
		}),
	}, nil
}

func bootstrapDefaults(opts bootstrapOptions) bootstrapOptions {
	if opts.SubstrateManifest == "" {
		opts.SubstrateManifest = defaultSubstrateManifest
	}
	if opts.SubstrateSeed == "" {
		opts.SubstrateSeed = defaultSubstrateSeed
	}
	if opts.SubstrateCache == "" {
		opts.SubstrateCache = containerCacheRoot
	}
	if opts.SubstrateRoot == "" {
		opts.SubstrateRoot = defaultSubstrateRoot
	}
	if opts.AgentHome == "" {
		opts.AgentHome = defaultAgentHome
	}
	if opts.AgentComposeBin == "" {
		opts.AgentComposeBin = "agent-compose"
	}
	if opts.AOSGuardBinary == "" {
		opts.AOSGuardBinary = defaultAOSGuardBinary
	}
	if opts.AOSGuardSkill == "" {
		opts.AOSGuardSkill = defaultAOSGuardSkill
	}
	return opts
}

func selectedContextLayout(layout string) (instruction, skills string, err error) {
	switch strings.TrimSpace(layout) {
	case "claude":
		return ".claude/CLAUDE.md", ".claude/skills", nil
	case "codex":
		return ".codex/AGENTS.md", ".agents/skills", nil
	case "goose":
		return ".config/goose/.goosehints", ".agents/skills", nil
	case "opencode":
		return ".config/opencode/AGENTS.md", ".agents/skills", nil
	default:
		return "", "", fmt.Errorf("AOS has no staged-home layout for agent %q", layout)
	}
}

func stageAOSGuardContext(layout, role, home, source string) error {
	instruction, skills, err := selectedContextLayout(layout)
	if err != nil {
		return err
	}
	instructionPath := filepath.Join(home, filepath.FromSlash(instruction))
	if _, err := os.Stat(instructionPath); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(instructionPath), 0o755); err != nil {
			return fmt.Errorf("create guarded instruction directory: %w", err)
		}
		body := "# AOS guarded launch context\n\n" +
			"AOS attached the `aosguard` operator tool and its generated skill. " +
			"The selected role slug is `" + role + "`. " +
			"The role slug selects context only and grants no authority.\n"
		if err := os.WriteFile(instructionPath, []byte(body), 0o644); err != nil {
			return fmt.Errorf("write guarded instruction: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("inspect selected instruction file: %w", err)
	}

	info, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("inspect generated aosguard skill: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("generated aosguard skill %s is not a directory", source)
	}
	target := filepath.Join(home, filepath.FromSlash(skills), "aosguard")
	if _, err := os.Lstat(target); err == nil {
		return fmt.Errorf("refusing to replace composed skill at %s", target)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect aosguard skill target: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create selected skill root: %w", err)
	}
	if err := os.CopyFS(target, os.DirFS(source)); err != nil {
		return fmt.Errorf("stage generated aosguard skill: %w", err)
	}
	return nil
}

func loadSubstrateRepos(path string) ([]substrateRepo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read substrate manifest %s: %w", path, err)
	}
	var repos []substrateRepo
	seen := map[string]bool{}
	for index, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 1 {
			return nil, fmt.Errorf("substrate manifest line %d: want owner/name", index+1)
		}
		parts := strings.Split(fields[0], "/")
		if len(parts) != 2 || !safePathSegment(parts[0]) || !safePathSegment(parts[1]) {
			return nil, fmt.Errorf("substrate manifest line %d: %q is not a safe owner/name", index+1, fields[0])
		}
		if seen[fields[0]] {
			return nil, fmt.Errorf("substrate manifest line %d: duplicate %s", index+1, fields[0])
		}
		seen[fields[0]] = true
		repos = append(repos, substrateRepo{Owner: parts[0], Name: parts[1]})
	}
	if len(repos) == 0 {
		return nil, fmt.Errorf("substrate manifest %s is empty", path)
	}
	return repos, nil
}

func safePathSegment(value string) bool {
	if value == "" || value == "." || value == ".." {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			strings.ContainsRune("._-", r) {
			continue
		}
		return false
	}
	return true
}

func prepareSubstrate(
	ctx context.Context,
	opts bootstrapOptions,
	repos []substrateRepo,
	runner commandRunner,
) (string, error) {
	if err := os.MkdirAll(opts.SubstrateCache, 0o755); err != nil {
		return "", fmt.Errorf("create substrate cache: %w", err)
	}
	provider := ""
	for _, repo := range repos {
		destination := filepath.Join(opts.SubstrateRoot, repo.Owner, repo.Name)
		if opts.NoSubstrate && repo.Ref() != aosProviderRef {
			continue
		}
		if opts.NoSubstrate {
			destination = noSubstrateProviderRoot
		}
		if err := materializeSubstrateRepo(ctx, opts, repo, destination, runner); err != nil {
			return "", err
		}
		if repo.Ref() == aosProviderRef {
			provider = destination
		}
	}
	if provider == "" {
		return "", fmt.Errorf("substrate manifest does not contain required provider %s", aosProviderRef)
	}
	return provider, nil
}

func materializeSubstrateRepo(
	ctx context.Context,
	opts bootstrapOptions,
	repo substrateRepo,
	destination string,
	runner commandRunner,
) error {
	seed := filepath.Join(opts.SubstrateSeed, repo.MirrorName())
	if info, err := os.Stat(seed); err != nil || !info.IsDir() {
		return fmt.Errorf("substrate seed for %s is absent at %s", repo.Ref(), seed)
	}
	mirror := filepath.Join(opts.SubstrateCache, repo.MirrorName())
	if info, err := os.Stat(mirror); err != nil || !info.IsDir() {
		if err := runner.Run(ctx, "git", "clone", "--mirror", seed, mirror); err != nil {
			return fmt.Errorf("seed substrate mirror %s: %w", repo.Ref(), err)
		}
	} else {
		if err := runner.Run(ctx, "git", "-C", mirror, "remote", "set-url", "origin", seed); err != nil {
			return fmt.Errorf("repoint substrate mirror %s: %w", repo.Ref(), err)
		}
		if err := runner.Run(ctx, "git", "-C", mirror, "remote", "update", "--prune"); err != nil {
			return fmt.Errorf("refresh substrate mirror %s from image seed: %w", repo.Ref(), err)
		}
	}
	if err := os.RemoveAll(destination); err != nil {
		return fmt.Errorf("replace substrate checkout %s: %w", repo.Ref(), err)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return fmt.Errorf("create substrate owner directory %s: %w", repo.Owner, err)
	}
	if err := runner.Run(ctx, "git", "clone", "--no-hardlinks", mirror, destination); err != nil {
		return fmt.Errorf("materialize substrate checkout %s: %w", repo.Ref(), err)
	}
	if err := runner.Run(ctx, "git", "-C", destination, "remote", "set-url", "--push", "origin", "no-push://substrate"); err != nil {
		return fmt.Errorf("disable substrate push %s: %w", repo.Ref(), err)
	}
	return nil
}

func composeHome(
	ctx context.Context,
	opts bootstrapOptions,
	provider string,
	runner commandRunner,
) error {
	modelClass, err := modelClassForLayout(opts.Layout)
	if err != nil {
		return err
	}
	request, err := os.CreateTemp(provider, ".aos-compose-*.kdl")
	if err != nil {
		return fmt.Errorf("create compose request: %w", err)
	}
	requestPath := request.Name()
	defer os.Remove(requestPath)
	body := "compose {\n" +
		"    role " + strconv.Quote(opts.Role) + "\n" +
		"    delivery " + strconv.Quote(opts.Delivery) + "\n" +
		"    model-class " + strconv.Quote(modelClass) + "\n" +
		"    source \"aos-public\" root=\".\" required=#true\n" +
		"}\n"
	if _, err := io.WriteString(request, body); err != nil {
		request.Close()
		return fmt.Errorf("write compose request: %w", err)
	}
	if err := request.Close(); err != nil {
		return fmt.Errorf("close compose request: %w", err)
	}
	bundles, err := os.MkdirTemp("", "aos-bundles-")
	if err != nil {
		return fmt.Errorf("create bundle cache: %w", err)
	}
	defer os.RemoveAll(bundles)
	if err := runner.Run(ctx, opts.AgentComposeBin, "compose", requestPath, "--out", bundles); err != nil {
		return fmt.Errorf("compose role %s: %w", opts.Role, err)
	}
	bundle, err := findSingleBundle(bundles)
	if err != nil {
		return err
	}
	if err := runner.Run(ctx, opts.AgentComposeBin, "verify", bundle); err != nil {
		return fmt.Errorf("verify composed role %s: %w", opts.Role, err)
	}
	if err := runner.Run(
		ctx,
		opts.AgentComposeBin,
		"project",
		bundle,
		"--layout",
		opts.Layout,
		"--scope",
		"home",
		"--target",
		opts.AgentHome,
	); err != nil {
		return fmt.Errorf("project composed role %s: %w", opts.Role, err)
	}
	return nil
}

func findSingleBundle(root string) (string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", fmt.Errorf("read bundle cache: %w", err)
	}
	var bundles []string
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		path := filepath.Join(root, entry.Name())
		if info, err := os.Stat(filepath.Join(path, "manifest.json")); err == nil && info.Mode().IsRegular() {
			bundles = append(bundles, path)
		}
	}
	sort.Strings(bundles)
	if len(bundles) != 1 {
		return "", fmt.Errorf("agent-compose wrote %d verified bundle candidates under %s, want 1", len(bundles), root)
	}
	return bundles[0], nil
}

// stageHarnessDefaults carries container-boundary settings that the selected harness
// cannot infer from projected role context (agentic-os#723, agentic-os#724).
func stageHarnessDefaults(role, layout, home, workspace string) error {
	if layout != "codex" {
		return nil
	}
	if strings.TrimSpace(workspace) == "" {
		return fmt.Errorf("codex workspace must not be empty")
	}
	dir := filepath.Join(home, ".codex")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create codex config directory: %w", err)
	}
	profile, err := standaloneHarnessLaunchProfileFor(role, layout)
	if err != nil {
		return err
	}
	body := "# Written by the AOS container bootstrap: the container is the security boundary.\n" +
		"model = " + tomlBasicString(profile.Model) + "\n" +
		"model_reasoning_effort = " + tomlBasicString(profile.ReasoningEffort) + "\n" +
		"model_verbosity = " + tomlBasicString(profile.Verbosity) + "\n" +
		"approval_policy = \"never\"\n" +
		"sandbox_mode = \"danger-full-access\"\n\n" +
		"[notice]\n" +
		"hide_rate_limit_model_nudge = true\n\n" +
		"# Trust the exact bind-mounted workspace selected by the host launcher.\n" +
		"[projects." + tomlBasicString(workspace) + "]\n" +
		"trust_level = \"trusted\"\n"
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(body), 0o644); err != nil {
		return fmt.Errorf("write codex config: %w", err)
	}
	return nil
}

func tomlBasicString(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return `"` + value + `"`
}

func stageHarnessAuth(layout, home string) error {
	type authCopy struct {
		source string
		target string
	}
	var candidate authCopy
	switch layout {
	case "codex":
		candidate = authCopy{
			source: containerAuthRoot + "/codex.json",
			target: filepath.Join(home, ".codex", "auth.json"),
		}
	case "claude":
		candidate = authCopy{
			source: containerAuthRoot + "/claude.json",
			target: filepath.Join(home, ".claude", ".credentials.json"),
		}
	case "goose":
		candidate = authCopy{
			source: containerAuthRoot + "/goose.yaml",
			target: filepath.Join(home, ".config", "goose", "config.yaml"),
		}
	default:
		return nil
	}
	info, err := os.Stat(candidate.source)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("inspect staged %s auth: %w", layout, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("staged %s auth is not a regular file", layout)
	}
	if err := os.MkdirAll(filepath.Dir(candidate.target), 0o700); err != nil {
		return fmt.Errorf("create %s auth directory: %w", layout, err)
	}
	if err := copyFile(candidate.source, candidate.target, 0o600); err != nil {
		return fmt.Errorf("stage %s auth: %w", layout, err)
	}
	return nil
}

func copyFile(source, target string, mode fs.FileMode) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(output, input); err != nil {
		output.Close()
		return err
	}
	return output.Close()
}

func chownTree(root string, uid, gid int) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		return chownPath(path, entry.Type()&os.ModeSymlink != 0, uid, gid)
	})
}

func makeTreeReadOnly(root string) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		return os.Chmod(path, info.Mode().Perm()&^0o222)
	})
}

func environmentWith(overrides map[string]string) []string {
	values := map[string]string{}
	for _, pair := range os.Environ() {
		key, value, ok := strings.Cut(pair, "=")
		if ok {
			values[key] = value
		}
	}
	for key, value := range overrides {
		values[key] = value
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, key+"="+values[key])
	}
	return result
}
