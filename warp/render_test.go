package main

import (
	"strings"
	"testing"
)

func TestProfileUsesRepoRootForCommonShell(t *testing.T) {
	data := templateData{
		RepoRoot:   "X:/projects/coilyco-flight-deck/agentic-os",
		StartupDir: "X:/projects",
	}
	rendered, err := render("profile.ps1.tmpl", data)
	if err != nil {
		t.Fatalf("render(profile.ps1.tmpl): %v", err)
	}
	got := string(rendered)
	if !strings.Contains(got, "Set-Location -Path 'X:/projects'") {
		t.Fatalf("profile should land in StartupDir, got:\n%s", got)
	}
	if !strings.Contains(got, "source 'X:/projects/coilyco-flight-deck/agentic-os/shell/common.sh'") {
		t.Fatalf("profile should source common.sh from RepoRoot, got:\n%s", got)
	}
	if strings.Contains(got, "source 'X:/projects/shell/common.sh'") {
		t.Fatalf("profile sourced common.sh from StartupDir instead of RepoRoot")
	}
}
