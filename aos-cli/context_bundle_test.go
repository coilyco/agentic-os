package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildContextBundlePlanUsesAOSImageAsMaterializer(t *testing.T) {
	t.Parallel()
	issuePins := filepath.Join(t.TempDir(), "issue-pins.md")
	if err := os.WriteFile(issuePins, []byte("pins\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	digest := strings.Repeat("a", 64)
	plan, err := buildContextBundlePlan(contextBundlePlanOptions{
		Image:    "aos:test",
		Role:     "platform",
		Agent:    "codex",
		Delivery: "native-skills",
		Composed: true,
		Guarded:  true,
		Output:   "/cache/staging",
		UID:      1000,
		GID:      1000,
		Bundle:   "/cache/bundles/verified",
		IssuePinContext: issuePinLaunchContext{
			HostPath: issuePins,
			Digest:   digest,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(plan.DockerArgs, "\n")
	for _, want := range []string{
		"type=bind,source=/cache/staging,target=/output",
		"type=bind,source=/cache/bundles/verified,target=/opt/agent-compose-bundle,readonly",
		"type=bind,source=" + issuePins + ",target=" + containerIssuePinContext + ",readonly",
		"--entrypoint\n/usr/local/bin/aos",
		"aos:test",
		"--role\nplatform",
		"--agent\ncodex",
		"--composed",
		"--guarded",
		"_container-context-bundle",
		"--issue-pin-context\n" + containerIssuePinContext,
		"--issue-pin-digest\n" + digest,
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("context materialization plan missing %q:\n%s", want, joined)
		}
	}
}

func TestValidateContextBundleAcceptsIssuePinProvenance(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "home", ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "home", ".codex", "AGENTS.md"), []byte("context\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	digest := strings.Repeat("a", 64)
	manifest := `{"format":"ward.context-bundle.v1","role":"platform","agent":"codex","repositories":["owner/one"],"issue_pins":{"digest":"` + digest + `"}}`
	if err := os.WriteFile(filepath.Join(root, contextBundleManifestName), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateContextBundleOutput(root, contextBundleMaterializeOptions{
		Role: "platform", Agent: "codex", Composed: true,
	}); err != nil {
		t.Fatal(err)
	}
	manifest = `{"format":"ward.context-bundle.v1","role":"platform","agent":"codex","repositories":["owner/one"],"issue_pins":{"digest":"not-hex"}}`
	if err := os.WriteFile(filepath.Join(root, contextBundleManifestName), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateContextBundleOutput(root, contextBundleMaterializeOptions{
		Role: "platform", Agent: "codex", Composed: true,
	}); err == nil {
		t.Fatal("invalid issue-pin digest passed validation")
	}
}

func TestContainerContextBundleAcceptsVerifiedBundleFlag(t *testing.T) {
	t.Parallel()
	command := newCommand()
	err := command.Run(context.Background(), []string{
		"aos",
		"--role", "platform",
		"--agent", "codex",
		"--layout", "codex",
		"--composed",
		"_container-context-bundle",
		"--output", "/output",
		"--uid", "1000",
		"--gid", "1000",
		"--bundle", containerComposeBundle,
	})
	if err == nil || !strings.Contains(err.Error(), "_container-context-bundle is internal") {
		t.Fatalf("container context-bundle parser error = %v", err)
	}
}

func TestStageAOSGuardContextCreatesOnlySelectedLoadPoints(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	source := filepath.Join(root, "source")
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(filepath.Join(source, "references"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("---\nname: aosguard\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "references", "commands.yaml"), []byte("commands: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := stageAOSGuardContext("codex", "platform", home, source); err != nil {
		t.Fatal(err)
	}
	if err := validateStagedHome(home, "codex"); err != nil {
		t.Fatal(err)
	}
	instruction, err := os.ReadFile(filepath.Join(home, ".codex", "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(instruction), "grants no authority") {
		t.Fatalf("guarded instruction omitted authority boundary:\n%s", instruction)
	}
	if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "aosguard", "references", "commands.yaml")); err != nil {
		t.Fatalf("generated skill was not staged: %v", err)
	}
}

func TestValidateStagedHomeRejectsProducerBookkeeping(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".codex", "AGENTS.md"), []byte("context\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".agent-compose"), 0o755); err != nil {
		t.Fatal(err)
	}
	err := validateStagedHome(home, "codex")
	if err == nil || !strings.Contains(err.Error(), ".agent-compose") {
		t.Fatalf("producer bookkeeping error = %v", err)
	}
}

func TestValidateContextBundleOutputBindsRoleAndAgent(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "home", ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, "home", ".codex", "AGENTS.md"),
		[]byte("context\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, contextBundleManifestName),
		[]byte("{\"format\":\"ward.context-bundle.v1\",\"role\":\"platform\",\"agent\":\"codex\",\"repositories\":[\"coilyco-flight-deck/agentic-os\"]}\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	expected := contextBundleMaterializeOptions{
		Role: "platform", Agent: "codex", Composed: true,
	}
	if err := validateContextBundleOutput(root, expected); err != nil {
		t.Fatal(err)
	}
	expected.Role = "eval"
	if err := validateContextBundleOutput(root, expected); err == nil {
		t.Fatal("role-mismatched context bundle passed")
	}
}

func TestReadBundleRepositoriesRejectsUnsortedSelection(t *testing.T) {
	t.Parallel()
	bundle := t.TempDir()
	manifest := `{"format":"agent-compose.bundle","role":"platform","repositories":[{"identity":"owner/two"},{"identity":"owner/one"}]}`
	if err := os.WriteFile(filepath.Join(bundle, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := readBundleRepositories(bundle, "platform")
	if err == nil || !strings.Contains(err.Error(), "sorted") {
		t.Fatalf("unsorted repository error = %v", err)
	}
}

func TestValidateContextBundleRejectsRepositoriesWithoutComposition(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "home", ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "home", ".codex", "AGENTS.md"), []byte("context\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := `{"format":"ward.context-bundle.v1","role":"platform","agent":"codex","repositories":["owner/one"]}`
	if err := os.WriteFile(filepath.Join(root, contextBundleManifestName), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	err := validateContextBundleOutput(root, contextBundleMaterializeOptions{
		Role: "platform", Agent: "codex", Composed: false,
	})
	if err == nil || !strings.Contains(err.Error(), "uncomposed") {
		t.Fatalf("uncomposed repository error = %v", err)
	}
}
