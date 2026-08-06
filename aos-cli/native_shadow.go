package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/urfave/cli/v3"
)

const (
	nativeSweepInterval         = 10 * time.Minute
	nativeDeadSessionGrace      = 24 * time.Hour
	nativeDeleteScans           = 3
	agentComposeModelTierEnv    = "AGENT_COMPOSE_MODEL_TIER"
	agentComposeModelClassEnv   = "AGENT_COMPOSE_MODEL_CLASS"
	agentComposeRuntimeHomeEnv  = "AGENT_COMPOSE_RUNTIME_HOME"
	claudeDisableAutoUpdaterEnv = "DISABLE_AUTOUPDATER"
)

type nativeArtifact struct {
	Repository string `json:"repository"`
	Worktree   string `json:"worktree"`
	Branch     string `json:"branch"`
}

type nativeLease struct {
	Format          string           `json:"format"`
	ID              string           `json:"id"`
	Harness         string           `json:"harness"`
	PID             int              `json:"pid"`
	ProcessStart    string           `json:"process_start"`
	OriginalCWD     string           `json:"original_cwd"`
	SessionRoot     string           `json:"session_root"`
	SessionProjects string           `json:"session_projects"`
	SessionHome     string           `json:"session_home,omitempty"`
	DeadSince       *time.Time       `json:"dead_since,omitempty"`
	Artifacts       []nativeArtifact `json:"artifacts"`
}

type nativeCandidate struct {
	Fingerprint string `json:"fingerprint"`
	Scans       int    `json:"scans"`
}

type nativeSweepState struct {
	Format     string                     `json:"format"`
	LastSweep  time.Time                  `json:"last_sweep"`
	Candidates map[string]nativeCandidate `json:"candidates"`
}

type nativeRepository struct {
	Owner string
	Name  string
	Path  string
}

type nativeWorktree struct {
	Path   string
	Branch string
}

type nativeLiveWorktrees struct {
	paths     map[string]struct{}
	uncertain bool
}

type nativeRuntime struct {
	Now          time.Time
	PID          int
	ProcessStart string
	CWD          string
	Home         string
	ProjectsRoot string
	StateRoot    string
	SessionsRoot string
	PlanFile     string
	FleetFile    string
	Stderr       *os.File
}

func runNativeShadow(ctx context.Context, cmd *cli.Command) error {
	if cmd.Bool("probe") {
		return nil
	}
	command := argvAfterDash(os.Args)
	if len(command) == 0 {
		return fmt.Errorf("_native-shadow needs a command after `--`")
	}
	harness := strings.TrimSpace(cmd.String("harness"))
	switch harness {
	case "claude", "codex", "goose", "opencode":
	default:
		return fmt.Errorf("_native-shadow has unsupported harness %q", harness)
	}
	runtime, err := resolveNativeRuntime()
	if err != nil {
		return err
	}
	if err := convergeNativeEnvironment(ctx, runtime); err != nil {
		return fmt.Errorf("converge native environment: %w", err)
	}
	launchCWD, err := prepareNativeLaunchWithOptions(runtime, harness, nativeLaunchOptions{
		WorkspaceRoot: cmd.Bool("assigned-role"),
	})
	if err != nil {
		return err
	}
	if err := os.Chdir(launchCWD); err != nil {
		return fmt.Errorf("enter native session workspace %s: %w", launchCWD, err)
	}
	_, isolated := relativeWithin(runtime.SessionsRoot, launchCWD)
	if err := protectNativeHarnessInstall(harness, isolated); err != nil {
		return err
	}
	if isolated && harness == "codex" {
		command = trustNativeCodexWorkspace(command, harness, nativeCodexProject(launchCWD))
	}
	if err := applyNativeModelTier(harness, command); err != nil {
		return err
	}
	if cmd.Bool("assigned-role") {
		if harness == "codex" {
			trusted, err := trustNativeCodexAttributionHook(ctx, launchCWD, runtime.Home)
			if err != nil {
				fmt.Fprintf(
					runtime.Stderr,
					"aos: warning: trust native Codex Git attribution hook: %v\n",
					err,
				)
			} else if trusted > 0 {
				fmt.Fprintf(
					runtime.Stderr,
					"aos: trusted %d native Codex Git attribution hook(s)\n",
					trusted,
				)
			}
		}
	}
	return execNative(command)
}

func convergeNativeEnvironment(ctx context.Context, runtime nativeRuntime) error {
	result, err := convergeEnvironment(ctx, environmentConvergeOptions{
		Home: runtime.Home,
	})
	if err != nil {
		return err
	}
	for _, warning := range result.Warnings {
		fmt.Fprintf(runtime.Stderr, "aos: warning: %s\n", warning)
	}
	return nil
}

func protectNativeHarnessInstall(harness string, isolated bool) error {
	if harness != "claude" || !isolated {
		return nil
	}
	if err := os.Setenv(claudeDisableAutoUpdaterEnv, "1"); err != nil {
		return fmt.Errorf("disable Claude auto-updater in native shadow: %w", err)
	}
	return nil
}

