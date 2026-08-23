package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveLayout(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name     string
		explicit string
		command  string
		want     string
	}{
		{name: "codex", command: "codex", want: "codex"},
		{name: "claude path", command: "/usr/local/bin/claude", want: "claude"},
		{name: "windows exe", command: `C:\tools\goose.exe`, explicit: "goose", want: "goose"},
		{name: "explicit", command: "wrapper", explicit: "opencode", want: "opencode"},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := resolveLayout(test.explicit, test.command)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("resolveLayout() = %q, want %q", got, test.want)
			}
		})
	}
	if _, err := resolveLayout("", "bash"); err == nil {
		t.Fatal("unknown command inferred a layout")
	}
}

func TestCodexAuthPipelineUsesCodexHomeAndStagesPrivateFile(t *testing.T) {
	clearCodexAuthEnvironment(t)
	hostHome := filepath.Join(t.TempDir(), "host-codex")
	t.Setenv("CODEX_HOME", hostHome)
	t.Setenv("HOME", filepath.Join(t.TempDir(), "unselected-home"))
	payload := []byte(`{"tokens":{"access_token":"synthetic"}}`)
	hostAuth := filepath.Join(hostHome, "auth.json")
	if err := os.MkdirAll(hostHome, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hostAuth, payload, 0o600); err != nil {
		t.Fatal(err)
	}

	projection, err := authForLaunchWithKeyring(
		context.Background(), true, "codex", unexpectedKeyringRead(t),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer projection.Close()
	if len(projection.Mounts) != 1 || projection.Mounts[0].HostPath != hostAuth ||
		projection.Mounts[0].ContainerPath != containerAuthRoot+"/codex.json" {
		t.Fatalf("Codex auth mounts = %#v", projection.Mounts)
	}
	plan, err := buildLaunchPlan(launchOptions{
		Image: "agentic-os:test", Role: "platform", Layout: "codex",
		Delivery: "native-skills", Composed: true, CWD: t.TempDir(),
		Command: []string{"codex", "exec", "probe"}, UID: 1000, GID: 1000,
		AuthMounts: projection.Mounts,
	})
	if err != nil {
		t.Fatal(err)
	}
	wantMount := "type=bind,source=" + hostAuth + ",target=" + containerAuthRoot + "/codex.json,readonly"
	if !containsArg(plan.DockerArgs, wantMount) {
		t.Fatalf("launch plan omitted read-only Codex auth projection:\n%s", strings.Join(plan.DockerArgs, "\n"))
	}

	authRoot := filepath.Join(t.TempDir(), "container-auth")
	if err := os.MkdirAll(authRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(authRoot, "codex.json"), payload, 0o400); err != nil {
		t.Fatal(err)
	}
	agentHome := filepath.Join(t.TempDir(), "agent-home")
	if err := stageHarnessAuthFromRoot("codex", agentHome, authRoot); err != nil {
		t.Fatal(err)
	}
	staged := filepath.Join(agentHome, ".codex", "auth.json")
	got, err := os.ReadFile(staged)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Fatal("staged Codex auth differs from the synthetic source")
	}
	info, err := os.Stat(staged)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("staged Codex auth mode = %o, want 600", info.Mode().Perm())
	}
}

