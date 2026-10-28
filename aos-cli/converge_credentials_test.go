package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// An apply exports the credentials before it projects, so the registry is judged
// against what the SSM read produced. Check never calls the export.
func TestConvergeEnvironmentExportsCredentialsBeforeProjecting(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	configDir := filepath.Join(root, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "mcporter.json"), []byte(linearHeaderInventory), 0o600); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "converge.yaml")
	if err := os.WriteFile(configPath, []byte("mcp:\n  inventory: mcporter.json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{}
	exports := 0
	converge := func(check bool, exported map[string]string) environmentConvergeResult {
		t.Helper()
		result, err := convergeEnvironment(context.Background(), environmentConvergeOptions{
			ConfigPath:  configPath,
			Home:        home,
			Check:       check,
			Environment: func(name string) (string, bool) { value, ok := env[name]; return value, ok },
			ExportCredentials: func(context.Context) {
				exports++
				for name, value := range exported {
					env[name] = value
				}
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	registry := func() string {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(home, ".claude.json"))
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}

	failed := converge(false, nil)
	if exports != 1 || strings.Contains(registry(), `"Authorization"`) {
		t.Fatalf("a failed read: exports = %d, registry:\n%s", exports, registry())
	}
	if len(failed.Warnings) != 1 || !strings.Contains(failed.Warnings[0], "aws sso login") {
		t.Fatalf("a failed read does not name the fix: %q", failed.Warnings)
	}

	if converge(true, map[string]string{"LINEAR_MCP_AUTHORIZATION": "Bearer s3cret"}).Changed != false || exports != 1 {
		t.Fatalf("check exported credentials or drifted: exports = %d", exports)
	}

	read := converge(false, map[string]string{"LINEAR_MCP_AUTHORIZATION": "Bearer s3cret"})
	if exports != 2 || !strings.Contains(registry(), "${LINEAR_MCP_AUTHORIZATION:-}") || len(read.Warnings) != 0 {
		t.Fatalf("a readable read: exports = %d, warnings = %q, registry:\n%s", exports, read.Warnings, registry())
	}
	if strings.Contains(registry(), "s3cret") {
		t.Fatal("the credential value reached the registry")
	}
}
