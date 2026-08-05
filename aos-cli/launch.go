package main

import (
	"bytes"
	"context"
	"encoding/json"
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

type authMount struct {
	HostPath      string
	ContainerPath string
}

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
}

type launchPlan struct {
	DockerArgs []string
	Workspace  string
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

	args := []string{"run", "--rm", "--interactive"}
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

func authMountsForLaunch(enabled bool, layout string) ([]authMount, error) {
	if !enabled {
		return nil, nil
	}
	return discoverAuthMounts(layout)
}

func discoverAuthMounts(layout string) ([]authMount, error) {
	if layout == "codex" {
		return discoverCodexAuthMounts()
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, nil
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
		return nil, nil
	}
	info, err := os.Stat(source)
	if err != nil || !info.Mode().IsRegular() {
		return nil, nil
	}
	return []authMount{{HostPath: source, ContainerPath: target}}, nil
}

func discoverCodexAuthMounts() ([]authMount, error) {
	if codexEnvironmentAuthPresent() {
		return nil, nil
	}
	codexHome := strings.TrimSpace(os.Getenv("CODEX_HOME"))
	if codexHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("codex auth: resolve host home: %w", err)
		}
		codexHome = filepath.Join(home, ".codex")
	}
	absHome, err := filepath.Abs(codexHome)
	if err != nil {
		return nil, fmt.Errorf("codex auth: resolve CODEX_HOME: %w", err)
	}
	source := filepath.Join(absHome, "auth.json")
	info, err := os.Stat(source)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf(
				"codex auth: file-backed credentials not found at %s; AOS cannot project keyring-only credentials (use --auth=false for unauthenticated commands)",
				source,
			)
		}
		return nil, fmt.Errorf("codex auth: credentials at %s are unreadable: %w", source, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("codex auth: unsupported credential source at %s: want a regular auth.json file", source)
	}
	if err := validateCodexAuthFile(source, func(path string) (io.ReadCloser, error) {
		return os.Open(path)
	}); err != nil {
		return nil, err
	}
	return []authMount{{HostPath: source, ContainerPath: containerAuthRoot + "/codex.json"}}, nil
}

func validateCodexAuthFile(path string, openFile func(string) (io.ReadCloser, error)) error {
	input, err := openFile(path)
	if err != nil {
		return fmt.Errorf("codex auth: credentials at %s are unreadable: %w", path, err)
	}
	defer input.Close()

	var payload map[string]json.RawMessage
	decoder := json.NewDecoder(input)
	if err := decoder.Decode(&payload); err != nil || ensureJSONEnd(decoder) != nil ||
		!supportedCodexAuthPayload(payload) {
		return fmt.Errorf(
			"codex auth: unsupported credentials at %s: want a Codex auth.json containing file-backed API-key or token data",
			path,
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