func nativeCodexProject(cwd string) string {
	if root, err := nativeGit(cwd, "rev-parse", "--show-toplevel"); err == nil &&
		filepath.IsAbs(root) {
		cwd = root
	}
	if resolved, err := filepath.EvalSymlinks(cwd); err == nil {
		return resolved
	}
	return filepath.Clean(cwd)
}

func trustNativeCodexWorkspace(command []string, harness, project string) []string {
	if harness != "codex" || len(command) == 0 {
		return command
	}
	override := "projects={" + tomlBasicString(project) + "={trust_level=\"trusted\"}}"
	insert := func(index int) []string {
		trusted := make([]string, 0, len(command)+2)
		trusted = append(trusted, command[:index]...)
		trusted = append(trusted, "--config", override)
		return append(trusted, command[index:]...)
	}
	base := func(value string) string {
		return strings.TrimSuffix(filepath.Base(value), filepath.Ext(value))
	}
	if base(command[0]) == "codex" {
		return insert(1)
	}
	if base(command[0]) != "agent-compose" {
		return command
	}
	if len(command) >= 4 && command[1] == "launch" && command[3] == "codex" {
		return insert(4)
	}
	for index := 1; index+1 < len(command); index++ {
		if command[index] == "--" && base(command[index+1]) == "codex" {
			return insert(index + 2)
		}
	}
	return command
}

func applyNativeModelTier(harness string, command []string) error {
	if err := os.Unsetenv(agentComposeModelClassEnv); err != nil {
		return fmt.Errorf("unset retired agent-compose model class: %w", err)
	}
	model, err := nativeRuntimeModel(harness, command)
	if err != nil {
		return err
	}
	modelTier, err := modelTierForModel(model)
	if err != nil {
		return fmt.Errorf("resolve native %s model tier: %w", harness, err)
	}
	if err := os.Setenv(agentComposeModelTierEnv, modelTier); err != nil {
		return fmt.Errorf("set agent-compose model tier: %w", err)
	}
	return nil
}

func resolveNativeRuntime() (nativeRuntime, error) {
	cwd, err := filepath.Abs(".")
	if err != nil {
		return nativeRuntime{}, fmt.Errorf("resolve native launch directory: %w", err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nativeRuntime{}, fmt.Errorf("resolve native home: %w", err)
	}
	projects := strings.TrimSpace(os.Getenv("PROJECTS_ROOT"))
	if projects == "" {
		projects = filepath.Join(home, "projects")
	}
	projects, err = filepath.Abs(projects)
	if err != nil {
		return nativeRuntime{}, fmt.Errorf("resolve projects root: %w", err)
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return nativeRuntime{}, fmt.Errorf("resolve native cache: %w", err)
	}
	stateRoot := strings.TrimSpace(os.Getenv("AOS_NATIVE_STATE_DIR"))
	if stateRoot == "" {
		stateRoot = filepath.Join(cache, "agentic-os", "native-shadow")
	}
	sessionsRoot := strings.TrimSpace(os.Getenv("AOS_NATIVE_SESSIONS_DIR"))
	if sessionsRoot == "" {
		sessionsRoot = aosTempPath("native")
	}
	config := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME"))
	if config == "" {
		config = filepath.Join(home, ".config")
	}
	plan := strings.TrimSpace(os.Getenv("AOS_REPOSITORY_PLAN"))
	if plan == "" {
		plan = repositoryPlanPath(home)
	}
	fleet := strings.TrimSpace(os.Getenv("AOS_FLEET_ORGS"))
	if fleet == "" {
		fleet = filepath.Join(config, "agentic-os", "fleet-orgs.txt")
	}
	processStart, err := processStartIdentity(os.Getpid())
	if err != nil {
		return nativeRuntime{}, fmt.Errorf("identify native launch process: %w", err)
	}
	return nativeRuntime{
		Now:          time.Now().UTC(),
		PID:          os.Getpid(),
		ProcessStart: processStart,
		CWD:          cwd,
		Home:         home,
		ProjectsRoot: projects,
		StateRoot:    stateRoot,
		SessionsRoot: sessionsRoot,
		PlanFile:     plan,
		FleetFile:    fleet,
		Stderr:       os.Stderr,
	}, nil
}

type nativeLaunchOptions struct {
	WorkspaceRoot  bool
	StandaloneHome bool
}

type nativeLaunchWorkspace struct {
	CWD             string
	SessionProjects string
	SessionHome     string
}

func prepareNativeLaunch(runtime nativeRuntime, harness string) (string, error) {
	return prepareNativeLaunchWithOptions(runtime, harness, nativeLaunchOptions{})
}

func prepareNativeLaunchWithOptions(
	runtime nativeRuntime,
	harness string,
	options nativeLaunchOptions,
) (string, error) {
	workspace, err := prepareNativeLaunchWorkspaceWithOptions(runtime, harness, options)
	if err != nil {
		return "", err
	}
	return workspace.CWD, nil
}

func prepareNativeLaunchWorkspaceWithOptions(
	runtime nativeRuntime,
	harness string,
	options nativeLaunchOptions,
) (nativeLaunchWorkspace, error) {
	if err := os.MkdirAll(runtime.StateRoot, 0o700); err != nil {
		return nativeLaunchWorkspace{}, fmt.Errorf("create native state root: %w", err)
	}
	var workspace nativeLaunchWorkspace
	err := withNativeStartupLock(runtime, func() error {
		live, err := cleanDeadNativeSessions(runtime)
		if err != nil {
			return err
		}
		repositories, expected, err := resolveExpectedRepositories(runtime)
		if err != nil {
			return err
		}
		if due, state := nativeSweepDue(runtime); due {
			if err := runNativeWorkspaceSweep(runtime, repositories, expected, live, state); err != nil {
				return err
			}
		}
		workspace, err = createNativeSession(
			runtime,
			harness,
			repositories,
			options,
		)
		return err
	})
	if err != nil {
		return nativeLaunchWorkspace{}, err
	}
	if workspace.CWD == "" {
		workspace.CWD = runtime.CWD
	}
	return workspace, nil
}

func withNativeStartupLock(runtime nativeRuntime, action func() error) error {
	lock := filepath.Join(runtime.StateRoot, "startup.lock")
	for attempt := 0; attempt < 200; attempt++ {
		err := os.Mkdir(lock, 0o700)
		if err == nil {
			defer os.Remove(lock)
			return action()
		}
		if !errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("acquire native startup lock: %w", err)
		}
		if info, statErr := os.Stat(lock); statErr == nil &&
			runtime.Now.Sub(info.ModTime()) > 2*time.Minute {
			_ = os.Remove(lock)
			continue
		}
		time.Sleep(25 * time.Millisecond)
	}
	return fmt.Errorf("native startup cleanup is already running")
}

