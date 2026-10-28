package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"
)

// Per-role MCP narrowing for a spec-mode launch, moved from agent-compose
// mcpscope. docs/native-harness-config.md

type mcpScopeServer struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
	Cwd     string            `json:"cwd"`
	URL     string            `json:"url"`
	BaseURL string            `json:"baseUrl"`
	Headers map[string]string `json:"headers"`
	AOS     struct {
		Roles []string `json:"roles"`
	} `json:"x-aos"`
}

type mcpScopeSelection struct {
	Role     string
	Selected []string
	Omitted  []string
	Scoped   int
	servers  map[string]mcpScopeServer
	// lookup resolves the ${VAR} sources of a header, os.LookupEnv unless a test sets it.
	lookup environmentLookup
}

var mcpScopeEnvRef = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(?::-([^}]*))?\}`)

// unreadableHeaders maps a header to its variables when every ${VAR} in the value
// is unset or blank with no default. A present header turns Claude's OAuth fallback off.
func unreadableHeaders(headers map[string]string, lookup environmentLookup) map[string][]string {
	dropped := map[string][]string{}
	for header, value := range headers {
		refs := mcpScopeEnvRef.FindAllStringSubmatch(value, -1)
		if len(refs) == 0 {
			continue
		}
		var names []string
		for _, ref := range refs {
			if current, _ := lookup(ref[1]); strings.TrimSpace(current) != "" || strings.TrimSpace(ref[2]) != "" {
				names = nil
				break
			}
			names = append(names, ref[1])
		}
		if names != nil {
			dropped[header] = names
		}
	}
	return dropped
}

func (selection mcpScopeSelection) headerLookup() environmentLookup {
	if selection.lookup == nil {
		return os.LookupEnv
	}
	return selection.lookup
}

// headerOmissions words one line per header left off the generated config, so
// the launch names the cause. The value is never read into the line.
func (selection mcpScopeSelection) headerOmissions() []string {
	var lines []string
	for _, name := range selection.Selected {
		dropped := unreadableHeaders(selection.servers[name].Headers, selection.headerLookup())
		for _, header := range sortedNativeKeys(selection.servers[name].Headers) {
			if variables, ok := dropped[header]; ok {
				lines = append(lines, fmt.Sprintf(
					"aos: MCP %s: no %s header, %s is unset or empty. The server starts unauthenticated. Fix the credential read warned above, then relaunch",
					name, header, strings.Join(variables, ", ")))
			}
		}
	}
	return lines
}

var errNoMCPInventory = errors.New("no MCP inventory")

// loadMCPScopeInventory refuses a tag that names no roster role, so a renamed
// role cannot silently lose its servers. Retired aliases are not resolved here.
func loadMCPScopeInventory(path string, roles []string) (map[string]mcpScopeServer, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, errNoMCPInventory
	}
	if err != nil {
		return nil, fmt.Errorf("read MCP inventory %s: %w", path, err)
	}
	var top struct {
		Servers map[string]mcpScopeServer `json:"mcpServers"`
	}
	if err := json.Unmarshal(raw, &top); err != nil {
		return nil, fmt.Errorf("parse MCP inventory %s: %w", path, err)
	}
	known := map[string]bool{}
	for _, role := range roles {
		known[role] = true
	}
	for _, name := range mcpScopeNames(top.Servers) {
		for _, tag := range top.Servers[name].AOS.Roles {
			if !known[tag] {
				return nil, fmt.Errorf("MCP server %q is tagged for role %q, which is not a roster role", name, tag)
			}
		}
	}
	return top.Servers, nil
}

// selectMCPScope keeps every untagged server and every server tagged for role.
func selectMCPScope(servers map[string]mcpScopeServer, role string) mcpScopeSelection {
	selection := mcpScopeSelection{Role: role, servers: map[string]mcpScopeServer{}}
	for _, name := range mcpScopeNames(servers) {
		server := servers[name]
		tagged := false
		for _, tag := range server.AOS.Roles {
			tagged = tagged || tag == role
		}
		switch {
		case len(server.AOS.Roles) == 0:
			selection.Selected = append(selection.Selected, name)
			selection.servers[name] = server
		case tagged:
			selection.Selected = append(selection.Selected, name)
			selection.servers[name] = server
			selection.Scoped++
		default:
			selection.Omitted = append(selection.Omitted, name)
		}
	}
	return selection
}

func (selection mcpScopeSelection) summary() string {
	return fmt.Sprintf("aos: MCP for %s: %d servers (%d role-scoped), %d omitted",
		selection.Role, len(selection.Selected), selection.Scoped, len(selection.Omitted))
}

// codexOverrides disables each omitted server for one launch, since the Codex
// registry is a shared host file that cannot hold a per-seat list.
func (selection mcpScopeSelection) codexOverrides() []string {
	var args []string
	for _, name := range selection.Omitted {
		args = append(args, "-c", "mcp_servers."+name+".enabled=false")
	}
	return args
}

// writeClaudeConfig names the file by its content so seats of one role share it.
// ${HOME} expands against the inventory home, as the host projection does.
func (selection mcpScopeSelection) writeClaudeConfig(dir, home string) (string, error) {
	out := map[string]any{}
	for name, server := range selection.servers {
		out[name] = mcpScopeClaudeServer(server, home, selection.headerLookup())
	}
	payload, err := json.MarshalIndent(map[string]any{"mcpServers": out}, "", "  ")
	if err != nil {
		return "", err
	}
	payload = append(payload, '\n')
	sum := sha256.Sum256(payload)
	path := filepath.Join(dir, fmt.Sprintf("%s-%s.json", selection.Role, hex.EncodeToString(sum[:])[:12]))
	if existing, err := os.ReadFile(path); err == nil && string(existing) == string(payload) {
		return path, nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(dir, ".mcp-*.json")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(payload); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	return path, os.Rename(tmp.Name(), path)
}

func mcpScopeClaudeServer(server mcpScopeServer, home string, lookup environmentLookup) map[string]any {
	expand := func(value string) string { return strings.ReplaceAll(value, "${HOME}", home) }
	expandPath := func(value string) string {
		if strings.Contains(value, "${HOME}") {
			return filepath.Clean(filepath.FromSlash(expand(value)))
		}
		return value
	}
	expandMap := func(values map[string]string) map[string]string {
		out := make(map[string]string, len(values))
		for key, value := range values {
			out[key] = expand(value)
		}
		return out
	}
	endpoint := server.URL
	if strings.TrimSpace(endpoint) == "" {
		endpoint = server.BaseURL
	}
	if strings.TrimSpace(endpoint) != "" {
		out := map[string]any{"type": "http", "url": expand(endpoint)}
		// Unreadable headers are left off. The rest keep ${VAR} literal, so no secret
		// reaches the file.
		headers := map[string]string{}
		dropped := unreadableHeaders(server.Headers, lookup)
		for header, value := range server.Headers {
			if _, skip := dropped[header]; !skip {
				headers[header] = value
			}
		}
		if len(headers) > 0 {
			out["headers"] = expandMap(headers)
		}
		return out
	}
	out := map[string]any{"command": expandPath(server.Command)}
	if len(server.Args) > 0 {
		args := make([]string, len(server.Args))
		for index, arg := range server.Args {
			args[index] = expand(arg)
		}
		out["args"] = args
	}
	if len(server.Env) > 0 {
		out["env"] = expandMap(server.Env)
	}
	if strings.TrimSpace(server.Cwd) != "" {
		out["cwd"] = expandPath(server.Cwd)
	}
	return out
}

func mcpScopeNames(servers map[string]mcpScopeServer) []string {
	names := make([]string, 0, len(servers))
	for name := range servers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func agentComposeRoleSlugs(ctx context.Context) ([]string, error) {
	out, err := exec.CommandContext(ctx, "agent-compose", "catalog", "roles", "--json").Output()
	if err != nil {
		return nil, fmt.Errorf("agent-compose catalog roles: %w", err)
	}
	var catalog struct {
		Items []struct {
			Slug string `json:"slug"`
		} `json:"items"`
	}
	if err := json.Unmarshal(out, &catalog); err != nil {
		return nil, fmt.Errorf("parse role catalog: %w", err)
	}
	roles := make([]string, 0, len(catalog.Items))
	for _, item := range catalog.Items {
		roles = append(roles, item.Slug)
	}
	return roles, nil
}

// nativeSpecMCPScope narrows the inventory at <home>/.mcporter/mcporter.json to
// the role, and refuses without one, as agent-compose's launch does (#8260).
func nativeSpecMCPScope(ctx context.Context, spec nativeLaunchSpec, role, inventoryHome, stateDir string, args []string) (mcpScopeLaunch, error) {
	switch spec.Harness {
	case "claude":
		if nativeSpecArgsCarry(args, "--mcp-config") || nativeSpecArgsCarry(args, "--strict-mcp-config") {
			return mcpScopeLaunch{}, nil
		}
	case "codex":
	case "goose":
		if !gooseScopeApplies(args) {
			return mcpScopeLaunch{}, nil
		}
	case "opencode":
		if strings.TrimSpace(os.Getenv(openCodeConfigEnv)) != "" {
			return mcpScopeLaunch{}, nil
		}
	default:
		return mcpScopeLaunch{}, nil
	}
	roles, err := agentComposeRoleSlugs(ctx)
	if err != nil {
		return mcpScopeLaunch{}, err
	}
	servers, err := loadMCPScopeInventory(filepath.Join(inventoryHome, ".mcporter", "mcporter.json"), roles)
	if errors.Is(err, errNoMCPInventory) {
		return mcpScopeLaunch{}, fmt.Errorf("mcp-scope refused the launch: %w; pass --mcp-config to launch with a scope of your own", err)
	}
	if err != nil {
		return mcpScopeLaunch{}, err
	}
	selection := selectMCPScope(servers, role)
	fmt.Fprintln(os.Stderr, selection.summary())
	for _, line := range selection.headerOmissions() {
		fmt.Fprintln(os.Stderr, line)
	}
	switch spec.Harness {
	case "codex":
		return mcpScopeLaunch{Flags: selection.codexOverrides()}, nil
	case "goose":
		keep, err := gooseKeptExtensions(inventoryHome)
		if err != nil {
			return mcpScopeLaunch{}, err
		}
		flags, err := selection.gooseArgs(keep, inventoryHome)
		return mcpScopeLaunch{Flags: flags}, err
	case "opencode":
		// An env var rather than a flag, set on this process the way the spec's
		// own env is, since aos execs the harness with it.
		content, err := selection.openCodeConfig(inventoryHome)
		if err != nil {
			return mcpScopeLaunch{}, err
		}
		return mcpScopeLaunch{}, os.Setenv(openCodeConfigEnv, content)
	}
	path, err := selection.writeClaudeConfig(filepath.Join(stateDir, "mcp"), inventoryHome)
	if err != nil {
		return mcpScopeLaunch{}, err
	}
	if !connectorsApply(os.Stderr, spec, args) {
		return mcpScopeLaunch{Flags: []string{"--strict-mcp-config", "--mcp-config", path}}, nil
	}
	fmt.Fprintf(os.Stderr, "aos: claude.ai connectors for %s: %d, behind an allowlist\n", role, len(spec.ClaudeAIConnectors))
	return mcpScopeLaunch{Flags: []string{"--mcp-config", path}, Allow: selection.allowlist(spec.ClaudeAIConnectors, inventoryHome), Deny: connectorDenied(spec.ClaudeAIConnectors)}, nil
}

// openCodeConfigEnv is OpenCode's inline config, merged over every other layer.
const openCodeConfigEnv = "OPENCODE_CONFIG_CONTENT"

// gooseSessionVerbs load extensions. Bare `goose` starts a session too, but
// its root command parses none of the scope flags.
var gooseSessionVerbs = map[string]bool{"session": true, "s": true, "run": true}

// gooseScopeApplies is false for a verb that loads no extensions, a caller's
// own --no-profile, and a resume, which restores the set its session recorded.
func gooseScopeApplies(args []string) bool {
	if len(args) > 0 && !gooseSessionVerbs[args[0]] {
		return false
	}
	return !nativeSpecArgsCarry(args, "--no-profile") && !nativeSpecArgsCarry(args, "--resume") &&
		!nativeSpecArgsCarry(args, "-r")
}

// gooseCommand puts the scope after the session verb, adding `session` to a
// bare launch.
func gooseCommand(harness string, args, scope []string) []string {
	verb, rest := "session", args
	if len(args) > 0 {
		verb, rest = args[0], args[1:]
	}
	command := append([]string{harness, verb}, scope...)
	return append(command, rest...)
}

// gooseKeptExtensions lists the enabled builtin and platform extensions in the
// user's goose config, which --no-profile would otherwise drop with the rest.
func gooseKeptExtensions(home string) ([]string, error) {
	dir := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME"))
	if dir == "" {
		dir = filepath.Join(home, ".config")
	}
	path := filepath.Join(dir, "goose", "config.yaml")
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read goose config: %w", err)
	}
	var config struct {
		Extensions map[string]struct {
			Enabled bool   `yaml:"enabled"`
			Type    string `yaml:"type"`
		} `yaml:"extensions"`
	}
	if err := yaml.Unmarshal(raw, &config); err != nil {
		return nil, fmt.Errorf("parse goose config %s: %w", path, err)
	}
	var keep []string
	for name, extension := range config.Extensions {
		if extension.Enabled && (extension.Type == "builtin" || extension.Type == "platform") {
			keep = append(keep, name)
		}
	}
	sort.Strings(keep)
	return keep, nil
}

// gooseArgs replaces goose's configured extensions for one session with the
// role's servers, keeping the builtin and platform extensions named in keep.
func (selection mcpScopeSelection) gooseArgs(keep []string, home string) ([]string, error) {
	args := []string{"--no-profile"}
	if len(keep) > 0 {
		args = append(args, "--with-builtin", strings.Join(keep, ","))
	}
	for _, name := range selection.Selected {
		server := selection.servers[name]
		rendered := mcpScopeClaudeServer(server, home, selection.headerLookup())
		if url, ok := rendered["url"].(string); ok {
			// The flag takes a URL alone, so a header would be dropped silently.
			if len(server.Headers) > 0 {
				return nil, fmt.Errorf("goose cannot pass headers for MCP server %q", name)
			}
			args = append(args, "--with-streamable-http-extension", url)
			continue
		}
		spec, err := gooseStdio(name, server, rendered)
		if err != nil {
			return nil, err
		}
		args = append(args, "--with-extension", spec)
	}
	return args, nil
}

// gooseStdio renders `name:ENV=v command args` in the grammar goose splits it
// with: whitespace-separated, quotes group, and no backslash escapes.
func gooseStdio(name string, server mcpScopeServer, rendered map[string]any) (string, error) {
	if strings.TrimSpace(server.Cwd) != "" {
		return "", fmt.Errorf("goose cannot set a working directory for MCP server %q", name)
	}
	var parts []string
	env, _ := rendered["env"].(map[string]string)
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		parts = append(parts, key+"="+env[key])
	}
	parts = append(parts, rendered["command"].(string))
	if args, ok := rendered["args"].([]string); ok {
		parts = append(parts, args...)
	}
	quoted := make([]string, len(parts))
	for index, part := range parts {
		value, err := gooseQuote(part)
		if err != nil {
			return "", fmt.Errorf("goose cannot express an argument of MCP server %q: %w", name, err)
		}
		quoted[index] = value
	}
	return name + ":" + strings.Join(quoted, " "), nil
}

func gooseQuote(part string) (string, error) {
	switch {
	case part == "":
		return "", errors.New("an empty argument splits to nothing")
	case !strings.ContainsAny(part, " \t\n\"'"):
		return part, nil
	case !strings.Contains(part, `"`):
		return `"` + part + `"`, nil
	case !strings.Contains(part, "'"):
		return "'" + part + "'", nil
	}
	return "", errors.New("it holds both quote characters")
}

// openCodeConfig defines the role's servers and turns off each omitted one.
func (selection mcpScopeSelection) openCodeConfig(home string) (string, error) {
	mcp := map[string]any{}
	for _, name := range selection.Selected {
		server := selection.servers[name]
		rendered := mcpScopeClaudeServer(server, home, selection.headerLookup())
		if url, ok := rendered["url"].(string); ok {
			entry := map[string]any{"type": "remote", "url": url, "enabled": true}
			if headers, ok := rendered["headers"]; ok {
				entry["headers"] = headers
			}
			mcp[name] = entry
			continue
		}
		if strings.TrimSpace(server.Cwd) != "" {
			return "", fmt.Errorf("opencode cannot set a working directory for MCP server %q", name)
		}
		command := []string{rendered["command"].(string)}
		if args, ok := rendered["args"].([]string); ok {
			command = append(command, args...)
		}
		entry := map[string]any{"type": "local", "command": command, "enabled": true}
		if env, ok := rendered["env"]; ok {
			entry["environment"] = env
		}
		mcp[name] = entry
	}
	for _, name := range selection.Omitted {
		mcp[name] = map[string]any{"enabled": false}
	}
	payload, err := json.Marshal(map[string]any{"mcp": mcp})
	return string(payload), err
}
