package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
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
		"unknown harness":    "roles:\n  a:\n    agent: codex\n    harnesses:\n      aider: {model: gpt-5}\n",
		"codex with effort":  "roles:\n  a:\n    agent: codex\n    harnesses:\n      codex: {model: gpt-6-sol, effort: high}\n",
		"codex no model":     "roles:\n  a:\n    agent: codex\n    harnesses:\n      codex: {}\n",
		"opencode bare id":   "roles:\n  a:\n    agent: opencode\n    harnesses:\n      opencode: {model: deepseek-v4-pro}\n",
		"opencode provider":  "roles:\n  a:\n    agent: opencode\n    harnesses:\n      opencode: {provider: x, model: a/b}\n",
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

	if err := applyRoleModelEnvironment(context.Background(), "assistant", "claude", io.Discard, listedModels("evaluation/deepseek-v4-pro")); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("GOOSE_PROVIDER") != "" {
		t.Fatal("a claude launch exported goose environment")
	}
	if err := applyRoleModelEnvironment(context.Background(), "assistant", "goose", io.Discard, listedModels("evaluation/deepseek-v4-pro")); err != nil {
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

	listed := listedModels("claude-sonnet-5")
	got, err := applyRoleModelProfile(context.Background(),
		[]string{"agent-compose", "launch", "builder", "claude", "-p", "hi"}, "builder", "claude", io.Discard, listed)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"agent-compose", "launch", "builder", "claude", "--model", "sonnet", "--effort", "high", "-p", "hi"}
	if !slices.Equal(got, want) {
		t.Fatalf("native argv = %v, want %v", got, want)
	}
	got, err = applyRoleModelProfile(context.Background(), []string{"/usr/bin/claude"}, "builder", "claude", io.Discard, listed)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []string{"/usr/bin/claude", "--model", "sonnet", "--effort", "high"}) {
		t.Fatalf("container argv = %v", got)
	}

	if err := os.WriteFile(path, []byte("roles:\n  builder:\n    agent: claude\n    harnesses:\n      claude: {model: gpt-5}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = applyRoleModelProfile(context.Background(), []string{"claude"}, "builder", "claude", io.Discard, listed)
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

// listedModels is a model source that answers with ids, and never errors.
func listedModels(ids ...string) modelLister {
	return func(context.Context, string, harnessModelProfile) ([]string, string, error) {
		return ids, "the fixture source", nil
	}
}

func unreachableModels(context.Context, string, harnessModelProfile) ([]string, string, error) {
	return nil, "", errors.New("list models: connection refused")
}

func writeProfilesFixture(t *testing.T, body string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "profiles.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AOS_HARNESS_LAUNCH_PROFILES", path)
	t.Setenv("ANTHROPIC_MODEL", "")
	t.Setenv("CLAUDE_CODE_EFFORT_LEVEL", "")
}

func TestPinnedClaudeModelFallsBackWhenTheSourceDoesNotListIt(t *testing.T) {
	writeProfilesFixture(t, modelProfileFixture)
	for name, tc := range map[string]struct {
		list        modelLister
		want        []string
		wantNotices []string
	}{
		"absent pin launches on the default": {
			list: listedModels("claude-sonnet-5"),
			want: []string{"claude", "--effort", "xhigh"},
			wantNotices: []string{
				"aos: warning: role pinned pins claude model claude-sonnet-4-6, which the fixture source does not list; " +
					"launching on the claude default model",
			},
		},
		"present pin launches unchanged": {
			list: listedModels("claude-sonnet-5", "claude-sonnet-4-6"),
			want: []string{"claude", "--model", "claude-sonnet-4-6", "--effort", "xhigh"},
		},
		"unknown availability launches as pinned": {
			list: unreachableModels,
			want: []string{"claude", "--model", "claude-sonnet-4-6", "--effort", "xhigh"},
			wantNotices: []string{
				"aos: role pinned launches on pinned claude model claude-sonnet-4-6 unchecked: " +
					"list models: connection refused",
			},
		},
	} {
		var stderr strings.Builder
		got, err := applyRoleModelProfile(context.Background(), []string{"claude"}, "pinned", "claude", &stderr, tc.list)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("%s: argv = %v, want %v", name, got, tc.want)
		}
		notices := strings.FieldsFunc(stderr.String(), func(r rune) bool { return r == '\n' })
		if !slices.Equal(notices, tc.wantNotices) {
			t.Errorf("%s: stderr = %q, want %q", name, notices, tc.wantNotices)
		}
	}
}

func TestModelAvailabilityIsNotAskedWhenTheHumanChoseTheModel(t *testing.T) {
	writeProfilesFixture(t, modelProfileFixture)
	asked := false
	list := func(context.Context, string, harnessModelProfile) ([]string, string, error) {
		asked = true
		return nil, "", nil
	}
	got, err := applyRoleModelProfile(context.Background(),
		[]string{"claude", "--model", "opus"}, "pinned", "claude", io.Discard, list)
	if err != nil {
		t.Fatal(err)
	}
	if asked || !slices.Equal(got, []string{"claude", "--effort", "xhigh", "--model", "opus"}) {
		t.Errorf("asked=%v argv=%v", asked, got)
	}
}

