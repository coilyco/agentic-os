package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveAcomposeCheckinCodex(t *testing.T) {
	t.Parallel()
	spec, err := resolveAcomposeCheckin(" codex ")
	if err != nil {
		t.Fatal(err)
	}
	if spec.Agent != "codex" || spec.Layout != "codex" {
		t.Fatalf("resolveAcomposeCheckin() = %#v", spec)
	}
	want := []string{
		"bash",
		"-o",
		"pipefail",
		"-c",
		acomposeCheckinScript,
		"aos-acompose-checkin",
		"exec",
		"--ephemeral",
		"--sandbox",
		"read-only",
		"--skip-git-repo-check",
		"--color",
		"never",
		acomposeCheckinPrompt,
	}
	if strings.Join(spec.Command, "\n") != strings.Join(want, "\n") {
		t.Fatalf("resolveAcomposeCheckin() command = %q, want %q", spec.Command, want)
	}
}

func TestResolveAcomposeCheckinRejectsMissingAndUnknownAgents(t *testing.T) {
	t.Parallel()
	for _, agent := range []string{"", "claude"} {
		if _, err := resolveAcomposeCheckin(agent); err == nil {
			t.Fatalf("resolveAcomposeCheckin(%q) passed", agent)
		}
	}
}

func TestAcomposeCheckinCodexDryRun(t *testing.T) {
	var output bytes.Buffer
	cmd := newCommand()
	cmd.Writer = &output
	err := cmd.Run(context.Background(), []string{
		"aos",
		"--role", "engineer",
		"--agent", "codex",
		"--image", "agentic-os:test",
		"--auth=false",
		"--dry-run",
		"acompose-checkin",
	})
	if err != nil {
		t.Fatal(err)
	}
	rendered := output.String()
	for _, want := range []string{
		"agentic-os:test",
		"--role engineer",
		"--layout codex",
		"--composed",
		"--guarded",
		"--no-substrate",
		"-- bash -o pipefail -c",
		"aos-acompose-checkin exec --ephemeral --sandbox read-only",
		acomposeCheckinPrompt,
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("dry-run missing %q:\n%s", want, rendered)
		}
	}
}

func TestAcomposeCheckinScriptRendersTranscriptAndPreservesFailure(t *testing.T) {
	t.Parallel()
	fakeCodex := `codex() {
	printf '%s\n' \
		'OpenAI Codex' \
		'--------' \
		'workdir: /workspace' \
		'--------' \
		'user' \
		'prompt' \
		'warning: fallback' \
		'codex' \
		'ROLE-CONFIRMED: engineer' \
		'tokens used' \
		'7' >&2
	printf '%s\n' 'duplicate final stdout'
	return 17
}
`
	command := exec.Command(
		"bash",
		"-o",
		"pipefail",
		"-c",
		fakeCodex+acomposeCheckinScript,
		"aos-acompose-checkin-test",
	)
	output, err := command.Output()
	var exitError *exec.ExitError
	if !errors.As(err, &exitError) || exitError.ExitCode() != 17 {
		t.Fatalf("check-in formatter error = %v", err)
	}
	want := "\nOpenAI Codex\n\n--------\n\nworkdir: /workspace\n\n--------\n\n" +
		"user\nprompt\n\nwarning: fallback\n\ncodex\nROLE-CONFIRMED: engineer\n\n" +
		"tokens used\n7\n\n"
	if string(output) != want {
		t.Fatalf("check-in transcript:\n%q\nwant:\n%q", output, want)
	}
	if strings.Contains(string(output), "duplicate final stdout") {
		t.Fatal("check-in transcript included duplicate final stdout")
	}
}

func TestAcomposeCheckinRejectsConflictingLayout(t *testing.T) {
	cmd := newCommand()
	err := cmd.Run(context.Background(), []string{
		"aos",
		"--role", "engineer",
		"--agent", "codex",
		"--layout", "goose",
		"--dry-run",
		"acompose-checkin",
	})
	if err == nil || !strings.Contains(err.Error(), "conflicts") {
		t.Fatalf("acompose-checkin conflict error = %v", err)
	}
}

func TestAcomposeCheckinStagesCodexAuthByDefault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	auth := filepath.Join(home, ".codex", "auth.json")
	if err := os.MkdirAll(filepath.Dir(auth), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(auth, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	cmd := newCommand()
	cmd.Writer = &output
	err := cmd.Run(context.Background(), []string{
		"aos",
		"--role", "engineer",
		"--agent", "codex",
		"--dry-run",
		"acompose-checkin",
	})
	if err != nil {
		t.Fatal(err)
	}
	rendered := output.String()
	for _, want := range []string{auth, containerAuthRoot + "/codex.json"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("dry-run missing auth path %q:\n%s", want, rendered)
		}
	}
}
