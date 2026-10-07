package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"golang.org/x/term"
)

const (
	containerWorkspaceRoot = "/workspace"
	containerCacheRoot     = "/var/cache/aos/git"
	containerAuthRoot      = "/run/aos/auth"
	containerKubeconfig    = "/run/aos/kubeconfig"
	substrateVolume        = "aos-substrate-cache"
	runtimeTmpfsSize       = "512m"
)

var (
	workspaceNamePattern = regexp.MustCompile(`[^A-Za-z0-9._-]+`)
	lookPath             = exec.LookPath
)

func agentContainerName(role string) (string, error) {
	if !safeRoleSlug(role) {
		return "", fmt.Errorf("role %q is not a safe shared role slug", role)
	}
	random := make([]byte, 4)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("generate agent container suffix: %w", err)
	}
	return role + "-" + hex.EncodeToString(random), nil
}

type authMount struct {
	HostPath      string
	ContainerPath string
}

type authProjection struct {
	Mounts  []authMount
	cleanup func() error
}

func (projection authProjection) Close() error {
	if projection.cleanup == nil {
		return nil
	}
	if err := projection.cleanup(); err != nil {
		return fmt.Errorf("remove temporary auth projection: %w", err)
	}
	return nil
}

type codexKeyringReader func(context.Context, string, string) ([]byte, error)

type launchOptions struct {
	Image           string
	Role            string
	Layout          string
	Delivery        string
	Composed        bool
	Guarded         bool
	CWD             string
	WorkspaceSource string
	HomeSource      string
	Command         []string
	UID             int
	GID             int
	TTY             bool
	NoSubstrate     bool
	AuthMounts      []authMount
	ForwardedEnvs   []string
	// PinnedEnvs ride by value: the host never exported them.
	PinnedEnvs      map[string]string
	Kubeconfig      string
	HostNetwork     bool
	MCPInventory    string
	TailnetNetwork  string
	TailnetForwards []tailnetForward
	IssuePinContext issuePinLaunchContext
}

type launchPlan struct {
	DockerArgs []string
	Workspace  string
}

func resolveLayout(explicit, command string) (string, error) {
	if layout := strings.TrimSpace(explicit); layout != "" {
		if isSupportedHarness(layout) {
			return layout, nil
		}
		return "", fmt.Errorf("unknown --layout %q: want %s", layout, nativeHarnessList())
	}
	base := filepath.Base(command)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	if isSupportedHarness(base) {
		return base, nil
	}
	return "", fmt.Errorf("cannot infer a harness layout from %q; add --layout", command)
}

