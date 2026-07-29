package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeCommandRunner struct {
	commands   []string
	request    string
	composeErr error
}

func (f *fakeCommandRunner) Run(_ context.Context, name string, args ...string) error {
	f.commands = append(f.commands, strings.Join(append([]string{name}, args...), " "))
	if name == "git" && len(args) >= 4 && args[0] == "clone" && args[1] == "--mirror" {
		return os.MkdirAll(args[3], 0o755)
	}
	if name == "git" && len(args) >= 4 && args[0] == "clone" && args[1] == "--no-hardlinks" {
		destination := args[3]
		if err := os.MkdirAll(destination, 0o755); err != nil {
			return err
		}
		return nil
	}
	if name == "agent-compose" && len(args) >= 4 && args[0] == "compose" {
		data, err := os.ReadFile(args[1])
		if err != nil {
			return err
		}
		f.request = string(data)
		if f.composeErr != nil {
			return f.composeErr
		}
		out := args[3]
		bundle := filepath.Join(out, "0123456789abcdef")
		if err := os.MkdirAll(bundle, 0o755); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(bundle, "manifest.json"), []byte("{}\n"), 0o644)
	}
	if name == "agent-compose" && len(args) >= 8 && args[0] == "project" {
		target := args[7]
		if err := os.MkdirAll(filepath.Join(target, ".codex"), 0o755); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(target, ".codex", "AGENTS.md"), []byte("composed\n"), 0o644)
	}
	return nil
}

func TestComposeHomeSurfacesRoleCompatibilityFailure(t *testing.T) {
	t.Parallel()
	runner := &fakeCommandRunner{
		composeErr: errors.New(`role "ceo" requires a frontier model`),
	}
	opts := bootstrapOptions{
		Role:            "ceo",
		Layout:          "goose",
		Delivery:        "native-skills",
		AgentHome:       t.TempDir(),
		AgentComposeBin: "agent-compose",
	}
	err := composeHome(context.Background(), opts, t.TempDir(), runner)
	if err == nil || !strings.Contains(
		err.Error(),
		`compose role ceo: role "ceo" requires a frontier model`,
	) {
		t.Fatalf("compose error = %v", err)
	}
	modelClass, modelErr := modelClassForLayout(opts.Layout)
	if modelErr != nil {
		t.Fatal(modelErr)
	}
	if !strings.Contains(runner.request, `model-class "`+modelClass+`"`) {
		t.Fatalf("compose request omitted selected model class:\n%s", runner.request)
	}
}