func nativeStatePath(runtime nativeRuntime, parts ...string) string {
	return filepath.Join(append([]string{runtime.StateRoot}, parts...)...)
}

func nativePathKey(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("native path is empty")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve absolute native path %s: %w", path, err)
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("resolve native path %s: %w", path, err)
	}
	return filepath.Clean(resolved), nil
}

func (live *nativeLiveWorktrees) add(path string) {
	key, err := nativePathKey(path)
	if err != nil {
		live.uncertain = true
		return
	}
	if live.paths == nil {
		live.paths = map[string]struct{}{}
	}
	live.paths[key] = struct{}{}
}

func (live *nativeLiveWorktrees) addArtifacts(artifacts []nativeArtifact) {
	for _, artifact := range artifacts {
		live.add(artifact.Worktree)
	}
}

func (live nativeLiveWorktrees) contains(path string) bool {
	if live.uncertain {
		return true
	}
	if len(live.paths) == 0 {
		return false
	}
	key, err := nativePathKey(path)
	if err != nil {
		return true
	}
	_, found := live.paths[key]
	return found
}

func readNativeJSON(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

func writeNativeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".native-state-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempName, path)
}

func nativeGit(directory string, args ...string) (string, error) {
	command := exec.Command("git", append([]string{"-C", directory}, args...)...)
	command.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git -C %s %s: %w: %s",
			directory, strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}

func cleanDeadNativeSessions(runtime nativeRuntime) (nativeLiveWorktrees, error) {
	leaseDir := nativeStatePath(runtime, "leases")
	entries, err := os.ReadDir(leaseDir)
	if errors.Is(err, fs.ErrNotExist) {
		return nativeLiveWorktrees{}, nil
	}
	if err != nil {
		return nativeLiveWorktrees{}, fmt.Errorf("read native leases: %w", err)
	}
	live := nativeLiveWorktrees{}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(leaseDir, entry.Name())
		var lease nativeLease
		if err := readNativeJSON(path, &lease); err != nil {
			live.uncertain = true
			fmt.Fprintf(runtime.Stderr, "aos: preserving unreadable native lease %s\n", path)
			continue
		}
		if nativeLeaseIsLive(lease) {
			if lease.DeadSince != nil {
				lease.DeadSince = nil
				if err := writeNativeJSON(path, lease); err != nil {
					return nativeLiveWorktrees{}, fmt.Errorf("restore live native lease: %w", err)
				}
			}
			live.addArtifacts(lease.Artifacts)
			continue
		}
		if lease.DeadSince == nil {
			deadSince := runtime.Now
			lease.DeadSince = &deadSince
			live.addArtifacts(lease.Artifacts)
			if err := writeNativeJSON(path, lease); err != nil {
				return nativeLiveWorktrees{}, fmt.Errorf("start dead native lease grace: %w", err)
			}
			continue
		}
		if runtime.Now.Before(lease.DeadSince.Add(nativeDeadSessionGrace)) {
			live.addArtifacts(lease.Artifacts)
			continue
		}
		remaining := make([]nativeArtifact, 0, len(lease.Artifacts))
		for _, artifact := range lease.Artifacts {
			cleaned, err := cleanNativeArtifact(artifact)
			if err != nil {
				fmt.Fprintf(runtime.Stderr, "aos: preserving %s: %v\n", artifact.Worktree, err)
				remaining = append(remaining, artifact)
				continue
			}
			if !cleaned {
				remaining = append(remaining, artifact)
			}
		}
		if len(remaining) > 0 {
			lease.PID = 0
			lease.ProcessStart = ""
			lease.Artifacts = remaining
			if err := writeNativeJSON(path, lease); err != nil {
				return nativeLiveWorktrees{}, fmt.Errorf("update preserved native lease: %w", err)
			}
			continue
		}
		_ = os.RemoveAll(lease.SessionRoot)
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nativeLiveWorktrees{}, fmt.Errorf("remove native lease: %w", err)
		}
	}
	return live, nil
}

