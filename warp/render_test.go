package main

import (
	"os"
	"strings"
	"testing"
)

func TestProfileUsesNativePowerShellEnvironment(t *testing.T) {
	data := templateData{
		RepoRoot:    "X:/projects/coilyco-flight-deck/agentic-os",
		ProjectsDir: "X:/projects",
		StartupDir:  "X:/projects/coilysiren/coilysiren",
	}
	rendered, err := render("profile.ps1.tmpl", data)
	if err != nil {
		t.Fatalf("render(profile.ps1.tmpl): %v", err)
	}
	got := string(rendered)
	if !strings.Contains(got, "Set-Location -Path 'X:/projects/coilysiren/coilysiren'") {
		t.Fatalf("profile should land in StartupDir, got:\n%s", got)
	}
	if !strings.Contains(got, "$_commonShellPath = 'X:/projects/coilyco-flight-deck/agentic-os/shell/common.sh'") {
		t.Fatalf("profile should read shared exports from RepoRoot, got:\n%s", got)
	}
	if !strings.Contains(got, "$env:WARD_LOCKDOWN_ROOT = 'X:/projects'") {
		t.Fatalf("profile should keep lockdown at ProjectsDir")
	}
	for _, forbidden := range []string{"bash.exe", "--norc", "--noprofile", "source '"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("profile should not launch Bash, found %q", forbidden)
		}
	}
}

func TestCommonShellSharedEnvironmentBlockIsDeclarative(t *testing.T) {
	commonShell, err := os.ReadFile("../shell/common.sh")
	if err != nil {
		t.Fatalf("read common.sh: %v", err)
	}
	text := string(commonShell)
	start := strings.Index(text, "# shared-environment: begin")
	end := strings.Index(text, "# shared-environment: end")
	if start < 0 || end <= start {
		t.Fatal("common.sh should contain the ordered shared-environment markers")
	}
	exports := 0
	for _, line := range strings.Split(text[start:end], "\n") {
		if strings.HasPrefix(line, "export ") {
			exports++
			continue
		}
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		t.Fatalf("shared environment block must stay declarative, got %q", line)
	}
	if exports == 0 {
		t.Fatal("shared environment block should contain exports")
	}
}