func TestCodexAuthPipelineProjectsDirectMacOSKeyringPrivately(t *testing.T) {
	clearCodexAuthEnvironment(t)
	codexHome := filepath.Join(t.TempDir(), "codex-home")
	if err := os.MkdirAll(codexHome, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_HOME", codexHome)
	payload := []byte(`{"tokens":{"access_token":"synthetic-keyring-token"}}`)
	canonical, err := filepath.EvalSymlinks(codexHome)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(canonical))
	wantAccount := "cli|" + fmt.Sprintf("%x", digest[:8])
	readKeyring := func(_ context.Context, service, account string) ([]byte, error) {
		if service != codexDirectKeyringService || account != wantAccount {
			t.Fatalf("keyring lookup = %q / %q, want %q / %q", service, account, codexDirectKeyringService, wantAccount)
		}
		return payload, nil
	}

	projection, err := authForLaunchWithKeyring(context.Background(), true, "codex", readKeyring)
	if err != nil {
		t.Fatal(err)
	}
	if len(projection.Mounts) != 1 {
		t.Fatalf("Codex keyring mounts = %#v", projection.Mounts)
	}
	mount := projection.Mounts[0]
	if mount.ContainerPath != containerAuthRoot+"/codex.json" {
		t.Fatalf("Codex keyring container path = %q", mount.ContainerPath)
	}
	info, err := os.Stat(mount.HostPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("temporary Codex auth mode = %o, want 600", info.Mode().Perm())
	}
	directory := filepath.Dir(mount.HostPath)
	directoryInfo, err := os.Stat(directory)
	if err != nil {
		t.Fatal(err)
	}
	if directoryInfo.Mode().Perm() != 0o700 {
		t.Fatalf("temporary Codex auth directory mode = %o, want 700", directoryInfo.Mode().Perm())
	}
	plan, err := buildLaunchPlan(launchOptions{
		Image: "agentic-os:test", Role: "platform", Layout: "codex",
		Delivery: "native-skills", Composed: true, CWD: t.TempDir(),
		Command: []string{"codex", "exec", "probe"}, UID: 1000, GID: 1000,
		AuthMounts: projection.Mounts,
	})
	if err != nil {
		t.Fatal(err)
	}
	wantMount := "type=bind,source=" + mount.HostPath + ",target=" + containerAuthRoot + "/codex.json,readonly"
	if !containsArg(plan.DockerArgs, wantMount) {
		t.Fatalf("launch plan omitted read-only keyring projection:\n%s", strings.Join(plan.DockerArgs, "\n"))
	}
	if err := projection.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(directory); !os.IsNotExist(err) {
		t.Fatalf("temporary Codex auth directory survived cleanup: %v", err)
	}
}

func TestCodexAuthFailsBeforeLaunch(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		clearCodexAuthEnvironment(t)
		codexHome := t.TempDir()
		t.Setenv("CODEX_HOME", codexHome)
		_, err := authForLaunchWithKeyring(
			context.Background(), true, "codex", missingKeyringRead,
		)
		if err == nil || !strings.Contains(err.Error(), "were not found") {
			t.Fatalf("missing Codex auth error = %v", err)
		}
	})

	t.Run("keyring unreadable", func(t *testing.T) {
		clearCodexAuthEnvironment(t)
		t.Setenv("CODEX_HOME", t.TempDir())
		_, err := authForLaunchWithKeyring(
			context.Background(), true, "codex",
			func(context.Context, string, string) ([]byte, error) {
				return nil, errors.New("interaction denied")
			},
		)
		if err == nil || !strings.Contains(err.Error(), "Keychain credentials are unreadable") {
			t.Fatalf("unreadable Codex keyring error = %v", err)
		}
	})

	t.Run("unreadable", func(t *testing.T) {
		err := validateCodexAuthFile("/private/auth.json", func(string) (io.ReadCloser, error) {
			return nil, errors.New("permission denied")
		})
		if err == nil || !strings.Contains(err.Error(), "are unreadable") {
			t.Fatalf("unreadable Codex auth error = %v", err)
		}
	})

	t.Run("unsupported source", func(t *testing.T) {
		clearCodexAuthEnvironment(t)
		codexHome := t.TempDir()
		t.Setenv("CODEX_HOME", codexHome)
		if err := os.Mkdir(filepath.Join(codexHome, "auth.json"), 0o700); err != nil {
			t.Fatal(err)
		}
		_, err := authForLaunchWithKeyring(
			context.Background(), true, "codex", unexpectedKeyringRead(t),
		)
		if err == nil || !strings.Contains(err.Error(), "unsupported credential source") {
			t.Fatalf("unsupported Codex auth source error = %v", err)
		}
	})

	t.Run("unsupported payload", func(t *testing.T) {
		clearCodexAuthEnvironment(t)
		codexHome := t.TempDir()
		t.Setenv("CODEX_HOME", codexHome)
		if err := os.WriteFile(filepath.Join(codexHome, "auth.json"), []byte(`{"unknown":true}`), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := authForLaunchWithKeyring(
			context.Background(), true, "codex", unexpectedKeyringRead(t),
		)
		if err == nil || !strings.Contains(err.Error(), "unsupported credentials") {
			t.Fatalf("unsupported Codex auth payload error = %v", err)
		}
	})

	t.Run("disabled", func(t *testing.T) {
		clearCodexAuthEnvironment(t)
		t.Setenv("CODEX_HOME", t.TempDir())
		projection, err := authForLaunchWithKeyring(
			context.Background(), false, "codex", unexpectedKeyringRead(t),
		)
		if err != nil || len(projection.Mounts) != 0 {
			t.Fatalf("disabled Codex auth mounts = %#v, error = %v", projection.Mounts, err)
		}
	})
}

