package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/urfave/cli/v3"
)

// harnessModelProfile pins what one role runs a harness at: model and effort
// for claude, provider and model for goose. See docs/native-harness-config.md.
type harnessModelProfile struct {
	Model    string `yaml:"model"`
	Effort   string `yaml:"effort"`
	Provider string `yaml:"provider"`
}

// gooseSettingPattern is the spelling a goose provider or model id may take,
// which covers route ids such as evaluation/deepseek-v4-pro.
var gooseSettingPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]*$`)

var claudeEffortLevels = []string{"low", "medium", "high", "xhigh", "max"}

// Only aliases the live check can resolve to one model family are accepted.
var claudeModelAliases = map[string]string{
	"sonnet":     "claude-sonnet-",
	"sonnet[1m]": "claude-sonnet-",
	"opus":       "claude-opus-",
	"opus[1m]":   "claude-opus-",
	"haiku":      "claude-haiku-",
	"fable":      "claude-fable-",
}

var claudeModelIDPattern = regexp.MustCompile(`^claude-(sonnet|opus|haiku|fable)-[a-z0-9-]*[a-z0-9](\[1m\])?$`)

func validateHarnessModelProfile(role, harness string, profile harnessModelProfile) (harnessModelProfile, error) {
	profile.Model = strings.TrimSpace(profile.Model)
	profile.Effort = strings.TrimSpace(profile.Effort)
	profile.Provider = strings.TrimSpace(profile.Provider)
	switch harness {
	case "claude":
		return validateClaudeModelProfile(role, profile)
	case "goose":
		return validateGooseModelProfile(role, profile)
	}
	return profile, fmt.Errorf(
		"harness launch profile role %s sets a model for %q, but only claude and goose model profiles are supported",
		role, harness,
	)
}

// validateGooseModelProfile requires both halves, since a model without its
// provider is read by goose against whichever provider is active.
func validateGooseModelProfile(role string, profile harnessModelProfile) (harnessModelProfile, error) {
	if profile.Effort != "" {
		return profile, fmt.Errorf("harness launch profile role %s goose profile sets effort, which goose does not take", role)
	}
	for name, value := range map[string]string{"provider": profile.Provider, "model": profile.Model} {
		if !gooseSettingPattern.MatchString(value) {
			return profile, fmt.Errorf(
				"harness launch profile role %s goose %s %q: want a non-empty id of letters, digits, and . _ : / -",
				role, name, value,
			)
		}
	}
	return profile, nil
}

func validateClaudeModelProfile(role string, profile harnessModelProfile) (harnessModelProfile, error) {
	if profile.Provider != "" {
		return profile, fmt.Errorf("harness launch profile role %s claude profile sets a provider, which is a goose setting", role)
	}
	if profile.Model == "" && profile.Effort == "" {
		return profile, fmt.Errorf("harness launch profile role %s claude profile is empty", role)
	}
	if profile.Model != "" && claudeModelFamily(profile.Model) == "" {
		return profile, fmt.Errorf(
			"harness launch profile role %s claude model %q is neither a resolvable alias nor a claude model id",
			role, profile.Model,
		)
	}
	if profile.Effort != "" && !containsString(claudeEffortLevels, profile.Effort) {
		return profile, fmt.Errorf(
			"harness launch profile role %s claude effort %q: want one of %s",
			role, profile.Effort, strings.Join(claudeEffortLevels, ", "),
		)
	}
	return profile, nil
}

// claudeModelFamily returns the id prefix a model belongs to, or "" when the
// value is neither an accepted alias nor a well-formed id.
func claudeModelFamily(model string) string {
	if prefix, ok := claudeModelAliases[model]; ok {
		return prefix
	}
	match := claudeModelIDPattern.FindStringSubmatch(model)
	if match == nil {
		return ""
	}
	return "claude-" + match[1] + "-"
}

func containsString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

// roleModelArguments returns the flags a role's profile adds to its harness
// argv. A flag the human typed, or the env it would otherwise lose to, wins.
func roleModelArguments(
	document harnessLaunchProfileDocument,
	role, harness string,
	arguments []string,
	env func(string) string,
) []string {
	profile, ok := document.Roles[role].Harnesses[harness]
	// Only claude takes these flags. A goose profile reaches goose as env.
	if !ok || harness != "claude" {
		return nil
	}
	var flags []string
	if profile.Model != "" && !hasHarnessFlag(arguments, "--model") &&
		strings.TrimSpace(env("ANTHROPIC_MODEL")) == "" {
		flags = append(flags, "--model", profile.Model)
	}
	// Claude Code ranks CLAUDE_CODE_EFFORT_LEVEL above --effort.
	if profile.Effort != "" && !hasHarnessFlag(arguments, "--effort") &&
		strings.TrimSpace(env("CLAUDE_CODE_EFFORT_LEVEL")) == "" {
		flags = append(flags, "--effort", profile.Effort)
	}
	return flags
}

func hasHarnessFlag(arguments []string, flag string) bool {
	for _, argument := range arguments {
		if argument == "--" {
			return false
		}
		if argument == flag || strings.HasPrefix(argument, flag+"=") {
			return true
		}
	}
	return false
}

// applyRoleModelProfile inserts a role's model flags into the harness argv,
// which is either `<harness> ...` or `agent-compose launch <role> <harness> ...`.
func applyRoleModelProfile(command []string, role, harness string) ([]string, error) {
	if role == "" || len(command) == 0 {
		return command, nil
	}
	start := -1
	switch strings.TrimSuffix(filepath.Base(command[0]), filepath.Ext(command[0])) {
	case harness:
		start = 1
	case "agent-compose":
		if len(command) >= 4 && command[1] == "launch" && command[2] == role && command[3] == harness {
			start = 4
		}
	}
	if start < 0 {
		return command, nil
	}
	document, err := loadConfiguredHarnessLaunchProfiles()
	// Release builds embed the profiles, so only a bare dev build finds none.
	if errors.Is(err, errHarnessLaunchProfilesMissing) {
		return command, nil
	}
	if err != nil {
		return nil, fmt.Errorf("role %s launch refused: %w", role, err)
	}
	flags := roleModelArguments(document, role, harness, command[start:], os.Getenv)
	if len(flags) == 0 {
		return command, nil
	}
	applied := make([]string, 0, len(command)+len(flags))
	applied = append(applied, command[:start]...)
	applied = append(applied, flags...)
	return append(applied, command[start:]...), nil
}

// roleModelEnvironment returns the env a role's goose profile sets. Env the
// human already exported wins, as a typed flag does for claude.
func roleModelEnvironment(
	document harnessLaunchProfileDocument,
	role, harness string,
	env func(string) string,
) map[string]string {
	profile, ok := document.Roles[role].Harnesses[harness]
	if !ok || harness != "goose" {
		return nil
	}
	set := make(map[string]string)
	for name, value := range map[string]string{
		"GOOSE_PROVIDER": profile.Provider,
		"GOOSE_MODEL":    profile.Model,
	} {
		if value != "" && strings.TrimSpace(env(name)) == "" {
			set[name] = value
		}
	}
	return set
}

// applyRoleModelEnvironment exports a role's goose provider and model before the
// native launch replaces this process, which is where the harness reads them.
func applyRoleModelEnvironment(role, harness string) error {
	if role == "" || harness != "goose" {
		return nil
	}
	document, err := loadConfiguredHarnessLaunchProfiles()
	// Release builds embed the profiles, so only a bare dev build finds none.
	if errors.Is(err, errHarnessLaunchProfilesMissing) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("role %s launch refused: %w", role, err)
	}
	for name, value := range roleModelEnvironment(document, role, harness, os.Getenv) {
		if err := os.Setenv(name, value); err != nil {
			return fmt.Errorf("set %s for role %s: %w", name, role, err)
		}
	}
	return nil
}

type anthropicModel struct {
	ID           string    `json:"id"`
	CreatedAt    time.Time `json:"created_at"`
	Capabilities *struct {
		Effort map[string]json.RawMessage `json:"effort"`
	} `json:"capabilities"`
}

type anthropicModelPage struct {
	Data    []anthropicModel `json:"data"`
	HasMore bool             `json:"has_more"`
	LastID  *string          `json:"last_id"`
}

const anthropicModelsURL = "https://api.anthropic.com/v1/models"

func fetchAnthropicModels(ctx context.Context, client *http.Client, endpoint, apiKey string) ([]anthropicModel, error) {
	var models []anthropicModel
	after := ""
	for page := 0; page < 50; page++ {
		query := url.Values{"limit": {"1000"}}
		if after != "" {
			query.Set("after_id", after)
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+query.Encode(), nil)
		if err != nil {
			return nil, err
		}
		request.Header.Set("x-api-key", apiKey)
		request.Header.Set("anthropic-version", "2023-06-01")
		response, err := client.Do(request)
		if err != nil {
			return nil, fmt.Errorf("list models: %w", err)
		}
		body, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
		response.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("read models response: %w", err)
		}
		if response.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("list models: HTTP %d", response.StatusCode)
		}
		var decoded anthropicModelPage
		if err := json.Unmarshal(body, &decoded); err != nil {
			return nil, fmt.Errorf("decode models response: %w", err)
		}
		models = append(models, decoded.Data...)
		if !decoded.HasMore || decoded.LastID == nil || *decoded.LastID == "" {
			return models, nil
		}
		after = *decoded.LastID
	}
	return nil, fmt.Errorf("list models: pagination did not terminate")
}

type modelCheckResult struct {
	Failures []string
	Warnings []string
	Lines    []string
}

// checkRoleModelProfiles resolves each configured claude model against the
// provider's list, so a retired id or unsupported effort fails before launch.
func checkRoleModelProfiles(document harnessLaunchProfileDocument, models []anthropicModel) modelCheckResult {
	var result modelCheckResult
	roles := make([]string, 0, len(document.Roles))
	for role := range document.Roles {
		roles = append(roles, role)
	}
	sort.Strings(roles)
	for _, role := range roles {
		profile, ok := document.Roles[role].Harnesses["claude"]
		if !ok || profile.Model == "" {
			if ok && profile.Effort != "" {
				result.Warnings = append(result.Warnings, fmt.Sprintf(
					"role %s sets effort %s without a model, so the harness default model is not checked",
					role, profile.Effort))
			}
			continue
		}
		family := claudeModelFamily(profile.Model)
		var newest *anthropicModel
		var exact *anthropicModel
		wanted := strings.TrimSuffix(profile.Model, "[1m]")
		for index := range models {
			model := &models[index]
			if !strings.HasPrefix(model.ID, family) {
				continue
			}
			if newest == nil || model.CreatedAt.After(newest.CreatedAt) {
				newest = model
			}
			if model.ID == wanted {
				exact = model
			}
		}
		_, alias := claudeModelAliases[profile.Model]
		resolved := exact
		if alias {
			resolved = newest
		}
		if resolved == nil {
			result.Failures = append(result.Failures, fmt.Sprintf(
				"role %s claude model %s: not listed by the provider", role, profile.Model))
			continue
		}
		if !alias && newest != nil && newest.ID != resolved.ID {
			result.Warnings = append(result.Warnings, fmt.Sprintf(
				"role %s pins %s, but %s is the newest in its family", role, resolved.ID, newest.ID))
		}
		if profile.Effort != "" && !effortSupported(*resolved, profile.Effort) {
			result.Failures = append(result.Failures, fmt.Sprintf(
				"role %s claude model %s (%s) does not support effort %s", role, profile.Model, resolved.ID, profile.Effort))
			continue
		}
		result.Lines = append(result.Lines, fmt.Sprintf(
			"role %s claude %s -> %s effort %s ok", role, profile.Model, resolved.ID, orDefault(profile.Effort)))
	}
	return result
}

func effortSupported(model anthropicModel, level string) bool {
	if model.Capabilities == nil {
		return false
	}
	raw, ok := model.Capabilities.Effort[level]
	if !ok {
		return false
	}
	var support struct {
		Supported bool `json:"supported"`
	}
	return json.Unmarshal(raw, &support) == nil && support.Supported
}

func orDefault(value string) string {
	if value == "" {
		return "default"
	}
	return value
}

// runModelsCheck is `aos models check`. It needs a first-party API key, and
// refuses a Bedrock or Vertex session whose aliases resolve differently.
func runModelsCheck(ctx context.Context, cmd *cli.Command) error {
	if cmd.Bool("offline") {
		if _, err := loadConfiguredHarnessLaunchProfiles(); err != nil {
			return err
		}
		fmt.Fprintln(cmd.Root().Writer, "harness launch profiles are statically valid")
		return nil
	}
	for _, variable := range []string{"CLAUDE_CODE_USE_BEDROCK", "CLAUDE_CODE_USE_VERTEX", "CLAUDE_CODE_USE_FOUNDRY"} {
		if strings.TrimSpace(os.Getenv(variable)) != "" {
			return fmt.Errorf("models check covers the Anthropic API only, and %s is set", variable)
		}
	}
	apiKey := strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY"))
	if apiKey == "" {
		return fmt.Errorf("models check needs ANTHROPIC_API_KEY to list the provider's models")
	}
	document, err := loadConfiguredHarnessLaunchProfiles()
	if err != nil {
		return err
	}
	endpoint := anthropicModelsURL
	if override := strings.TrimSpace(os.Getenv("AOS_MODELS_API_URL")); override != "" {
		endpoint = override
	}
	models, err := fetchAnthropicModels(ctx, &http.Client{Timeout: 30 * time.Second}, endpoint, apiKey)
	if err != nil {
		return err
	}
	result := checkRoleModelProfiles(document, models)
	out := cmd.Root().Writer
	for _, line := range result.Lines {
		fmt.Fprintln(out, line)
	}
	for _, warning := range result.Warnings {
		fmt.Fprintf(cmd.Root().ErrWriter, "aos: warning: %s\n", warning)
	}
	if len(result.Failures) > 0 {
		return fmt.Errorf("models check failed:\n  %s", strings.Join(result.Failures, "\n  "))
	}
	return nil
}
