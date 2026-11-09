package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func screenFixture(t *testing.T, name string) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "claude-footers", name))
	if err != nil {
		raw, err = os.ReadFile(filepath.Join("testdata", "claude-screens", name))
	}
	if err != nil {
		t.Fatalf("fixture %s: %v", name, err)
	}
	return strings.Split(string(raw), "\n")
}

// The fixtures are footers read off a live seat on 2026-11-09, with a shell, then a
// monitor, then nothing running.
func TestBackgroundCountReadsTheClaudeFooter(t *testing.T) {
	for _, test := range []struct {
		fixture string
		seat    string
		count   int
		known   bool
	}{
		{"background-shell.txt", "claude", 1, true},
		{"background-monitor.txt", "claude", 1, true},
		{"no-background.txt", "claude", 0, true},
		{"idle-ready.txt", "claude", 0, true},
		// A footer the width cut, a card that replaced the footer, and another
		// harness's screen each read as unknown, never as zero.
		{"cut-footer.txt", "claude", 0, false},
		{"q1.txt", "claude", 0, false},
		{"background-shell.txt", "codex", 0, false},
	} {
		count, known := backgroundCount(test.seat, screenFixture(t, test.fixture))
		if count != test.count || known != test.known {
			t.Errorf("%s as %s = (%d, %v), want (%d, %v)", test.fixture, test.seat, count, known, test.count, test.known)
		}
	}
}

func TestBackgroundCountAddsEveryNumberedSegmentAndSkipsTheAgentHint(t *testing.T) {
	lines := []string{"  ⏵⏵ bypass permissions on · 2 shells · 1 monitor · ← 3 agents"}
	if count, known := backgroundCount("claude", lines); count != 3 || !known {
		t.Fatalf("= (%d, %v), want (3, true)", count, known)
	}
}

func TestAnEmptyScreenHasNoKnownBackgroundCount(t *testing.T) {
	if count, known := backgroundCount("claude", nil); known {
		t.Fatalf("an empty screen read as (%d, true)", count)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", dir}, args...)...)
	command.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}

// pushedRepo is a checkout whose only commit is on a remote.
func pushedRepo(t *testing.T, dir string) {
	t.Helper()
	remote := filepath.Join(t.TempDir(), "remote.git")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "init", "-q", "-b", "main")
	runGit(t, dir, "init", "-q", "--bare", remote)
	if err := os.WriteFile(filepath.Join(dir, "a"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "a")
	runGit(t, dir, "commit", "-q", "-m", "a")
	runGit(t, dir, "remote", "add", "origin", remote)
	runGit(t, dir, "push", "-q", "origin", "main")
}

func TestUnpushedWorkIsEmptyForACheckoutEveryRemoteHas(t *testing.T) {
	root := t.TempDir()
	pushedRepo(t, root)
	if work := unpushedWork(root); work != "" {
		t.Fatalf("a pushed, clean checkout read as holding %q", work)
	}
}

func TestUnpushedWorkNamesACommitNoRemoteHas(t *testing.T) {
	root := t.TempDir()
	pushedRepo(t, root)
	runGit(t, root, "commit", "-q", "--allow-empty", "-m", "local only")
	work := unpushedWork(root)
	if !strings.Contains(work, root) || !strings.Contains(work, "1 commit(s) on no remote") {
		t.Fatalf("unpushedWork = %q, want the path and one commit", work)
	}
}

func TestUnpushedWorkNamesAChangedFile(t *testing.T) {
	root := t.TempDir()
	pushedRepo(t, root)
	if err := os.WriteFile(filepath.Join(root, "b"), []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	if work := unpushedWork(root); !strings.Contains(work, "1 changed file(s)") {
		t.Fatalf("unpushedWork = %q, want one changed file", work)
	}
}

// A native session's directory is not a checkout. Its clones sit at <owner>/<repo>.
func TestUnpushedWorkFindsACheckoutTwoLevelsBelowTheSeatDirectory(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "coilyco", "agentic-os")
	pushedRepo(t, repo)
	runGit(t, repo, "commit", "-q", "--allow-empty", "-m", "local only")
	if work := unpushedWork(root); !strings.Contains(work, repo) {
		t.Fatalf("unpushedWork = %q, want the clone under the seat directory", work)
	}
}

func TestUnpushedWorkRefusesNothingFromARootOrAnEmptyDirectory(t *testing.T) {
	if work := unpushedWork("/"); work != "" {
		t.Fatalf("a seat started at / read as holding %q", work)
	}
	if work := unpushedWork(""); work != "" {
		t.Fatalf("an unknown directory read as holding %q", work)
	}
	if work := unpushedWork(t.TempDir()); work != "" {
		t.Fatalf("a directory with no checkout read as holding %q", work)
	}
}
