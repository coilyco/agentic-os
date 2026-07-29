package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

const wardLaunchProfilesFormat = "agentic-os.ward-launch-profiles.v1"

//go:embed ward_launch_profiles.json
var embeddedWardLaunchProfiles []byte

type wardLaunchProfileDocument struct {
	Format string                                  `json:"format"`
	Roles  map[string]map[string]wardLaunchProfile `json:"roles"`
}

type wardLaunchProfile struct {
	Model           string `json:"model"`
	ReasoningEffort string `json:"reasoning_effort"`
	Verbosity       string `json:"verbosity"`
}

func loadWardLaunchProfiles(data []byte) (map[string]map[string]wardLaunchProfile, error) {
	var document wardLaunchProfileDocument
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("decode Ward launch profiles: %w", err)
	}
	if err := ensureJSONEnd(decoder); err != nil {
		return nil, fmt.Errorf("decode Ward launch profiles trailer: %w", err)
	}
	if document.Format != wardLaunchProfilesFormat {
		return nil, fmt.Errorf("unsupported Ward launch profile format %q", document.Format)
	}
	if len(document.Roles) == 0 {
		return nil, fmt.Errorf("Ward launch profile registry is empty")
	}
	for role, profiles := range document.Roles {
		if !safeRoleSlug(role) {
			return nil, fmt.Errorf("Ward launch profile registry has unsafe role %q", role)
		}
		for agent, profile := range profiles {
			if strings.TrimSpace(agent) == "" {
				return nil, fmt.Errorf("Ward launch profile registry has an empty agent for role %q", role)
			}
			if strings.TrimSpace(profile.Model) == "" ||
				strings.TrimSpace(profile.ReasoningEffort) == "" ||
				strings.TrimSpace(profile.Verbosity) == "" {
				return nil, fmt.Errorf("Ward launch profile %s/%s has an empty Codex input", role, agent)
			}
		}
	}
	return document.Roles, nil
}

func wardLaunchInputsFor(role, agent string) ([]string, error) {
	if agent != "codex" {
		return nil, nil
	}
	roles, err := loadWardLaunchProfiles(embeddedWardLaunchProfiles)
	if err != nil {
		return nil, err
	}
	profile, ok := roles[role][agent]
	if !ok {
		return nil, fmt.Errorf("AOS has no Ward launch profile for %s/%s", role, agent)
	}
	return []string{
		"--config", "agent.codex.model=" + profile.Model,
		"--config", "agent.codex.effort=" + profile.ReasoningEffort,
		"--config", "agent.codex.verbosity=" + profile.Verbosity,
	}, nil
}
