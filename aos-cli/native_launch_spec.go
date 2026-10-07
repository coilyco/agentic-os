package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Spec mode: agent-compose composes and projects, then hands aos a launch spec,
// and aos owns every harness-specific flag and env. docs/native-harness-config.md
const nativeLaunchSpecEnv = "AOS_LAUNCH_SPEC"

const nativeLaunchSpecFormat = "agent-compose.launch-spec.v1"

type nativeLaunchSpec struct {
	Format       string            `json:"format"`
	Role         string            `json:"role"`
	Harness      string            `json:"harness"`
	ModelTier    string            `json:"model_tier"`
	SeatName     string            `json:"seat_name"`
	BundleDir    string            `json:"bundle_dir"`
	BundleReused bool              `json:"bundle_reused"`
	Projected    int               `json:"projected"`
	Warnings     []string          `json:"warnings"`
	RuntimeHome  string            `json:"runtime_home"`
	EnvSet       map[string]string `json:"env_set"`
	EnvUnset     []string          `json:"env_unset"`
}

func nativeLaunchSpecEnabled() bool {
	return strings.TrimSpace(os.Getenv(nativeLaunchSpecEnv)) == "1"
}

// splitAgentComposeLaunch reads `agent-compose launch <role> <harness> [args]`.
// Everything after the harness, the index-4 splices included, belongs to it.
func splitAgentComposeLaunch(command []string) (string, string, string, []string, bool) {
	if len(command) < 4 || command[1] != "launch" {
		return "", "", "", nil, false
	}
	if strings.TrimSuffix(filepath.Base(command[0]), filepath.Ext(command[0])) != "agent-compose" {
		return "", "", "", nil, false
	}
	return command[0], command[2], command[3], append([]string(nil), command[4:]...), true
}

// composerLaunchEnv is the environment the projection call gets. A launch with a
// runtime home of its own drops a parent seat's markers; the spec hands them back.
func composerLaunchEnv() []string {
	runtimeHome, canonicalHome := os.Getenv(agentComposeRuntimeHomeEnv), os.Getenv(nativeCanonicalHomeEnv)
	if strings.TrimSpace(runtimeHome) == "" || samePath(runtimeHome, canonicalHome) {
		return nil
	}
	dropped := map[string]bool{agentComposeLaunchEnv: true, agentComposeLaunchDepthEnv: true}
	kept := make([]string, 0, len(os.Environ()))
	for _, pair := range os.Environ() {
		if name, _, _ := strings.Cut(pair, "="); !dropped[name] {
			kept = append(kept, pair)
		}
	}
	return kept
}

