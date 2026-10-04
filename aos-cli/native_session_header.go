package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// A proxy-backed seat sends its aterm session name as x-agent-session-id, which the
// proxy keys usage by for the context meter. See the context-meter reference.
const (
	atermSessionEnv    = "ATERM_SESSION"
	agentSessionHeader = "x-agent-session-id"
	gooseHeadersEnv    = "OPENAI_CUSTOM_HEADERS"
	openCodeConfigFile = "OPENCODE_CONFIG"
	// agentProxyProvider is the opencode provider id the role profiles pin models under.
	agentProxyProvider = "agent-proxy"
)

// atermSessionName is the alphabet of a daemon session name, kept to what the proxy
// accepts and to what goose's comma and equals header syntax cannot misread.
var atermSessionName = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

// applyNativeSessionHeader makes a goose or opencode seat send its session name.
// A case where it cannot says so, since the seat would just show no meter.
func applyNativeSessionHeader(harness, sessionHome string, stderr io.Writer) {
	name := strings.TrimSpace(os.Getenv(atermSessionEnv))
	if name == "" || (harness != "goose" && harness != "opencode") {
		return
	}
	warn := func(format string, args ...any) {
		fmt.Fprintf(stderr, "aos: warning: no context meter for this %s seat: %s\n", harness, fmt.Sprintf(format, args...))
	}
	if !atermSessionName.MatchString(name) {
		warn("%s is not a session name Agent Proxy accepts", atermSessionEnv)
		return
	}
	if harness == "goose" {
		// Goose reads its extra provider keys from env only when the API key is
		// there too, and from its secret store otherwise.
		if strings.TrimSpace(os.Getenv("OPENAI_API_KEY")) == "" {
			warn("goose reads %s from its secret store when OPENAI_API_KEY is not in the environment", gooseHeadersEnv)
			return
		}
		if err := os.Setenv(gooseHeadersEnv, withSessionHeader(os.Getenv(gooseHeadersEnv), name)); err != nil {
			warn("%v", err)
		}
		return
	}
	if err := setOpenCodeSessionHeader(sessionHome, name); err != nil {
		warn("%v", err)
	}
}

// withSessionHeader is goose's `key=value,key=value` list with this seat's header
// last and any earlier copy of it dropped.
func withSessionHeader(existing, name string) string {
	var kept []string
	for _, pair := range strings.Split(existing, ",") {
		key, _, _ := strings.Cut(pair, "=")
		if strings.TrimSpace(pair) != "" && !strings.EqualFold(strings.TrimSpace(key), agentSessionHeader) {
			kept = append(kept, strings.TrimSpace(pair))
		}
	}
	return strings.Join(append(kept, agentSessionHeader+"="+name), ",")
}

// setOpenCodeSessionHeader writes a config holding only the provider header. OpenCode
// deep-merges it, so the host's provider and the MCP scope survive. A caller's wins.
func setOpenCodeSessionHeader(sessionHome, name string) error {
	if strings.TrimSpace(os.Getenv(openCodeConfigFile)) != "" {
		return fmt.Errorf("%s is already set and is left as it is", openCodeConfigFile)
	}
	if strings.TrimSpace(sessionHome) == "" {
		return fmt.Errorf("this launch has no session home to keep the header config in")
	}
	config, err := json.Marshal(map[string]any{
		"$schema": "https://opencode.ai/config.json",
		"provider": map[string]any{agentProxyProvider: map[string]any{
			"options": map[string]any{"headers": map[string]string{agentSessionHeader: name}},
		}},
	})
	if err != nil {
		return err
	}
	dir := filepath.Join(sessionHome, ".aos")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	path := filepath.Join(dir, "opencode-session-header.json")
	if err := os.WriteFile(path, append(config, '\n'), 0o600); err != nil {
		return err
	}
	return os.Setenv(openCodeConfigFile, path)
}
