package main

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func writeSeatsTestHost(t *testing.T, home string) {
	t.Helper()
	claude := filepath.Join(home, ".claude")
	for _, dir := range []string{filepath.Join(claude, "skills", "personal"), filepath.Join(claude, "themes")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for name, body := range map[string]string{
		filepath.Join(claude, "settings.json"):     "{}\n",
		filepath.Join(claude, "CLAUDE.md"):         "host prompt\n",
		filepath.Join(claude, ".credentials.json"): "{}\n",
		filepath.Join(home, ".claude.json"):        "{}\n",
	} {
		if err := os.WriteFile(name, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func seatsLinkTarget(t *testing.T, link string) string {
	t.Helper()
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("%s: %v", link, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("%s is not a symlink, mode %s", link, info.Mode())
	}
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatal(err)
	}
	return target
}

func TestStageNativeClaudeSeatsDirLinksTheHostExceptSeatLoadPointsAndLogin(t *testing.T) {
	home := t.TempDir()
	writeSeatsTestHost(t, home)

	created, err := stageNativeClaudeSeatsDir(home)
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("the first staging should report the directory as created")
	}
	seats := nativeClaudeSeatsDir(home)
	for name, want := range map[string]string{
		"settings.json": filepath.Join(home, ".claude", "settings.json"),
		"themes":        filepath.Join(home, ".claude", "themes"),
		".claude.json":  filepath.Join(home, ".claude.json"),
	} {
		if got := seatsLinkTarget(t, filepath.Join(seats, name)); got != want {
			t.Errorf("%s -> %s, want %s", name, got, want)
		}
	}
	for _, name := range []string{"CLAUDE.md", ".credentials.json"} {
		if _, err := os.Lstat(filepath.Join(seats, name)); err == nil {
			t.Errorf("%s must stay out of the shared directory", name)
		}
	}
	skills, err := os.Lstat(filepath.Join(seats, "skills"))
	if err != nil || !skills.IsDir() || skills.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("skills should be a real empty directory, got %v %v", skills, err)
	}
	if entries, _ := os.ReadDir(filepath.Join(seats, "skills")); len(entries) != 0 {
		t.Fatalf("the shared skills directory carries %d entries, want none", len(entries))
	}
}

func TestStageNativeClaudeSeatsDirRefreshesWithoutReplacing(t *testing.T) {
	home := t.TempDir()
	writeSeatsTestHost(t, home)
	if _, err := stageNativeClaudeSeatsDir(home); err != nil {
		t.Fatal(err)
	}
	seats := nativeClaudeSeatsDir(home)
	// What a seat wrote in the shared directory, and what the host grew since.
	own := filepath.Join(seats, "settings.json")
	if err := os.Remove(own); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(own, []byte("{\"seat\":true}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".claude", "plugins"), 0o755); err != nil {
		t.Fatal(err)
	}

	created, err := stageNativeClaudeSeatsDir(home)
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Fatal("a second staging must not report the directory as created")
	}
	if got := seatsLinkTarget(t, filepath.Join(seats, "plugins")); got != filepath.Join(home, ".claude", "plugins") {
		t.Errorf("plugins -> %s, want the host entry", got)
	}
	if raw, _ := os.ReadFile(own); string(raw) != "{\"seat\":true}\n" {
		t.Errorf("an entry already in the shared directory was replaced: %q", raw)
	}
}

func seatsCredential(now time.Time, ahead time.Duration) []byte {
	return []byte(fmt.Sprintf(`{"claudeAiOauth":{"accessToken":"t","refreshToken":"r","expiresAt":%d}}`,
		now.Add(ahead).UnixMilli()))
}

func TestSeedNativeClaudeSeatsCredentialCopiesAUsableHostLoginOnce(t *testing.T) {
	home := t.TempDir()
	now := time.Date(2026, 10, 7, 4, 0, 0, 0, time.UTC)
	writeSeatsTestHost(t, home)
	payload := seatsCredential(now, time.Hour)
	if err := os.WriteFile(canonicalClaudeCredentialPath(home), payload, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := stageNativeClaudeSeatsDir(home); err != nil {
		t.Fatal(err)
	}

	seeded, err := seedNativeClaudeSeatsCredential(home, now)
	if err != nil || !seeded {
		t.Fatalf("seeded = %v, %v; want true", seeded, err)
	}
	target := filepath.Join(nativeClaudeSeatsDir(home), ".credentials.json")
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode %s, want 0600", info.Mode().Perm())
	}
	if raw, _ := os.ReadFile(target); !reflect.DeepEqual(raw, payload) {
		t.Error("the shared directory did not receive the host login byte for byte")
	}

	if err := os.WriteFile(target, []byte("rotated\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if seeded, err := seedNativeClaudeSeatsCredential(home, now); err != nil || seeded {
		t.Fatalf("a present file must not be reseeded: seeded = %v, %v", seeded, err)
	}
	if raw, _ := os.ReadFile(target); string(raw) != "rotated\n" {
		t.Error("the second seed overwrote the shared login")
	}
}

func TestSeedNativeClaudeSeatsCredentialSkipsAnUnusableHostLogin(t *testing.T) {
	home := t.TempDir()
	now := time.Date(2026, 10, 7, 4, 0, 0, 0, time.UTC)
	writeSeatsTestHost(t, home)
	lapsed := []byte(fmt.Sprintf(`{"claudeAiOauth":{"accessToken":"t","expiresAt":%d}}`,
		now.Add(-time.Hour).UnixMilli()))
	if err := os.WriteFile(canonicalClaudeCredentialPath(home), lapsed, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := stageNativeClaudeSeatsDir(home); err != nil {
		t.Fatal(err)
	}

	seeded, err := seedNativeClaudeSeatsCredential(home, now)
	if err != nil || seeded {
		t.Fatalf("seeded = %v, %v; want false", seeded, err)
	}
	if _, err := os.Lstat(filepath.Join(nativeClaudeSeatsDir(home), ".credentials.json")); err == nil {
		t.Fatal("a lapsed login with no refresh token was copied anyway")
	}
}

func TestStageNativeClaudeSeatLinksTheLoadPoints(t *testing.T) {
	root := t.TempDir()
	sessionHome := filepath.Join(root, "home")
	if err := os.MkdirAll(filepath.Join(sessionHome, ".claude", "skills"), 0o755); err != nil {
		t.Fatal(err)
	}

	seat, err := stageNativeClaudeSeat(root, sessionHome)
	if err != nil {
		t.Fatal(err)
	}
	if seat != filepath.Join(root, "seat") {
		t.Fatalf("seat = %s", seat)
	}
	for _, name := range []string{"CLAUDE.md", "skills"} {
		want := filepath.Join(sessionHome, ".claude", name)
		if got := seatsLinkTarget(t, filepath.Join(seat, ".claude", name)); got != want {
			t.Errorf("%s -> %s, want %s", name, got, want)
		}
	}
	if _, err := stageNativeClaudeSeat(root, sessionHome); err != nil {
		t.Fatalf("restaging the seat should be a no-op: %v", err)
	}
}

func TestInsertNativeHarnessArgs(t *testing.T) {
	extra := []string{"--add-dir=/seat"}
	for _, tc := range []struct {
		name    string
		command []string
		want    []string
	}{
		{
			name:    "after the harness in an agent-compose launch, ahead of the caller's flags",
			command: []string{"agent-compose", "launch", "eng-platform", "claude", "--model", "opus", "--", "prompt"},
			want:    []string{"agent-compose", "launch", "eng-platform", "claude", "--add-dir=/seat", "--model", "opus", "--", "prompt"},
		},
		{
			name:    "after a bare harness",
			command: []string{"claude", "--model", "opus"},
			want:    []string{"claude", "--add-dir=/seat", "--model", "opus"},
		},
		{
			name:    "untouched when the command is neither",
			command: []string{"codex", "--model", "opus"},
			want:    []string{"codex", "--model", "opus"},
		},
	} {
		if got := insertNativeHarnessArgs(tc.command, "claude", extra); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s:\n got %q\nwant %q", tc.name, got, tc.want)
		}
	}
}

func TestNativeLaunchSharesTheClaudeSeatsDirectory(t *testing.T) {
	root := t.TempDir()
	repository, _ := createNativeTestRepository(t, root, "owner", "one")
	runtime := nativeTestRuntime(t, root)
	runtime.CWD = repository
	writeNativeTestPlan(t, runtime.PlanFile, "one")
	writeNativeTestList(t, runtime.FleetFile, "owner")
	writeSeatsTestHost(t, runtime.Home)
	t.Setenv(agentComposeRuntimeHomeEnv, "")
	t.Setenv(agentComposeClaudeConfigDirEnv, "")
	t.Setenv(claudeAdditionalDirectoriesClaudeMdEnv, "")

	workspace, err := prepareNativeLaunchWorkspaceWithOptions(
		runtime,
		"claude",
		nativeLaunchOptions{WorkspaceRoot: true},
	)
	if err != nil {
		t.Fatal(err)
	}

	_, lease := onlyNativeLease(t, runtime)
	seats := nativeClaudeSeatsDir(runtime.Home)
	if want := filepath.Join(lease.SessionRoot, "seat"); workspace.ClaudeSeat != want {
		t.Fatalf("seat = %s, want %s", workspace.ClaudeSeat, want)
	}
	if got := os.Getenv(agentComposeClaudeConfigDirEnv); got != seats {
		t.Errorf("%s = %q, want %q", agentComposeClaudeConfigDirEnv, got, seats)
	}
	if got := os.Getenv(claudeAdditionalDirectoriesClaudeMdEnv); got != "1" {
		t.Errorf("%s = %q, want 1", claudeAdditionalDirectoriesClaudeMdEnv, got)
	}
	if got := seatsLinkTarget(t, filepath.Join(workspace.ClaudeSeat, ".claude", "CLAUDE.md")); got != filepath.Join(lease.SessionHome, ".claude", "CLAUDE.md") {
		t.Errorf("seat CLAUDE.md -> %s", got)
	}
	// Trust and the external-import approval land on the host config through
	// the shared directory's link, keyed by the seat path too.
	projects, ok := readTestClaudeConfig(t, filepath.Join(runtime.Home, ".claude.json"))["projects"].(map[string]any)
	if !ok {
		t.Fatal("no projects were seeded on the host config")
	}
	entry, ok := projects[workspace.ClaudeSeat].(map[string]any)
	if !ok {
		t.Fatalf("seat %s is not a trusted project: %v", workspace.ClaudeSeat, projects)
	}
	for _, key := range nativeClaudeTrustKeys {
		if entry[key] != true {
			t.Errorf("%s = %v, want true", key, entry[key])
		}
	}
}

func TestNativeLaunchKeepsAStandaloneHomeOffTheSeatsDirectory(t *testing.T) {
	root := t.TempDir()
	repository, _ := createNativeTestRepository(t, root, "owner", "one")
	runtime := nativeTestRuntime(t, root)
	runtime.CWD = repository
	writeNativeTestPlan(t, runtime.PlanFile, "one")
	writeNativeTestList(t, runtime.FleetFile, "owner")
	writeSeatsTestHost(t, runtime.Home)
	t.Setenv(agentComposeRuntimeHomeEnv, "")
	t.Setenv(agentComposeClaudeConfigDirEnv, "")

	workspace, err := prepareNativeLaunchWorkspaceWithOptions(
		runtime,
		"claude",
		nativeLaunchOptions{WorkspaceRoot: true, StandaloneHome: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	if workspace.ClaudeSeat != "" {
		t.Fatalf("a standalone home staged a seat: %s", workspace.ClaudeSeat)
	}
	if got := os.Getenv(agentComposeClaudeConfigDirEnv); got != "" {
		t.Errorf("%s = %q, want unset", agentComposeClaudeConfigDirEnv, got)
	}
	if _, err := os.Lstat(nativeClaudeSeatsDir(runtime.Home)); err == nil {
		t.Error("a standalone launch created the shared directory")
	}
}
