package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/urfave/cli/v3"
)

const (
	agentComposeRepositoryPlanJSONFormat = "agent-compose.repositories.v1"
	agentComposeRepositoryPlanYAMLFormat = "agent-compose.repositories.v2"
	aosRepositoryResidencyFormat         = "aos.repository-residency.v1"
)

type aosRepositorySelection struct {
	Identity   string   `json:"identity" yaml:"identity"`
	Path       string   `json:"path" yaml:"path"`
	Source     string   `json:"source" yaml:"source"`
	Scope      string   `json:"scope" yaml:"scope"`
	Reason     string   `json:"reason" yaml:"reason"`
	Required   bool     `json:"required,omitempty" yaml:"required,omitempty"`
	Skills     []string `json:"skills,omitempty" yaml:"skills,omitempty"`
	Name       string   `json:"name,omitempty" yaml:"name,omitempty"`
	DeclaredBy string   `json:"declared_by,omitempty" yaml:"declared_by,omitempty"`
	// Carried, never read: the skill split is the composer's, and the strict
	// decode below rejects the whole plan without it (#1445).
	BindingSkills []string `json:"binding_skills,omitempty" yaml:"binding_skills,omitempty"`
}

type aosRepositoryPolicyInput struct {
	Path   string `yaml:"path"`
	SHA256 string `yaml:"sha256"`
}

type aosRepositoryPlanInput struct {
	Identity string                   `yaml:"identity"`
	Revision string                   `yaml:"revision"`
	Policy   aosRepositoryPolicyInput `yaml:"policy"`
}

type aosRepositoryPlan struct {
	Format       string                              `json:"format" yaml:"format"`
	ProjectsRoot string                              `json:"projects_root" yaml:"projects_root"`
	Inputs       []aosRepositoryPlanInput            `json:"-" yaml:"inputs,omitempty"`
	Roles        map[string][]aosRepositorySelection `json:"roles" yaml:"roles"`
	Residency    []aosRepositorySelection            `json:"residency" yaml:"residency"`
	// Legacy and Unverified travel with the plan rather than beside it, so a
	// caller cannot hold a plan and forget how much of it was proven.
	Legacy     bool `json:"-" yaml:"-"`
	Unverified bool `json:"-" yaml:"-"`
}

func defaultRepositoryPlanPath() string {
	if configured := strings.TrimSpace(os.Getenv("AOS_REPOSITORY_PLAN")); configured != "" {
		return configured
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".agent-compose", "repository-plan.yaml")
	}
	return repositoryPlanPath(home)
}

func repositoryPlanPath(home string) string {
	yamlPath := filepath.Join(home, ".agent-compose", "repository-plan.yaml")
	if _, err := os.Stat(yamlPath); err == nil || !errors.Is(err, os.ErrNotExist) {
		return yamlPath
	}
	jsonPath := filepath.Join(home, ".agent-compose", "repository-plan.json")
	if _, err := os.Stat(jsonPath); err == nil || !errors.Is(err, os.ErrNotExist) {
		return jsonPath
	}
	return yamlPath
}

func loadAOSRepositoryPlan(filename string) (aosRepositoryPlan, error) {
	raw, err := os.ReadFile(filename)
	if err != nil {
		return aosRepositoryPlan{}, fmt.Errorf("read Agent Compose repository plan %s: %w", filename, err)
	}
	var plan aosRepositoryPlan
	expectedFormat := agentComposeRepositoryPlanYAMLFormat
	if trimmed := bytes.TrimSpace(raw); len(trimmed) > 0 && trimmed[0] == '{' {
		expectedFormat = agentComposeRepositoryPlanJSONFormat
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&plan); err != nil {
			return aosRepositoryPlan{}, fmt.Errorf("decode Agent Compose repository plan %s: %w", filename, err)
		}
		var trailing json.RawMessage
		if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
			return aosRepositoryPlan{}, fmt.Errorf("decode Agent Compose repository plan %s: trailing JSON", filename)
		}
	} else if err := yaml.UnmarshalWithOptions(raw, &plan, yaml.Strict()); err != nil {
		return aosRepositoryPlan{}, fmt.Errorf("decode Agent Compose repository plan %s: %w", filename, err)
	}
	if plan.Format != expectedFormat {
		return aosRepositoryPlan{}, fmt.Errorf("Agent Compose repository plan format is %q, want %q", plan.Format, expectedFormat)
	}
	if !filepath.IsAbs(plan.ProjectsRoot) {
		return aosRepositoryPlan{}, fmt.Errorf("Agent Compose repository plan projects_root must be absolute")
	}
	if err := validateAOSRepositorySelections(plan.ProjectsRoot, "residency", plan.Residency); err != nil {
		return aosRepositoryPlan{}, err
	}
	for role, selections := range plan.Roles {
		if strings.TrimSpace(role) == "" {
			return aosRepositoryPlan{}, fmt.Errorf("Agent Compose repository plan contains an empty role")
		}
		if err := validateAOSRepositorySelections(plan.ProjectsRoot, "role "+role, selections); err != nil {
			return aosRepositoryPlan{}, err
		}
	}
	plan.Legacy = expectedFormat == agentComposeRepositoryPlanJSONFormat
	plan.Unverified = plan.Legacy
	return plan, nil
}

func validateAOSRepositorySelections(root, owner string, selections []aosRepositorySelection) error {
	prior := ""
	for _, selection := range selections {
		parts := strings.Split(selection.Identity, "/")
		if len(parts) != 2 || !safePathSegment(parts[0]) || !safePathSegment(parts[1]) || selection.Identity <= prior {
			return fmt.Errorf("Agent Compose repository plan %s has invalid, unsorted, or duplicate identity %q", owner, selection.Identity)
		}
		prior = selection.Identity
		if !filepath.IsAbs(selection.Path) || selection.Source == "" || selection.Scope == "" || selection.Reason == "" {
			return fmt.Errorf("Agent Compose repository plan %s repository %q lacks path or provenance", owner, selection.Identity)
		}
		rel, err := filepath.Rel(root, filepath.Clean(selection.Path))
		if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("Agent Compose repository plan %s repository %q is outside projects_root", owner, selection.Identity)
		}
	}
	return nil
}

func runRepositories(_ context.Context, cmd *cli.Command) error {
	plan, err := loadAOSRepositoryPlan(cmd.String("plan"))
	if err != nil {
		return err
	}
	switch cmd.String("format") {
	case "lines":
		for _, repository := range plan.Residency {
			fmt.Fprintln(cmd.Root().Writer, repository.Identity)
		}
		return nil
	case "json":
		payload := struct {
			Format       string                   `json:"format"`
			ProjectsRoot string                   `json:"projects_root"`
			Repositories []aosRepositorySelection `json:"repositories"`
		}{
			Format: aosRepositoryResidencyFormat, ProjectsRoot: plan.ProjectsRoot,
			Repositories: plan.Residency,
		}
		encoder := json.NewEncoder(cmd.Root().Writer)
		encoder.SetIndent("", "  ")
		return encoder.Encode(payload)
	default:
		return fmt.Errorf("repositories --format must be json or lines")
	}
}
