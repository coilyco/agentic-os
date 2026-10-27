package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// claude.ai connectors in a spec-mode launch. The grant table is agent-compose's, and
// the spec carries the role's rows, so aos keeps none. docs/native-harness-config.md

type nativeSpecConnector struct {
	URL  string   `json:"url"`
	Deny []string `json:"deny"`
}

// mcpScopeLaunch is what the MCP scope hands the launch. Allow and Deny stand in for
// --strict-mcp-config, which hides connectors.
type mcpScopeLaunch struct {
	Flags []string
	Allow []map[string]any
	Deny  []string
}

// connectorsApply is false when a caller's own --settings cannot merge the allowlist.
// The launch then stays strict, minus connectors, as agent-compose's does.
func connectorsApply(w io.Writer, spec nativeLaunchSpec, args []string) bool {
	if spec.Harness != "claude" || len(spec.ClaudeAIConnectors) == 0 {
		return false
	}
	if nativeSpecArgsCarry(args, "--settings") {
		fmt.Fprintln(w, "aos: a caller --settings cannot carry the connector allowlist, so this launch has no claude.ai connectors")
		return false
	}
	return true
}

// allowlist is the allowedMcpServers list that stands in for --strict-mcp-config:
// each selected server by URL or exact command, then each granted connector by URL.
func (selection mcpScopeSelection) allowlist(connectors []nativeSpecConnector, home string) []map[string]any {
	var out []map[string]any
	for _, name := range selection.Selected {
		rendered := mcpScopeClaudeServer(selection.servers[name], home, selection.headerLookup())
		if url, ok := rendered["url"].(string); ok {
			out = append(out, map[string]any{"serverUrl": url})
			continue
		}
		command := []string{rendered["command"].(string)}
		if args, ok := rendered["args"].([]string); ok {
			command = append(command, args...)
		}
		out = append(out, map[string]any{"serverCommand": command})
	}
	for _, connector := range connectors {
		out = append(out, map[string]any{"serverUrl": connector.URL})
	}
	return out
}

func connectorDenied(connectors []nativeSpecConnector) []string {
	var out []string
	for _, connector := range connectors {
		out = append(out, connector.Deny...)
	}
	return out
}

// writeConnectorSettings merges allow and deny into the settings fragment, since
// Claude takes one --settings file. Named by content, like the MCP config.
func writeConnectorSettings(dir, role, base string, allow []map[string]any, deny []string) (string, error) {
	doc := map[string]any{}
	if strings.TrimSpace(base) != "" {
		raw, err := os.ReadFile(base)
		if err != nil {
			return "", fmt.Errorf("read settings %s: %w", base, err)
		}
		if err := json.Unmarshal(raw, &doc); err != nil {
			return "", fmt.Errorf("parse settings %s: %w", base, err)
		}
	}
	doc["allowedMcpServers"] = allow
	if len(deny) > 0 {
		permissions, _ := doc["permissions"].(map[string]any)
		if permissions == nil {
			permissions = map[string]any{}
		}
		existing, _ := permissions["deny"].([]any)
		for _, rule := range deny {
			existing = append(existing, rule)
		}
		permissions["deny"] = existing
		doc["permissions"] = permissions
	}
	payload, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", err
	}
	payload = append(payload, '\n')
	sum := sha256.Sum256(payload)
	path := filepath.Join(dir, fmt.Sprintf("%s-%s-settings.json", role, hex.EncodeToString(sum[:])[:12]))
	if existing, err := os.ReadFile(path); err == nil && string(existing) == string(payload) {
		return path, nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(dir, ".settings-*.json")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(payload); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	return path, os.Rename(tmp.Name(), path)
}