func buildLaunchPlan(opts launchOptions) (launchPlan, error) {
	if strings.TrimSpace(opts.Image) == "" {
		return launchPlan{}, fmt.Errorf("image must not be empty")
	}
	if strings.TrimSpace(opts.Role) == "" {
		return launchPlan{}, fmt.Errorf("role must not be empty")
	}
	containerName, err := agentContainerName(opts.Role)
	if err != nil {
		return launchPlan{}, err
	}
	if len(opts.Command) == 0 {
		return launchPlan{}, fmt.Errorf("launch command must not be empty")
	}
	if !opts.Composed && !opts.Guarded {
		return launchPlan{}, fmt.Errorf("standalone launch needs --composed, --guarded, or both")
	}
	if opts.UID < 0 || opts.GID < 0 {
		return launchPlan{}, fmt.Errorf("uid and gid must be non-negative")
	}
	if len(opts.TailnetForwards) > 0 && opts.MCPInventory == "" {
		return launchPlan{}, fmt.Errorf("tailnet MCP forwarding needs an MCP inventory")
	}
	if len(opts.TailnetForwards) > 0 && opts.TailnetNetwork == "" {
		return launchPlan{}, fmt.Errorf("tailnet MCP forwarding needs a Docker network")
	}
	if opts.HostNetwork && opts.TailnetNetwork != "" {
		return launchPlan{}, fmt.Errorf("host networking conflicts with Docker network %q", opts.TailnetNetwork)
	}
	kubeconfig, err := resolveKubeconfigMount(opts.Kubeconfig)
	if err != nil {
		return launchPlan{}, err
	}
	cwd, err := filepath.Abs(opts.CWD)
	if err != nil {
		return launchPlan{}, fmt.Errorf("resolve workspace: %w", err)
	}
	mountSource := cwd
	mountTarget := ""
	workspace := ""
	if strings.TrimSpace(opts.WorkspaceSource) != "" {
		source, err := filepath.Abs(opts.WorkspaceSource)
		if err != nil {
			return launchPlan{}, fmt.Errorf("resolve workspace source: %w", err)
		}
		relative, inside := relativeWithin(source, cwd)
		if !inside {
			return launchPlan{}, fmt.Errorf("workspace %s is outside workspace source %s", cwd, source)
		}
		mountSource = source
		mountTarget = containerWorkspaceRoot
		workspace = containerWorkspaceRoot
		if relative != "." {
			workspace = path.Join(containerWorkspaceRoot, filepath.ToSlash(relative))
		}
	} else {
		name := workspaceNamePattern.ReplaceAllString(filepath.Base(cwd), "-")
		name = strings.Trim(name, "-.")
		if name == "" {
			name = "cwd"
		}
		workspace = containerWorkspaceRoot + "/" + name
		mountTarget = workspace
	}

	args := []string{"run", "--rm", "--interactive", "--name", containerName}
	if opts.Image == defaultImage {
		args = append(args, "--pull", "always")
	}
	if opts.TTY {
		args = append(args, "--tty")
	}
	args = append(args,
		"--label", "aos.container=1",
		"--label", "aos.role="+opts.Role,
		"--mount", "type=bind,source="+mountSource+",target="+mountTarget,
	)
	if opts.IssuePinContext.Digest != "" {
		args = append(args, "--label", "aos.issue-pin.digest="+opts.IssuePinContext.Digest)
	}
	if opts.Composed {
		args = append(
			args,
			"--mount", "type=volume,source="+substrateVolume+",target="+containerCacheRoot,
		)
	}
	if strings.TrimSpace(opts.HomeSource) != "" {
		homeSource, err := filepath.Abs(opts.HomeSource)
		if err != nil {
			return launchPlan{}, fmt.Errorf("resolve home source: %w", err)
		}
		info, err := os.Stat(homeSource)
		if err != nil {
			return launchPlan{}, fmt.Errorf("inspect home source: %w", err)
		}
		if !info.IsDir() {
			return launchPlan{}, fmt.Errorf("home source %s is not a directory", homeSource)
		}
		args = append(args,
			"--mount", "type=bind,source="+homeSource+",target="+defaultAgentHome,
		)
	}
	if opts.MCPInventory != "" {
		args = append(
			args,
			"--mount",
			"type=bind,source="+opts.MCPInventory+",target="+containerMCPInventory+",readonly",
		)
	}
	if kubeconfig != "" {
		args = append(
			args,
			"--mount",
			"type=bind,source="+kubeconfig+",target="+containerKubeconfig+",readonly",
			"--env", "KUBECONFIG="+containerKubeconfig,
		)
	}
	if opts.HostNetwork {
		args = append(args, "--network", "host")
	} else if opts.TailnetNetwork != "" {
		args = append(
			args,
			"--network", opts.TailnetNetwork,
			"--env", "AOS_TAILNET_SOCKS5="+tailnetSOCKS5URL,
		)
	}
	if strings.TrimSpace(opts.HomeSource) == "" {
		args = append(args, "--tmpfs", defaultAgentHome+":rw,exec,size="+runtimeTmpfsSize)
	}
	args = append(args,
		"--tmpfs", "/tmp:rw,exec,size="+runtimeTmpfsSize,
		"--workdir", workspace,
		"--env", "AOS_CONTAINER=1",
	)
	for _, key := range opts.ForwardedEnvs {
		if _, pinned := opts.PinnedEnvs[key]; !pinned {
			args = append(args, "--env", key)
		}
	}
	for _, key := range slices.Sorted(maps.Keys(opts.PinnedEnvs)) {
		args = append(args, "--env", key+"="+opts.PinnedEnvs[key])
	}
	for _, mount := range opts.AuthMounts {
		args = append(args, "--mount",
			"type=bind,source="+mount.HostPath+",target="+mount.ContainerPath+",readonly")
	}
	if opts.IssuePinContext.HostPath != "" {
		info, err := os.Stat(opts.IssuePinContext.HostPath)
		if err != nil {
			return launchPlan{}, fmt.Errorf("inspect issue-pin context: %w", err)
		}
		if !info.Mode().IsRegular() {
			return launchPlan{}, fmt.Errorf("issue-pin context %s is not a regular file", opts.IssuePinContext.HostPath)
		}
		args = append(args, "--mount",
			"type=bind,source="+opts.IssuePinContext.HostPath+",target="+containerIssuePinContext+",readonly")
	}
	args = append(args,
		"--entrypoint", "/usr/local/bin/aos",
		opts.Image,
		"--role", opts.Role,
		"--layout", opts.Layout,
		"--delivery", opts.Delivery,
	)
	if opts.Composed {
		args = append(args, "--composed")
	}
	if opts.Guarded {
		args = append(args, "--guarded")
	}
	if opts.NoSubstrate {
		args = append(args, "--no-substrate")
	}
	args = append(args,
		"_container-acompose",
		"--workspace", workspace,
		"--uid", fmt.Sprintf("%d", opts.UID),
		"--gid", fmt.Sprintf("%d", opts.GID),
	)
	if opts.MCPInventory != "" {
		args = append(args, "--mcp-inventory", containerMCPInventory)
	}
	if opts.IssuePinContext.HostPath != "" {
		args = append(
			args,
			"--issue-pin-context", containerIssuePinContext,
			"--issue-pin-digest", opts.IssuePinContext.Digest,
		)
	}
	for _, forward := range opts.TailnetForwards {
		encoded, err := forward.encode()
		if err != nil {
			return launchPlan{}, fmt.Errorf("encode tailnet forward: %w", err)
		}
		args = append(args, "--tailnet-forward", encoded)
	}
	args = append(args, "--")
	args = append(args, opts.Command...)
	return launchPlan{DockerArgs: args, Workspace: workspace}, nil
}

