package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// An unscoped seat loads the whole user-level MCP set, so the spec path refuses
// without an inventory, as agent-compose's own launch does (#8260).
func TestNativeSpecMCPArgsRefusesWithoutAnInventory(t *testing.T) {
	stubRoleCatalog(t)
	t.Setenv(openCodeConfigEnv, "")
	for _, harness := range []string{"claude", "codex", "goose", "opencode"} {
		spec := nativeLaunchSpec{Harness: harness}
		_, err := nativeSpecMCPScope(context.Background(), spec, "eng-platform", t.TempDir(), t.TempDir(), nil)
		if !errors.Is(err, errNoMCPInventory) {
			t.Fatalf("%s: a host with no inventory should refuse, got %v", harness, err)
		}
	}
	spec := nativeLaunchSpec{Harness: "claude"}
	got, err := nativeSpecMCPScope(context.Background(), spec, "eng-platform", t.TempDir(), t.TempDir(), []string{"--mcp-config", "mine.json"})
	if err != nil || got.Flags != nil || got.Allow != nil {
		t.Fatalf("a caller's own scope is used as given, got %v, %v", got, err)
	}
}

func stubRoleCatalog(t *testing.T) {
	t.Helper()
	bin := t.TempDir()
	stub := "#!/bin/sh\necho '{\"items\":[{\"slug\":\"eng-platform\"},{\"slug\":\"sysadmin-senior\"}]}'\n"
	if err := os.WriteFile(filepath.Join(bin, "agent-compose"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func scopeInventoryHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	inventory := `{"mcpServers": {
  "remote": {"url": "https://example.invalid/mcp"},
  "script": {"command": "sh", "args": ["-c", "PATH=${HOME}/bin:$PATH exec tool"]},
  "pw_sysadmin": {"command": "npx", "x-aos": {"roles": ["sysadmin-senior"]}}
}}`
	if err := os.MkdirAll(filepath.Join(home, ".mcporter"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".mcporter", "mcporter.json"), []byte(inventory), 0o600); err != nil {
		t.Fatal(err)
	}
	goose := "extensions:\n  developer: {enabled: true, type: builtin}\n  teable: {enabled: true, type: streamable_http}\n"
	if err := os.MkdirAll(filepath.Join(home, ".config", "goose"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".config", "goose", "config.yaml"), []byte(goose), 0o600); err != nil {
		t.Fatal(err)
	}
	return home
}

// Goose takes the role's servers in place of its configured set, after the
// session verb, which is the only place its root command lets them parse.
func TestSpecGooseScopeReplacesTheProfile(t *testing.T) {
	stubRoleCatalog(t)
	t.Setenv("XDG_CONFIG_HOME", "")
	home := scopeInventoryHome(t)
	scope, err := nativeSpecMCPScope(context.Background(), nativeLaunchSpec{Harness: "goose"}, "eng-platform", home, t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	flags := scope.Flags
	want := "--no-profile|--with-builtin|developer|--with-streamable-http-extension|https://example.invalid/mcp|" +
		"--with-extension|script:sh -c \"PATH=" + home + "/bin:$PATH exec tool\""
	if got := strings.Join(flags, "|"); got != want {
		t.Fatalf("flags =\n%s\nwant\n%s", got, want)
	}
	command := strings.Join(gooseCommand("goose", nil, []string{"--no-profile"}), " ")
	if command != "goose session --no-profile" {
		t.Fatalf("bare goose command = %q", command)
	}
	if gooseScopeApplies([]string{"session", "--resume"}) || gooseScopeApplies([]string{"configure"}) {
		t.Fatal("a resume and a verb that loads nothing are left alone")
	}
}

// OpenCode takes an inline config that merges over its other layers.
func TestSpecOpenCodeScopeSetsTheInlineConfig(t *testing.T) {
	stubRoleCatalog(t)
	t.Setenv(openCodeConfigEnv, "")
	home := scopeInventoryHome(t)
	if _, err := nativeSpecMCPScope(context.Background(), nativeLaunchSpec{Harness: "opencode"}, "eng-platform", home, t.TempDir(), nil); err != nil {
		t.Fatal(err)
	}
	var config struct {
		MCP map[string]map[string]any `json:"mcp"`
	}
	if err := json.Unmarshal([]byte(os.Getenv(openCodeConfigEnv)), &config); err != nil {
		t.Fatalf("inline config: %v", err)
	}
	if config.MCP["remote"]["type"] != "remote" || config.MCP["script"]["type"] != "local" ||
		config.MCP["pw_sysadmin"]["enabled"] != false || len(config.MCP["pw_sysadmin"]) != 1 {
		t.Fatalf("mcp = %v", config.MCP)
	}
}

// A header whose ${VAR} is unset stays off the generated config. With the variable
// set the entry is unchanged and the value never reaches the file.
func TestWriteClaudeConfigLeavesOffAHeaderWhoseVariableIsUnset(t *testing.T) {
	servers := map[string]mcpScopeServer{
		"public_coilyco_linear": {URL: "https://example.invalid/mcp", Headers: map[string]string{
			"Authorization": "${LINEAR_MCP_AUTHORIZATION:-}", "X-Static": "fixed",
		}},
		"defaulted": {URL: "https://example.invalid/d", Headers: map[string]string{"X-Mode": "${MODE:-ro}"}},
	}
	write := func(env map[string]string) (map[string]map[string]any, string) {
		t.Helper()
		selection := selectMCPScope(servers, "eng-platform")
		selection.lookup = func(name string) (string, bool) { value, ok := env[name]; return value, ok }
		path, err := selection.writeClaudeConfig(t.TempDir(), t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var config struct {
			Servers map[string]map[string]any `json:"mcpServers"`
		}
		if err := json.Unmarshal(raw, &config); err != nil {
			t.Fatal(err)
		}
		return config.Servers, strings.Join(selection.headerOmissions(), "\n")
	}

	for name, env := range map[string]map[string]string{"unset": {}, "blank": {"LINEAR_MCP_AUTHORIZATION": " \n"}} {
		got, omitted := write(env)
		headers, _ := got["public_coilyco_linear"]["headers"].(map[string]any)
		if _, present := headers["Authorization"]; present {
			t.Fatalf("%s: Authorization still in the config: %v", name, headers)
		}
		if headers["X-Static"] != "fixed" {
			t.Fatalf("%s: a static header was dropped: %v", name, headers)
		}
		if !strings.Contains(omitted, "public_coilyco_linear") || !strings.Contains(omitted, "LINEAR_MCP_AUTHORIZATION") {
			t.Fatalf("%s: the launch does not name the cause: %q", name, omitted)
		}
		if strings.Contains(omitted, "X-Mode") || got["defaulted"]["headers"] == nil {
			t.Fatalf("%s: a header with a default was treated as unreadable: %v %q", name, got["defaulted"], omitted)
		}
	}

	got, omitted := write(map[string]string{"LINEAR_MCP_AUTHORIZATION": "Bearer s3cret"})
	headers, _ := got["public_coilyco_linear"]["headers"].(map[string]any)
	if headers["Authorization"] != "${LINEAR_MCP_AUTHORIZATION:-}" || omitted != "" {
		t.Fatalf("a readable secret changed the entry: %v %q", headers, omitted)
	}
}
