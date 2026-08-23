package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeAssignedComposeRoleAcceptsOnlyTheAssignedHarnessLaunch(t *testing.T) {
	role, binary, ok := nativeAssignedComposeRole(
		[]string{"/usr/local/bin/agent-compose", "launch", "platform", "codex", "--model", "gpt"},
		"codex",
	)
	if !ok || role != "platform" || binary != "/usr/local/bin/agent-compose" {
		t.Fatalf("nativeAssignedComposeRole() = %q, %q, %t", role, binary, ok)
	}
	for _, command := range [][]string{
		{"agent-compose", "compose", "--", "codex"},
		{"agent-compose", "launch", "platform", "claude"},
		{"agent-compose", "launch", "Engineer", "codex"},
		{"other", "launch", "platform", "codex"},
	} {
		if _, _, ok := nativeAssignedComposeRole(command, "codex"); ok {
			t.Fatalf("nativeAssignedComposeRole(%q) accepted an unrelated command", command)
		}
	}
}

func TestLoadNativeTerminalAnnotationUsesTheOverlayContract(t *testing.T) {
	directory := t.TempDir()
	agentCompose := filepath.Join(directory, "agent-compose")
	script := "#!/bin/sh\nprintf '%s\\n' '{\"format\":\"agent-compose.overlay.v1\",\"annotation\":\"Angie [she] (Engineer)\"}'\n"
	if err := os.WriteFile(agentCompose, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	annotation, err := loadNativeTerminalAnnotation(context.Background(), agentCompose, "platform", "codex")
	if err != nil {
		t.Fatal(err)
	}
	if annotation != "Angie [she] (Engineer)" {
		t.Fatalf("annotation = %q", annotation)
	}
}

func TestNativeTerminalTitleSupportedNeedsATerminal(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if nativeTerminalTitleSupported(file, func(string) string { return "xterm-256color" }) {
		t.Fatal("regular file enabled terminal titles")
	}
}

func TestValidateNativeTerminalTitleRejectsUnsafeValues(t *testing.T) {
	for _, annotation := range []string{"", "Angie\n[she]", strings.Repeat("a", nativeTerminalTitleMaxRunes+1)} {
		if _, err := validateNativeTerminalTitle(annotation); err == nil {
			t.Fatalf("validateNativeTerminalTitle(%q) succeeded", annotation)
		}
	}
}

func TestNativeTerminalTitleIncludesTheShortSessionID(t *testing.T) {
	title, err := nativeTerminalTitle("Angie [she] (Engineer)", "sm89")
	if err != nil {
		t.Fatal(err)
	}
	if title != "Angie [she] (Engineer) // sm89" {
		t.Fatalf("title = %q", title)
	}
}

func TestWriteNativeTerminalTitleUsesOSC2(t *testing.T) {
	var output bytes.Buffer
	if err := writeNativeTerminalTitle(&output, "Angie [she] (Engineer) // sm89"); err != nil {
		t.Fatal(err)
	}
	if got, want := output.String(), "\x1b]2;Angie [she] (Engineer) // sm89\a"; got != want {
		t.Fatalf("title control sequence = %q, want %q", got, want)
	}
}