func TestClaudeAliasIsListedByAnyFamilyMember(t *testing.T) {
	t.Parallel()
	if !modelListed("claude", "sonnet", []string{"claude-opus-5", "claude-sonnet-5"}) {
		t.Error("sonnet alias not listed beside a sonnet id")
	}
	if modelListed("claude", "haiku", []string{"claude-sonnet-5"}) {
		t.Error("haiku alias listed with no haiku id")
	}
	if !modelListed("claude", "claude-opus-5[1m]", []string{"claude-opus-5"}) {
		t.Error("[1m] pin not matched to its base id")
	}
}

func TestPinnedGooseModelFallsBackAsAPair(t *testing.T) {
	writeProfilesFixture(t, gooseProfileFixture)
	t.Setenv("GOOSE_PROVIDER", "")
	t.Setenv("GOOSE_MODEL", "")
	var stderr strings.Builder
	if err := applyRoleModelEnvironment(context.Background(), "assistant", "goose", &stderr,
		listedModels("chat/default")); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("GOOSE_PROVIDER") != "" || os.Getenv("GOOSE_MODEL") != "" {
		t.Errorf("absent pin exported %q and %q", os.Getenv("GOOSE_PROVIDER"), os.Getenv("GOOSE_MODEL"))
	}
	want := "aos: warning: role assistant pins goose model evaluation/deepseek-v4-pro, " +
		"which the fixture source does not list; launching on the goose default model\n"
	if stderr.String() != want {
		t.Errorf("stderr = %q, want %q", stderr.String(), want)
	}

	stderr.Reset()
	if err := applyRoleModelEnvironment(context.Background(), "assistant", "goose", &stderr,
		unreachableModels); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("GOOSE_MODEL") != "evaluation/deepseek-v4-pro" || !strings.Contains(stderr.String(), "unchecked") {
		t.Errorf("unknown availability exported %q with stderr %q", os.Getenv("GOOSE_MODEL"), stderr.String())
	}
}

func TestListGooseModelsAsksTheHostGooseIsConfiguredWith(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"chat/default"},{"id":"evaluation/deepseek-v4-pro"}]}`))
	}))
	t.Cleanup(server.Close)
	config := filepath.Join(t.TempDir(), "config.yaml")
	body := "OPENAI_HOST: " + server.URL + "\nOPENAI_BASE_PATH: v1/chat/completions\nextensions: {}\n"
	if err := os.WriteFile(config, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	noEnv := func(string) string { return "" }
	profile := harnessModelProfile{Provider: "openai", Model: "chat/default"}
	ids, source, err := listGooseModels(context.Background(), server.Client(), profile, noEnv, config)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ids, []string{"chat/default", "evaluation/deepseek-v4-pro"}) || source != server.URL+"/v1/models" {
		t.Errorf("ids=%v source=%s", ids, source)
	}
	if _, _, err := listGooseModels(context.Background(), server.Client(),
		harnessModelProfile{Provider: "ollama", Model: "m"}, noEnv, config); err == nil {
		t.Error("a provider with no list source was treated as determined")
	}
	if _, _, err := listGooseModels(context.Background(), server.Client(), profile, noEnv,
		filepath.Join(t.TempDir(), "absent.yaml")); err == nil {
		t.Error("a missing OPENAI_HOST was treated as determined")
	}
}

func TestClaudePinsAreNeverCheckedAtLaunch(t *testing.T) {
	// A claude list needs an API key, and a launch must never carry one.
	t.Setenv("ANTHROPIC_API_KEY", "synthetic")
	t.Setenv("ANTHROPIC_MODELS_API_KEY", "synthetic")
	profile := harnessModelProfile{Model: "claude-sonnet-9-9"}
	got, notice := resolvePinnedModel(context.Background(), "pinned", "claude", profile, listHarnessModels)
	if got != profile || notice != "" {
		t.Errorf("claude pin = %+v with notice %q, want it kept silently", got, notice)
	}
}

const modelOnlyProfileFixture = `
roles:
  operator:
    agent: codex
    harnesses:
      codex:
        model: gpt-6-sol
  junior:
    agent: opencode
    harnesses:
      opencode:
        model: agent-proxy/evaluation/deepseek-v4-pro