func nativeLeaseIsLive(lease nativeLease) bool {
	if lease.PID <= 0 || lease.ProcessStart == "" {
		return false
	}
	identity, err := processStartIdentity(lease.PID)
	return err == nil && identity == lease.ProcessStart
}

func cleanNativeArtifact(artifact nativeArtifact) (bool, error) {
	if _, err := os.Stat(artifact.Worktree); errors.Is(err, fs.ErrNotExist) {
		_, _ = nativeGit(artifact.Repository, "worktree", "prune")
		return deleteNativeBranchIfRemote(artifact.Repository, artifact.Branch)
	}
	if humanWorkdir(artifact.Worktree) {
		return false, fmt.Errorf("human workdir is outside automation")
	}
	clean, err := nativeWorktreeClean(artifact.Worktree, false)
	if err != nil || !clean {
		return false, err
	}
	safe, err := nativeHeadIsRemote(artifact.Worktree)
	if err != nil || !safe {
		return false, err
	}
	if _, err := nativeGit(artifact.Repository, "worktree", "remove", artifact.Worktree); err != nil {
		return false, err
	}
	return deleteNativeBranchIfRemote(artifact.Repository, artifact.Branch)
}

func nativeWorktreeClean(path string, includeIgnored bool) (bool, error) {
	status, err := nativeGit(path, "status", "--porcelain=v2", "--untracked-files=all")
	if err != nil || status != "" {
		return false, err
	}
	if !includeIgnored {
		return true, nil
	}
	ignored, err := nativeGit(path, "ls-files", "--others", "--ignored", "--exclude-standard")
	return ignored == "", err
}

func nativeHeadIsRemote(path string) (bool, error) {
	output, err := nativeGit(path, "rev-list", "HEAD", "--not", "--remotes=origin")
	return output == "", err
}

func deleteNativeBranchIfRemote(repository, branch string) (bool, error) {
	if branch == "" || branch == "main" {
		return true, nil
	}
	if _, err := nativeGit(repository, "show-ref", "--verify", "--quiet", "refs/heads/"+branch); err != nil {
		return true, nil
	}
	output, err := nativeGit(repository,
		"rev-list", "refs/heads/"+branch, "--not", "--remotes=origin")
	if err != nil || output != "" {
		return false, err
	}
	if _, err := nativeGit(repository, "branch", "-D", branch); err != nil {
		return false, err
	}
	return true, nil
}

func nativeSweepDue(runtime nativeRuntime) (bool, nativeSweepState) {
	state := nativeSweepState{
		Format:     "agentic-os.native-sweep.v1",
		Candidates: map[string]nativeCandidate{},
	}
	if err := readNativeJSON(nativeStatePath(runtime, "sweep.json"), &state); err != nil {
		return true, state
	}
	if state.Candidates == nil {
		state.Candidates = map[string]nativeCandidate{}
	}
	return runtime.Now.Sub(state.LastSweep) >= nativeSweepInterval, state
}

func runNativeWorkspaceSweep(
	runtime nativeRuntime,
	repositories []nativeRepository,
	expected nativeExpected,
	live nativeLiveWorktrees,
	state nativeSweepState,
) error {
	for _, repository := range repositories {
		if _, err := nativeGit(repository.Path, "fetch", "--prune", "origin"); err != nil {
			fmt.Fprintf(runtime.Stderr, "aos: fetch skipped for %s/%s: %v\n",
				repository.Owner, repository.Name, err)
			continue
		}
		if err := normalizeNativeRepository(runtime, repository, live); err != nil {
			fmt.Fprintf(runtime.Stderr, "aos: normalization skipped for %s: %v\n",
				repository.Path, err)
		}
	}
	next := map[string]nativeCandidate{}
	for _, repository := range scanNativeRepositories(runtime.ProjectsRoot) {
		if expected.matches(repository.Owner, repository.Name) {
			continue
		}
		eligible, fingerprint := unexpectedCloneEligible(runtime, repository, live, expected.FleetOrgs)
		if !eligible {
			continue
		}
		candidate := state.Candidates[repository.Path]
		if candidate.Fingerprint == fingerprint {
			candidate.Scans++
		} else {
			candidate = nativeCandidate{Fingerprint: fingerprint, Scans: 1}
		}
		if candidate.Scans >= nativeDeleteScans {
			if err := os.RemoveAll(repository.Path); err != nil {
				return fmt.Errorf("remove unexpected clone %s: %w", repository.Path, err)
			}
			fmt.Fprintf(runtime.Stderr, "aos: removed unexpected clone %s after three startup scans\n",
				repository.Path)
			continue
		}
		next[repository.Path] = candidate
		fmt.Fprintf(runtime.Stderr, "aos: unexpected clone %s eligible for cleanup (%d/3)\n",
			repository.Path, candidate.Scans)
	}
	state.LastSweep = runtime.Now
	state.Candidates = next
	if err := writeNativeJSON(nativeStatePath(runtime, "sweep.json"), state); err != nil {
		return fmt.Errorf("write native sweep state: %w", err)
	}
	return nil
}

