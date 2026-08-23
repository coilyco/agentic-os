package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

//go:embed testdata/tpm-overlay.json
var tpmOverlay string

func tpmRequest() launchRequest {
	return launchRequest{
		Role:             "tpm",
		Seat:             "codex",
		Expression:       "acting",
		TaskTitle:        "agentic-os#730",
		WorkingDirectory: ".",
		AgentComposeBin:  defaultOverlayBin,
		AOSComposeBin:    defaultAOSComposeBin,
		AlacrittyBin:     defaultAlacrittyBin,
		Child:            []string{"ward", "agent", "tpm", "--repo", "coilyco-flight-deck/agentic-os"},
	}
}

func TestVersionFlagReportsReleaseVersion(t *testing.T) {
	original := version
	version = "aos-v1.2.3"
	t.Cleanup(func() { version = original })

	var output strings.Builder
	command := newCommand(commandDeps{}, "agent-terminal")
	command.Writer = &output
	if err := command.Run(
		context.Background(),
		[]string{"agent-terminal", "--version"},
	); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(output.String()); got != "agent-terminal version aos-v1.2.3" {
		t.Fatalf("version output = %q", got)
	}
}

func TestAOSTermVersionFlagReportsInvocationName(t *testing.T) {
	original := version
	version = "aos-v1.2.3"
	t.Cleanup(func() { version = original })

	var output strings.Builder
	command := newCommand(commandDeps{}, "aosterm")
	command.Writer = &output
	if err := command.Run(
		context.Background(),
		[]string{"aosterm", "--version"},
	); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(output.String()); got != "aosterm version aos-v1.2.3" {
		t.Fatalf("version output = %q", got)
	}
}

func TestCommandNameRecognizesAOSTermReleaseAssets(t *testing.T) {
	for _, argv := range [][]string{
		{"/usr/local/bin/aosterm"},
		{"/usr/local/bin/aosterm-darwin-arm64"},
		{`C:\Users\kai\scoop\apps\aos\aosterm-windows-amd64.exe`},
	} {
		if got := commandName(argv); got != "aosterm" {
			t.Fatalf("commandName(%#v) = %q", argv, got)
		}
	}
}

func TestDefaultWorkingDirectoryUsesProjectsRoot(t *testing.T) {
	projects := t.TempDir()
	t.Setenv("PROJECTS_ROOT", projects)

	if got := defaultWorkingDirectory(); got != projects {
		t.Fatalf("defaultWorkingDirectory() = %q, want %q", got, projects)
	}
}

func TestDefaultWorkingDirectoryFallsBackToHomeProjects(t *testing.T) {
	t.Setenv("PROJECTS_ROOT", "")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}

	want := filepath.Join(home, "projects")
	if got := defaultWorkingDirectory(); got != want {
		t.Fatalf("defaultWorkingDirectory() = %q, want %q", got, want)
	}
}

func TestBuildLaunchPlanUsesCanonicalIdentity(t *testing.T) {
	request := tpmRequest()
	request.AlacrittyBin = "alacritty-preview"
	document, err := parseOverlay([]byte(tpmOverlay), request)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := buildLaunchPlan(document, request, "/workspace")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Format != launchFormat {
		t.Fatalf("format = %q", plan.Format)
	}
	if plan.Identity.Name != "Saiya" || plan.Identity.FavoriteColor != "#e36966" ||
		plan.Identity.Annotation != "Saiya [they] (Technical Program Manager)" {
		t.Fatalf("identity = %+v", plan.Identity)
	}
	if plan.Brand.Title != "✂ ↗ Saiya [they] (Technical Program Manager) // acting // agentic-os#730" {
		t.Fatalf("title = %q", plan.Brand.Title)
	}
	if plan.Brand.Background != "#21191c" || plan.Brand.SelectionText != baseBackground {
		t.Fatalf("brand = %+v", plan.Brand)
	}
	if plan.Executable != "alacritty-preview" {
		t.Fatalf("executable = %q", plan.Executable)
	}
	if strings.Contains(strings.Join(plan.Arguments, ""), "\x1b") {
		t.Fatal("launch arguments contain an escape sequence")
	}
	wantTail := []string{
		"-e", "ward", "agent", "tpm", "--repo", "coilyco-flight-deck/agentic-os",
	}
	if !reflect.DeepEqual(plan.Arguments[len(plan.Arguments)-len(wantTail):], wantTail) {
		t.Fatalf("arguments tail = %#v", plan.Arguments)
	}
}

