package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func headerEnv(t *testing.T, session string) {
	t.Helper()
	t.Setenv(atermSessionEnv, session)
	t.Setenv(gooseHeadersEnv, "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv(openCodeConfigFile, "")
}

func TestGooseSendsTheSessionNameWhenItsKeyIsInTheEnvironment(t *testing.T) {
	headerEnv(t, "eng-platform-beetle-ox-ep48")
	t.Setenv("OPENAI_API_KEY", "sk-test")
	var stderr bytes.Buffer
	applyNativeSessionHeader("goose", t.TempDir(), &stderr)
	if got := os.Getenv(gooseHeadersEnv); got != "x-agent-session-id=eng-platform-beetle-ox-ep48" || stderr.Len() != 0 {
		t.Fatalf("headers = %q, stderr = %q", got, stderr.String())
	}
}

func TestGooseKeepsOtherHeadersAndReplacesAnEarlierSessionHeader(t *testing.T) {
	headerEnv(t, "seat-b")
	t.Setenv("OPENAI_API_KEY", "sk-test")
	t.Setenv(gooseHeadersEnv, "X-Team=coilyco, X-Agent-Session-Id=stale ,x-other=1")
	applyNativeSessionHeader("goose", t.TempDir(), &bytes.Buffer{})
	if got, want := os.Getenv(gooseHeadersEnv), "X-Team=coilyco,x-other=1,x-agent-session-id=seat-b"; got != want {
		t.Fatalf("headers = %q, want %q", got, want)
	}
}

func TestGooseWithoutAnEnvironmentKeyIsLeftAloneAndSaysWhy(t *testing.T) {
	headerEnv(t, "seat-b")
	var stderr bytes.Buffer
	applyNativeSessionHeader("goose", t.TempDir(), &stderr)
	if os.Getenv(gooseHeadersEnv) != "" {
		t.Fatalf("headers set to %q, which goose would ignore", os.Getenv(gooseHeadersEnv))
	}
	if !strings.Contains(stderr.String(), "no context meter for this goose seat") || !strings.Contains(stderr.String(), "OPENAI_API_KEY") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestOpenCodeGetsAConfigFileHoldingOnlyTheProviderHeader(t *testing.T) {
	headerEnv(t, "seat-c")
	home := t.TempDir()
	var stderr bytes.Buffer
	applyNativeSessionHeader("opencode", home, &stderr)
	path := os.Getenv(openCodeConfigFile)
	if path != filepath.Join(home, ".aos", "opencode-session-header.json") || stderr.Len() != 0 {
		t.Fatalf("OPENCODE_CONFIG = %q, stderr = %q", path, stderr.String())
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Provider map[string]struct {
			Options struct {
				Headers map[string]string `json:"headers"`
			} `json:"options"`
			Models map[string]any `json:"models"`
		} `json:"provider"`
		MCP map[string]any `json:"mcp"`
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatal(err)
	}
	entry := config.Provider["agent-proxy"]
	if len(config.Provider) != 1 || entry.Options.Headers["x-agent-session-id"] != "seat-c" || entry.Models != nil || config.MCP != nil {
		t.Fatalf("config = %s", raw)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, err = %v", info.Mode(), err)
	}
}

func TestOpenCodeLeavesACallersConfigPathAndSaysSo(t *testing.T) {
	headerEnv(t, "seat-c")
	t.Setenv(openCodeConfigFile, "/caller/opencode.json")
	var stderr bytes.Buffer
	applyNativeSessionHeader("opencode", t.TempDir(), &stderr)
	if os.Getenv(openCodeConfigFile) != "/caller/opencode.json" || !strings.Contains(stderr.String(), "already set") {
		t.Fatalf("OPENCODE_CONFIG = %q, stderr = %q", os.Getenv(openCodeConfigFile), stderr.String())
	}
}

func TestOpenCodeWithNoSessionHomeSaysSo(t *testing.T) {
	headerEnv(t, "seat-c")
	var stderr bytes.Buffer
	applyNativeSessionHeader("opencode", "", &stderr)
	if os.Getenv(openCodeConfigFile) != "" || !strings.Contains(stderr.String(), "no session home") {
		t.Fatalf("OPENCODE_CONFIG = %q, stderr = %q", os.Getenv(openCodeConfigFile), stderr.String())
	}
}

func TestNoHeaderOutsideAtermOrOnAnotherHarnessOrWithAMalformedName(t *testing.T) {
	for name, tc := range map[string]struct{ harness, session string }{
		"outside aterm":   {"goose", ""},
		"claude":          {"claude", "seat-a"},
		"codex":           {"codex", "seat-a"},
		"comma in a name": {"goose", "a,x-evil=1"},
		"space":           {"opencode", "a b"},
	} {
		headerEnv(t, tc.session)
		t.Setenv("OPENAI_API_KEY", "sk-test")
		var stderr bytes.Buffer
		applyNativeSessionHeader(tc.harness, t.TempDir(), &stderr)
		if os.Getenv(gooseHeadersEnv) != "" || os.Getenv(openCodeConfigFile) != "" {
			t.Errorf("%s: a header was set", name)
		}
		if wantWarning := name == "comma in a name" || name == "space"; wantWarning != (stderr.Len() > 0) {
			t.Errorf("%s: stderr = %q", name, stderr.String())
		}
	}
}
