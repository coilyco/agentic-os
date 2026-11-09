package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// The launch spec is the only place aos learns a role's grants, so a connector row
// that stopped decoding would drop every connector without a sound.
func TestLaunchSpecDecodesTheRolesConnectorGrants(t *testing.T) {
	raw := `{"format":"agent-compose.launch-spec.v1","claude_ai_connectors":[{"url":"https://c.invalid/mcp","deny":["mcp__x__send"]}]}`
	var spec nativeLaunchSpec
	if err := json.Unmarshal([]byte(raw), &spec); err != nil {
		t.Fatal(err)
	}
	want := []nativeSpecConnector{{URL: "https://c.invalid/mcp", Deny: []string{"mcp__x__send"}}}
	if !reflect.DeepEqual(spec.ClaudeAIConnectors, want) {
		t.Fatalf("connectors = %v, want %v", spec.ClaudeAIConnectors, want)
	}
}

func connectorSpec(t *testing.T) nativeLaunchSpec {
	t.Helper()
	spec := specFixture(t, "claude")
	spec.ClaudeAIConnectors = []nativeSpecConnector{
		{URL: "https://c.invalid/mcp", Deny: []string{"mcp__c__send", "mcp__c__write"}},
		{URL: "https://d.invalid"},
	}
	return spec
}

// A granted role runs behind an allowlist in place of --strict-mcp-config: its servers,
// then each connector, with the denials merged into the one settings file.
func TestSpecLaunchAdmitsGrantedConnectorsBehindAnAllowlist(t *testing.T) {
	spec := connectorSpec(t)
	fakeSpecComposer(t, spec)
	home := writeMCPInventory(t, scopedInventory)
	got, err := resolveSpecLaunch(context.Background(), []string{"agent-compose", "launch", "platform", "claude"})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(got, " ")
	if strings.Contains(joined, "--strict-mcp-config") || got[1] != "--mcp-config" {
		t.Fatalf("a connector grant replaces strict mode, got %q", got)
	}
	settingsPath := got[indexOf(t, got, "--settings")+1]
	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	var settings struct {
		Allow       []map[string]any `json:"allowedMcpServers"`
		Permissions struct {
			Deny []string `json:"deny"`
		} `json:"permissions"`
		Theme string `json:"theme"`
	}
	if err := json.Unmarshal(raw, &settings); err != nil {
		t.Fatal(err)
	}
	wantAllow := []map[string]any{
		{"serverUrl": "https://x.invalid/" + home},
		{"serverCommand": []any{filepath.Join(home, "bin", "shared")}},
		{"serverUrl": "https://c.invalid/mcp"},
		{"serverUrl": "https://d.invalid"},
	}
	if !reflect.DeepEqual(settings.Allow, wantAllow) {
		t.Errorf("allowedMcpServers = %v, want %v", settings.Allow, wantAllow)
	}
	if want := []string{"mcp__c__send", "mcp__c__write"}; !reflect.DeepEqual(settings.Permissions.Deny, want) {
		t.Errorf("permissions.deny = %v, want %v", settings.Permissions.Deny, want)
	}
	if settings.Theme == "" {
		t.Error("the role's own settings fragment must survive the merge")
	}
}

// A caller's --settings cannot merge, so the launch stays strict minus connectors.
func TestSpecLaunchDropsConnectorsWhenTheCallerBringsSettings(t *testing.T) {
	spec := connectorSpec(t)
	fakeSpecComposer(t, spec)
	writeMCPInventory(t, scopedInventory)
	got, err := resolveSpecLaunch(context.Background(),
		[]string{"agent-compose", "launch", "platform", "claude", "--settings=/mine.json"})
	if err != nil {
		t.Fatal(err)
	}
	if got[1] != "--strict-mcp-config" || strings.Count(strings.Join(got, " "), "--settings") != 1 {
		t.Fatalf("the fail-closed fallback is strict and leaves the caller's settings, got %q", got)
	}
}

// A grant reaches Claude only. Another harness has no allowlist to carry it.
func TestSpecLaunchIgnoresConnectorsOnOtherHarnesses(t *testing.T) {
	spec := connectorSpec(t)
	spec.Harness = "codex"
	if connectorsApply(os.Stderr, spec, nil) {
		t.Fatal("connectors apply to claude only")
	}
}

func TestWriteConnectorSettingsMergesAndIsContentNamed(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "base.json")
	if err := os.WriteFile(base, []byte(`{"theme":"t","permissions":{"deny":["Bash(rm:*)"],"allow":["Read"]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	allow := []map[string]any{{"serverUrl": "https://c.invalid"}}
	first, err := writeConnectorSettings(filepath.Join(dir, "out"), "eng-platform", base, allow, []string{"mcp__c__send"})
	if err != nil {
		t.Fatal(err)
	}
	again, err := writeConnectorSettings(filepath.Join(dir, "out"), "eng-platform", base, allow, []string{"mcp__c__send"})
	if err != nil || again != first {
		t.Fatalf("same content, same file: %q vs %q (%v)", first, again, err)
	}
	var doc struct {
		Permissions struct {
			Deny  []string `json:"deny"`
			Allow []string `json:"allow"`
		} `json:"permissions"`
	}
	raw, _ := os.ReadFile(first)
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if want := []string{"Bash(rm:*)", "mcp__c__send"}; !reflect.DeepEqual(doc.Permissions.Deny, want) || len(doc.Permissions.Allow) != 1 {
		t.Fatalf("existing rules must survive: %+v", doc.Permissions)
	}
	if _, err := writeConnectorSettings(dir, "eng-platform", filepath.Join(dir, "missing.json"), allow, nil); err == nil {
		t.Error("an unreadable base settings file must refuse, not launch with the allowlist alone")
	}
}

func indexOf(t *testing.T, args []string, want string) int {
	t.Helper()
	for i, arg := range args {
		if arg == want {
			return i
		}
	}
	t.Fatalf("%q not in %q", want, args)
	return -1
}