// TestSeatAnnotationFallsBackForOlderAgentCompose covers the release window
// where agent-compose predates the composed field.
func TestSeatAnnotationFallsBackForOlderAgentCompose(t *testing.T) {
	request := tpmRequest()
	raw := strings.Replace(
		strings.Replace(
			tpmOverlay,
			"  \"annotation\": \"Saiya [they] (Technical Program Manager)\",\n",
			"",
			1,
		),
		"  \"role_display_name\": \"Technical Program Manager\",\n",
		"",
		1,
	)
	if strings.Contains(raw, "annotation") || strings.Contains(raw, "role_display_name") {
		t.Fatal("fixture still carries the composed fields")
	}
	document, err := parseOverlay([]byte(raw), request)
	if err != nil {
		t.Fatal(err)
	}
	// The role slug stands in for the label an older document never carried.
	if got := seatAnnotation(document); got != "Saiya [they] (tpm)" {
		t.Fatalf("fallback annotation = %q", got)
	}
}

// TestSeatAnnotationDegradesWithoutPronouns keeps an external person package
// that omits pronouns launchable instead of rendering an empty bracket.
func TestSeatAnnotationDegradesWithoutPronouns(t *testing.T) {
	request := tpmRequest()
	raw := strings.Replace(tpmOverlay, `"pronouns": "they"`, `"pronouns": ""`, 1)
	raw = strings.Replace(raw, `"annotation": "Saiya [they] (Technical Program Manager)",`, "", 1)
	document, err := parseOverlay([]byte(raw), request)
	if err != nil {
		t.Fatal(err)
	}
	if got := seatAnnotation(document); got != "Saiya (Technical Program Manager)" {
		t.Fatalf("annotation without pronouns = %q", got)
	}
}

func TestParseOverlayRejectsContractAndSelectionDrift(t *testing.T) {
	request := tpmRequest()
	for name, replace := range map[string][2]string{
		"format":     {`"agent-compose.overlay.v1"`, `"other"`},
		"schema":     {`"schema_version": 1`, `"schema_version": 2`},
		"role":       {`"role": "tpm"`, `"role": "platform"`},
		"seat":       {`"harness": "codex"`, `"harness": "claude"`},
		"expression": {`"expression": "acting"`, `"expression": "blocked"`},
		"color":      {`"#e36966"`, `"olive"`},
	} {
		t.Run(name, func(t *testing.T) {
			raw := strings.Replace(tpmOverlay, replace[0], replace[1], 1)
			if _, err := parseOverlay([]byte(raw), request); err == nil {
				t.Fatal("invalid overlay was accepted")
			}
		})
	}
}

