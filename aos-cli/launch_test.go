package main

import (
	"errors"
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

	mounts, err := authMountsForLaunch(true, "codex")
	if err != nil {
		t.Fatal(err)
	}
	if len(mounts) != 1 || mounts[0].HostPath != hostAuth ||
		mounts[0].ContainerPath != containerAuthRoot+"/codex.json" {
		t.Fatalf("Codex auth mounts = %#v", mounts)
	}
	plan, err := buildLaunchPlan(launchOptions{
		Image: "agentic-os:test", Role: "engineer", Layout: "codex",
		Delivery: "native-skills", Composed: true, CWD: t.TempDir(),
		Command: []string{"codex", "exec", "probe"}, UID: 1000, GID: 1000,
		AuthMounts: mounts,
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

func TestCodexAuthFailsBeforeLaunch(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		clearCodexAuthEnvironment(t)
		codexHome := t.TempDir()
		t.Setenv("CODEX_HOME", codexHome)
		_, err := authMountsForLaunch(true, "codex")
		if err == nil || !strings.Contains(err.Error(), "file-backed credentials not found") {
			t.Fatalf("missing Codex auth error = %v", err)
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
		_, err := authMountsForLaunch(true, "codex")
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
		_, err := authMountsForLaunch(true, "codex")
		if err == nil || !strings.Contains(err.Error(), "unsupported credentials") {
			t.Fatalf("unsupported Codex auth payload error = %v", err)
		}
	})

	t.Run("disabled", func(t *testing.T) {
		clearCodexAuthEnvironment(t)
		t.Setenv("CODEX_HOME", t.TempDir())
		mounts, err := authMountsForLaunch(false, "codex")
		if err != nil || len(mounts) != 0 {
			t.Fatalf("disabled Codex auth mounts = %#v, error = %v", mounts, err)
		}
	})
}

func TestCodexEnvironmentAuthAndDisabledAuthForwarding(t *testing.T) {
	clearCodexAuthEnvironment(t)
	t.Setenv("CODEX_API_KEY", "synthetic")
	t.Setenv("ANTHROPIC_API_KEY", "synthetic")
	t.Setenv("GOOSE_MODEL", "synthetic-model")

	mounts, err := authMountsForLaunch(true, "codex")
	if err != nil || len(mounts) != 0 {
		t.Fatalf("Codex environment auth mounts = %#v, error = %v", mounts, err)
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

func TestBuildLaunchPlanMountsCWDAndRunsInternalCompose(t *testing.T) {
	t.Parallel()
	cwd := filepath.Join(t.TempDir(), "my repo")
	plan, err := buildLaunchPlan(launchOptions{
		Image:         "agentic-os:test",
		Role:          "engineer",
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
		"--role\nengineer",
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

func TestBuildLaunchPlanPullsMovingReleaseImage(t *testing.T) {
	t.Parallel()
	plan, err := buildLaunchPlan(launchOptions{
		Image:    defaultImage,
		Role:     "director",
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
		Role:        "strats",
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
		Role:            "engineer",
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
		Role:           "ops",
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

func TestBuildLaunchPlanRejectsInvalidKubeconfigForAuthorizedRoles(t *testing.T) {
	t.Parallel()
	t.Run("missing", func(t *testing.T) {
		t.Parallel()
		_, err := buildLaunchPlan(launchOptions{
			Image: "agentic-os:test", Role: "director", Layout: "codex",
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
			Image: "agentic-os:test", Role: "ops", Layout: "codex",
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
			Image: "agentic-os:test", Role: "ops", Layout: "codex",
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
			Image: "agentic-os:test", Role: "ops", Layout: "codex",
			Delivery: "native-skills", Composed: true, CWD: t.TempDir(),
			Command: []string{"codex"}, UID: 1000, GID: 1000,
			Kubeconfig: path,
		})
		if err == nil || !strings.Contains(err.Error(), "is malformed") {
			t.Fatalf("error = %v, want multiple-document kubeconfig error", err)
		}
	})
}

func TestBuildLaunchPlanOmitsKubeconfigForSealedRoles(t *testing.T) {
	t.Parallel()
	for _, role := range []string{"engineer", "qa"} {
		role := role
		t.Run(role, func(t *testing.T) {
			t.Parallel()
			source := filepath.Join(t.TempDir(), "intentionally absent.yaml")
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
			if strings.Contains(joined, source) || strings.Contains(joined, "KUBECONFIG") {
				t.Fatalf("%s launch received kubeconfig projection:\n%s", role, joined)
			}
		})
	}
}

func TestArgvAfterDash(t *testing.T) {
	t.Parallel()
	got := argvAfterDash([]string{"aos", "--role", "engineer", "acompose", "--", "codex", "exec"})
	if strings.Join(got, " ") != "codex exec" {
		t.Fatalf("argvAfterDash() = %q", got)
	}
}

func TestShellJoinDoesNotExposeForwardedEnvironmentValues(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "do-not-print")
	plan, err := buildLaunchPlan(launchOptions{
		Image:         "agentic-os:test",
		Role:          "engineer",
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
