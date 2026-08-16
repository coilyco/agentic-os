// Claude Code reads its config from CLAUDE_CONFIG_DIR, which sits one level
// below the home-root .claude.json most installs use. See docs/native-harness-config.md.

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// nativeClaudeConfigPath is the single owner of the config-location question.
// Both the MCP projection and the shadow staging resolve through it.
func nativeClaudeConfigPath(home string) string {
	scoped := filepath.Join(home, ".claude", ".claude.json")
	if _, err := os.Lstat(scoped); err == nil {
		return scoped
	}
	return filepath.Join(home, ".claude.json")
}

func linkNativeClaudeConfig(sourceHome, targetHome string) error {
	target := filepath.Join(targetHome, ".claude", ".claude.json")
	if _, err := os.Lstat(target); err == nil {
		return nil
	}
	source := nativeClaudeConfigPath(sourceHome)
	if _, err := os.Stat(source); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("inspect host Claude config %s: %w", source, err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return fmt.Errorf("create native Claude config directory: %w", err)
	}
	if err := os.Symlink(source, target); err != nil {
		return fmt.Errorf("link host Claude config %s: %w", source, err)
	}
	return nil
}

// A shadow home reaches the config through a symlink, and an atomic rename
// would replace that link with a copy, so writers resolve it first.
func nativeClaudeWritePath(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return path
}

// A staged session home is always its own CLAUDE_CONFIG_DIR, so its config sits
// inside .claude even when the host keeps the home-root spelling.
func nativeClaudeSessionConfigPath(sessionHome string) string {
	return filepath.Join(sessionHome, ".claude", ".claude.json")
}

// Folder trust is keyed by absolute project path, and every native session
// mints a fresh one, so the launcher pre-accepts the paths it just created.
func seedNativeClaudeTrust(configPath string, projects []string) error {
	path := nativeClaudeWritePath(configPath)
	top := map[string]json.RawMessage{}
	raw, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := json.Unmarshal(raw, &top); err != nil {
			return fmt.Errorf("parse Claude config %s: %w", path, err)
		}
	case errors.Is(err, fs.ErrNotExist):
	default:
		return fmt.Errorf("read Claude config %s: %w", path, err)
	}
	entries := map[string]map[string]json.RawMessage{}
	if current := top["projects"]; len(current) > 0 {
		if err := json.Unmarshal(current, &entries); err != nil {
			return fmt.Errorf("parse Claude project state %s: %w", path, err)
		}
	}
	changed := false
	for _, project := range nativeClaudeTrustPaths(projects) {
		entry := entries[project]
		if entry == nil {
			entry = map[string]json.RawMessage{}
		}
		for _, key := range []string{"hasTrustDialogAccepted", "hasCompletedProjectOnboarding"} {
			if string(entry[key]) == "true" {
				continue
			}
			entry[key] = json.RawMessage("true")
			changed = true
		}
		entries[project] = entry
	}
	if !changed {
		return nil
	}
	encoded, err := json.Marshal(entries)
	if err != nil {
		return fmt.Errorf("render Claude project state: %w", err)
	}
	top["projects"] = encoded
	payload, err := json.MarshalIndent(top, "", "  ")
	if err != nil {
		return fmt.Errorf("render Claude config: %w", err)
	}
	return writeAtomicFile(path, appendNativeTrailingNewline(payload), 0o600)
}

// macOS resolves /var to /private/var, and the harness records whichever form
// it was launched with, so both spellings get seeded.
func nativeClaudeTrustPaths(projects []string) []string {
	seen := map[string]bool{}
	resolved := make([]string, 0, len(projects)*2)
	for _, project := range projects {
		if project == "" {
			continue
		}
		candidates := []string{project}
		if evaluated, err := filepath.EvalSymlinks(project); err == nil {
			candidates = append(candidates, evaluated)
		}
		for _, candidate := range candidates {
			if seen[candidate] {
				continue
			}
			seen[candidate] = true
			resolved = append(resolved, candidate)
		}
	}
	return resolved
}