func authForLaunch(ctx context.Context, enabled bool, layout string) (authProjection, error) {
	return authForLaunchWithKeyring(ctx, enabled, layout, readCodexKeyring)
}

func authForLaunchWithKeyring(
	ctx context.Context,
	enabled bool,
	layout string,
	readKeyring codexKeyringReader,
) (authProjection, error) {
	if !enabled {
		return authProjection{}, nil
	}
	return discoverAuthProjection(ctx, layout, readKeyring)
}

func discoverAuthProjection(
	ctx context.Context,
	layout string,
	readKeyring codexKeyringReader,
) (authProjection, error) {
	if layout == "codex" {
		return discoverCodexAuthProjection(ctx, readKeyring)
	}
	if layout == "claude" {
		return discoverClaudeAuthProjection(ctx, readClaudeKeyringSecret)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return authProjection{}, nil
	}
	var source, target string
	switch layout {
	case "goose":
		source = filepath.Join(home, ".config", "goose", "config.yaml")
		target = containerAuthRoot + "/goose.yaml"
	default:
		return authProjection{}, nil
	}
	info, err := os.Stat(source)
	if err != nil || !info.Mode().IsRegular() {
		return authProjection{}, nil
	}
	return authProjection{Mounts: []authMount{{HostPath: source, ContainerPath: target}}}, nil
}

// claudeKeyringReader isolates the platform keyring so the discovery order is
// testable without a real Keychain.
type claudeKeyringReader func(ctx context.Context, service, account string) ([]byte, error)

func readClaudeKeyringSecret(ctx context.Context, service, account string) ([]byte, error) {
	return readClaudeKeyring(ctx, service, account)
}

