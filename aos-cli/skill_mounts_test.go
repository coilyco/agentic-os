package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A bare repo on disk stands in for the forge, so the clone path is exercised
// rather than stubbed and no test reaches the network.
func fakeForge(t *testing.T, repo, skillDir string) string {
	t.Helper()
	forge := t.TempDir()
	work := t.TempDir()
	root := filepath.Join(work, "src")
	if err := os.MkdirAll(filepath.Join(root, skillDir), 0o755); err != nil {
		t.Fatal(err)
	}
	body := []byte("---\nname: mounted\n---\n\nfrom the source repo.\n")
	if err := os.WriteFile(filepath.Join(root, skillDir, "SKILL.md"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init", "--quiet", "-b", "main"},
		{"add", "-A"},
		{"-c", "user.email=t@t", "-c", "user.name=t", "commit", "--quiet", "-m", "seed"},
	} {
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	bare := filepath.Join(forge, repo+".git")
	if err := os.MkdirAll(filepath.Dir(bare), 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "clone", "--bare", "--quiet", root, bare).CombinedOutput(); err != nil {
		t.Fatalf("bare clone: %v %s", err, out)
	}
	return "file://" + forge
}

// A consumer checkout: a git repo whose .agents/skills is ignored, which is the
// state a mount requires.
func consumerRepo(t *testing.T, manifest string, ignore bool) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".agents", "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, skillMountManifest), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	rules := "node_modules/\n"
	if ignore {
		rules += ".agents/skills/sirens-game-enshrouded/\n"
	}
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte(rules), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", root, "init", "--quiet", "-b", "main").CombinedOutput(); err != nil {
		t.Fatalf("init: %v %s", err, out)
	}
	return root
}

const oneMount = `mounts:
  - root: sirens-game-enshrouded
    repo: coilyco-gaming/enshrouded
    source: skills/sirens-game-enshrouded
`

func TestSkillMountMaterialisesTheSourceRoot(t *testing.T) {
	forge := fakeForge(t, "coilyco-gaming/enshrouded", "skills/sirens-game-enshrouded")
	root := consumerRepo(t, oneMount, true)
	if err := runSkillMount(context.Background(), root, forge, false, os.Stdout); err != nil {
		t.Fatalf("mount: %v", err)
	}
	landed := filepath.Join(root, ".agents", "skills", "sirens-game-enshrouded", "SKILL.md")
	body, err := os.ReadFile(landed)
	if err != nil {
		t.Fatalf("mounted skill missing: %v", err)
	}
	if !strings.Contains(string(body), "from the source repo") {
		t.Fatalf("mounted content = %q", body)
	}
}

// Mounting into a tracked path would commit another repo's content into this
// one's history, so it refuses before the clone rather than after the copy.
func TestSkillMountRefusesATrackedTarget(t *testing.T) {
	forge := fakeForge(t, "coilyco-gaming/enshrouded", "skills/sirens-game-enshrouded")
	root := consumerRepo(t, oneMount, false)
	err := runSkillMount(context.Background(), root, forge, false, os.Stdout)
	if err == nil {
		t.Fatal("mounted into a tracked path")
	}
	if !strings.Contains(err.Error(), "not ignored") {
		t.Fatalf("err = %v, want it to name the ignore rule", err)
	}
}

func TestSkillMountIsIdempotent(t *testing.T) {
	forge := fakeForge(t, "coilyco-gaming/enshrouded", "skills/sirens-game-enshrouded")
	root := consumerRepo(t, oneMount, true)
	for i := 0; i < 2; i++ {
		if err := runSkillMount(context.Background(), root, forge, false, os.Stdout); err != nil {
			t.Fatalf("mount %d: %v", i, err)
		}
	}
	entries, err := os.ReadDir(filepath.Join(root, ".agents", "skills"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("%d roots after two mounts, want 1", len(entries))
	}
}

// --check must fail on an absent mount, because a consumer that silently
// builds without a declared skill is the defect this mechanism replaces.
func TestSkillMountCheckFailsWhenAbsent(t *testing.T) {
	root := consumerRepo(t, oneMount, true)
	err := runSkillMount(context.Background(), root, "file:///nowhere", true, os.Stdout)
	if err == nil {
		t.Fatal("check passed with nothing mounted")
	}
	if !strings.Contains(err.Error(), "absent") {
		t.Fatalf("err = %v", err)
	}
}

func TestSkillMountCheckPassesOnceMounted(t *testing.T) {
	forge := fakeForge(t, "coilyco-gaming/enshrouded", "skills/sirens-game-enshrouded")
	root := consumerRepo(t, oneMount, true)
	if err := runSkillMount(context.Background(), root, forge, false, os.Stdout); err != nil {
		t.Fatal(err)
	}
	if err := runSkillMount(context.Background(), root, forge, true, os.Stdout); err != nil {
		t.Fatalf("check after mount: %v", err)
	}
}

func TestSkillMountRejectsAnEscapingSource(t *testing.T) {
	root := consumerRepo(t, "mounts:\n  - root: x\n    repo: o/n\n    source: ../../etc\n", true)
	if _, err := readSkillMounts(filepath.Join(root, skillMountManifest)); err == nil {
		t.Fatal("accepted a source escaping the source repo")
	}
}

func TestSkillMountRejectsADuplicateRoot(t *testing.T) {
	dup := oneMount + strings.TrimPrefix(oneMount, "mounts:\n")
	root := consumerRepo(t, dup, true)
	if _, err := readSkillMounts(filepath.Join(root, skillMountManifest)); err == nil {
		t.Fatal("accepted a duplicate root")
	}
}
