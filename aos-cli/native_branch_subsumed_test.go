package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const eightLines = "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\n"

// mergeTreeConflicts proves a fixture is the hard shape rather than a clean
// no-op, so a regression that only handles the easy half still fails here.
func mergeTreeConflicts(t *testing.T, repository, branch string) bool {
	t.Helper()
	_, err := nativeGit(repository, "merge-tree", "--write-tree",
		"origin/main", "refs/heads/"+branch)
	return err != nil
}

// The 10 clean branches the warning named: pushed, squash-merged, remote pruned.
// agentic-os#7687
func TestASquashMergedBranchHoldsNothingMainLacks(t *testing.T) {
	root := t.TempDir()
	repository, _ := createNativeTestRepository(t, root, "owner", "one")
	testGit(t, repository, "switch", "-c", "aos/claude/ee88")
	commitFile(t, repository, "one.txt", "first")
	commitFile(t, repository, "two.txt", "second")
	testGit(t, repository, "push", "-u", "origin", "aos/claude/ee88")
	squashMerge(t, repository, "aos/claude/ee88")

	if count := testGit(t, repository, "rev-list", "--count",
		"refs/heads/aos/claude/ee88", "--not", "--remotes=origin"); strings.TrimSpace(count) == "0" {
		t.Fatal("the fixture no longer reproduces the count the old check read")
	}
	if !nativeBranchSubsumed(repository, "aos/claude/ee88") {
		t.Fatal("a squash-merged branch holds nothing main lacks")
	}
}

// The 4 that conflict: landed, then main edited the same region again. A check
// that only handles the clean no-op leaves these crying wolf. agentic-os#7687
func TestABranchMainEditedPastHoldsNothingMainLacks(t *testing.T) {
	root := t.TempDir()
	repository, _ := createNativeTestRepository(t, root, "owner", "one")
	commitFile(t, repository, "f.txt", eightLines)
	testGit(t, repository, "push", "origin", "main")

	testGit(t, repository, "switch", "-c", "aos/claude/ff99")
	commitFile(t, repository, "f.txt", strings.Replace(eightLines, "line4", "line4-branch", 1))
	testGit(t, repository, "push", "-u", "origin", "aos/claude/ff99")
	squashMerge(t, repository, "aos/claude/ff99")
	commitFile(t, repository, "f.txt", strings.Replace(eightLines, "line4", "line4-tuned", 1))
	testGit(t, repository, "push", "origin", "main")
	testGit(t, repository, "fetch", "origin")

	if !mergeTreeConflicts(t, repository, "aos/claude/ff99") {
		t.Fatal("the fixture merges cleanly, so it is not the conflicting shape")
	}
	if !nativeBranchSubsumed(repository, "aos/claude/ff99") {
		t.Fatal("main resolved that region itself, so the branch holds nothing it lacks")
	}
}

func TestABranchHoldingContentMainLacksIsNotSubsumed(t *testing.T) {
	root := t.TempDir()
	repository, _ := createNativeTestRepository(t, root, "owner", "one")
	commitFile(t, repository, "f.txt", eightLines)
	testGit(t, repository, "push", "origin", "main")

	// A file main never took. The shape the warning exists for.
	testGit(t, repository, "switch", "-c", "aos/claude/gg11")
	commitFile(t, repository, "kept.txt", "kept")
	testGit(t, repository, "push", "-u", "origin", "aos/claude/gg11")
	testGit(t, repository, "push", "origin", "--delete", "aos/claude/gg11")
	testGit(t, repository, "fetch", "--prune", "origin")
	if nativeBranchSubsumed(repository, "aos/claude/gg11") {
		t.Fatal("a file main never took is content main lacks")
	}

	// A hunk main never touched, in a file main did touch elsewhere. `-X ours`
	// drops only the conflicting hunks, so this one has to survive.
	testGit(t, repository, "switch", "main")
	testGit(t, repository, "switch", "-c", "aos/claude/hh22")
	commitFile(t, repository, "f.txt", strings.Replace(eightLines, "line8", "line8-branch", 1))
	testGit(t, repository, "switch", "main")
	commitFile(t, repository, "f.txt", strings.Replace(eightLines, "line1", "line1-main", 1))
	testGit(t, repository, "push", "origin", "main")
	testGit(t, repository, "fetch", "origin")
	if nativeBranchSubsumed(repository, "aos/claude/hh22") {
		t.Fatal("a hunk main never touched is content main lacks")
	}
}

