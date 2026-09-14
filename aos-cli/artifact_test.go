package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func scaffoldInto(t *testing.T, name string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), name)
	if err := artifactScaffold(root, name); err != nil {
		t.Fatalf("scaffold: %v", err)
	}
	return root
}

// Acceptance: the generated tree carries every file a build needs, with the
// vendored kit and its pin beside each other.
func TestArtifactScaffoldWritesACompleteProject(t *testing.T) {
	root := scaffoldInto(t, "demo")
	for _, want := range []string{
		"index.html", "main.js", "app.css", "build.mjs", "package.json",
		"README.md", ".prettierrc", ".prettierignore", ".gitignore",
		"vendor/coilyco-kit.css", "vendor/coilyco-kit.json",
	} {
		if _, err := os.Stat(filepath.Join(root, want)); err != nil {
			t.Errorf("missing %s: %v", want, err)
		}
	}
}

// .k scopes `.k :focus-visible`, so a page without it loses the kit's focus
// treatment entirely. All three classes are load-bearing (website#7645).
func TestArtifactShellCarriesTheAutoCanvas(t *testing.T) {
	root := scaffoldInto(t, "demo")
	html, err := os.ReadFile(filepath.Join(root, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(html), `<body class="k k-canvas k-canvas--auto">`) {
		t.Fatalf("shell does not carry the auto canvas:\n%s", html)
	}
}

// Nothing may reach the website repo, or a CDN, at runtime.
func TestArtifactShellReferencesOnlyLocalRelativeAssets(t *testing.T) {
	root := scaffoldInto(t, "demo")
	html, err := os.ReadFile(filepath.Join(root, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"http://", "https://", `href="/`, `src="/`} {
		if strings.Contains(string(html), forbidden) {
			t.Errorf("shell reaches outside the project: %q", forbidden)
		}
	}
}

func TestArtifactScaffoldSubstitutesTheName(t *testing.T) {
	root := scaffoldInto(t, "my-page")
	raw, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	var pkg struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &pkg); err != nil {
		t.Fatalf("package.json is not valid JSON: %v", err)
	}
	if pkg.Name != "my-page" {
		t.Fatalf("package name = %q, want my-page", pkg.Name)
	}
}

// A half-written project is harder to diagnose than a refusal.
func TestArtifactScaffoldRefusesAnExistingDirectory(t *testing.T) {
	root := scaffoldInto(t, "demo")
	if err := artifactScaffold(root, "demo"); err == nil {
		t.Fatal("scaffolded over an existing directory")
	}
}

func TestArtifactCheckPassesOnAFreshProject(t *testing.T) {
	root := scaffoldInto(t, "demo")
	result, err := artifactCheck(root)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if result.Stale {
		t.Fatal("a freshly scaffolded project reported stale")
	}
}

// The case a count cannot catch, and the reason the check compares hashes:
// one literal swapped for another moves the hash and no count at all.
func TestArtifactCheckReportsStaleWhenOnlyTheHashMoved(t *testing.T) {
	root := scaffoldInto(t, "demo")
	pinPath := filepath.Join(root, "vendor", "coilyco-kit.json")
	raw, err := os.ReadFile(pinPath)
	if err != nil {
		t.Fatal(err)
	}
	var pin kitPin
	if err := json.Unmarshal(raw, &pin); err != nil {
		t.Fatal(err)
	}
	moved := kitPin{Schema: pin.Schema, Hash: "0000000000000000", Count: pin.Count}
	body, err := json.Marshal(moved)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pinPath, body, 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := artifactCheck(root)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if !result.Stale {
		t.Fatal("a moved hash with an unchanged count did not report stale")
	}
}

// Comparing two different schemas is not a staleness answer, so it refuses.
func TestArtifactCheckRefusesAForeignSchema(t *testing.T) {
	root := scaffoldInto(t, "demo")
	pinPath := filepath.Join(root, "vendor", "coilyco-kit.json")
	body := []byte(`{"schema":"something.else.v9","hash":"abc","count":1}`)
	if err := os.WriteFile(pinPath, body, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := artifactCheck(root); err == nil {
		t.Fatal("compared across schemas instead of refusing")
	}
}

func TestEmbeddedKitPinIsWellFormed(t *testing.T) {
	pin, err := embeddedKitPin()
	if err != nil {
		t.Fatalf("embedded pin: %v", err)
	}
	if pin.Schema != "coilyco.kit.v1" {
		t.Errorf("schema = %q", pin.Schema)
	}
	if len(pin.Hash) != 16 {
		t.Errorf("hash = %q, want 16 chars", pin.Hash)
	}
	if pin.Count <= 0 {
		t.Errorf("count = %d", pin.Count)
	}
}

// The pin describes a stylesheet, so the check has to open it. Before this,
// a project whose vendored CSS had been edited still reported current.
func TestArtifactCheckCatchesAnEditedStylesheet(t *testing.T) {
	root := scaffoldInto(t, "demo")
	css := filepath.Join(root, "vendor", "coilyco-kit.css")
	body, err := os.ReadFile(css)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(css, append(body, []byte("\n.injected{}\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := artifactCheck(root)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if result.CSSProblem == "" {
		t.Fatal("an edited stylesheet reported intact")
	}
}

// An absent stylesheet is not a current one, under any reading of the help.
func TestArtifactCheckCatchesADeletedStylesheet(t *testing.T) {
	root := scaffoldInto(t, "demo")
	if err := os.Remove(filepath.Join(root, "vendor", "coilyco-kit.css")); err != nil {
		t.Fatal(err)
	}
	result, err := artifactCheck(root)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if !strings.Contains(result.CSSProblem, "absent") {
		t.Fatalf("CSSProblem = %q, want it to name the absence", result.CSSProblem)
	}
}

// A project scaffolded before the digest existed must be told it cannot be
// verified, rather than passing on a check that never ran.
func TestArtifactCheckNamesAPinWithoutADigest(t *testing.T) {
	root := scaffoldInto(t, "demo")
	pinPath := filepath.Join(root, "vendor", "coilyco-kit.json")
	raw, err := os.ReadFile(pinPath)
	if err != nil {
		t.Fatal(err)
	}
	var pin kitPin
	if err := json.Unmarshal(raw, &pin); err != nil {
		t.Fatal(err)
	}
	pin.CSS = ""
	body, err := json.Marshal(pin)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pinPath, body, 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := artifactCheck(root)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if !strings.Contains(result.CSSProblem, "css_sha256") {
		t.Fatalf("CSSProblem = %q", result.CSSProblem)
	}
}

func TestArtifactCheckPassesOnAnIntactStylesheet(t *testing.T) {
	root := scaffoldInto(t, "demo")
	result, err := artifactCheck(root)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if result.CSSProblem != "" || result.Stale {
		t.Fatalf("fresh project: stale=%v problem=%q", result.Stale, result.CSSProblem)
	}
}
