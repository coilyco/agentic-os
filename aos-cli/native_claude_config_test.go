package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeTestClaudeConfig(t *testing.T, path string, payload map[string]any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

func readTestClaudeConfig(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	return decoded
}

func TestNativeClaudeConfigPathPrefersScopedConfig(t *testing.T) {
	home := t.TempDir()
	if got, want := nativeClaudeConfigPath(home), filepath.Join(home, ".claude.json"); got != want {
		t.Fatalf("home-root config path = %s, want %s", got, want)
	}
	scoped := filepath.Join(home, ".claude", ".claude.json")
	writeTestClaudeConfig(t, scoped, map[string]any{})
	if got := nativeClaudeConfigPath(home); got != scoped {
		t.Fatalf("scoped config path = %s, want %s", got, scoped)
	}
}

func TestStageNativeRoleHomeLinksHomeRootClaudeConfig(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source")
	target := filepath.Join(t.TempDir(), "target")
	if err := os.MkdirAll(filepath.Join(source, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(source, ".claude.json")
	writeTestClaudeConfig(t, config, map[string]any{"mcpServers": map[string]any{"forgejo": map[string]any{}}})

	if err := stageNativeRoleHome(source, target, ""); err != nil {
		t.Fatal(err)
	}

	staged := filepath.Join(target, ".claude", ".claude.json")
	resolved, err := filepath.EvalSymlinks(staged)
	if err != nil {
		t.Fatalf("staged Claude config is missing from CLAUDE_CONFIG_DIR: %v", err)
	}
	if !samePath(resolved, config) {
		t.Fatalf("staged Claude config resolves to %s, want %s", resolved, config)
	}
	if _, ok := readTestClaudeConfig(t, staged)["mcpServers"]; !ok {
		t.Fatal("staged Claude config lost the host MCP registry")
	}
}

func TestSeedNativeClaudeTrustAcceptsSessionPathsAndPreservesState(t *testing.T) {
	home := t.TempDir()
	config := filepath.Join(home, ".claude.json")
	writeTestClaudeConfig(t, config, map[string]any{
		"mcpServers": map[string]any{"forgejo": map[string]any{}},
		"projects": map[string]any{
			"/existing": map[string]any{"allowedTools": []string{"Bash"}},
		},
	})
	session := filepath.Join(t.TempDir(), "projects")
	if err := os.MkdirAll(session, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := seedNativeClaudeTrust(nativeClaudeConfigPath(home), []string{session}); err != nil {
		t.Fatal(err)
	}

	decoded := readTestClaudeConfig(t, config)
	if _, ok := decoded["mcpServers"]; !ok {
		t.Fatal("seeding trust dropped the MCP registry")
	}
	projects, ok := decoded["projects"].(map[string]any)
	if !ok {
		t.Fatalf("projects is not an object: %T", decoded["projects"])
	}
	if _, ok := projects["/existing"]; !ok {
		t.Fatal("seeding trust dropped an unrelated project entry")
	}
	entry, ok := projects[session].(map[string]any)
	if !ok {
		t.Fatalf("session project %s is missing from %+v", session, projects)
	}
	for _, key := range nativeClaudeTrustKeys {
		if entry[key] != true {
			t.Errorf("%s = %v, want true", key, entry[key])
		}
	}
	if resolved, err := filepath.EvalSymlinks(session); err == nil && resolved != session {
		if _, ok := projects[resolved]; !ok {
			t.Errorf("symlink-resolved path %s is missing from %+v", resolved, projects)
		}
	}
}

func TestSeedNativeClaudeTrustWritesThroughShadowSymlink(t *testing.T) {
	hostHome := t.TempDir()
	config := filepath.Join(hostHome, ".claude.json")
	writeTestClaudeConfig(t, config, map[string]any{})
	sessionHome := filepath.Join(t.TempDir(), "home")
	if err := stageNativeRoleHome(hostHome, sessionHome, ""); err != nil {
		t.Fatal(err)
	}
	session := filepath.Join(t.TempDir(), "projects")
	if err := os.MkdirAll(session, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := seedNativeClaudeTrust(nativeClaudeSessionConfigPath(sessionHome), []string{session}); err != nil {
		t.Fatal(err)
	}

	staged := filepath.Join(sessionHome, ".claude", ".claude.json")
	info, err := os.Lstat(staged)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("writing trust replaced the staged symlink with a copy")
	}
	projects, ok := readTestClaudeConfig(t, config)["projects"].(map[string]any)
	if !ok {
		t.Fatal("trust did not reach the host config through the symlink")
	}
	if _, ok := projects[session]; !ok {
		t.Fatalf("session project %s is missing from the host config", session)
	}
}

func TestSeedNativeClaudeTrustCreatesStandaloneSessionConfig(t *testing.T) {
	hostHome := t.TempDir()
	hostConfig := filepath.Join(hostHome, ".claude.json")
	writeTestClaudeConfig(t, hostConfig, map[string]any{"mcpServers": map[string]any{}})
	sessionHome := filepath.Join(t.TempDir(), "home")
	if err := stageStandaloneRoleHome(hostHome, sessionHome); err != nil {
		t.Fatal(err)
	}
	session := filepath.Join(t.TempDir(), "projects")
	if err := os.MkdirAll(session, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := seedNativeClaudeTrust(nativeClaudeSessionConfigPath(sessionHome), []string{session}); err != nil {
		t.Fatal(err)
	}

	projects, ok := readTestClaudeConfig(t, nativeClaudeSessionConfigPath(sessionHome))["projects"].(map[string]any)
	if !ok {
		t.Fatal("standalone session config has no trusted projects")
	}
	if _, ok := projects[session]; !ok {
		t.Fatalf("session project %s is missing from %+v", session, projects)
	}
	if _, ok := readTestClaudeConfig(t, hostConfig)["projects"]; ok {
		t.Fatal("standalone trust leaked into the host config")
	}
}