// End to end, on the conflicting shape because the reaper deletes the clean one
// before the report sees it. agentic-os#7687
func TestASquashMergedPurgedWorktreeIsNotReported(t *testing.T) {
	root := t.TempDir()
	repository, _ := createNativeTestRepository(t, root, "owner", "one")
	commitFile(t, repository, "f.txt", eightLines)
	testGit(t, repository, "push", "origin", "main")
	runtime := nativeTestRuntime(t, root)
	stderr := captureNativeStderr(t, &runtime)
	writeNativeTestPlan(t, runtime.PlanFile, "one")
	writeNativeTestList(t, runtime.FleetFile, "owner")
	if _, err := prepareNativeLaunch(runtime, "codex"); err != nil {
		t.Fatal(err)
	}
	leasePath, lease := onlyNativeLease(t, runtime)
	worktree := lease.Artifacts[0].Worktree
	branch := lease.Artifacts[0].Branch
	testGit(t, worktree, "config", "user.email", "test@example.com")
	testGit(t, worktree, "config", "user.name", "AOS Test")
	edited := strings.Replace(eightLines, "line4", "line4-branch", 1)
	if err := os.WriteFile(filepath.Join(worktree, "f.txt"), []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	testGit(t, worktree, "commit", "-am", "work that lands")
	testGit(t, worktree, "push", "-u", "origin", branch)
	squashMerge(t, repository, branch)
	commitFile(t, repository, "f.txt", strings.Replace(eightLines, "line4", "line4-tuned", 1))
	testGit(t, repository, "push", "origin", "main")
	testGit(t, repository, "fetch", "origin")

	// The branch has to survive the reaper, or the report is silent for a
	// reason that has nothing to do with the check under test.
	if nativeBranchLanded(repository, branch) {
		t.Fatal("the fixture is deleted before the report runs")
	}
	if err := os.RemoveAll(worktree); err != nil {
		t.Fatal(err)
	}
	lease.PID = 0
	lease.ProcessStart = ""
	deadSince := runtime.Now.Add(-nativeDeadSessionGrace)
	lease.DeadSince = &deadSince
	if err := writeNativeJSON(leasePath, lease); err != nil {
		t.Fatal(err)
	}
	if _, err := cleanDeadNativeSessions(runtime); err != nil {
		t.Fatal(err)
	}

	if report := stderr(); strings.Contains(report, "unpushed commit") {
		t.Fatalf("a squash-merged branch was reported as stuck: %q", report)
	}
}

// The orphan reading is the same defect one startup later: patch-id marks a
// squash that collapsed two commits `+` forever. agentic-os#7687
func TestASquashedMultiCommitBranchIsNotAnOrphan(t *testing.T) {
	root := t.TempDir()
	repository, _ := createNativeTestRepository(t, root, "owner", "one")
	target := nativeRepository{Owner: "owner", Name: "one", Path: repository}
	commitFile(t, repository, "f.txt", eightLines)
	testGit(t, repository, "push", "origin", "main")

	testGit(t, repository, "switch", "-c", "task/two-commits")
	commitFile(t, repository, "one.txt", "first")
	commitFile(t, repository, "two.txt", "second")
	testGit(t, repository, "push", "-u", "origin", "task/two-commits")
	squashMerge(t, repository, "task/two-commits")
	if unmerged := testGit(t, repository, "cherry", "origin/main",
		"task/two-commits"); !strings.Contains(unmerged, "+") {
		t.Fatalf("the fixture is not a squash patch-id misses: %q", unmerged)
	}

	testGit(t, repository, "switch", "-c", "task/edited-past", "main")
	commitFile(t, repository, "f.txt", strings.Replace(eightLines, "line4", "line4-branch", 1))
	testGit(t, repository, "push", "-u", "origin", "task/edited-past")
	squashMerge(t, repository, "task/edited-past")
	commitFile(t, repository, "f.txt", strings.Replace(eightLines, "line4", "line4-tuned", 1))
	testGit(t, repository, "push", "origin", "main")
	testGit(t, repository, "fetch", "origin")

	if orphans := readNativeOrphanBranches(target, nativeLiveWorktrees{}); len(orphans) != 0 {
		t.Fatalf("landed branches were reported as holding work: %+v", orphans)
	}
}