func normalizeNativeRepository(
	runtime nativeRuntime,
	repository nativeRepository,
	live nativeLiveWorktrees,
) error {
	if live.contains(repository.Path) || humanWorkdir(repository.Path) {
		return nil
	}
	if inProgress, err := nativeGitOperationInProgress(repository.Path); err != nil || inProgress {
		return err
	}
	clean, err := nativeWorktreeClean(repository.Path, false)
	if err != nil || !clean {
		return err
	}
	branch, err := nativeGit(repository.Path, "symbolic-ref", "--short", "-q", "HEAD")
	if err != nil {
		return err
	}
	if branch != "main" {
		safe, err := nativeHeadIsRemote(repository.Path)
		if err != nil || !safe {
			return err
		}
		if err := cleanUnleasedNativeWorktrees(runtime, repository, live); err != nil {
			return err
		}
		if _, err := nativeGit(repository.Path, "switch", "main"); err != nil {
			return err
		}
		if _, err := nativeGit(repository.Path, "branch", "-D", branch); err != nil {
			return err
		}
		fmt.Fprintf(runtime.Stderr, "aos: returned %s to main and removed local branch %s\n",
			repository.Path, branch)
	}
	if _, err := nativeGit(repository.Path, "merge", "--ff-only", "origin/main"); err != nil {
		return err
	}
	if branch == "main" {
		return cleanUnleasedNativeWorktrees(runtime, repository, live)
	}
	return nil
}

func cleanUnleasedNativeWorktrees(
	runtime nativeRuntime,
	repository nativeRepository,
	live nativeLiveWorktrees,
) error {
	worktrees, err := listNativeWorktrees(repository.Path)
	if err != nil {
		return err
	}
	for _, worktree := range worktrees {
		if samePath(worktree.Path, repository.Path) || live.contains(worktree.Path) ||
			humanWorkdir(worktree.Path) {
			continue
		}
		artifact := nativeArtifact{
			Repository: repository.Path,
			Worktree:   worktree.Path,
			Branch:     worktree.Branch,
		}
		cleaned, err := cleanNativeArtifact(artifact)
		if err != nil {
			fmt.Fprintf(runtime.Stderr, "aos: preserving worktree %s: %v\n", worktree.Path, err)
			continue
		}
		if cleaned {
			fmt.Fprintf(runtime.Stderr, "aos: removed inactive worktree %s\n", worktree.Path)
		}
	}
	return nil
}

func nativeGitOperationInProgress(path string) (bool, error) {
	for _, name := range []string{
		"MERGE_HEAD", "CHERRY_PICK_HEAD", "REVERT_HEAD", "BISECT_LOG",
		"rebase-apply", "rebase-merge", "sequencer",
	} {
		gitPath, err := nativeGit(path, "rev-parse", "--git-path", name)
		if err != nil {
			return false, err
		}
		if !filepath.IsAbs(gitPath) {
			gitPath = filepath.Join(path, gitPath)
		}
		if _, err := os.Stat(gitPath); err == nil {
			return true, nil
		} else if !errors.Is(err, fs.ErrNotExist) {
			return false, err
		}
	}
	return false, nil
}

