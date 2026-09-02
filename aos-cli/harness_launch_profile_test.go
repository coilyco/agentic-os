package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/urfave/cli/v3"
)

func TestLoadHarnessLaunchProfilesUsesRoleAgents(t *testing.T) {
	t.Parallel()
	document, err := loadHarnessLaunchProfiles([]byte(`
roles:
  platform:
    agent: codex
  director:
    agent: claude
`))
	if err != nil {
		t.Fatal(err)
	}
	if document.Roles["platform"].Agent != "codex" {
		t.Fatalf("platform role was not decoded: %+v", document.Roles["platform"])
	}
	if document.DefaultAgents["director"] != "claude" {
		t.Fatalf("default agent was not decoded: %+v", document.DefaultAgents)
	}
}

func TestLoadHarnessLaunchProfilesRejectsMalformedRegistry(t *testing.T) {
	t.Parallel()
	for _, data := range [][]byte{
		[]byte(`{}`),
		[]byte("roles: {}\n"),
		[]byte("roles:\n  bad/role:\n    agent: codex\n"),
		[]byte("roles:\n  platform: {}\n"),
		[]byte("roles:\n  platform:\n    agent: other\n"),
		[]byte("roles:\n  platform:\n    agent: codex\n    model: gpt-5\n"),
		[]byte("defaults:\n  codex: {model: gpt-5}\n"),
	} {
		if _, err := loadHarnessLaunchProfiles(data); err == nil {
			t.Fatalf("loadHarnessLaunchProfiles accepted %q", data)
		}
	}
}

// Resolution is the behavior under test, not the agent a role points at: the
// profiles own that tunable, so naming values here would restate config.
func TestStandaloneDefaultAgentForRole(t *testing.T) {
	// Not parallel - reads the global cwd and the compiled fallback that the
	// sibling tests below swap out.
	document, err := loadConfiguredHarnessLaunchProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(document.DefaultAgents) == 0 {
		t.Fatal("the configured launch profiles declared no roles")
	}
	// Asserting got == document.DefaultAgents[role] would compare the loader
	// against itself. Value resolution is covered by the fixture tests below.
	for role := range document.DefaultAgents {
		got, err := standaloneDefaultAgentForRole(role)
		if err != nil {
			t.Fatalf("standaloneDefaultAgentForRole(%s): %v", role, err)
		}
		if got == "" {
			t.Fatalf("standaloneDefaultAgentForRole(%s) resolved to an empty agent", role)
		}
	}
	for _, role := range []string{"", "bad/role", "story-architect"} {
		if _, err := standaloneDefaultAgentForRole(role); err == nil {
			t.Fatalf("standaloneDefaultAgentForRole(%q) succeeded", role)
		}
	}
}

func TestConfiguredHarnessLaunchProfilesReadsAgentsFile(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, harnessLaunchProfilesRelativePath)
	if err := os.WriteFile(path, []byte(`
roles:
  platform:
    agent: goose
`), 0o644); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(root, "child")
	if err := os.Mkdir(child, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(child)
	oldCompiled := compiledHarnessLaunchProfilesBase64
	compiledHarnessLaunchProfilesBase64 = base64.StdEncoding.EncodeToString([]byte(`
roles:
  platform:
    agent: codex
`))
	t.Cleanup(func() { compiledHarnessLaunchProfilesBase64 = oldCompiled })

	agent, err := standaloneDefaultAgentForRole("platform")
	if err != nil {
		t.Fatal(err)
	}
	if agent != "goose" {
		t.Fatalf("standaloneDefaultAgentForRole(platform) = %q, want goose", agent)
	}
}

func TestConfiguredHarnessLaunchProfilesUsesCompiledFallback(t *testing.T) {
	t.Chdir(t.TempDir())
	oldCompiled := compiledHarnessLaunchProfilesBase64
	compiledHarnessLaunchProfilesBase64 = base64.StdEncoding.EncodeToString([]byte(`
roles:
  platform:
    agent: opencode
`))
	t.Cleanup(func() { compiledHarnessLaunchProfilesBase64 = oldCompiled })

	agent, err := standaloneDefaultAgentForRole("platform")
	if err != nil {
		t.Fatal(err)
	}
	if agent != "opencode" {
		t.Fatalf("standaloneDefaultAgentForRole(platform) = %q, want opencode", agent)
	}
}

// The shell asks this verb instead of parsing the profiles, so its contract is
// one bare agent name on stdout and a non-zero exit for anything else.
func TestLaunchAgentVerbPrintsTheConfiguredAgent(t *testing.T) {
	profiles := filepath.Join(t.TempDir(), "harness-launch-profiles.yaml")
	body := "roles:\n  platform:\n    agent: codex\n  gamedev:\n    agent: goose\n"
	if err := os.WriteFile(profiles, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AOS_HARNESS_LAUNCH_PROFILES", profiles)

	for role, want := range map[string]string{"platform": "codex", "gamedev": "goose"} {
		out := &bytes.Buffer{}
		command := &cli.Command{Name: "aos", Writer: out, Action: runLaunchAgent}
		if err := command.Run(context.Background(), []string{"aos", role}); err != nil {
			t.Fatalf("resolve %s: %v", role, err)
		}
		if got := strings.TrimSpace(out.String()); got != want {
			t.Fatalf("%s resolved to %q, want %q", role, got, want)
		}
	}
}

func TestLaunchAgentVerbRefusesAnUnusableRole(t *testing.T) {
	profiles := filepath.Join(t.TempDir(), "harness-launch-profiles.yaml")
	if err := os.WriteFile(profiles, []byte("roles:\n  platform:\n    agent: claude\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AOS_HARNESS_LAUNCH_PROFILES", profiles)

	for _, args := range [][]string{{"aos"}, {"aos", "engineer"}, {"aos", "../etc"}} {
		out := &bytes.Buffer{}
		command := &cli.Command{Name: "aos", Writer: out, Action: runLaunchAgent}
		if err := command.Run(context.Background(), args); err == nil {
			t.Fatalf("%v should be refused", args)
		}
		if out.String() != "" {
			t.Fatalf("a refusal must print no agent: %q", out.String())
		}
	}
}