// Accepting a key and forwarding it are one decision. A key accepted but not
// forwarded projects nothing, so the harness starts logged out.
var (
	claudeEnvironmentAuthKeys = []string{
		"ANTHROPIC_AUTH_TOKEN",
		"CLAUDE_CODE_OAUTH_TOKEN",
	}
	codexEnvironmentAuthKeys = []string{
		"CODEX_API_KEY",
		"CODEX_ACCESS_TOKEN",
		"OPENAI_API_KEY",
	}
)

// withheldAnthropicKeys never reach a harness, because an Anthropic API key is
// for operational checks only. See docs/aos-auth.md.
var withheldAnthropicKeys = []string{"ANTHROPIC_API_KEY", "ANTHROPIC_MODELS_API_KEY"}

// harnessEnvironment is environ without the withheld keys.
func harnessEnvironment(environ []string) []string {
	kept := make([]string, 0, len(environ))
	for _, pair := range environ {
		key, _, _ := strings.Cut(pair, "=")
		if !containsString(withheldAnthropicKeys, key) {
			kept = append(kept, pair)
		}
	}
	return kept
}

func environmentAuthPresent(keys []string) bool {
	for _, key := range keys {
		if value, ok := os.LookupEnv(key); ok && strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

// claudeEnvironmentAuthPresent mirrors the Codex path: a credential already in
// the environment crosses by name, so no file needs projecting.
func claudeEnvironmentAuthPresent() bool {
	return environmentAuthPresent(claudeEnvironmentAuthKeys)
}

// discoverClaudeAuthProjection stages the host login for a container. Claude
// Code keeps it in the Keychain on macOS, so a missing file is not an absence.
func discoverClaudeAuthProjection(
	ctx context.Context,
	readKeyring claudeKeyringReader,
) (authProjection, error) {
	if claudeEnvironmentAuthPresent() {
		return authProjection{}, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return authProjection{}, fmt.Errorf("claude auth: resolve host home: %w", err)
	}
	source := filepath.Join(home, ".claude", ".credentials.json")
	info, err := os.Stat(source)
	if err == nil {
		if !info.Mode().IsRegular() {
			return authProjection{}, fmt.Errorf(
				"claude auth: unsupported credential source at %s: want a regular .credentials.json file",
				source,
			)
		}
		return authProjection{Mounts: []authMount{{
			HostPath: source, ContainerPath: containerAuthRoot + "/claude.json",
		}}}, nil
	}
	if !os.IsNotExist(err) {
		return authProjection{}, fmt.Errorf("claude auth: credentials at %s are unreadable: %w", source, err)
	}

	service := nativeClaudeKeychainService(home, os.Getenv("CLAUDE_CONFIG_DIR"))
	payload, keyringErr := readKeyring(ctx, service, nativeClaudeKeychainAccount())
	if keyringErr == nil {
		if !json.Valid(payload) {
			return authProjection{}, fmt.Errorf(
				"claude auth: macOS Keychain credential for service %s is not valid JSON",
				service,
			)
		}
		return writeTemporaryClaudeAuth(payload)
	}
	if errors.Is(keyringErr, errClaudeKeyringUnsupported) {
		return authProjection{}, fmt.Errorf(
			"claude auth: file-backed credentials not found at %s and host keyring projection is unsupported on this platform (use --auth=false for unauthenticated commands)",
			source,
		)
	}
	if !errors.Is(keyringErr, errClaudeKeyringNotFound) {
		return authProjection{}, fmt.Errorf("claude auth: macOS Keychain credentials are unreadable: %w", keyringErr)
	}
	return authProjection{}, fmt.Errorf(
		"claude auth: credentials were not found at %s or in the macOS Keychain under service %s; run `claude /login` on the host, export CLAUDE_CODE_OAUTH_TOKEN, or use --auth=false for unauthenticated commands",
		source,
		service,
	)
}

func writeTemporaryClaudeAuth(payload []byte) (authProjection, error) {
	directory, err := os.MkdirTemp("", "aos-claude-auth-")
	if err != nil {
		return authProjection{}, fmt.Errorf("claude auth: create private projection directory: %w", err)
	}
	cleanup := func() error { return os.RemoveAll(directory) }
	fail := func(err error) (authProjection, error) {
		_ = cleanup()
		return authProjection{}, err
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return fail(fmt.Errorf("claude auth: secure private projection directory: %w", err))
	}
	path := filepath.Join(directory, "claude.json")
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		return fail(fmt.Errorf("claude auth: write private projection: %w", err))
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fail(fmt.Errorf("claude auth: secure private projection: %w", err))
	}
	return authProjection{
		Mounts:  []authMount{{HostPath: path, ContainerPath: containerAuthRoot + "/claude.json"}},
		cleanup: cleanup,
	}, nil
}

func discoverCodexAuthProjection(
	ctx context.Context,
	readKeyring codexKeyringReader,
) (authProjection, error) {
	if codexEnvironmentAuthPresent() {
		return authProjection{}, nil
	}
	absHome, err := resolveCodexHome()
	if err != nil {
		return authProjection{}, err
	}
	source := filepath.Join(absHome, "auth.json")
	info, err := os.Stat(source)
	if err == nil {
		if !info.Mode().IsRegular() {
			return authProjection{}, fmt.Errorf("codex auth: unsupported credential source at %s: want a regular auth.json file", source)
		}
		if err := validateCodexAuthFile(source, func(path string) (io.ReadCloser, error) {
			return os.Open(path)
		}); err != nil {
			return authProjection{}, err
		}
		return authProjection{Mounts: []authMount{{
			HostPath: source, ContainerPath: containerAuthRoot + "/codex.json",
		}}}, nil
	}
	if !os.IsNotExist(err) {
		return authProjection{}, fmt.Errorf("codex auth: credentials at %s are unreadable: %w", source, err)
	}

	account := codexKeyringAccount(absHome, "cli")
	payload, keyringErr := readKeyring(ctx, codexDirectKeyringService, account)
	if keyringErr == nil {
		if err := validateCodexAuthReader("macOS Keychain", bytes.NewReader(payload)); err != nil {
			return authProjection{}, err
		}
		return writeTemporaryCodexAuth(payload)
	}
	if errors.Is(keyringErr, errCodexKeyringUnsupported) {
		return authProjection{}, fmt.Errorf(
			"codex auth: file-backed credentials not found at %s and host keyring projection is unsupported on this platform (use --auth=false for unauthenticated commands)",
			source,
		)
	}
	if !errors.Is(keyringErr, errCodexKeyringNotFound) {
		return authProjection{}, fmt.Errorf("codex auth: macOS Keychain credentials are unreadable: %w", keyringErr)
	}

	encrypted := filepath.Join(absHome, "secrets", "codex_auth.age")
	if encryptedInfo, encryptedErr := os.Stat(encrypted); encryptedErr == nil && encryptedInfo.Mode().IsRegular() {
		return authProjection{}, fmt.Errorf(
			"codex auth: encrypted keyring credentials at %s are not supported for standalone projection; select Codex direct keyring or file storage",
			encrypted,
		)
	}
	return authProjection{}, fmt.Errorf(
		"codex auth: credentials were not found at %s or in the macOS Keychain (use --auth=false for unauthenticated commands)",
		source,
	)
}

func resolveCodexHome() (string, error) {
	codexHome := strings.TrimSpace(os.Getenv("CODEX_HOME"))
	if codexHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("codex auth: resolve host home: %w", err)
		}
		codexHome = filepath.Join(home, ".codex")
	}
	absHome, err := filepath.Abs(codexHome)
	if err != nil {
		return "", fmt.Errorf("codex auth: resolve CODEX_HOME: %w", err)
	}
	return absHome, nil
}

