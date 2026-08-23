package main

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadHarnessLaunchProfilesUsesRoleAgents(t *testing.T) {
	t.Parallel()
	document, err := loadHarnessLaunchProfiles([]byte(`
roles:
  platform:
    agent: codex
  tpm:
    agent: claude
`))
	if err != nil {
		t.Fatal(err)
	}
	if document.Roles["platform"].Agent != "codex" {
		t.Fatalf("platform role was not decoded: %+v", document.Roles["platform"])
	}
	if document.DefaultAgents["tpm"] != "claude" {
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
	for role, want := range document.DefaultAgents {
		got, err := standaloneDefaultAgentForRole(role)
		if err != nil {
			t.Fatalf("standaloneDefaultAgentForRole(%s): %v", role, err)
		}
		if got != want {
			t.Fatalf("standaloneDefaultAgentForRole(%s) = %q, want %q", role, got, want)
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