func TestCodexEnvironmentAuthAndDisabledAuthForwarding(t *testing.T) {
	clearCodexAuthEnvironment(t)
	t.Setenv("CODEX_API_KEY", "synthetic")
	t.Setenv("ANTHROPIC_API_KEY", "synthetic")
	t.Setenv("GOOSE_MODEL", "synthetic-model")

	projection, err := authForLaunchWithKeyring(
		context.Background(), true, "codex", unexpectedKeyringRead(t),
	)
	if err != nil || len(projection.Mounts) != 0 {
		t.Fatalf("Codex environment auth mounts = %#v, error = %v", projection.Mounts, err)
	}
	withAuth := forwardedEnvironment(true)
	for _, want := range []string{"CODEX_API_KEY", "ANTHROPIC_API_KEY", "GOOSE_MODEL"} {
		if !containsArg(withAuth, want) {
			t.Errorf("authenticated environment omitted %s: %v", want, withAuth)
		}
	}
	withoutAuth := forwardedEnvironment(false)
	if containsArg(withoutAuth, "CODEX_API_KEY") || containsArg(withoutAuth, "ANTHROPIC_API_KEY") {
		t.Fatalf("--auth=false forwarded authentication variables: %v", withoutAuth)
	}
	if !containsArg(withoutAuth, "GOOSE_MODEL") {
		t.Fatalf("--auth=false omitted non-secret harness tuning: %v", withoutAuth)
	}
}

// Reading both key sets rather than restating them keeps this an invariant: a
// key added to either one has to cross the boundary or this fails.
func TestEveryAcceptedEnvironmentCredentialCrossesTheBoundary(t *testing.T) {
	accepted := append(
		append([]string(nil), claudeEnvironmentAuthKeys...),
		codexEnvironmentAuthKeys...,
	)
	for _, key := range accepted {
		t.Setenv(key, "synthetic")
	}

	forwarded := forwardedEnvironment(true)
	for _, key := range accepted {
		if !containsArg(forwarded, key) {
			t.Errorf("environment auth accepts %s but never forwards it: %v", key, forwarded)
		}
	}

	unauthenticated := forwardedEnvironment(false)
	for _, key := range accepted {
		if containsArg(unauthenticated, key) {
			t.Errorf("--auth=false forwarded credential %s: %v", key, unauthenticated)
		}
	}
}

