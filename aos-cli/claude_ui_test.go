package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Fixtures come from agent-compose v2.166.0: a trimmed `catalog snapshot` and
// `native-ui`, less the cluster deny agent-compose later removed. Parity is the contract.
func TestClaudeUIMatchesAgentComposeNativeUI(t *testing.T) {
	var snapshot claudeUISnapshot
	readJSON(t, filepath.Join("testdata", "claude-ui", "snapshot.json"), &snapshot)
	var want []json.RawMessage
	readJSON(t, filepath.Join("testdata", "claude-ui", "expected.json"), &want)
	if len(want) != len(snapshot.RoleOrder) {
		t.Fatalf("fixture covers %d roles, snapshot orders %d", len(want), len(snapshot.RoleOrder))
	}
	for index, role := range snapshot.RoleOrder {
		got, err := buildClaudeUI(snapshot, role, "")
		if err != nil {
			t.Fatalf("%s: %v", role, err)
		}
		var gotAny, wantAny any
		raw, _ := json.Marshal(got)
		_ = json.Unmarshal(raw, &gotAny)
		_ = json.Unmarshal(want[index], &wantAny)
		if !reflect.DeepEqual(gotAny, wantAny) {
			t.Errorf("%s differs from agent-compose native-ui:\n got %s\nwant %s", role, raw, want[index])
		}
	}
}

func TestClaudeUIRefusesUnknownRoleAndPersonality(t *testing.T) {
	snapshot := claudeUISnapshot{
		Roles: map[string]claudeUIRole{"r": {FavoriteColor: "#709b62", Personalities: []string{"ghost"}}},
	}
	if _, err := buildClaudeUI(snapshot, "nope", ""); err == nil {
		t.Error("an unknown role must be refused")
	}
	if _, err := buildClaudeUI(snapshot, "r", ""); err == nil {
		t.Error("a missing personality must be refused")
	}
}

func TestWriteClaudeUIUsesTheNativeUILayout(t *testing.T) {
	dir := t.TempDir()
	bundle := claudeUIBundle{Role: "r", Slug: "aos-r"}
	if err := writeClaudeUI(dir, bundle); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"themes/aos-r.json", "settings.r.json"} {
		if _, err := os.Stat(filepath.Join(dir, path)); err != nil {
			t.Errorf("%s not written: %v", path, err)
		}
	}
}

func readJSON(t *testing.T, path string, into any) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, into); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
}
