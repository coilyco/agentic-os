package main

import (
	"context"
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
	Image         string
	Role          string
	Layout        string
	Delivery      string
	CWD           string
	Command       []string
	UID           int
	GID           int
	TTY           bool
	NoSubstrate   bool
	AuthMounts    []authMount
	ForwardedEnvs []string
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
	if opts.UID < 0 || opts.GID < 0 {
		return launchPlan{}, fmt.Errorf("uid and gid must be non-negative")
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
		"--mount", "type=volume,source="+substrateVolume+",target="+containerCacheRoot,
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
	if opts.NoSubstrate {
		args = append(args, "--no-substrate")
	}
	args = append(args,
		"_container-acompose",
		"--workspace", workspace,
		"--uid", fmt.Sprintf("%d", opts.UID),
		"--gid", fmt.Sprintf("%d", opts.GID),
		"--",
	)
	args = append(args, opts.Command...)
	return launchPlan{DockerArgs: args, Workspace: workspace}, nil
}

func discoverAuthMounts(layout string) []authMount {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	var source, target string
	switch layout {
	case "codex":
		source = filepath.Join(home, ".codex", "auth.json")
		target = containerAuthRoot + "/codex.json"
	case "claude":
		source = filepath.Join(home, ".claude", ".credentials.json")
		target = containerAuthRoot + "/claude.json"
	case "goose":
		source = filepath.Join(home, ".config", "goose", "config.yaml")
		target = containerAuthRoot + "/goose.yaml"
	default:
		return nil
	}
	info, err := os.Stat(source)
	if err != nil || !info.Mode().IsRegular() {
		return nil
	}
	return []authMount{{HostPath: source, ContainerPath: target}}
}

func forwardedEnvironment() []string {
	keys := []string{
		"ANTHROPIC_API_KEY",
		"OPENAI_API_KEY",
		"GOOSE_PROVIDER",
		"GOOSE_MODEL",
		"OLLAMA_HOST",
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