func listNativeWorktrees(repository string) ([]nativeWorktree, error) {
	output, err := nativeGit(repository, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	var result []nativeWorktree
	current := nativeWorktree{}
	flush := func() {
		if current.Path != "" {
			result = append(result, current)
		}
		current = nativeWorktree{}
	}
	for _, line := range strings.Split(output, "\n") {
		switch {
		case line == "":
			flush()
		case strings.HasPrefix(line, "worktree "):
			current.Path = strings.TrimPrefix(line, "worktree ")
		case strings.HasPrefix(line, "branch refs/heads/"):
			current.Branch = strings.TrimPrefix(line, "branch refs/heads/")
		}
	}
	flush()
	return result, nil
}

func unexpectedCloneEligible(
	runtime nativeRuntime,
	repository nativeRepository,
	live nativeLiveWorktrees,
	fleetOrgs map[string]bool,
) (bool, string) {
	if !fleetOrgs[repository.Owner] || live.contains(repository.Path) ||
		humanWorkdir(repository.Path) {
		return false, ""
	}
	if _, err := nativeGit(repository.Path, "fetch", "--prune", "origin"); err != nil {
		return false, ""
	}
	origin, err := nativeGit(repository.Path, "remote", "get-url", "origin")
	if err != nil || originOwner(origin) != repository.Owner {
		return false, ""
	}
	branch, err := nativeGit(repository.Path, "symbolic-ref", "--short", "-q", "HEAD")
	if err != nil || branch != "main" {
		return false, ""
	}
	clean, err := nativeWorktreeClean(repository.Path, true)
	if err != nil || !clean {
		return false, ""
	}
	inProgress, err := nativeGitOperationInProgress(repository.Path)
	if err != nil || inProgress {
		return false, ""
	}
	head, err := nativeGit(repository.Path, "rev-parse", "HEAD")
	if err != nil {
		return false, ""
	}
	remoteHead, err := nativeGit(repository.Path, "rev-parse", "origin/main")
	if err != nil || head != remoteHead {
		return false, ""
	}
	worktrees, err := listNativeWorktrees(repository.Path)
	if err != nil || len(worktrees) != 1 || !samePath(worktrees[0].Path, repository.Path) {
		return false, ""
	}
	localOnly, err := nativeGit(repository.Path,
		"rev-list", "--all", "--reflog", "--not", "--remotes=origin")
	if err != nil || localOnly != "" {
		return false, ""
	}
	if _, err := os.Stat(filepath.Join(repository.Path, ".gitmodules")); err == nil {
		return false, ""
	}
	fingerprint := strings.Join([]string{origin, head, branch}, "\x00")
	return true, fingerprint
}

func createNativeSession(
	runtime nativeRuntime,
	harness string,
	repositories []nativeRepository,
	options nativeLaunchOptions,
) (nativeLaunchWorkspace, error) {
	relative, inside := relativeWithin(runtime.ProjectsRoot, runtime.CWD)
	id, err := nativeSessionID(runtime)
	if err != nil {
		return nativeLaunchWorkspace{}, err
	}
	sessionRoot := filepath.Join(runtime.SessionsRoot, id)
	sessionProjects := filepath.Join(sessionRoot, "projects")
	sessionHome := ""
	if options.WorkspaceRoot || options.StandaloneHome {
		sessionHome = filepath.Join(sessionRoot, "home")
		stageHome := stageNativeRoleHome
		if options.StandaloneHome {
			stageHome = stageStandaloneRoleHome
		}
		if err := stageHome(runtime.Home, sessionHome); err != nil {
			_ = os.RemoveAll(sessionRoot)
			return nativeLaunchWorkspace{}, err
		}
	}
	branch := "aos/" + harness + "/" + id
	artifacts := make([]nativeArtifact, 0, len(repositories))
	created := map[string]string{}
	for _, repository := range repositories {
		target := filepath.Join(sessionProjects, repository.Owner, repository.Name)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return nativeLaunchWorkspace{}, err
		}
		if _, err := nativeGit(repository.Path,
			"worktree", "add", "--quiet", "-b", branch, target, "origin/main"); err != nil {
			fmt.Fprintf(runtime.Stderr, "aos: worktree skipped for %s/%s: %v\n",
				repository.Owner, repository.Name, err)
			continue
		}
		artifacts = append(artifacts, nativeArtifact{
			Repository: repository.Path,
			Worktree:   target,
			Branch:     branch,
		})
		created[filepath.Join(repository.Owner, repository.Name)] = target
	}
	if len(artifacts) == 0 {
		if sessionHome == "" {
			_ = os.RemoveAll(sessionRoot)
			return nativeLaunchWorkspace{CWD: runtime.CWD}, nil
		}
		if err := writeNativeJSON(nativeStatePath(runtime, "leases", id+".json"), nativeLease{
			Format:          "agentic-os.native-lease.v1",
			ID:              id,
			Harness:         harness,
			PID:             runtime.PID,
			ProcessStart:    runtime.ProcessStart,
			OriginalCWD:     runtime.CWD,
			SessionRoot:     sessionRoot,
			SessionProjects: sessionProjects,
			SessionHome:     sessionHome,
			Artifacts:       artifacts,
		}); err != nil {
			return nativeLaunchWorkspace{}, fmt.Errorf("write native lease: %w", err)
		}
		if err := os.Setenv(agentComposeRuntimeHomeEnv, sessionHome); err != nil {
			return nativeLaunchWorkspace{}, fmt.Errorf("set agent-compose runtime home: %w", err)
		}
		return nativeLaunchWorkspace{CWD: runtime.CWD, SessionHome: sessionHome}, nil
	}
	lease := nativeLease{
		Format:          "agentic-os.native-lease.v1",
		ID:              id,
		Harness:         harness,
		PID:             runtime.PID,
		ProcessStart:    runtime.ProcessStart,
		OriginalCWD:     runtime.CWD,
		SessionRoot:     sessionRoot,
		SessionProjects: sessionProjects,
		SessionHome:     sessionHome,
		Artifacts:       artifacts,
	}
	if err := writeNativeJSON(nativeStatePath(runtime, "leases", id+".json"), lease); err != nil {
		return nativeLaunchWorkspace{}, fmt.Errorf("write native lease: %w", err)
	}
	if sessionHome != "" {
		if err := os.Setenv(agentComposeRuntimeHomeEnv, sessionHome); err != nil {
			return nativeLaunchWorkspace{}, fmt.Errorf("set agent-compose runtime home: %w", err)
		}
	}
	launch := runtime.CWD
	if options.WorkspaceRoot {
		launch = sessionProjects
	} else if inside {
		launch = sessionProjects
		parts := strings.Split(relative, string(filepath.Separator))
		switch {
		case len(parts) == 1 && parts[0] != ".":
			candidate := filepath.Join(sessionProjects, parts[0])
			if info, err := os.Stat(candidate); err == nil && info.IsDir() {
				launch = candidate
			}
		case len(parts) >= 2:
			if target := created[filepath.Join(parts[0], parts[1])]; target != "" {
				launch = target
				candidate := filepath.Join(append([]string{target}, parts[2:]...)...)
				if info, err := os.Stat(candidate); err == nil && info.IsDir() {
					launch = candidate
				}
			}
		}
	}
	fmt.Fprintf(runtime.Stderr, "aos: native session workspace %s\n", sessionProjects)
	return nativeLaunchWorkspace{
		CWD:             launch,
		SessionProjects: sessionProjects,
		SessionHome:     sessionHome,
	}, nil
}

