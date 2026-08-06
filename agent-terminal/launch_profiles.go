package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const harnessLaunchProfilesRelativePath = ".agents/harness-launch-profiles.yaml"

var compiledHarnessLaunchProfilesBase64 string

type harnessLaunchProfilesSource struct {
	label string
	data  []byte
}

func defaultSeatForRole(role string) (string, error) {
	source, err := resolveHarnessLaunchProfilesSource()
	if err != nil {
		return "", err
	}
	agents, err := parseHarnessLaunchProfiles(source.data)
	if err != nil {
		return "", fmt.Errorf("load %s: %w", source.label, err)
	}
	agent, ok := agents[role]
	if !ok {
		return "", fmt.Errorf("AOS has no default agent for role %s; add a seat", role)
	}
	return agent, nil
}

func resolveHarnessLaunchProfilesSource() (harnessLaunchProfilesSource, error) {
	if path := strings.TrimSpace(os.Getenv("AOS_HARNESS_LAUNCH_PROFILES")); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return harnessLaunchProfilesSource{}, fmt.Errorf("read %s: %w", path, err)
		}
		return harnessLaunchProfilesSource{label: path, data: data}, nil
	}
	for _, path := range harnessLaunchProfileCandidatePaths() {
		data, err := os.ReadFile(path)
		if err == nil {
			return harnessLaunchProfilesSource{label: path, data: data}, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return harnessLaunchProfilesSource{}, fmt.Errorf("read %s: %w", path, err)
		}
	}
	if strings.TrimSpace(compiledHarnessLaunchProfilesBase64) != "" {
		data, err := base64.StdEncoding.DecodeString(compiledHarnessLaunchProfilesBase64)
		if err != nil {
			return harnessLaunchProfilesSource{}, fmt.Errorf("decode compiled harness launch profiles: %w", err)
		}
		return harnessLaunchProfilesSource{label: "compiled harness launch profiles", data: data}, nil
	}
	return harnessLaunchProfilesSource{}, fmt.Errorf(
		"AOS could not find %s from the working directory or executable path",
		harnessLaunchProfilesRelativePath,
	)
}

func harnessLaunchProfileCandidatePaths() []string {
	var starts []string
	if cwd, err := os.Getwd(); err == nil {
		starts = append(starts, cwd)
	}
	if executable, err := os.Executable(); err == nil {
		starts = append(starts, filepath.Dir(executable))
	}
	paths := make([]string, 0)
	seen := make(map[string]bool)
	for _, start := range starts {
		start, err := filepath.Abs(start)
		if err != nil {
			continue
		}
		for {
			path := filepath.Join(start, harnessLaunchProfilesRelativePath)
			if !seen[path] {
				seen[path] = true
				paths = append(paths, path)
			}
			parent := filepath.Dir(start)
			if parent == start {
				break
			}
			start = parent
		}
	}
	return paths
}

func parseHarnessLaunchProfiles(data []byte) (map[string]string, error) {
	roles := make(map[string]bool)
	agents := make(map[string]string)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	inRoles := false
	currentRole := ""
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		raw := strings.TrimRight(scanner.Text(), " \t\r")
		text := strings.TrimSpace(raw)
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		indent := leadingSpaces(raw)
		switch {
		case indent == 0 && text == "roles:":
			inRoles = true
			currentRole = ""
		case indent == 0:
			return nil, fmt.Errorf("line %d: unsupported top-level key %q", lineNumber, text)
		case !inRoles:
			return nil, fmt.Errorf("line %d: nested key before roles", lineNumber)
		case indent == 2 && strings.HasSuffix(text, ":"):
			role := strings.TrimSuffix(text, ":")
			if !safeRoleSlug(role) {
				return nil, fmt.Errorf("line %d: unsafe role %q", lineNumber, role)
			}
			if roles[role] {
				return nil, fmt.Errorf("line %d: duplicate role %q", lineNumber, role)
			}
			roles[role] = true
			currentRole = role
		case indent == 4 && strings.HasPrefix(text, "agent:"):
			if currentRole == "" {
				return nil, fmt.Errorf("line %d: agent without role", lineNumber)
			}
			agent := trimYAMLScalar(strings.TrimSpace(strings.TrimPrefix(text, "agent:")))
			if !isSupportedHarness(agent) {
				return nil, fmt.Errorf("line %d: unsupported agent %q", lineNumber, agent)
			}
			agents[currentRole] = agent
		default:
			return nil, fmt.Errorf("line %d: unsupported harness profile syntax", lineNumber)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(roles) == 0 {
		return nil, fmt.Errorf("harness launch profile roles are empty")
	}
	for role := range roles {
		if agents[role] == "" {
			return nil, fmt.Errorf("role %s has no default agent", role)
		}
	}
	return agents, nil
}

func leadingSpaces(value string) int {
	count := 0
	for count < len(value) && value[count] == ' ' {
		count++
	}
	return count
}

func trimYAMLScalar(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		if (value[0] == '\'' && value[len(value)-1] == '\'') ||
			(value[0] == '"' && value[len(value)-1] == '"') {
			return strings.TrimSpace(value[1 : len(value)-1])
		}
	}
	return value
}

func safeRoleSlug(value string) bool {
	if value == "" || value[0] < 'a' || value[0] > 'z' {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			continue
		}
		return false
	}
	return true
}

func isSupportedHarness(value string) bool {
	switch value {
	case "claude", "codex", "goose", "opencode":
		return true
	default:
		return false
	}
}
