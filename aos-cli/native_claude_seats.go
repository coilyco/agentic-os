// Every native Claude seat launches with the same CLAUDE_CONFIG_DIR, so one
// Keychain login serves them all. The per-seat load points ride in as an
// additional directory instead. docs/native-claude-credentials.md
package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// nativeClaudeSeatsDirName sits beside the host .claude, under the canonical
	// home, so its path string is the same in every session.
	nativeClaudeSeatsDirName = ".claude-seats"
	// nativeClaudeSeatDirName is the session-root directory the harness gets as
	// --add-dir, holding only the two per-seat load points.
	nativeClaudeSeatDirName = "seat"
	// agentComposeClaudeConfigDirEnv hands agent-compose the shared directory,
	// since its own launch exports CLAUDE_CONFIG_DIR from the runtime home.
	agentComposeClaudeConfigDirEnv = "AGENT_COMPOSE_CLAUDE_CONFIG_DIR"
	// claudeAdditionalDirectoriesClaudeMdEnv makes the harness load CLAUDE.md
	// from an --add-dir, which it skips by default.
	claudeAdditionalDirectoriesClaudeMdEnv = "CLAUDE_CODE_ADDITIONAL_DIRECTORIES_CLAUDE_MD"
)

// nativeClaudeSeatsUnlinked never link into the shared directory: the per-seat
// load points, and the login the shared Keychain item holds instead.
var nativeClaudeSeatsUnlinked = map[string]bool{
	"CLAUDE.md":         true,
	"skills":            true,
	".credentials.json": true,
}

// nativeClaudeSeatLoadPoints are what agent-compose projects per seat.
var nativeClaudeSeatLoadPoints = []string{"CLAUDE.md", "skills"}

func nativeClaudeSeatsDir(home string) string {
	return filepath.Join(home, nativeClaudeSeatsDirName)
}

// stageNativeClaudeSeatsDir links each host .claude entry on first use, adds
// missing links after, never replaces one, and reports a fresh creation.
func stageNativeClaudeSeatsDir(home string) (bool, error) {
	source := filepath.Join(home, ".claude")
	target := nativeClaudeSeatsDir(home)
	created := false
	if _, err := os.Lstat(target); err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return false, fmt.Errorf("inspect shared Claude config %s: %w", target, err)
		}
		created = true
	}
	if err := os.MkdirAll(target, 0o700); err != nil {
		return false, fmt.Errorf("create shared Claude config %s: %w", target, err)
	}
	entries, err := os.ReadDir(source)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, fmt.Errorf("read host Claude config %s: %w", source, err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if nativeClaudeSeatsUnlinked[name] {
			continue
		}
		link := filepath.Join(target, name)
		if _, err := os.Lstat(link); err == nil {
			continue
		}
		if err := os.Symlink(filepath.Join(source, name), link); err != nil {
			return false, fmt.Errorf("link shared Claude config entry %s: %w", name, err)
		}
	}
	// Empty on purpose: the role-filtered skills arrive through the seat.
	if err := os.MkdirAll(filepath.Join(target, "skills"), 0o700); err != nil {
		return false, fmt.Errorf("create shared Claude skills directory: %w", err)
	}
	config := filepath.Join(target, ".claude.json")
	if _, err := os.Lstat(config); errors.Is(err, fs.ErrNotExist) {
		if hostConfig := nativeClaudeConfigPath(home); fileExists(hostConfig) {
			if err := os.Symlink(hostConfig, config); err != nil {
				return false, fmt.Errorf("link host Claude config %s: %w", hostConfig, err)
			}
		}
	}
	return created, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// seedNativeClaudeSeatsCredential copies a usable host login into a shared
// directory that has none, so the first seat refreshes instead of prompting.
func seedNativeClaudeSeatsCredential(home string, now time.Time) (bool, error) {
	target := filepath.Join(nativeClaudeSeatsDir(home), ".credentials.json")
	if _, err := os.Lstat(target); err == nil {
		return false, nil
	}
	source := canonicalClaudeCredentialPath(home)
	usable, err := claudeCredentialUsable(source, now)
	if err != nil || !usable {
		return false, err
	}
	payload, err := os.ReadFile(source)
	if err != nil {
		return false, fmt.Errorf("read %s: %w", source, err)
	}
	if err := os.WriteFile(target, payload, 0o600); err != nil {
		return false, fmt.Errorf("write %s: %w", target, err)
	}
	return true, nil
}

// stageNativeClaudeSeat links the two per-seat load points out of the session
// home into <session root>/seat/.claude and returns the seat directory.
func stageNativeClaudeSeat(sessionRoot, sessionHome string) (string, error) {
	seat := filepath.Join(sessionRoot, nativeClaudeSeatDirName)
	dir := filepath.Join(seat, ".claude")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create seat load points %s: %w", dir, err)
	}
	for _, name := range nativeClaudeSeatLoadPoints {
		link := filepath.Join(dir, name)
		if _, err := os.Lstat(link); err == nil {
			continue
		}
		if err := os.Symlink(filepath.Join(sessionHome, ".claude", name), link); err != nil {
			return "", fmt.Errorf("link seat load point %s: %w", name, err)
		}
	}
	return seat, nil
}

// insertNativeHarnessArgs places flags right after the harness token, ahead of
// anything the caller passed, so a trailing `--` prompt stays last.
func insertNativeHarnessArgs(command []string, harness string, extra []string) []string {
	if len(extra) == 0 || len(command) == 0 {
		return command
	}
	at := 1
	if _, _, _, _, ok := splitAgentComposeLaunch(command); ok {
		at = 4
	} else if strings.TrimSuffix(filepath.Base(command[0]), filepath.Ext(command[0])) != harness {
		return command
	}
	out := make([]string, 0, len(command)+len(extra))
	out = append(out, command[:at]...)
	out = append(out, extra...)
	return append(out, command[at:]...)
}
