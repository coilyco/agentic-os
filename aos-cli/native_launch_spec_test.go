package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// fakeSpecComposer puts an agent-compose on PATH that writes the given spec for
// `launch --spec-out` and serves the claude-ui snapshot fixture.
func fakeSpecComposer(t *testing.T, spec nativeLaunchSpec) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake composer is a shell script")
	}
	bin := t.TempDir()
	raw, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "spec.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	snapshot, err := filepath.Abs(filepath.Join("testdata", "claude-ui", "snapshot.json"))
	if err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = launch ] && [ \"$2\" = --spec-out ]; then cp '" + filepath.Join(bin, "spec.json") + "' \"$3\"; exit 0; fi\n" +
		"if [ \"$1\" = catalog ] && [ \"$2\" = snapshot ]; then cat '" + snapshot + "'; exit 0; fi\n" +
		"if [ \"$1\" = catalog ] && [ \"$2\" = roles ]; then echo '{\"items\":[{\"slug\":\"eng-platform\"},{\"slug\":\"scientist\"}]}'; exit 0; fi\n" +
		"exit 9\n"
	if err := os.WriteFile(filepath.Join(bin, "agent-compose"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	// No inventory unless a test writes one, so the host's own is never read.
	t.Setenv(nativeCanonicalHomeEnv, t.TempDir())
	for _, name := range []string{"HOME", "USERPROFILE", "CODEX_HOME", "XDG_CONFIG_HOME", "CLAUDE_CONFIG_DIR",
		"AGENT_COMPOSE_LAUNCH", "AGENT_COMPOSE_SESSION_BUNDLE", "AGENT_COMPOSE_MODEL_TIER"} {
		t.Setenv(name, os.Getenv(name))
	}
}

func specFixture(t *testing.T, harness string) nativeLaunchSpec {
	t.Helper()
	home, bundle := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(bundle, "manifest.json"), []byte(`{"role":"eng-platform"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	return nativeLaunchSpec{
		Format:      nativeLaunchSpecFormat,
		Role:        "platform",
		Harness:     harness,
		SeatName:    "Beetle-Ox-ab12",
		BundleDir:   bundle,
		RuntimeHome: home,
		EnvSet:      map[string]string{"AGENT_COMPOSE_LAUNCH": "1", "AGENT_COMPOSE_SESSION_BUNDLE": bundle},
		EnvUnset:    []string{"AGENT_COMPOSE_MODEL_TIER"},
	}
}

func TestSpecLaunchBuildsTheClaudeCommandAndEnvironment(t *testing.T) {
	spec := specFixture(t, "claude")
	fakeSpecComposer(t, spec)
	t.Setenv("AGENT_COMPOSE_MODEL_TIER", "frontier")

	got, err := resolveSpecLaunch(context.Background(),
		[]string{"agent-compose", "launch", "platform", "claude", "--model", "opus"})
	if err != nil {
		t.Fatal(err)
	}
	settings := filepath.Join(spec.RuntimeHome, ".claude", "settings.eng-platform.json")
	want := []string{"claude", "--name", "Beetle-Ox-ab12", "--settings", settings, "--model", "opus"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("argv\n got %q\nwant %q", got, want)
	}
	if _, err := os.Stat(filepath.Join(spec.RuntimeHome, ".claude", "themes", "aos-eng-platform.json")); err != nil {
		t.Errorf("theme not installed where CLAUDE_CONFIG_DIR points: %v", err)
	}
	for name, value := range map[string]string{
		"HOME":                 spec.RuntimeHome,
		"CLAUDE_CONFIG_DIR":    filepath.Join(spec.RuntimeHome, ".claude"),
		"XDG_CONFIG_HOME":      filepath.Join(spec.RuntimeHome, ".config"),
		"AGENT_COMPOSE_LAUNCH": "1",
	} {
		if got := os.Getenv(name); got != value {
			t.Errorf("%s = %q, want %q", name, got, value)
		}
	}
	if _, set := os.LookupEnv("AGENT_COMPOSE_MODEL_TIER"); set {
		t.Error("env_unset must remove the selector")
	}
}

func TestSpecLaunchLeavesCodexIdentityAndKeepsCallerFlags(t *testing.T) {
	spec := specFixture(t, "codex")
	fakeSpecComposer(t, spec)
	got, err := resolveSpecLaunch(context.Background(),
		[]string{"agent-compose", "launch", "platform", "codex", "--config", "x=1"})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"codex", "--config", "x=1"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("argv %q, want %q", got, want)
	}
	if got := os.Getenv("CODEX_HOME"); got != filepath.Join(spec.RuntimeHome, ".codex") {
		t.Errorf("CODEX_HOME = %q", got)
	}
}

func TestSpecLaunchLetsTheCallerSettingsWin(t *testing.T) {
	spec := specFixture(t, "claude")
	fakeSpecComposer(t, spec)
	got, err := resolveSpecLaunch(context.Background(),
		[]string{"agent-compose", "launch", "platform", "claude", "--settings=/mine.json"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(strings.Join(got, " "), "--settings") != 1 {
		t.Fatalf("a caller --settings must suppress aos's, got %q", got)
	}
}

func TestSpecLaunchRefusesWhatIsNotAnAgentComposeLaunch(t *testing.T) {
	if _, err := resolveSpecLaunch(context.Background(), []string{"claude"}); err == nil {
		t.Error("a bare harness command must be refused in spec mode")
	}
	spec := specFixture(t, "codex")
	fakeSpecComposer(t, spec)
	if _, err := resolveSpecLaunch(context.Background(),
		[]string{"agent-compose", "launch", "platform", "claude"}); err == nil {
		t.Error("a spec for another harness must be refused")
	}
}

func writeMCPInventory(t *testing.T, body string) string {
	t.Helper()
	home := os.Getenv(nativeCanonicalHomeEnv)
	if err := os.MkdirAll(filepath.Join(home, ".mcporter"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".mcporter", "mcporter.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return home
}

const scopedInventory = `{"mcpServers":{
 "shared":{"command":"${HOME}/bin/shared"},
 "mine":{"url":"https://x.invalid/${HOME}","x-aos":{"roles":["eng-platform"]}},
 "theirs":{"command":"t","x-aos":{"roles":["scientist"]}}}}`

func TestSpecLaunchScopesClaudeMCPToTheRole(t *testing.T) {
	spec := specFixture(t, "claude")
	fakeSpecComposer(t, spec)
	home := writeMCPInventory(t, scopedInventory)
	got, err := resolveSpecLaunch(context.Background(), []string{"agent-compose", "launch", "platform", "claude"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) < 4 || got[1] != "--strict-mcp-config" || got[2] != "--mcp-config" {
		t.Fatalf("claude argv must lead with the scoped MCP config, got %q", got)
	}
	raw, err := os.ReadFile(got[3])
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Servers map[string]map[string]any `json:"mcpServers"`
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatal(err)
	}
	if _, ok := config.Servers["theirs"]; ok || len(config.Servers) != 2 {
		t.Errorf("want shared and mine only, got %v", config.Servers)
	}
	if cmd := config.Servers["shared"]["command"]; cmd != filepath.Join(home, "bin", "shared") {
		t.Errorf("${HOME} must expand against the inventory home, got %v", cmd)
	}
}

func TestSpecLaunchDisablesOtherRolesServersForCodex(t *testing.T) {
	spec := specFixture(t, "codex")
	fakeSpecComposer(t, spec)
	writeMCPInventory(t, scopedInventory)
	got, err := resolveSpecLaunch(context.Background(), []string{"agent-compose", "launch", "platform", "codex"})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"codex", "-c", "mcp_servers.theirs.enabled=false"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("argv %q, want %q", got, want)
	}
}

func TestSpecLaunchRefusesAnUnknownRoleTag(t *testing.T) {
	spec := specFixture(t, "codex")
	fakeSpecComposer(t, spec)
	writeMCPInventory(t, `{"mcpServers":{"x":{"command":"x","x-aos":{"roles":["ghost"]}}}}`)
	if _, err := resolveSpecLaunch(context.Background(), []string{"agent-compose", "launch", "platform", "codex"}); err == nil {
		t.Fatal("a tag naming no roster role must refuse the launch")
	}
}

func TestSpecLaunchLeavesACallerMCPConfigAlone(t *testing.T) {
	spec := specFixture(t, "claude")
	fakeSpecComposer(t, spec)
	writeMCPInventory(t, scopedInventory)
	got, err := resolveSpecLaunch(context.Background(),
		[]string{"agent-compose", "launch", "platform", "claude", "--mcp-config", "/mine.json"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(got, " "), "--strict-mcp-config") {
		t.Fatalf("a caller --mcp-config must suppress scoping, got %q", got)
	}
}
