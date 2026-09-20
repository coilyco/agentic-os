package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

const modelProfileFixture = `
roles:
  builder:
    agent: claude
    harnesses:
      claude:
        model: sonnet
        effort: high
  pinned:
    agent: claude
    harnesses:
      claude:
        model: claude-sonnet-4-6
        effort: xhigh
  bystander:
    agent: claude
`

func loadModelFixture(t *testing.T, data string) harnessLaunchProfileDocument {
	t.Helper()
	document, err := loadHarnessLaunchProfiles([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	return document
}

func TestLoadHarnessLaunchProfilesRejectsMalformedModelProfiles(t *testing.T) {
	t.Parallel()
	for name, body := range map[string]string{
		"non-claude harness": "roles:\n  a:\n    agent: codex\n    harnesses:\n      codex: {model: gpt-5}\n",
		"effort off enum":    "roles:\n  a:\n    agent: claude\n    harnesses:\n      claude: {model: sonnet, effort: extreme}\n",
		"unresolvable alias": "roles:\n  a:\n    agent: claude\n    harnesses:\n      claude: {model: best}\n",
		"not a claude id":    "roles:\n  a:\n    agent: claude\n    harnesses:\n      claude: {model: gpt-5}\n",
		"empty profile":      "roles:\n  a:\n    agent: claude\n    harnesses:\n      claude: {}\n",
		"unknown key":        "roles:\n  a:\n    agent: claude\n    harnesses:\n      claude: {model: sonnet, verbosity: low}\n",
		"goose no provider":  "roles:\n  a:\n    agent: goose\n    harnesses:\n      goose: {model: evaluation/x}\n",
		"goose no model":     "roles:\n  a:\n    agent: goose\n    harnesses:\n      goose: {provider: openai}\n",
		"goose with effort":  "roles:\n  a:\n    agent: goose\n    harnesses:\n      goose: {provider: openai, model: m, effort: high}\n",
		"goose spaced model": "roles:\n  a:\n    agent: goose\n    harnesses:\n      goose: {provider: openai, model: 'two words'}\n",
		"claude provider":    "roles:\n  a:\n    agent: claude\n    harnesses:\n      claude: {model: sonnet, provider: openai}\n",
	} {
		if _, err := loadHarnessLaunchProfiles([]byte(body)); err == nil {
			t.Errorf("%s: loader accepted %q", name, body)
		}
	}
}

func TestRoleModelArgumentsYieldToTheHuman(t *testing.T) {
	t.Parallel()
	document := loadModelFixture(t, modelProfileFixture)
	noEnv := func(string) string { return "" }
	for name, tc := range map[string]struct {
		role string
		args []string
		env  func(string) string
		want []string
	}{
		"profile applies":  {"builder", nil, noEnv, []string{"--model", "sonnet", "--effort", "high"}},
		"unlisted role":    {"bystander", nil, noEnv, nil},
		"typed model wins": {"builder", []string{"--model", "opus"}, noEnv, []string{"--effort", "high"}},
		"inline effort":    {"builder", []string{"--effort=low"}, noEnv, []string{"--model", "sonnet"}},
		"after terminator": {"builder", []string{"--", "--model"}, noEnv, []string{"--model", "sonnet", "--effort", "high"}},
		"effort env wins": {"builder", nil, func(name string) string {
			if name == "CLAUDE_CODE_EFFORT_LEVEL" {
				return "max"
			}
			return ""
		}, []string{"--model", "sonnet"}},
	} {
		got := roleModelArguments(document, tc.role, "claude", tc.args, tc.env)
		if !slices.Equal(got, tc.want) {
			t.Errorf("%s: got %v, want %v", name, got, tc.want)
		}
	}
}

const gooseProfileFixture = `
roles:
  assistant:
    agent: goose
    harnesses:
      goose:
        provider: openai
        model: evaluation/deepseek-v4-pro
  builder:
    agent: claude
    harnesses:
      claude:
        model: sonnet
`

func TestGooseProfileReachesGooseAsEnvironmentAndNeverAsFlags(t *testing.T) {
	t.Parallel()
	document := loadModelFixture(t, gooseProfileFixture)
	noEnv := func(string) string { return "" }
	got := roleModelEnvironment(document, "assistant", "goose", noEnv)
	want := map[string]string{"GOOSE_PROVIDER": "openai", "GOOSE_MODEL": "evaluation/deepseek-v4-pro"}
	if len(got) != len(want) || got["GOOSE_PROVIDER"] != want["GOOSE_PROVIDER"] || got["GOOSE_MODEL"] != want["GOOSE_MODEL"] {
		t.Fatalf("environment = %v, want %v", got, want)
	}
	// goose takes no --model, so a goose profile must add nothing to its argv.
	if flags := roleModelArguments(document, "assistant", "goose", nil, noEnv); len(flags) != 0 {
		t.Errorf("goose profile produced flags %v", flags)
	}
	// A claude profile is no goose environment, and the reverse holds too.
	if got := roleModelEnvironment(document, "builder", "claude", noEnv); len(got) != 0 {
		t.Errorf("claude profile produced environment %v", got)
	}
	if got := roleModelEnvironment(document, "assistant", "claude", noEnv); len(got) != 0 {
		t.Errorf("a role with no claude profile produced environment %v", got)
	}
}

func TestExportedGooseEnvironmentYieldsToTheHuman(t *testing.T) {
	t.Parallel()
	document := loadModelFixture(t, gooseProfileFixture)
	human := func(name string) string {
		if name == "GOOSE_MODEL" {
			return "qwen3-coder:30b"
		}
		return ""
	}
	got := roleModelEnvironment(document, "assistant", "goose", human)
	if _, overridden := got["GOOSE_MODEL"]; overridden {
		t.Errorf("the profile overrode a model the human exported: %v", got)
	}
	if got["GOOSE_PROVIDER"] != "openai" {
		t.Errorf("the provider was dropped along with the model: %v", got)
	}
}

func TestApplyRoleModelEnvironmentExportsBeforeLaunch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profiles.yaml")
	if err := os.WriteFile(path, []byte(gooseProfileFixture), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AOS_HARNESS_LAUNCH_PROFILES", path)
	// Registered so the test's own exports are undone afterwards.
	t.Setenv("GOOSE_PROVIDER", "")
	t.Setenv("GOOSE_MODEL", "")

	if err := applyRoleModelEnvironment("assistant", "claude"); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("GOOSE_PROVIDER") != "" {
		t.Fatal("a claude launch exported goose environment")
	}
	if err := applyRoleModelEnvironment("assistant", "goose"); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("GOOSE_PROVIDER") != "openai" || os.Getenv("GOOSE_MODEL") != "evaluation/deepseek-v4-pro" {
		t.Errorf("exported %q and %q", os.Getenv("GOOSE_PROVIDER"), os.Getenv("GOOSE_MODEL"))
	}
}

func TestApplyRoleModelProfileInsertsAfterTheHarness(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profiles.yaml")
	if err := os.WriteFile(path, []byte(modelProfileFixture), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AOS_HARNESS_LAUNCH_PROFILES", path)
	t.Setenv("ANTHROPIC_MODEL", "")
	t.Setenv("CLAUDE_CODE_EFFORT_LEVEL", "")

	got, err := applyRoleModelProfile(
		[]string{"agent-compose", "launch", "builder", "claude", "-p", "hi"}, "builder", "claude")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"agent-compose", "launch", "builder", "claude", "--model", "sonnet", "--effort", "high", "-p", "hi"}
	if !slices.Equal(got, want) {
		t.Fatalf("native argv = %v, want %v", got, want)
	}
	got, err = applyRoleModelProfile([]string{"/usr/bin/claude"}, "builder", "claude")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []string{"/usr/bin/claude", "--model", "sonnet", "--effort", "high"}) {
		t.Fatalf("container argv = %v", got)
	}

	if err := os.WriteFile(path, []byte("roles:\n  builder:\n    agent: claude\n    harnesses:\n      claude: {model: gpt-5}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = applyRoleModelProfile([]string{"claude"}, "builder", "claude")
	if err == nil || !strings.Contains(err.Error(), "builder") || !strings.Contains(err.Error(), "gpt-5") {
		t.Fatalf("invalid profile error = %v, want one naming the role and model", err)
	}
}

func modelFixture(id string, created string, efforts ...string) anthropicModel {
	model := anthropicModel{ID: id}
	model.CreatedAt, _ = time.Parse(time.RFC3339, created)
	model.Capabilities = &struct {
		Effort map[string]json.RawMessage `json:"effort"`
	}{Effort: map[string]json.RawMessage{}}
	for _, level := range claudeEffortLevels {
		model.Capabilities.Effort[level] = json.RawMessage(`{"supported":` +
			map[bool]string{true: "true", false: "false"}[slices.Contains(efforts, level)] + `}`)
	}
	return model
}

var providerModels = []anthropicModel{
	modelFixture("claude-sonnet-5", "2026-06-01T00:00:00Z", "low", "medium", "high", "xhigh", "max"),
	modelFixture("claude-sonnet-4-6", "2026-02-01T00:00:00Z", "low", "medium", "high", "max"),
}

func TestCheckRoleModelProfiles(t *testing.T) {
	t.Parallel()
	result := checkRoleModelProfiles(loadModelFixture(t, modelProfileFixture), providerModels)
	if !slices.Contains(result.Lines, "role builder claude sonnet -> claude-sonnet-5 effort high ok") {
		t.Errorf("alias did not resolve to the newest family member: %v", result.Lines)
	}
	if len(result.Failures) != 1 || !strings.Contains(result.Failures[0], "does not support effort xhigh") {
		t.Errorf("unsupported effort not failed: %v", result.Failures)
	}
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "claude-sonnet-5 is the newest") {
		t.Errorf("stale pin not warned: %v", result.Warnings)
	}

	bogus := loadModelFixture(t, "roles:\n  a:\n    agent: claude\n    harnesses:\n      claude: {model: claude-sonnet-9-9}\n")
	result = checkRoleModelProfiles(bogus, providerModels)
	if len(result.Failures) != 1 || !strings.Contains(result.Failures[0], "not listed by the provider") {
		t.Errorf("unlisted id not failed: %v", result.Failures)
	}
}

func TestFetchAnthropicModelsFollowsPages(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "fixture" || r.Header.Get("anthropic-version") == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Query().Get("after_id") == "" {
			_, _ = w.Write([]byte(`{"data":[{"id":"claude-sonnet-5"}],"has_more":true,"last_id":"claude-sonnet-5"}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"claude-sonnet-4-6"}],"has_more":false,"last_id":"claude-sonnet-4-6"}`))
	}))
	t.Cleanup(server.Close)
	models, err := fetchAnthropicModels(context.Background(), server.Client(), server.URL, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 {
		t.Fatalf("models = %+v, want both pages", models)
	}
	if _, err := fetchAnthropicModels(context.Background(), server.Client(), server.URL, "wrong"); err == nil {
		t.Fatal("an unauthorized response did not fail")
	}
}