func TestLoadSubstrateRepos(t *testing.T) {
	t.Parallel()
	manifest := filepath.Join(t.TempDir(), "repos.txt")
	if err := os.WriteFile(manifest, []byte(`
# public references
coilyco-flight-deck/agentic-os
coilyco-flight-deck/ward
`), 0o644); err != nil {
		t.Fatal(err)
	}
	repos, err := loadSubstrateRepos(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 2 || repos[0].MirrorName() != "coilyco-flight-deck__agentic-os.git" {
		t.Fatalf("unexpected substrate repos: %+v", repos)
	}
}

func TestLoadSubstrateReposRejectsTraversalAndDuplicates(t *testing.T) {
	t.Parallel()
	for _, body := range []string{
		"../agentic-os\n",
		"coilyco-flight-deck/agentic-os\ncoilyco-flight-deck/agentic-os\n",
		"owner/name cache\n",
	} {
		manifest := filepath.Join(t.TempDir(), "repos.txt")
		if err := os.WriteFile(manifest, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := loadSubstrateRepos(manifest); err == nil {
			t.Fatalf("manifest %q passed validation", body)
		}
	}
}

func TestPrepareContainerHydratesSubstrateAndProjectsHome(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	manifest := filepath.Join(root, "repos.txt")
	seed := filepath.Join(root, "seed")
	cache := filepath.Join(root, "cache")
	substrate := filepath.Join(root, "substrate")
	home := filepath.Join(root, "home")
	t.Cleanup(func() {
		_ = filepath.WalkDir(substrate, func(path string, _ os.DirEntry, _ error) error {
			_ = os.Chmod(path, 0o755)
			return nil
		})
	})
	for _, ref := range []string{
		"coilyco-flight-deck/agentic-os",
		"coilyco-flight-deck/ward",
	} {
		parts := strings.Split(ref, "/")
		mirror := filepath.Join(seed, parts[0]+"__"+parts[1]+".git")
		if err := os.MkdirAll(mirror, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(
		manifest,
		[]byte("coilyco-flight-deck/agentic-os\ncoilyco-flight-deck/ward\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	runner := &fakeCommandRunner{}
	uid, gid := hostIdentity()
	spec, err := prepareContainer(context.Background(), bootstrapOptions{
		Role:              "engineer",
		Layout:            "codex",
		Delivery:          "native-skills",
		Composed:          true,
		Workspace:         filepath.Join(root, "workspace"),
		UID:               uid,
		GID:               gid,
		Command:           []string{"codex", "exec", "task"},
		SubstrateManifest: manifest,
		SubstrateSeed:     seed,
		SubstrateCache:    cache,
		SubstrateRoot:     substrate,
		AgentHome:         home,
	}, runner)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(spec.Command, " ") != "codex exec task" {
		t.Fatalf("exec command = %q", spec.Command)
	}
	if _, err := os.Stat(filepath.Join(home, ".codex", "AGENTS.md")); err != nil {
		t.Fatalf("composed Codex HOME is absent: %v", err)
	}
	config, err := os.ReadFile(filepath.Join(home, ".codex", "config.toml"))
	if err != nil {
		t.Fatalf("Codex container defaults are absent: %v", err)
	}
	profile, err := standaloneHarnessLaunchProfileFor("engineer", "codex")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"model = " + tomlBasicString(profile.Model),
		"model_reasoning_effort = " + tomlBasicString(profile.ReasoningEffort),
		"model_verbosity = " + tomlBasicString(profile.Verbosity),
		`approval_policy = "never"`,
		`sandbox_mode = "danger-full-access"`,
		`[notice]`,
		`hide_rate_limit_model_nudge = true`,
		"[projects." + tomlBasicString(filepath.Join(root, "workspace")) + "]",
		`trust_level = "trusted"`,
	} {
		if !strings.Contains(string(config), want) {
			t.Errorf("Codex config missing %q:\n%s", want, config)
		}
	}
	for _, path := range []string{
		filepath.Join(substrate, "coilyco-flight-deck", "agentic-os"),
		filepath.Join(substrate, "coilyco-flight-deck", "ward"),
	} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0o222 != 0 {
			t.Fatalf("substrate path remained writable: %s %o", path, info.Mode().Perm())
		}
	}
	modelClass, err := modelClassForLayout("codex")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`role "engineer"`,
		`delivery "native-skills"`,
		`model-class "` + modelClass + `"`,
		`source "aos-public" root="." required=#true`,
	} {
		if !strings.Contains(runner.request, want) {
			t.Errorf("compose request missing %q:\n%s", want, runner.request)
		}
	}
	if !environmentContains(spec.Environment, "HOME="+home) {
		t.Fatalf("exec environment omitted HOME: %v", spec.Environment)
	}
	joined := strings.Join(runner.commands, "\n")
	for _, want := range []string{
		"agent-compose compose",
		"agent-compose verify",
		"agent-compose project",
		"--scope home",
		"no-push://substrate",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("bootstrap commands omitted %q:\n%s", want, joined)
		}
	}
	for _, command := range runner.commands {
		if strings.HasPrefix(command, "ward ") || command == "ward" {
			t.Fatalf("Ward command leaked into container bootstrap:\n%s", joined)
		}
	}
}

func TestModelClassForLayout(t *testing.T) {
	t.Parallel()
	layouts, err := loadLayoutModelClasses(embeddedLayoutModelClasses)
	if err != nil {
		t.Fatal(err)
	}
	for layout, want := range layouts {
		got, err := modelClassForLayout(layout)
		if err != nil {
			t.Fatalf("modelClassForLayout(%q): %v", layout, err)
		}
		if got != want {
			t.Fatalf("modelClassForLayout(%q) = %q, want %q", layout, got, want)
		}
	}
	if _, err := modelClassForLayout("unknown"); err == nil {
		t.Fatal("modelClassForLayout accepted an unknown layout")
	}
}

func TestLoadLayoutModelClassesRejectsMalformedRegistry(t *testing.T) {
	t.Parallel()
	for _, data := range [][]byte{
		[]byte(`{}`),
		[]byte(`{"format":"agentic-os.layout-model-classes.v1","layouts":{}}`),
		[]byte(`{"format":"agentic-os.layout-model-classes.v1","layouts":{"test":"unknown"}}`),
		[]byte(`{"format":"agentic-os.layout-model-classes.v1","layouts":{"test":"frontier"}} trailing`),
	} {
		if _, err := loadLayoutModelClasses(data); err == nil {
			t.Fatalf("loadLayoutModelClasses accepted %q", data)
		}
	}
}

func TestStageHarnessDefaultsEscapesCodexWorkspaceAndIgnoresOtherLayouts(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	workspace := `/workspace/repo\"quoted`
	if err := stageHarnessDefaults("engineer", "codex", home, workspace); err != nil {
		t.Fatal(err)
	}
	config, err := os.ReadFile(filepath.Join(home, ".codex", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`model = "`,
		`model_reasoning_effort = "`,
		`model_verbosity = "`,
		`hide_rate_limit_model_nudge = true`,
		`[projects."/workspace/repo\\\"quoted"]`,
	} {
		if !strings.Contains(string(config), want) {
			t.Errorf("Codex config missing %q:\n%s", want, config)
		}
	}

	otherHome := t.TempDir()
	if err := stageHarnessDefaults("engineer", "claude", otherHome, workspace); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(otherHome, ".codex", "config.toml")); !os.IsNotExist(err) {
		t.Fatalf("non-Codex layout received Codex defaults: %v", err)
	}
}

func TestPrepareContainerWithoutSubstrateStillMaterializesProvider(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	manifest := filepath.Join(root, "repos.txt")
	seed := filepath.Join(root, "seed")
	for _, name := range []string{"agentic-os", "ward"} {
		if err := os.MkdirAll(
			filepath.Join(seed, "coilyco-flight-deck__"+name+".git"),
			0o755,
		); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(
		manifest,
		[]byte("coilyco-flight-deck/agentic-os\ncoilyco-flight-deck/ward\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	runner := &fakeCommandRunner{}
	uid, gid := hostIdentity()
	_, err := prepareContainer(context.Background(), bootstrapOptions{
		Role:              "advisor",
		Layout:            "codex",
		Delivery:          "compiled",
		Composed:          true,
		Workspace:         filepath.Join(root, "workspace"),
		UID:               uid,
		GID:               gid,
		Command:           []string{"codex"},
		NoSubstrate:       true,
		SubstrateManifest: manifest,
		SubstrateSeed:     seed,
		SubstrateCache:    filepath.Join(root, "cache"),
		SubstrateRoot:     filepath.Join(root, "substrate"),
		AgentHome:         filepath.Join(root, "home"),
	}, runner)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(runner.commands, "\n")
	if strings.Contains(joined, "coilyco-flight-deck__ward.git") {
		t.Fatalf("--no-substrate materialized Ward:\n%s", joined)
	}
	if !strings.Contains(joined, "coilyco-flight-deck__agentic-os.git") {
		t.Fatalf("--no-substrate omitted the required provider:\n%s", joined)
	}
}

func TestFindSingleBundleFailsClosed(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if _, err := findSingleBundle(root); err == nil {
		t.Fatal("empty bundle cache passed")
	}
	for _, name := range []string{"one", "two"} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "manifest.json"), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := findSingleBundle(root); err == nil {
		t.Fatal("ambiguous bundle cache passed")
	}
}

func environmentContains(environment []string, want string) bool {
	for _, value := range environment {
		if value == want {
			return true
		}
	}
	return false
}
