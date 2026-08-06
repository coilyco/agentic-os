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
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
	Command         []string
	UID             int
	GID             int
	TTY             bool
	NoSubstrate     bool
	AuthMounts      []authMount
	ForwardedEnvs   []string
	Kubeconfig      string
	MCPInventory    string
	TailnetNetwork  string
	TailnetForwards []tailnetForward
	LocalServices   []localServiceRoute
}

type launchPlan struct {
	DockerArgs []string
	Workspace  string
}

type localServiceRoute struct {
	Port int
}

func resolveLayout(explicit, command string) (string, error) {
	if layout := strings.TrimSpace(explicit); layout != "" {
		switch layout {
		case "claude", "codex", "goose", "opencode":
			return layout, nil
		default:
			return "", fmt.Errorf("unknown --layout %q: want claude, codex, goose, or opencode", layout)
		}
	}
	base := filepath.Base(command)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	switch base {
	case "claude", "codex", "goose", "opencode":
		return base, nil
	default:
		return "", fmt.Errorf("cannot infer a harness layout from %q; add --layout", command)
	}
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
	if err := validateLocalServiceRoutes(opts.LocalServices); err != nil {
		return launchPlan{}, err
	}
	kubeconfig, err := resolveKubeconfigMount(opts.Role, opts.Kubeconfig)
	if err != nil {
		return launchPlan{}, err
	}
	cwd, err := filepath.Abs(opts.CWD)
	if err != nil {
		return launchPlan{}, fmt.Errorf("resolve workspace: %w", err)
	}
	name := workspaceNamePattern.ReplaceAllString(filepath.Base(cwd), "-")
	name = strings.Trim(name, "-.")
	if name == "" {
		name = "cwd"
	}
	workspace := containerWorkspaceRoot + "/" + name

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
		"--mount", "type=bind,source="+cwd+",target="+workspace,
	)
	if opts.Composed {
		args = append(
			args,
			"--mount", "type=volume,source="+substrateVolume+",target="+containerCacheRoot,
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
	if opts.TailnetNetwork != "" {
		args = append(
			args,
			"--network", opts.TailnetNetwork,
			"--env", "AOS_TAILNET_SOCKS5="+tailnetSOCKS5URL,
		)
	}
	if len(opts.LocalServices) > 0 {
		urls := make([]string, 0, len(opts.LocalServices))
		for _, service := range opts.LocalServices {
			url := fmt.Sprintf("http://127.0.0.1:%d", service.Port)
			urls = append(urls, url)
			args = append(
				args,
				"--publish", fmt.Sprintf("127.0.0.1:%d:%d", service.Port, service.Port),
			)
		}
		args = append(args, "--env", "AOS_LOCAL_SERVICE_URLS="+strings.Join(urls, ","))
		if len(urls) == 1 {
			args = append(args, "--env", "AOS_LOCAL_SERVICE_URL="+urls[0])
		}
	}
	args = append(args,
		"--tmpfs", defaultAgentHome+":rw,exec,size="+runtimeTmpfsSize,
		"--tmpfs", "/tmp:rw,exec,size="+runtimeTmpfsSize,
		"--workdir", workspace,
		"--env", "AOS_CONTAINER=1",
	)
	for _, key := range opts.ForwardedEnvs {
		args = append(args, "--env", key)
	}
	for _, mount := range opts.AuthMounts {
		args = append(args, "--mount",
			"type=bind,source="+mount.HostPath+",target="+mount.ContainerPath+",readonly")
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

func validateLocalServiceRoutes(services []localServiceRoute) error {
	seen := map[int]bool{}
	for _, service := range services {
		if service.Port < 1 || service.Port > 65535 {
			return fmt.Errorf("local service port %d is outside 1..65535", service.Port)
		}
		if seen[service.Port] {
			return fmt.Errorf("local service port %d is duplicated", service.Port)
		}
		seen[service.Port] = true
	}
	return nil
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
	home, err := os.UserHomeDir()
	if err != nil {
		return authProjection{}, nil
	}
	var source, target string
	switch layout {
	case "claude":
		source = filepath.Join(home, ".claude", ".credentials.json")
		target = containerAuthRoot + "/claude.json"
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
	for _, key := range []string{"CODEX_API_KEY", "CODEX_ACCESS_TOKEN", "OPENAI_API_KEY"} {
		if value, ok := os.LookupEnv(key); ok && strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

func forwardedEnvironment(includeAuth bool) []string {
	keys := []string{
		"GOOSE_PROVIDER",
		"GOOSE_MODEL",
		"OLLAMA_HOST",
	}
	if includeAuth {
		keys = append(keys,
			"ANTHROPIC_API_KEY",
			"CODEX_API_KEY",
			"CODEX_ACCESS_TOKEN",
			"OPENAI_API_KEY",
		)
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