func TestBuildTitleRejectsControlCharactersAndTruncates(t *testing.T) {
	request := tpmRequest()
	document, err := parseOverlay([]byte(tpmOverlay), request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := buildTitle(document, "unsafe\nvalue"); err == nil {
		t.Fatal("control character was accepted")
	}
	title, err := buildTitle(document, strings.Repeat("x", 200))
	if err != nil {
		t.Fatal(err)
	}
	if len([]rune(title)) != maxTitleRunes || !strings.HasSuffix(title, "…") {
		t.Fatalf("truncated title = %q", title)
	}
}

func TestColorDerivationIsDeterministicAndReadable(t *testing.T) {
	first, err := mixHex(baseBackground, "#e36966", backgroundTint)
	if err != nil {
		t.Fatal(err)
	}
	second, err := mixHex(baseBackground, "#e36966", backgroundTint)
	if err != nil {
		t.Fatal(err)
	}
	if first != "#21191c" || second != first {
		t.Fatalf("mixed colors = %q, %q", first, second)
	}
	text, err := mostReadable("#222222", baseBackground, lightForeground)
	if err != nil {
		t.Fatal(err)
	}
	if text != lightForeground {
		t.Fatalf("selection text = %q", text)
	}
}

func TestRunLaunchDryRunDoesNotRequireAlacritty(t *testing.T) {
	request := tpmRequest()
	request.DryRun = true
	var lookedUp []string
	deps := commandDeps{
		lookPath: func(name string) (string, error) {
			lookedUp = append(lookedUp, name)
			if name == defaultOverlayBin {
				return "/bin/agent-compose", nil
			}
			if name == defaultAOSComposeBin {
				return "/bin/aoscompose", nil
			}
			return "", errors.New("missing")
		},
		output: func(_ context.Context, name string, args ...string) ([]byte, error) {
			if name != "/bin/agent-compose" || len(args) == 0 {
				t.Fatalf("overlay command = %q %#v", name, args)
			}
			return []byte(tpmOverlay), nil
		},
		run: func(context.Context, string, ...string) error {
			t.Fatal("dry-run launched Alacritty")
			return nil
		},
	}
	var output strings.Builder
	if err := runLaunch(context.Background(), request, deps, &output); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(lookedUp, []string{defaultOverlayBin, defaultAOSComposeBin}) {
		t.Fatalf("binary lookups = %#v", lookedUp)
	}
	var plan launchPlan
	if err := json.Unmarshal([]byte(output.String()), &plan); err != nil {
		t.Fatalf("dry-run output: %v\n%s", err, output.String())
	}
	if plan.Identity.Name != "Saiya" || plan.Executable != defaultAlacrittyBin {
		t.Fatalf("dry-run plan = %+v", plan)
	}
	wantTail := []string{
		"-e", "/bin/aoscompose", "tpm", "codex", "ward", "agent",
		"tpm", "--repo", "coilyco-flight-deck/agentic-os",
	}
	if !reflect.DeepEqual(plan.Arguments[len(plan.Arguments)-len(wantTail):], wantTail) {
		t.Fatalf("arguments tail = %#v", plan.Arguments)
	}
}

func TestAOSComposeCommandCollapsesRoleSeatAndChild(t *testing.T) {
	request := tpmRequest()
	request.Child = []string{"--version"}

	got := aoscomposeCommand(request, "/bin/aoscompose")
	want := []string{"/bin/aoscompose", "tpm", "codex", "--version"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("aoscompose command = %#v, want %#v", got, want)
	}
}

func TestResolveAOSComposeInvocationAcceptsPositionals(t *testing.T) {
	role, seat, args, err := resolveAOSComposeInvocation(
		"",
		"",
		[]string{"tpm", "codex", "--version"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if role != "tpm" || seat != "codex" || !reflect.DeepEqual(args, []string{"--version"}) {
		t.Fatalf("resolved role=%q seat=%q args=%#v", role, seat, args)
	}
}

func TestResolveAOSComposeInvocationUsesDefaultSeat(t *testing.T) {
	profiles := filepath.Join(t.TempDir(), "profiles.yaml")
	if err := os.WriteFile(profiles, []byte("roles:\n  platform:\n    agent: codex\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AOS_HARNESS_LAUNCH_PROFILES", profiles)

	role, seat, args, err := resolveAOSComposeInvocation("", "", []string{"platform"})
	if err != nil {
		t.Fatal(err)
	}
	if role != "platform" || seat != "codex" || len(args) != 0 {
		t.Fatalf("resolved role=%q seat=%q args=%#v", role, seat, args)
	}
}

func TestRunLaunchValidatesChildDirectoryAndBinaries(t *testing.T) {
	request := tpmRequest()
	file := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	request.WorkingDirectory = file
	if err := runLaunch(context.Background(), request, commandDeps{}, &strings.Builder{}); err == nil {
		t.Fatal("file working directory was accepted")
	}

	request = tpmRequest()
	deps := commandDeps{
		lookPath: func(string) (string, error) { return "", errors.New("missing") },
	}
	err := runLaunch(context.Background(), request, deps, &strings.Builder{})
	if err == nil || !strings.Contains(err.Error(), "agent-compose binary") {
		t.Fatalf("missing binary error = %v", err)
	}
}