// resolveSpecLaunch runs agent-compose up to the exec and returns the harness
// argv aos will exec itself, with the process environment already applied.
func resolveSpecLaunch(ctx context.Context, command []string) ([]string, error) {
	composer, role, harness, args, ok := splitAgentComposeLaunch(command)
	if !ok {
		return nil, fmt.Errorf("%s=1 needs `agent-compose launch <role> <harness>`, got %q",
			nativeLaunchSpecEnv, strings.Join(command, " "))
	}
	dir, err := os.MkdirTemp("", "aos-launch-spec-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	specPath := filepath.Join(dir, "spec.json")
	composeRun := exec.CommandContext(ctx, composer, "launch", "--spec-out", specPath, role, harness)
	composeRun.Env = composerLaunchEnv()
	composeRun.Stdin, composeRun.Stdout, composeRun.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := composeRun.Run(); err != nil {
		return nil, fmt.Errorf("agent-compose launch --spec-out: %w", err)
	}
	spec, err := readNativeLaunchSpec(specPath)
	if err != nil {
		return nil, err
	}
	if spec.Harness != harness {
		return nil, fmt.Errorf("launch spec is for harness %q, not %q", spec.Harness, harness)
	}
	// The spec env rewrites HOME, and the MCP inventory lives in the host's.
	inventoryHome := strings.TrimSpace(os.Getenv(nativeCanonicalHomeEnv))
	if inventoryHome == "" {
		if inventoryHome, err = os.UserHomeDir(); err != nil {
			return nil, err
		}
	}
	if err := applyNativeLaunchSpecEnvironment(spec); err != nil {
		return nil, err
	}
	identity, err := nativeSpecIdentityArgs(ctx, spec, inventoryHome, args)
	if err != nil {
		return nil, err
	}
	// goose parses the scope only after its session verb.
	if harness == "goose" && len(identity) > 0 {
		return gooseCommand(harness, args, identity), nil
	}
	return append(append([]string{harness}, identity...), args...), nil
}

func readNativeLaunchSpec(path string) (nativeLaunchSpec, error) {
	var spec nativeLaunchSpec
	raw, err := os.ReadFile(path)
	if err != nil {
		return spec, fmt.Errorf("read launch spec: %w", err)
	}
	if err := json.Unmarshal(raw, &spec); err != nil {
		return spec, fmt.Errorf("parse launch spec: %w", err)
	}
	if spec.Format != nativeLaunchSpecFormat {
		return spec, fmt.Errorf("launch spec format %q, want %q", spec.Format, nativeLaunchSpecFormat)
	}
	return spec, nil
}

// applyNativeLaunchSpecEnvironment mirrors what agent-compose set before its
// own exec: the spec's env, then the runtime home the harness reads.
func applyNativeLaunchSpecEnvironment(spec nativeLaunchSpec) error {
	for _, name := range spec.EnvUnset {
		if err := os.Unsetenv(name); err != nil {
			return err
		}
	}
	for name, value := range spec.EnvSet {
		if err := os.Setenv(name, value); err != nil {
			return err
		}
	}
	if strings.TrimSpace(spec.RuntimeHome) == "" {
		return nil
	}
	home, err := filepath.Abs(spec.RuntimeHome)
	if err != nil {
		return err
	}
	if info, err := os.Stat(home); err != nil || !info.IsDir() {
		return fmt.Errorf("launch spec runtime home %s is not a directory", home)
	}
	codexHome := filepath.Join(home, ".codex")
	if spec.Harness == "codex" {
		if resolved, err := filepath.EvalSymlinks(codexHome); err == nil {
			codexHome = resolved
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("resolve native Codex home %s: %w", codexHome, err)
		}
	}
	environment := map[string]string{
		"HOME":            home,
		"USERPROFILE":     home,
		"CODEX_HOME":      codexHome,
		"XDG_CONFIG_HOME": filepath.Join(home, ".config"),
	}
	if spec.Harness == "claude" {
		environment["CLAUDE_CONFIG_DIR"] = filepath.Join(home, ".claude")
		// The shared seats directory, when the workspace staged one. The
		// handoff variable is agent-compose's and stops here, as it does there.
		if shared := strings.TrimSpace(os.Getenv(agentComposeClaudeConfigDirEnv)); shared != "" {
			environment["CLAUDE_CONFIG_DIR"] = shared
		}
		if err := os.Unsetenv(agentComposeClaudeConfigDirEnv); err != nil {
			return err
		}
	}
	for name, value := range environment {
		if err := os.Setenv(name, value); err != nil {
			return err
		}
	}
	return nil
}

// nativeSpecIdentityArgs is agent-compose's nativeIdentityArgs, in its order:
// MCP scope, then --name, then --settings. A flag the caller already passed wins.
func nativeSpecIdentityArgs(ctx context.Context, spec nativeLaunchSpec, inventoryHome string, args []string) ([]string, error) {
	switch spec.Harness {
	case "claude", "codex", "goose", "opencode":
	default:
		return nil, nil
	}
	role, err := nativeBundleRole(spec.BundleDir)
	if err != nil {
		return nil, err
	}
	flags, err := nativeSpecMCPArgs(ctx, spec, role, inventoryHome, nativeSpecStateDir(spec), args)
	if err != nil || spec.Harness != "claude" {
		return flags, err
	}
	if spec.SeatName != "" && !nativeSpecArgsCarry(args, "--name") {
		flags = append(flags, "--name", spec.SeatName)
	}
	if !nativeSpecArgsCarry(args, "--settings") {
		settings, err := writeNativeClaudeSettings(ctx, spec, role)
		if err != nil {
			return nil, err
		}
		flags = append(flags, "--settings", settings)
	}
	return flags, nil
}

// nativeSpecStateDir holds what aos renders for one launch.
func nativeSpecStateDir(spec nativeLaunchSpec) string {
	if strings.TrimSpace(spec.RuntimeHome) != "" {
		return filepath.Join(spec.RuntimeHome, ".aos")
	}
	return spec.BundleDir
}

func nativeSpecArgsCarry(args []string, flag string) bool {
	for _, arg := range args {
		if arg == "--" {
			return false
		}
		if arg == flag || strings.HasPrefix(arg, flag+"=") {
			return true
		}
	}
	return false
}

// writeNativeClaudeSettings renders the role's settings fragment and installs
// its theme where CLAUDE_CONFIG_DIR points, so `custom:aos-<role>` resolves.
func writeNativeClaudeSettings(ctx context.Context, spec nativeLaunchSpec, role string) (string, error) {
	snapshot, err := loadClaudeUISnapshot(ctx)
	if err != nil {
		return "", err
	}
	bundle, err := buildClaudeUI(snapshot, role, "")
	if err != nil {
		return "", err
	}
	root := spec.RuntimeHome
	if strings.TrimSpace(root) == "" {
		root = spec.BundleDir
	}
	claudeDir := filepath.Join(root, ".claude")
	if configured := strings.TrimSpace(os.Getenv("CLAUDE_CONFIG_DIR")); configured != "" {
		claudeDir = configured
	}
	if err := writeClaudeUI(claudeDir, bundle); err != nil {
		return "", err
	}
	return filepath.Join(claudeDir, "settings."+bundle.Role+".json"), nil
}

// nativeBundleRole reads the canonical role from the bundle manifest, since the
// spec carries the role as typed and an alias would miss the snapshot.
func nativeBundleRole(bundleDir string) (string, error) {
	raw, err := os.ReadFile(filepath.Join(bundleDir, "manifest.json"))
	if err != nil {
		return "", fmt.Errorf("read bundle manifest: %w", err)
	}
	var manifest struct {
		Role string `json:"role"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil || manifest.Role == "" {
		return "", fmt.Errorf("bundle manifest %s names no role", bundleDir)
	}
	return manifest.Role, nil
}