func codexKeyringAccount(codexHome, prefix string) string {
	canonical := codexHome
	if resolved, err := filepath.EvalSymlinks(codexHome); err == nil {
		canonical = resolved
	}
	digest := sha256.Sum256([]byte(canonical))
	return fmt.Sprintf("%s|%x", prefix, digest[:8])
}

func writeTemporaryCodexAuth(payload []byte) (authProjection, error) {
	directory, err := os.MkdirTemp("", "aos-codex-auth-")
	if err != nil {
		return authProjection{}, fmt.Errorf("codex auth: create private projection directory: %w", err)
	}
	cleanup := func() error { return os.RemoveAll(directory) }
	fail := func(err error) (authProjection, error) {
		_ = cleanup()
		return authProjection{}, err
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return fail(fmt.Errorf("codex auth: secure private projection directory: %w", err))
	}
	path := filepath.Join(directory, "auth.json")
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		return fail(fmt.Errorf("codex auth: write private projection: %w", err))
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fail(fmt.Errorf("codex auth: secure private projection: %w", err))
	}
	return authProjection{
		Mounts:  []authMount{{HostPath: path, ContainerPath: containerAuthRoot + "/codex.json"}},
		cleanup: cleanup,
	}, nil
}

func validateCodexAuthFile(path string, openFile func(string) (io.ReadCloser, error)) error {
	input, err := openFile(path)
	if err != nil {
		return fmt.Errorf("codex auth: credentials at %s are unreadable: %w", path, err)
	}
	defer input.Close()
	return validateCodexAuthReader(path, input)
}