`

func TestCodexAndOpencodePinsReachTheirHarness(t *testing.T) {
	t.Parallel()
	document := loadModelFixture(t, modelOnlyProfileFixture)
	noEnv := func(string) string { return "" }
	for name, tc := range map[string]struct {
		role, harness string
		args          []string
		want          []string
	}{
		"codex pin":           {"operator", "codex", nil, []string{"-c", `model="gpt-6-sol"`}},
		"codex typed -m":      {"operator", "codex", []string{"-m", "o3"}, nil},
		"codex typed -c":      {"operator", "codex", []string{"-c", "model=o3"}, nil},
		"codex inline config": {"operator", "codex", []string{"--config=model=o3"}, nil},
		"codex prompt text":   {"operator", "codex", []string{"exec", "model=o3"}, []string{"-c", `model="gpt-6-sol"`}},
		"codex other -c":      {"operator", "codex", []string{"-c", "sandbox_mode=x"}, []string{"-c", `model="gpt-6-sol"`}},
		"opencode pin":        {"junior", "opencode", nil, []string{"--model", "agent-proxy/evaluation/deepseek-v4-pro"}},
		"opencode typed -m":   {"junior", "opencode", []string{"-m", "a/b"}, nil},
		"wrong harness":       {"junior", "codex", nil, nil},
	} {
		got := roleModelArguments(document, tc.role, tc.harness, tc.args, noEnv)
		if !slices.Equal(got, tc.want) {
			t.Errorf("%s: got %v, want %v", name, got, tc.want)
		}
	}
}

func TestPinnedCodexAndOpencodeModelsFallBack(t *testing.T) {
	writeProfilesFixture(t, modelOnlyProfileFixture)
	for name, tc := range map[string]struct {
		command    []string
		role       string
		list       modelLister
		want       []string
		wantNotice string
	}{
		"codex absent": {
			[]string{"codex"}, "operator", listedModels("gpt-6-astra"),
			[]string{"codex"}, "role operator pins codex model gpt-6-sol, which the fixture source does not list",
		},
		"codex present": {
			[]string{"codex", "exec"}, "operator", listedModels("gpt-6-sol"),
			[]string{"codex", "-c", `model="gpt-6-sol"`, "exec"}, "",
		},
		"opencode absent": {
			[]string{"opencode"}, "junior", listedModels("agent-proxy/evaluation/deepseek-v4-flash"),
			[]string{"opencode"}, "role junior pins opencode model agent-proxy/evaluation/deepseek-v4-pro",
		},
		"opencode unknown": {
			[]string{"opencode"}, "junior", unreachableModels,
			[]string{"opencode", "--model", "agent-proxy/evaluation/deepseek-v4-pro"}, "unchecked",
		},
	} {
		var stderr strings.Builder
		got, err := applyRoleModelProfile(context.Background(), tc.command, tc.role, tc.command[0], &stderr, tc.list)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("%s: argv = %v, want %v", name, got, tc.want)
		}
		if tc.wantNotice == "" && stderr.Len() != 0 || !strings.Contains(stderr.String(), tc.wantNotice) {
			t.Errorf("%s: stderr = %q, want %q", name, stderr.String(), tc.wantNotice)
		}
	}
}

func TestListCodexModelsRPCFollowsCursors(t *testing.T) {
	t.Parallel()
	requestsR, requestsW := io.Pipe()
	responsesR, responsesW := io.Pipe()
	go func() {
		decoder := json.NewDecoder(requestsR)
		encoder := json.NewEncoder(responsesW)
		for {
			var request struct {
				ID     *int           `json:"id"`
				Method string         `json:"method"`
				Params map[string]any `json:"params"`
			}
			if err := decoder.Decode(&request); err != nil {
				return
			}
			if request.ID == nil {
				continue
			}
			result := map[string]any{}
			if request.Method == "model/list" {
				if request.Params["includeHidden"] != true {
					t.Errorf("model/list without includeHidden: %v", request.Params)
				}
				if request.Params["cursor"] == nil {
					result = map[string]any{"data": []map[string]string{{"id": "gpt-6-sol", "model": "gpt-6-sol"}}, "nextCursor": "page2"}
				} else {
					result = map[string]any{"data": []map[string]string{{"id": "gpt-5.5", "model": "gpt-5.5"}}, "nextCursor": nil}
				}
			}
			_ = encoder.Encode(map[string]any{"id": *request.ID, "result": result})
		}
	}()
	t.Cleanup(func() { _ = requestsW.Close(); _ = responsesW.Close() })
	ids, err := listCodexModelsRPC(json.NewEncoder(requestsW), json.NewDecoder(responsesR))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(ids, "gpt-6-sol") || !slices.Contains(ids, "gpt-5.5") {
		t.Errorf("ids = %v, want both pages", ids)
	}
}

func TestParseOpencodeModelsKeepsProviderModelLines(t *testing.T) {
	t.Parallel()
	got := parseOpencodeModels([]byte("agent-proxy/evaluation/deepseek-v4-flash\nagent-proxy/evaluation/deepseek-v4-pro\n\nWarning: cache stale\n"))
	want := []string{"agent-proxy/evaluation/deepseek-v4-flash", "agent-proxy/evaluation/deepseek-v4-pro"}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
