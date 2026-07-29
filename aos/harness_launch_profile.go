package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

const harnessLaunchProfilesFormat = "agentic-os.harness-launch-profiles.v1"

//go:embed harness_launch_profiles.json
var embeddedHarnessLaunchProfiles []byte

type harnessLaunchProfileDocument struct {
	Format   string                                     `json:"format"`
	Defaults map[string]harnessLaunchProfile            `json:"defaults"`
	Roles    map[string]map[string]harnessLaunchProfile `json:"roles"`
}

type harnessLaunchProfile struct {
	Model           string `json:"model"`
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
	Verbosity       string `json:"verbosity,omitempty"`
	Endpoint        string `json:"endpoint,omitempty"`
}

func loadHarnessLaunchProfiles(data []byte) (harnessLaunchProfileDocument, error) {
	var document harnessLaunchProfileDocument
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return harnessLaunchProfileDocument{}, fmt.Errorf("decode harness launch profiles: %w", err)
	}
	if err := ensureJSONEnd(decoder); err != nil {
		return harnessLaunchProfileDocument{}, fmt.Errorf("decode harness launch profiles trailer: %w", err)
	}
	if document.Format != harnessLaunchProfilesFormat {
		return harnessLaunchProfileDocument{}, fmt.Errorf(
			"unsupported harness launch profile format %q",
			document.Format,
		)
	}
	if len(document.Defaults) == 0 {
		return harnessLaunchProfileDocument{}, fmt.Errorf("harness launch profile defaults are empty")
	}
	for harness, profile := range document.Defaults {
		if err := validateHarnessLaunchProfile("default", harness, profile); err != nil {
			return harnessLaunchProfileDocument{}, err
		}
	}
	for role, profiles := range document.Roles {
		if !safeRoleSlug(role) {
			return harnessLaunchProfileDocument{}, fmt.Errorf(
				"harness launch profile registry has unsafe role %q",
				role,
			)
		}
		for harness, profile := range profiles {
			if err := validateHarnessLaunchProfile(role, harness, profile); err != nil {
				return harnessLaunchProfileDocument{}, err
			}
		}
	}
	return document, nil
}

func validateHarnessLaunchProfile(role, harness string, profile harnessLaunchProfile) error {
	switch harness {
	case "claude", "codex", "goose", "opencode":
	default:
		return fmt.Errorf("harness launch profile %s has unsupported harness %q", role, harness)
	}
	if strings.TrimSpace(profile.Model) == "" {
		return fmt.Errorf("harness launch profile %s/%s has an empty model", role, harness)
	}
	return nil
}

func harnessLaunchProfileFor(role, harness string) (harnessLaunchProfile, error) {
	document, err := loadHarnessLaunchProfiles(embeddedHarnessLaunchProfiles)
	if err != nil {
		return harnessLaunchProfile{}, err
	}
	profile, ok := document.Defaults[harness]
	if !ok {
		return harnessLaunchProfile{}, fmt.Errorf("AOS has no default launch profile for %s", harness)
	}
	if override, ok := document.Roles[role][harness]; ok {
		if override.Model != "" {
			profile.Model = override.Model
		}
		if override.ReasoningEffort != "" {
			profile.ReasoningEffort = override.ReasoningEffort
		}
		if override.Verbosity != "" {
			profile.Verbosity = override.Verbosity
		}
		if override.Endpoint != "" {
			profile.Endpoint = override.Endpoint
		}
	}
	return profile, nil
}

func wardLaunchInputsFor(role, harness string) ([]string, error) {
	profile, err := harnessLaunchProfileFor(role, harness)
	if err != nil {
		return nil, err
	}
	var pairs [][2]string
	switch harness {
	case "claude":
		pairs = [][2]string{
			{"agent.claude.model", profile.Model},
			{"agent.claude.effort", profile.ReasoningEffort},
		}
	case "codex":
		pairs = [][2]string{
			{"agent.codex.model", profile.Model},
			{"agent.codex.effort", profile.ReasoningEffort},
			{"agent.codex.verbosity", profile.Verbosity},
		}
	case "goose":
		pairs = [][2]string{{"agent.goose.model", profile.Model}}
	case "opencode":
		pairs = [][2]string{
			{"agent.opencode.model", profile.Model},
			{"agent.opencode.endpoint", profile.Endpoint},
		}
	}
	inputs := make([]string, 0, len(pairs)*2)
	for _, pair := range pairs {
		if strings.TrimSpace(pair[1]) == "" {
			continue
		}
		inputs = append(inputs, "--config", pair[0]+"="+pair[1])
	}
	return inputs, nil
}