func validateCodexAuthReader(source string, input io.Reader) error {
	var payload map[string]json.RawMessage
	decoder := json.NewDecoder(input)
	if err := decoder.Decode(&payload); err != nil || ensureJSONEnd(decoder) != nil ||
		!supportedCodexAuthPayload(payload) {
		return fmt.Errorf(
			"codex auth: unsupported credentials from %s: want Codex API-key or token data",
			source,
		)
	}
	return nil
}

func supportedCodexAuthPayload(payload map[string]json.RawMessage) bool {
	if raw := payload["OPENAI_API_KEY"]; len(raw) > 0 {
		var value string
		if json.Unmarshal(raw, &value) == nil && strings.TrimSpace(value) != "" {
			return true
		}
	}
	if raw := payload["tokens"]; len(raw) > 0 {
		var tokens map[string]json.RawMessage
		if json.Unmarshal(raw, &tokens) == nil {
			for _, value := range tokens {
				if trimmed := bytes.TrimSpace(value); len(trimmed) > 0 && !bytes.Equal(trimmed, []byte("null")) {
					return true
				}
			}
		}
	}
	return false
}

func codexEnvironmentAuthPresent() bool {
	return environmentAuthPresent(codexEnvironmentAuthKeys)
}

func forwardedEnvironment(includeAuth bool) []string {
	keys := []string{
		"GOOSE_PROVIDER",
		"GOOSE_MODEL",
		"OLLAMA_HOST",
	}
	if includeAuth {
		keys = append(keys, claudeEnvironmentAuthKeys...)
		keys = append(keys, codexEnvironmentAuthKeys...)
	}
	var present []string
	for _, key := range keys {
		if _, ok := os.LookupEnv(key); ok {
			present = append(present, key)
		}
	}
	return present
}

func runDocker(ctx context.Context, args []string) error {
	command := exec.CommandContext(ctx, "docker", args...)
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("docker %s: %w", args[0], err)
	}
	return nil
}

func isTerminal(reader io.Reader) bool {
	file, ok := reader.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(file.Fd()))
}

func shellJoin(args []string) string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		if arg != "" && strings.IndexFunc(arg, func(r rune) bool {
			return !(r >= 'a' && r <= 'z') &&
				!(r >= 'A' && r <= 'Z') &&
				!(r >= '0' && r <= '9') &&
				!strings.ContainsRune("@%_+=:,./-", r)
		}) == -1 {
			quoted = append(quoted, arg)
			continue
		}
		quoted = append(quoted, "'"+strings.ReplaceAll(arg, "'", "'\"'\"'")+"'")
	}
	return strings.Join(quoted, " ")
}
