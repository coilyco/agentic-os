package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
)

func writeRepositoryPlan(t *testing.T, root string, identities ...string) string {
	t.Helper()
	selections := make([]aosRepositorySelection, 0, len(identities))
	for _, identity := range identities {
		selections = append(selections, aosRepositorySelection{
			Identity: identity,
			Path:     filepath.Join(root, filepath.FromSlash(identity)),
			Source:   "test", Scope: "role-union", Reason: "test selection",
		})
	}
	plan := aosRepositoryPlan{
		Format: agentComposeRepositoryPlanYAMLFormat, ProjectsRoot: root,
		Inputs: []aosRepositoryPlanInput{{
			Identity: "owner/policy", Revision: "0123456789abcdef",
			Policy: aosRepositoryPolicyInput{Path: ".agents/roles.kdl", SHA256: "sha256:test"},
		}},
		Roles:     map[string][]aosRepositorySelection{"platform": selections},
		Residency: selections,
	}
	raw, err := yaml.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "repository-plan.yaml")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeLegacyRepositoryPlan(t *testing.T, root string, identities ...string) string {
	t.Helper()
	path := writeRepositoryPlan(t, root, identities...)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var plan aosRepositoryPlan
	if err := yaml.Unmarshal(raw, &plan); err != nil {
		t.Fatal(err)
	}
	plan.Format = agentComposeRepositoryPlanJSONFormat
	raw, err = json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	path = filepath.Join(t.TempDir(), "repository-plan.json")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadAOSRepositoryPlanRejectsUnsortedResidency(t *testing.T) {
	root := t.TempDir()
	path := writeRepositoryPlan(t, root, "owner/two", "owner/one")
	_, err := loadAOSRepositoryPlan(path)
	if err == nil || !strings.Contains(err.Error(), "unsorted") {
		t.Fatalf("unsorted plan error = %v", err)
	}
}

func TestLoadAOSRepositoryPlanRejectsUnknownFields(t *testing.T) {
	root := t.TempDir()
	path := writeRepositoryPlan(t, root, "owner/one")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	raw = append([]byte("unknown: true\n"), raw...)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadAOSRepositoryPlan(path); err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("unknown-field error = %v", err)
	}
}

func TestLoadAOSRepositoryPlanAcceptsLegacyJSON(t *testing.T) {
	root := t.TempDir()
	path := writeLegacyRepositoryPlan(t, root, "owner/one")
	plan, err := loadAOSRepositoryPlan(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := plan.Residency[0].Identity; got != "owner/one" {
		t.Fatalf("legacy residency identity = %q", got)
	}
}

func TestRepositoryPlanPathPrefersYAMLAndFallsBackToJSON(t *testing.T) {
	home := t.TempDir()
	planDir := filepath.Join(home, ".agent-compose")
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatal(err)
	}
	yamlPath := filepath.Join(planDir, "repository-plan.yaml")
	jsonPath := filepath.Join(planDir, "repository-plan.json")
	if got := repositoryPlanPath(home); got != yamlPath {
		t.Fatalf("missing plan path = %q, want %q", got, yamlPath)
	}
	if err := os.WriteFile(jsonPath, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := repositoryPlanPath(home); got != jsonPath {
		t.Fatalf("legacy plan path = %q, want %q", got, jsonPath)
	}
	if err := os.WriteFile(yamlPath, []byte("format: test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := repositoryPlanPath(home); got != yamlPath {
		t.Fatalf("current plan path = %q, want %q", got, yamlPath)
	}
}

func TestRepositoriesCommandEmitsCompiledResidency(t *testing.T) {
	root := t.TempDir()
	path := writeRepositoryPlan(t, root, "owner/one", "owner/two")
	command := newCommand()
	var output bytes.Buffer
	command.Writer = &output
	if err := command.Run(
		context.Background(),
		[]string{"aos", "repositories", "--plan", path, "--format", "lines"},
	); err != nil {
		t.Fatal(err)
	}
	if got := output.String(); got != "owner/one\nowner/two\n" {
		t.Fatalf("lines output = %q", got)
	}
}

// Literal YAML in the PRODUCER's shape: marshalling this package's own struct
// would prove only that it round-trips itself, and the struct was wrong (#1445).
func TestLoadAOSRepositoryPlanAcceptsBindingSkills(t *testing.T) {
	root := t.TempDir()
	plan := "format: " + agentComposeRepositoryPlanYAMLFormat + "\n" +
		"projects_root: " + root + "\n" +
		"inputs:\n" +
		"  - identity: owner/policy\n" +
		"    revision: 0123456789abcdef\n" +
		"    policy:\n" +
		"      path: .agents/roles.kdl\n" +
		"      sha256: sha256:test\n" +
		"roles:\n" +
		"  advocate:\n" +
		"    - identity: owner/lore\n" +
		"      path: " + filepath.Join(root, "owner", "lore") + "\n" +
		"      source: owner/policy\n" +
		"      scope: provider\n" +
		"      reason: role \"advocate\" uses skill-provider repository \"lore\"\n" +
		"      required: true\n" +
		"      skills:\n" +
		"        - '*'\n" +
		"      binding_skills:\n" +
		"        - lore-method-*\n" +
		"        - lore-self-compensation\n" +
		"      name: lore\n" +
		"      declared_by: owner/policy\n" +
		"residency:\n" +
		"  - identity: owner/lore\n" +
		"    path: " + filepath.Join(root, "owner", "lore") + "\n" +
		"    source: owner/policy\n" +
		"    scope: provider\n" +
		"    reason: role \"advocate\" uses skill-provider repository \"lore\"\n"
	path := filepath.Join(t.TempDir(), "repository-plan.yaml")
	if err := os.WriteFile(path, []byte(plan), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadAOSRepositoryPlan(path)
	if err != nil {
		t.Fatalf("binding_skills rejected: %v", err)
	}
	selection := loaded.Roles["advocate"][0]
	// Dropped rather than carried would still pass the load above.
	if len(selection.BindingSkills) != 2 || selection.BindingSkills[0] != "lore-method-*" {
		t.Errorf("BindingSkills = %v, want both entries in order", selection.BindingSkills)
	}
	if len(selection.Skills) != 1 || selection.Skills[0] != "*" {
		t.Errorf("Skills = %v, want the wildcard beside it", selection.Skills)
	}
	if selection.Name != "lore" || !selection.Required {
		t.Errorf("selection = %+v, want the provider fields preserved", selection)
	}
}
