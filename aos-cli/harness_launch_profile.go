package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/goccy/go-yaml"
)

const harnessLaunchProfilesRelativePath = ".agents/harness-launch-profiles.yaml"

var compiledHarnessLaunchProfilesBase64 string

var errHarnessLaunchProfilesMissing = errors.New("harness launch profiles missing")

type harnessLaunchProfileDocument struct {
	Roles         map[string]harnessLaunchRole `yaml:"roles"`
	Permissions   map[string]string            `yaml:"permissions"`
	DefaultAgents map[string]string            `yaml:"-"`
}

type harnessLaunchRole struct {
	Agent     string                         `yaml:"agent"`
	Harnesses map[string]harnessModelProfile `yaml:"harnesses"`
}

func loadHarnessLaunchProfiles(data []byte) (harnessLaunchProfileDocument, error) {
	var document harnessLaunchProfileDocument
	if err := yaml.UnmarshalWithOptions(data, &document, yaml.Strict()); err != nil {
		return harnessLaunchProfileDocument{}, fmt.Errorf("decode harness launch profiles: %w", err)
	}
	if len(document.Roles) == 0 {
		return harnessLaunchProfileDocument{}, fmt.Errorf("harness launch profile roles are empty")
	}
	if err := validateHarnessPermissions(document.Permissions); err != nil {
		return harnessLaunchProfileDocument{}, err
	}
	document.DefaultAgents = make(map[string]string, len(document.Roles))
	for role, profile := range document.Roles {
		if !safeRoleSlug(role) {
			return harnessLaunchProfileDocument{}, fmt.Errorf(
				"harness launch profile registry has unsafe role %q",
				role,
			)
		}
		agent := strings.TrimSpace(profile.Agent)
		if !isSupportedHarness(agent) {
			return harnessLaunchProfileDocument{}, fmt.Errorf(
				"harness launch profile role %s has unsupported agent %q",
				role,
				profile.Agent,
			)
		}
		harnesses := make(map[string]harnessModelProfile, len(profile.Harnesses))
		for harness, model := range profile.Harnesses {
			validated, err := validateHarnessModelProfile(role, harness, model)
			if err != nil {
				return harnessLaunchProfileDocument{}, err
			}
			harnesses[harness] = validated
		}
		document.Roles[role] = harnessLaunchRole{Agent: agent, Harnesses: harnesses}
		document.DefaultAgents[role] = agent
	}
	return document, nil
}

type harnessLaunchProfilesSource struct {
	label string
	data  []byte
}

func loadConfiguredHarnessLaunchProfiles() (harnessLaunchProfileDocument, error) {
	source, err := resolveHarnessLaunchProfilesSource()
	if err != nil {
		return harnessLaunchProfileDocument{}, err
	}
	document, err := loadHarnessLaunchProfiles(source.data)
	if err != nil {
		return harnessLaunchProfileDocument{}, fmt.Errorf("load %s: %w", source.label, err)
	}
	return document, nil
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
		"%w: AOS could not find %s from the working directory or executable path",
		errHarnessLaunchProfilesMissing,
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
	var paths []string
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

// runLaunchAgent prints the agent a role launches on. The shell asks rather than
// parsing the profiles itself, so this file stays the one loader. See docs/aterm.md.
func runLaunchAgent(_ context.Context, cmd *cli.Command) error {
	role := strings.TrimSpace(cmd.Args().First())
	if role == "" {
		return fmt.Errorf("_launch-agent needs a role")
	}
	agent, err := standaloneDefaultAgentForRole(role)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(cmd.Root().Writer, agent)
	return err
}

func standaloneDefaultAgentForRole(role string) (string, error) {
	if !safeRoleSlug(role) {
		return "", fmt.Errorf("AOS has no default agent for unsafe role %q", role)
	}
	document, err := loadConfiguredHarnessLaunchProfiles()
	if err != nil {
		return "", err
	}
	agent, ok := document.DefaultAgents[role]
	if !ok {
		return "", fmt.Errorf("AOS has no default agent for role %s; add --agent", role)
	}
	return agent, nil
}
