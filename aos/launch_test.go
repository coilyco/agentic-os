package main

import (
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

func TestBuildLaunchPlanMountsCWDAndRunsInternalCompose(t *testing.T) {
	t.Parallel()
	cwd := filepath.Join(t.TempDir(), "my repo")
	plan, err := buildLaunchPlan(launchOptions{
		Image:         "agentic-os:test",
		Role:          "engineer",
		Layout:        "codex",
		Density:       "full",
		Delivery:      "native-skills",
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
}

func TestBuildLaunchPlanCanSkipSubstrate(t *testing.T) {
	t.Parallel()
	plan, err := buildLaunchPlan(launchOptions{
		Image:       "agentic-os:test",
		Role:        "advisor",
		Layout:      "claude",
		Density:     "brief",
		Delivery:    "compiled",
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

func TestBuildLaunchPlanAddsPrivateAccessWithoutLocalDaemon(t *testing.T) {
	t.Parallel()
	proxy := "socks5h://private-proxy:1055"
	plan, err := buildLaunchPlan(launchOptions{
		Image:    "agentic-os:test",
		Role:     "director",
		Layout:   "codex",
		Density:  "full",
		Delivery: "native-skills",
		CWD:      t.TempDir(),
		Command:  []string{"codex"},
		UID:      1000,
		GID:      1000,
		PrivateAccess: &privateAccessConfig{
			Network: "private-network",
			Proxy:   proxy,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(plan.DockerArgs, "\n")
	for _, want := range []string{
		"--network\nprivate-network",
		"--env\nALL_PROXY",
		"--env\nHTTP_PROXY",
		"--env\nHTTPS_PROXY",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("launch plan missing %q\n%s", want, joined)
		}
	}
	for _, forbidden := range []string{
		proxy,
		"tailscaled",
		"--cap-add",
		"NET_ADMIN",
		"/var/run/tailscale",
	} {
		if strings.Contains(joined, forbidden) {
			t.Errorf("launch plan must not require local private-network runtime %q\n%s", forbidden, joined)
		}
	}
	environment := strings.Join(plan.DockerEnvironment, "\n")
	for _, want := range []string{
		"ALL_PROXY=" + proxy,
		"HTTP_PROXY=" + proxy,
		"HTTPS_PROXY=" + proxy,
	} {
		if !strings.Contains(environment, want) {
			t.Errorf("Docker environment missing %q\n%s", want, environment)
		}
	}
	rendered := shellJoin(append([]string{"docker"}, plan.DockerArgs...))
	if strings.Contains(rendered, proxy) {
		t.Fatalf("dry-run exposed the private proxy: %s", rendered)
	}
}

func TestMergeEnvironmentReplacesExistingProxyValues(t *testing.T) {
	t.Parallel()
	got := mergeEnvironment(
		[]string{"PATH=/usr/bin", "ALL_PROXY=socks5h://stale:1"},
		[]string{"ALL_PROXY=socks5h://private-proxy:1055"},
	)
	joined := strings.Join(got, "\n")
	if strings.Contains(joined, "stale") {
		t.Fatalf("merged environment retained the stale proxy:\n%s", joined)
	}
	if !strings.Contains(joined, "PATH=/usr/bin") ||
		!strings.Contains(joined, "ALL_PROXY=socks5h://private-proxy:1055") {
		t.Fatalf("merged environment lost required entries:\n%s", joined)
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
		Density:       "full",
		Delivery:      "native-skills",
		CWD:           t.TempDir(),
		Command:       []string{"codex"},
		UID:           1000,
		GID:           1000,
		ForwardedEnvs: forwardedEnvironment(),
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

func containsArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}