func TestBuildLaunchPlanMountsCWDAndRunsInternalCompose(t *testing.T) {
	t.Parallel()
	cwd := filepath.Join(t.TempDir(), "my repo")
	plan, err := buildLaunchPlan(launchOptions{
		Image:         "agentic-os:test",
		Role:          "platform",
		Layout:        "codex",
		Delivery:      "native-skills",
		Composed:      true,
		CWD:           cwd,
		Command:       []string{"codex", "exec", "fix it"},
		UID:           501,
		GID:           20,
		TTY:           true,
		AuthMounts:    []authMount{{HostPath: "/host/auth.json", ContainerPath: "/run/aos/auth/codex.json"}},
		ForwardedEnvs: []string{"OPENAI_API_KEY"},
	})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(plan.DockerArgs, "\n")
	for _, want := range []string{
		"run",
		"--rm",
		"--interactive",
		"--tty",
		"type=bind,source=" + cwd + ",target=/workspace/my-repo",
		"type=volume,source=aos-substrate-cache,target=/var/cache/aos/git",
		"--tmpfs\n" + defaultAgentHome + ":rw,exec,size=" + runtimeTmpfsSize,
		"--tmpfs\n/tmp:rw,exec,size=" + runtimeTmpfsSize,
		"--workdir\n/workspace/my-repo",
		"--env\nAOS_CONTAINER=1",
		"--env\nOPENAI_API_KEY",
		"type=bind,source=/host/auth.json,target=/run/aos/auth/codex.json,readonly",
		"--entrypoint\n/usr/local/bin/aos",
		"agentic-os:test",
		"--role\nplatform",
		"_container-acompose",
		"--workspace\n/workspace/my-repo",
		"--uid\n501",
		"--gid\n20",
		"--\ncodex\nexec\nfix it",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("launch plan missing %q\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "--tmpfs\n/home:") {
		t.Fatalf("launch plan hides the image-owned /home tree:\n%s", joined)
	}
	if strings.Contains(joined, "ward") {
		t.Fatalf("Ward leaked into standalone launch plan:\n%s", joined)
	}
	if containsArg(plan.DockerArgs, "--pull") {
		t.Fatalf("custom image unexpectedly forced a registry pull:\n%s", joined)
	}
}

func TestBuildLaunchPlanMountsWorkspaceSourceAndNestedCWD(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	source := filepath.Join(root, "projects")
	cwd := filepath.Join(source, "owner", "repo", "docs")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	plan, err := buildLaunchPlan(launchOptions{
		Image:           "agentic-os:test",
		Role:            "platform",
		Layout:          "codex",
		Delivery:        "native-skills",
		Composed:        true,
		CWD:             cwd,
		WorkspaceSource: source,
		Command:         []string{"codex"},
		UID:             1000,
		GID:             1000,
	})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(plan.DockerArgs, "\n")
	for _, want := range []string{
		"type=bind,source=" + source + ",target=/workspace",
		"--workdir\n/workspace/owner/repo/docs",
		"--workspace\n/workspace/owner/repo/docs",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("launch plan missing %q:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "source="+cwd) {
		t.Fatalf("launch plan mounted nested CWD directly:\n%s", joined)
	}
}

func TestBuildLaunchPlanMountsHomeSource(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	plan, err := buildLaunchPlan(launchOptions{
		Image:      "agentic-os:test",
		Role:       "platform",
		Layout:     "codex",
		Delivery:   "native-skills",
		Composed:   true,
		CWD:        t.TempDir(),
		HomeSource: home,
		Command:    []string{"codex"},
		UID:        1000,
		GID:        1000,
	})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(plan.DockerArgs, "\n")
	for _, want := range []string{
		"type=bind,source=" + home + ",target=" + defaultAgentHome,
		"--tmpfs\n/tmp:rw,exec,size=" + runtimeTmpfsSize,
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("launch plan missing %q:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "--tmpfs\n"+defaultAgentHome) {
		t.Fatalf("launch plan kept blank HOME tmpfs with a home source:\n%s", joined)
	}
}

func TestBuildLaunchPlanMountsIssuePinContext(t *testing.T) {
	t.Parallel()
	source := filepath.Join(t.TempDir(), "issue-pins.md")
	if err := os.WriteFile(source, []byte("pins\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	digest := strings.Repeat("a", 64)
	plan, err := buildLaunchPlan(launchOptions{
		Image:    "agentic-os:test",
		Role:     "tpm",
		Layout:   "codex",
		Delivery: "native-skills",
		Composed: true,
		CWD:      t.TempDir(),
		Command:  []string{"codex"},
		UID:      1000,
		GID:      1000,
		IssuePinContext: issuePinLaunchContext{
			HostPath: source,
			Digest:   digest,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(plan.DockerArgs, "\n")
	for _, want := range []string{
		"--label\naos.issue-pin.digest=" + digest,
		"type=bind,source=" + source + ",target=" + containerIssuePinContext + ",readonly",
		"--issue-pin-context\n" + containerIssuePinContext,
		"--issue-pin-digest\n" + digest,
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("launch plan missing %q:\n%s", want, joined)
		}
	}
}

func TestBuildLaunchPlanNamesAgentContainerForRole(t *testing.T) {
	t.Parallel()
	plan, err := buildLaunchPlan(launchOptions{
		Image:    "agentic-os:test",
		Role:     "platform",
		Layout:   "codex",
		Delivery: "native-skills",
		Composed: true,
		CWD:      t.TempDir(),
		Command:  []string{"codex"},
		UID:      1000,
		GID:      1000,
	})
	if err != nil {
		t.Fatal(err)
	}
	name, ok := flagValue(plan.DockerArgs, "--name")
	if !ok {
		t.Fatalf("launch plan omitted Docker container name:\n%s", strings.Join(plan.DockerArgs, "\n"))
	}
	suffix, ok := strings.CutPrefix(name, "platform-")
	if !ok {
		t.Fatalf("container name = %q, want role-prefixed name", name)
	}
	if len(suffix) != 8 {
		t.Fatalf("container name suffix = %q, want 8 hex characters", suffix)
	}
	if _, err := hex.DecodeString(suffix); err != nil {
		t.Fatalf("container name suffix = %q, want hex: %v", suffix, err)
	}
}

func TestBuildLaunchPlanUsesHostNetwork(t *testing.T) {
	t.Parallel()
	plan, err := buildLaunchPlan(launchOptions{
		Image:       "agentic-os:test",
		Role:        "platform",
		Layout:      "codex",
		Delivery:    "native-skills",
		Composed:    true,
		CWD:         t.TempDir(),
		Command:     []string{"codex"},
		UID:         1000,
		GID:         1000,
		HostNetwork: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	network, ok := flagValue(plan.DockerArgs, "--network")
	if !ok || network != "host" {
		t.Fatalf("host-network launch plan network = %q, ok = %v:\n%s", network, ok, strings.Join(plan.DockerArgs, "\n"))
	}
}

func TestBuildLaunchPlanRejectsConflictingNetworks(t *testing.T) {
	t.Parallel()
	_, err := buildLaunchPlan(launchOptions{
		Image:          "agentic-os:test",
		Role:           "platform",
		Layout:         "codex",
		Delivery:       "native-skills",
		Composed:       true,
		CWD:            t.TempDir(),
		Command:        []string{"codex"},
		UID:            1000,
		GID:            1000,
		HostNetwork:    true,
		TailnetNetwork: tailnetDockerNetwork,
	})
	if err == nil || !strings.Contains(err.Error(), "host networking conflicts") {
		t.Fatalf("conflicting network error = %v", err)
	}
}

func TestBuildLaunchPlanPullsMovingReleaseImage(t *testing.T) {
	t.Parallel()
	plan, err := buildLaunchPlan(launchOptions{
		Image:    defaultImage,
		Role:     "tpm",
		Layout:   "codex",
		Delivery: "native-skills",
		Composed: true,
		CWD:      t.TempDir(),
		Command:  []string{"codex"},
		UID:      1000,
		GID:      1000,
	})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(plan.DockerArgs, "\n")
	if !strings.Contains(joined, "--pull\nalways") {
		t.Fatalf("moving release image did not force a fresh pull:\n%s", joined)
	}
}

func TestBuildLaunchPlanCanSkipSubstrate(t *testing.T) {
	t.Parallel()
	plan, err := buildLaunchPlan(launchOptions{
		Image:       "agentic-os:test",
		Role:        "tpm",
		Layout:      "claude",
		Delivery:    "compiled",
		Composed:    true,
		CWD:         t.TempDir(),
		Command:     []string{"claude"},
		UID:         1000,
		GID:         1000,
		NoSubstrate: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !containsArg(plan.DockerArgs, "--no-substrate") {
		t.Fatal("launch plan did not forward --no-substrate")
	}
}

func TestBuildLaunchPlanProjectsMCPAndJoinsTailnet(t *testing.T) {
	t.Parallel()
	forward := tailnetForward{
		Server:     "internal",
		TargetHost: "internal.example",
		TargetPort: 30082,
		ListenPort: 39000,
	}
	plan, err := buildLaunchPlan(launchOptions{
		Image:           "agentic-os:test",
		Role:            "platform",
		Layout:          "codex",
		Delivery:        "native-skills",
		Composed:        true,
		CWD:             t.TempDir(),
		Command:         []string{"codex"},
		UID:             1000,
		GID:             1000,
		MCPInventory:    "/host/mcporter.json",
		TailnetNetwork:  tailnetDockerNetwork,
		TailnetForwards: []tailnetForward{forward},
	})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(plan.DockerArgs, "\n")
	for _, want := range []string{
		"type=bind,source=/host/mcporter.json,target=" + containerMCPInventory + ",readonly",
		"--network\n" + tailnetDockerNetwork,
		"--env\nAOS_TAILNET_SOCKS5=" + tailnetSOCKS5URL,
		"_container-acompose",
		"--mcp-inventory\n" + containerMCPInventory,
		"--tailnet-forward",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("launch plan missing %q:\n%s", want, joined)
		}
	}
	encoded, err := forward.encode()
	if err != nil {
		t.Fatal(err)
	}
	if !containsArg(plan.DockerArgs, encoded) {
		t.Fatal("launch plan omitted encoded tailnet forward")
	}
}

func TestBuildLaunchPlanMountsKubeconfigWithTailnet(t *testing.T) {
	t.Parallel()
	kubeconfig := writeTestKubeconfig(
		t,
		filepath.Join(t.TempDir(), "operator config", "cluster config.yaml"),
	)
	plan, err := buildLaunchPlan(launchOptions{
		Image:          "agentic-os:test",
		Role:           "sysadmin",
		Layout:         "codex",
		Delivery:       "native-skills",
		Composed:       true,
		CWD:            t.TempDir(),
		Command:        []string{"codex"},
		UID:            1000,
		GID:            1000,
		Kubeconfig:     kubeconfig,
		MCPInventory:   "/host/mcporter.json",
		TailnetNetwork: tailnetDockerNetwork,
	})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(plan.DockerArgs, "\n")
	for _, want := range []string{
		"type=bind,source=" + kubeconfig + ",target=" + containerKubeconfig + ",readonly",
		"--env\nKUBECONFIG=" + containerKubeconfig,
		"--network\n" + tailnetDockerNetwork,
		"--env\nAOS_TAILNET_SOCKS5=" + tailnetSOCKS5URL,
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("launch plan missing %q:\n%s", want, joined)
		}
	}
}

func TestBuildLaunchPlanRejectsInvalidKubeconfig(t *testing.T) {
	t.Parallel()
	t.Run("missing", func(t *testing.T) {
		t.Parallel()
		_, err := buildLaunchPlan(launchOptions{
			Image: "agentic-os:test", Role: "tpm", Layout: "codex",
			Delivery: "native-skills", Composed: true, CWD: t.TempDir(),
			Command: []string{"codex"}, UID: 1000, GID: 1000,
			Kubeconfig: filepath.Join(t.TempDir(), "missing.yaml"),
		})
		if err == nil || !strings.Contains(err.Error(), "does not exist") {
			t.Fatalf("error = %v, want missing kubeconfig error", err)
		}
	})
	t.Run("malformed", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "malformed.yaml")
		if err := os.WriteFile(path, []byte("apiVersion: [\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := buildLaunchPlan(launchOptions{
			Image: "agentic-os:test", Role: "sysadmin", Layout: "codex",
			Delivery: "native-skills", Composed: true, CWD: t.TempDir(),
			Command: []string{"codex"}, UID: 1000, GID: 1000,
			Kubeconfig: path,
		})
		if err == nil || !strings.Contains(err.Error(), "is malformed") {
			t.Fatalf("error = %v, want malformed kubeconfig error", err)
		}
	})
	t.Run("not regular", func(t *testing.T) {
		t.Parallel()
		path := t.TempDir()
		_, err := buildLaunchPlan(launchOptions{
			Image: "agentic-os:test", Role: "sysadmin", Layout: "codex",
			Delivery: "native-skills", Composed: true, CWD: t.TempDir(),
			Command: []string{"codex"}, UID: 1000, GID: 1000,
			Kubeconfig: path,
		})
		if err == nil || !strings.Contains(err.Error(), "is not a regular file") {
			t.Fatalf("error = %v, want regular-file kubeconfig error", err)
		}
	})
	t.Run("multiple documents", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "multiple.yaml")
		body := "apiVersion: v1\nkind: Config\n---\napiVersion: v1\nkind: Config\n"
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := buildLaunchPlan(launchOptions{
			Image: "agentic-os:test", Role: "sysadmin", Layout: "codex",
			Delivery: "native-skills", Composed: true, CWD: t.TempDir(),
			Command: []string{"codex"}, UID: 1000, GID: 1000,
			Kubeconfig: path,
		})
		if err == nil || !strings.Contains(err.Error(), "is malformed") {
			t.Fatalf("error = %v, want multiple-document kubeconfig error", err)
		}
	})
}

func TestBuildLaunchPlanMountsKubeconfigForEveryRole(t *testing.T) {
	t.Parallel()
	for _, role := range []string{"tpm", "ops", "platform", "eval", "frontend"} {
		role := role
		t.Run(role, func(t *testing.T) {
			t.Parallel()
			source := writeTestKubeconfig(t, filepath.Join(t.TempDir(), "config.yaml"))
			plan, err := buildLaunchPlan(launchOptions{
				Image: "agentic-os:test", Role: role, Layout: "codex",
				Delivery: "native-skills", Composed: true, CWD: t.TempDir(),
				Command: []string{"codex"}, UID: 1000, GID: 1000,
				Kubeconfig: source,
			})
			if err != nil {
				t.Fatal(err)
			}
			joined := strings.Join(plan.DockerArgs, "\n")
			if !strings.Contains(joined, source) || !strings.Contains(joined, "KUBECONFIG") {
				t.Fatalf("%s launch lost its kubeconfig projection:\n%s", role, joined)
			}
		})
	}
}

func TestArgvAfterDash(t *testing.T) {
	t.Parallel()
	got := argvAfterDash([]string{"aos", "--role", "platform", "acompose", "--", "codex", "exec"})
	if strings.Join(got, " ") != "codex exec" {
		t.Fatalf("argvAfterDash() = %q", got)
	}
}

func TestShellJoinDoesNotExposeForwardedEnvironmentValues(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "do-not-print")
	plan, err := buildLaunchPlan(launchOptions{
		Image:         "agentic-os:test",
		Role:          "platform",
		Layout:        "codex",
		Delivery:      "native-skills",
		Composed:      true,
		CWD:           t.TempDir(),
		Command:       []string{"codex"},
		UID:           1000,
		GID:           1000,
		ForwardedEnvs: forwardedEnvironment(true),
	})
	if err != nil {
		t.Fatal(err)
	}
	rendered := shellJoin(append([]string{"docker"}, plan.DockerArgs...))
	if strings.Contains(rendered, "do-not-print") {
		t.Fatal("dry-run rendered an environment value")
	}
	if !strings.Contains(rendered, "--env OPENAI_API_KEY") {
		t.Fatalf("dry-run omitted environment name: %s", rendered)
	}
}

func clearCodexAuthEnvironment(t *testing.T) {
	t.Helper()
	for _, key := range []string{"CODEX_API_KEY", "CODEX_ACCESS_TOKEN", "OPENAI_API_KEY"} {
		t.Setenv(key, "")
	}
}

func missingKeyringRead(context.Context, string, string) ([]byte, error) {
	return nil, errCodexKeyringNotFound
}

func unexpectedKeyringRead(t *testing.T) codexKeyringReader {
	t.Helper()
	return func(context.Context, string, string) ([]byte, error) {
		t.Fatal("unexpected Codex keyring read")
		return nil, nil
	}
}

func TestValidateLegacyDensity(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"", "full", " full "} {
		if err := validateLegacyDensity(value); err != nil {
			t.Fatalf("legacy density %q failed: %v", value, err)
		}
	}
	if err := validateLegacyDensity("brief"); err == nil {
		t.Fatal("retired brief density passed validation")
	}
}

func containsArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func flagValue(args []string, name string) (string, bool) {
	for i, arg := range args {
		if arg == name && i+1 < len(args) {
			return args[i+1], true
		}
	}
	return "", false
}

func writeTestKubeconfig(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "apiVersion: v1\n" +
		"kind: Config\n" +
		"clusters:\n" +
		"  - name: local\n" +
		"    cluster:\n" +
		"      server: https://cluster.example.invalid\n" +
		"contexts:\n" +
		"  - name: local\n" +
		"    context:\n" +
		"      cluster: local\n" +
		"      user: operator\n" +
		"current-context: local\n" +
		"users:\n" +
		"  - name: operator\n" +
		"    user:\n" +
		"      token: test-token\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// Claude Code keeps its macOS login in the Keychain, so an absent credentials
// file is not an absence and must never yield a container that starts logged out.
func TestClaudeAuthProjectionStagesTheKeychainCredential(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	for _, key := range []string{"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "CLAUDE_CODE_OAUTH_TOKEN"} {
		t.Setenv(key, "")
	}

	secret := []byte(`{"claudeAiOauth":{"accessToken":"test"}}`)
	projection, err := discoverClaudeAuthProjection(
		context.Background(),
		func(context.Context, string, string) ([]byte, error) { return secret, nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	defer projection.Close()
	if len(projection.Mounts) != 1 {
		t.Fatalf("keychain credential did not produce one mount: %+v", projection.Mounts)
	}
	mount := projection.Mounts[0]
	if mount.ContainerPath != containerAuthRoot+"/claude.json" {
		t.Errorf("unexpected container path %q", mount.ContainerPath)
	}
	staged, err := os.ReadFile(mount.HostPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(staged) != string(secret) {
		t.Errorf("staged payload = %q, want %q", staged, secret)
	}
	info, err := os.Stat(mount.HostPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("staged credential mode = %v, want 0600", info.Mode().Perm())
	}
}

// The reported failure was a container that composed, projected, and only then
// said "Not logged in". Discovery fails closed instead.
func TestClaudeAuthProjectionFailsClosedWithoutACredential(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	for _, key := range []string{"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "CLAUDE_CODE_OAUTH_TOKEN"} {
		t.Setenv(key, "")
	}

	_, err := discoverClaudeAuthProjection(
		context.Background(),
		func(context.Context, string, string) ([]byte, error) { return nil, errClaudeKeyringNotFound },
	)
	if err == nil {
		t.Fatal("a missing Claude credential must fail before the container starts")
	}
	if !strings.Contains(err.Error(), "--auth=false") {
		t.Errorf("diagnostic does not name the unauthenticated escape hatch: %v", err)
	}
}

// An environment credential crosses by name, so no file needs projecting.
func TestClaudeAuthProjectionDefersToTheEnvironment(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("ANTHROPIC_API_KEY", "sk-test")

	projection, err := discoverClaudeAuthProjection(
		context.Background(),
		func(context.Context, string, string) ([]byte, error) {
			t.Fatal("environment credential must short-circuit the keyring read")
			return nil, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(projection.Mounts) != 0 {
		t.Errorf("environment credential still projected a mount: %+v", projection.Mounts)
	}
}
