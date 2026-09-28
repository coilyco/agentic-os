package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
)

// modelLister returns the model ids that serve a harness profile and names the
// source it asked. An error means availability could not be determined.
type modelLister func(ctx context.Context, harness string, profile harnessModelProfile) ([]string, string, error)

// modelAvailabilityTimeout bounds the one list call a launch makes, so an
// unreachable source costs a launch seconds and never blocks it.
const modelAvailabilityTimeout = 3 * time.Second

// resolvePinnedModel drops a pin its source does not list and keeps one it
// cannot check. The returned line says which happened.
func resolvePinnedModel(
	ctx context.Context,
	role, harness string,
	profile harnessModelProfile,
	list modelLister,
) (harnessModelProfile, string) {
	if profile.Model == "" || list == nil {
		return profile, ""
	}
	ctx, cancel := context.WithTimeout(ctx, modelAvailabilityTimeout)
	defer cancel()
	ids, source, err := list(ctx, harness, profile)
	if err != nil {
		return profile, fmt.Sprintf(
			"aos: role %s launches on pinned %s model %s unchecked: %v",
			role, harness, profile.Model, err)
	}
	if modelListed(harness, profile.Model, ids) {
		return profile, ""
	}
	fallback := profile
	fallback.Model = ""
	// A goose provider without its model runs goose's configured model on
	// the wrong provider, so the pair falls back together.
	if harness == "goose" {
		fallback.Provider = ""
	}
	return fallback, fmt.Sprintf(
		"aos: warning: role %s pins %s model %s, which %s does not list; launching on the %s default model",
		role, harness, profile.Model, source, harness)
}

func modelListed(harness, model string, ids []string) bool {
	if harness == "claude" {
		if prefix, alias := claudeModelAliases[model]; alias {
			for _, id := range ids {
				if strings.HasPrefix(id, prefix) {
					return true
				}
			}
			return false
		}
		model = strings.TrimSuffix(model, "[1m]")
	}
	return containsString(ids, model)
}

// listHarnessModels is the launch-time source for each harness. See
// docs/native-harness-config.md for why each harness asks where it does.
func listHarnessModels(ctx context.Context, harness string, profile harnessModelProfile) ([]string, string, error) {
	client := &http.Client{Timeout: modelAvailabilityTimeout}
	switch harness {
	case "claude":
		return listClaudeModels(ctx, client, os.Getenv)
	case "goose":
		return listGooseModels(ctx, client, profile, os.Getenv, gooseConfigPath())
	}
	return nil, "", fmt.Errorf("no model list source for %s", harness)
}

func listClaudeModels(ctx context.Context, client *http.Client, env func(string) string) ([]string, string, error) {
	for _, variable := range []string{"CLAUDE_CODE_USE_BEDROCK", "CLAUDE_CODE_USE_VERTEX", "CLAUDE_CODE_USE_FOUNDRY"} {
		if strings.TrimSpace(env(variable)) != "" {
			return nil, "", fmt.Errorf("%s is set, and only the Anthropic API is checked", variable)
		}
	}
	apiKey := strings.TrimSpace(env("ANTHROPIC_API_KEY"))
	if apiKey == "" {
		return nil, "", errors.New("no ANTHROPIC_API_KEY to list the Anthropic API models")
	}
	endpoint := anthropicModelsURL
	if override := strings.TrimSpace(env("AOS_MODELS_API_URL")); override != "" {
		endpoint = override
	}
	models, err := fetchAnthropicModels(ctx, client, endpoint, apiKey)
	if err != nil {
		return nil, "", err
	}
	ids := make([]string, 0, len(models))
	for _, model := range models {
		ids = append(ids, model.ID)
	}
	return ids, "the Anthropic API", nil
}

// listGooseModels asks the OpenAI-compatible host goose itself is configured
// with, which on this fleet is Agent Proxy.
func listGooseModels(
	ctx context.Context,
	client *http.Client,
	profile harnessModelProfile,
	env func(string) string,
	configPath string,
) ([]string, string, error) {
	if profile.Provider != "openai" {
		return nil, "", fmt.Errorf("goose provider %s has no model list source", profile.Provider)
	}
	config := readGooseOpenAIConfig(configPath)
	host := firstNonEmpty(env("OPENAI_HOST"), config.Host)
	if host == "" {
		return nil, "", errors.New("no OPENAI_HOST in the environment or goose config")
	}
	basePath := firstNonEmpty(env("OPENAI_BASE_PATH"), config.BasePath, "v1/chat/completions")
	endpoint := strings.TrimRight(host, "/") + "/" +
		strings.TrimPrefix(strings.TrimSuffix(basePath, "chat/completions"), "/") + "models"
	ids, err := fetchOpenAIModels(ctx, client, endpoint, strings.TrimSpace(env("OPENAI_API_KEY")))
	if err != nil {
		return nil, "", err
	}
	return ids, endpoint, nil
}

type gooseOpenAIConfig struct {
	Host     string `yaml:"OPENAI_HOST"`
	BasePath string `yaml:"OPENAI_BASE_PATH"`
}

// readGooseOpenAIConfig reads only the two host keys. Goose keeps its secrets
// in the keyring or a separate file, never this one.
func readGooseOpenAIConfig(path string) gooseOpenAIConfig {
	var config gooseOpenAIConfig
	data, err := os.ReadFile(path)
	if err != nil {
		return config
	}
	_ = yaml.Unmarshal(data, &config)
	config.Host = strings.TrimSpace(config.Host)
	config.BasePath = strings.TrimSpace(config.BasePath)
	return config
}

func gooseConfigPath() string {
	if root := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); root != "" {
		return filepath.Join(root, "goose", "config.yaml")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "goose", "config.yaml")
}

func fetchOpenAIModels(ctx context.Context, client *http.Client, endpoint, apiKey string) ([]string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+apiKey)
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("list models: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("read models response: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list models: HTTP %d", response.StatusCode)
	}
	var decoded struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, fmt.Errorf("decode models response: %w", err)
	}
	ids := make([]string, 0, len(decoded.Data))
	for _, model := range decoded.Data {
		ids = append(ids, model.ID)
	}
	return ids, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