func stageNativeRoleHome(source, target string) error {
	info, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("inspect native host home %s: %w", source, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("native host home %s is not a directory", source)
	}
	if err := os.MkdirAll(target, 0o700); err != nil {
		return fmt.Errorf("create native role home: %w", err)
	}
	entries, err := os.ReadDir(source)
	if err != nil {
		return fmt.Errorf("read native host home: %w", err)
	}
	filtered := map[string]bool{
		".agents": true,
		".claude": true,
	}
	for _, entry := range entries {
		name := entry.Name()
		if filtered[name] {
			if err := stageNativeRoleConfigDirectory(
				filepath.Join(source, name),
				filepath.Join(target, name),
			); err != nil {
				return err
			}
			continue
		}
		if err := os.Symlink(
			filepath.Join(source, name),
			filepath.Join(target, name),
		); err != nil {
			return fmt.Errorf("link native home entry %s: %w", name, err)
		}
	}
	for _, name := range []string{".agents", ".claude"} {
		skills := filepath.Join(target, name, "skills")
		if err := os.MkdirAll(skills, 0o700); err != nil {
			return fmt.Errorf("create filtered native skill directory %s: %w", skills, err)
		}
	}
	return nil
}

func stageStandaloneRoleHome(source, target string) error {
	info, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("inspect standalone host home %s: %w", source, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("standalone host home %s is not a directory", source)
	}
	if err := os.MkdirAll(target, 0o700); err != nil {
		return fmt.Errorf("create standalone role home: %w", err)
	}
	for _, spec := range []struct {
		name    string
		blocked map[string]bool
	}{
		{name: ".agents", blocked: map[string]bool{"skills": true}},
		{name: ".claude", blocked: map[string]bool{"skills": true, ".credentials.json": true}},
	} {
		if err := copyStandaloneHomeDirectory(
			filepath.Join(source, spec.name),
			filepath.Join(target, spec.name),
			spec.blocked,
		); err != nil {
			return err
		}
	}
	for _, name := range []string{".agents", ".claude"} {
		skills := filepath.Join(target, name, "skills")
		if err := os.MkdirAll(skills, 0o700); err != nil {
			return fmt.Errorf("create standalone skill directory %s: %w", skills, err)
		}
	}
	return nil
}

func copyStandaloneHomeDirectory(source, target string, blocked map[string]bool) error {
	entries, err := os.ReadDir(source)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read standalone home directory %s: %w", source, err)
	}
	if err := os.MkdirAll(target, 0o700); err != nil {
		return fmt.Errorf("create standalone home directory %s: %w", target, err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if blocked[name] || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		sourcePath := filepath.Join(source, name)
		targetPath := filepath.Join(target, name)
		if entry.IsDir() {
			if err := copyStandaloneHomeDirectory(sourcePath, targetPath, nil); err != nil {
				return err
			}
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("inspect standalone home entry %s: %w", sourcePath, err)
		}
		if !info.Mode().IsRegular() {
			continue
		}
		if err := copyFile(sourcePath, targetPath, info.Mode().Perm()); err != nil {
			return fmt.Errorf("copy standalone home entry %s: %w", sourcePath, err)
		}
	}
	return nil
}

func stageNativeRoleConfigDirectory(source, target string) error {
	if err := os.MkdirAll(target, 0o700); err != nil {
		return fmt.Errorf("create filtered native config %s: %w", target, err)
	}
	entries, err := os.ReadDir(source)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read native config %s: %w", source, err)
	}
	for _, entry := range entries {
		if entry.Name() == "skills" {
			continue
		}
		if err := os.Symlink(
			filepath.Join(source, entry.Name()),
			filepath.Join(target, entry.Name()),
		); err != nil {
			return fmt.Errorf("link native config entry %s: %w", entry.Name(), err)
		}
	}
	return nil
}

func nativeSessionID(runtime nativeRuntime) (string, error) {
	random := make([]byte, 4)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("generate native session id: %w", err)
	}
	return fmt.Sprintf("%s-%d-%s",
		runtime.Now.UTC().Format("20060102t150405z"),
		runtime.PID,
		hex.EncodeToString(random),
	), nil
}

type nativeExpected struct {
	Full      map[string]bool
	FleetOrgs map[string]bool
}

func (expected nativeExpected) matches(owner, name string) bool {
	return expected.Full[filepath.Join(owner, name)]
}

func resolveExpectedRepositories(
	runtime nativeRuntime,
) ([]nativeRepository, nativeExpected, error) {
	plan, err := loadAOSRepositoryPlan(runtime.PlanFile)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nativeExpected{
				Full:      map[string]bool{},
				FleetOrgs: readNativeListSet(runtime.FleetFile),
			}, nil
		}
		return nil, nativeExpected{}, err
	}
	if plan.ProjectsRoot == "" || !samePath(plan.ProjectsRoot, runtime.ProjectsRoot) {
		return nil, nativeExpected{}, fmt.Errorf("Agent Compose repository plan projects_root %q does not match %s", plan.ProjectsRoot, runtime.ProjectsRoot)
	}
	expected := nativeExpected{
		Full:      map[string]bool{},
		FleetOrgs: readNativeListSet(runtime.FleetFile),
	}
	prior := ""
	for _, entry := range plan.Residency {
		parts := strings.Split(entry.Identity, "/")
		if len(parts) != 2 || !safePathSegment(parts[0]) || !safePathSegment(parts[1]) || entry.Identity <= prior {
			return nil, nativeExpected{}, fmt.Errorf("Agent Compose repository plan has invalid, unsorted, or duplicate residency identity %q", entry.Identity)
		}
		prior = entry.Identity
		expected.Full[filepath.FromSlash(entry.Identity)] = true
	}
	var repositories []nativeRepository
	for _, repository := range scanNativeRepositories(runtime.ProjectsRoot) {
		if expected.matches(repository.Owner, repository.Name) {
			repositories = append(repositories, repository)
		}
	}
	sort.Slice(repositories, func(i, j int) bool {
		return repositories[i].Path < repositories[j].Path
	})
	return repositories, expected, nil
}

func readNativeListSet(path string) map[string]bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]bool{}
	}
	result := map[string]bool{}
	for _, entry := range parseNativeList(data) {
		result[entry] = true
	}
	return result
}

func parseNativeList(data []byte) []string {
	var result []string
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(strings.SplitN(raw, "#", 2)[0])
		if line != "" {
			result = append(result, line)
		}
	}
	return result
}

func scanNativeRepositories(projectsRoot string) []nativeRepository {
	var result []nativeRepository
	owners, err := os.ReadDir(projectsRoot)
	if err != nil {
		return nil
	}
	for _, owner := range owners {
		if !owner.IsDir() || humanWorkdir(owner.Name()) {
			continue
		}
		ownerPath := filepath.Join(projectsRoot, owner.Name())
		repos, err := os.ReadDir(ownerPath)
		if err != nil {
			continue
		}
		for _, repo := range repos {
			if !repo.IsDir() || humanWorkdir(repo.Name()) {
				continue
			}
			path := filepath.Join(ownerPath, repo.Name())
			if _, err := os.Stat(filepath.Join(path, ".git")); err != nil {
				continue
			}
			result = append(result, nativeRepository{
				Owner: owner.Name(),
				Name:  repo.Name(),
				Path:  path,
			})
		}
	}
	return result
}

func originOwner(remote string) string {
	remote = strings.TrimSuffix(strings.TrimSpace(remote), ".git")
	if index := strings.LastIndex(remote, ":"); index >= 0 &&
		!strings.Contains(remote[index+1:], "\\") {
		remote = remote[index+1:]
	}
	remote = strings.TrimSuffix(remote, "/")
	parts := strings.Split(strings.ReplaceAll(remote, "\\", "/"), "/")
	if len(parts) < 2 {
		return ""
	}
	return parts[len(parts)-2]
}

func relativeWithin(root, path string) (string, bool) {
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == ".." ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", false
	}
	return relative, true
}

func samePath(left, right string) bool {
	leftInfo, leftStatErr := os.Stat(left)
	rightInfo, rightStatErr := os.Stat(right)
	if leftStatErr == nil && rightStatErr == nil {
		return os.SameFile(leftInfo, rightInfo)
	}
	leftAbs, leftErr := filepath.Abs(left)
	rightAbs, rightErr := filepath.Abs(right)
	return leftErr == nil && rightErr == nil && filepath.Clean(leftAbs) == filepath.Clean(rightAbs)
}

func humanWorkdir(path string) bool {
	return strings.HasSuffix(filepath.Base(filepath.Clean(path)), "-workdir")
}
