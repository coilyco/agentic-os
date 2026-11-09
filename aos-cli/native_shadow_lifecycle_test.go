package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The three states a reader has to be able to tell apart, since a dry-run that
// overstates what it would delete is worse than no dry-run. agentic-os#1260
func TestShadowListSeparatesLiveGraceAndReleasable(t *testing.T) {
	root := t.TempDir()
	createNativeTestRepository(t, root, "owner", "one")
	runtime := nativeTestRuntime(t, root)
	leases := nativeStatePath(runtime, "leases")
	if err := os.MkdirAll(leases, 0o700); err != nil {
		t.Fatal(err)
	}
	fresh := runtime.Now.Add(-time.Hour)
	stale := runtime.Now.Add(-2 * nativeDeadSessionGrace)
	write := func(id string, lease nativeLease) {
		lease.ID = id
		lease.Harness = "claude"
		lease.SessionRoot = filepath.Join(root, "sessions", id)
		if err := writeNativeJSON(filepath.Join(leases, id+".json"), lease); err != nil {
			t.Fatal(err)
		}
	}
	write("aa44", nativeLease{PID: os.Getpid(), ProcessStart: runtime.ProcessStart})
	write("bb55", nativeLease{DeadSince: &fresh})
	write("cc66", nativeLease{DeadSince: &stale})
	write("dd77", nativeLease{DeadSince: &fresh, Released: &fresh})

	report, err := inspectNativeShadows(runtime)
	if err != nil {
		t.Fatal(err)
	}
	verdicts := map[string]nativeShadowSession{}
	for _, session := range report.Sessions {
		verdicts[session.ID] = session
	}
	if len(verdicts) != 4 {
		t.Fatalf("sessions = %d, want 4", len(verdicts))
	}
	if !verdicts["aa44"].Live || verdicts["aa44"].Releasable {
		t.Fatalf("a running session must be held: %+v", verdicts["aa44"])
	}
	if verdicts["bb55"].Releasable || !strings.Contains(verdicts["bb55"].Held, "grace") {
		t.Fatalf("a lease inside the grace must say so: %+v", verdicts["bb55"])
	}
	if !verdicts["cc66"].Releasable {
		t.Fatalf("a lease past the grace holding nothing is releasable: %+v", verdicts["cc66"])
	}
	if !verdicts["dd77"].Releasable {
		t.Fatalf("a released lease skips the grace: %+v", verdicts["dd77"])
	}
}

// A branch with commits on no remote is the one thing worth keeping forever.
func TestShadowListHoldsASessionWithUnpushedCommits(t *testing.T) {
	root := t.TempDir()
	repository, _ := createNativeTestRepository(t, root, "owner", "one")
	testGit(t, repository, "switch", "-c", "aos/claude/ee88")
	commitFile(t, repository, "local.txt", "local")
	testGit(t, repository, "switch", "main")
	runtime := nativeTestRuntime(t, root)
	leases := nativeStatePath(runtime, "leases")
	if err := os.MkdirAll(leases, 0o700); err != nil {
		t.Fatal(err)
	}
	stale := runtime.Now.Add(-2 * nativeDeadSessionGrace)
	if err := writeNativeJSON(filepath.Join(leases, "ee88.json"), nativeLease{
		ID:          "ee88",
		Harness:     "claude",
		DeadSince:   &stale,
		SessionRoot: filepath.Join(root, "sessions", "ee88"),
		Artifacts: []nativeArtifact{{
			Repository: repository,
			Worktree:   filepath.Join(root, "sessions", "ee88", "one"),
			Branch:     "aos/claude/ee88",
		}},
	}); err != nil {
		t.Fatal(err)
	}
	report, err := inspectNativeShadows(runtime)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Sessions) != 1 {
		t.Fatalf("sessions = %d, want 1", len(report.Sessions))
	}
	session := report.Sessions[0]
	if session.Releasable {
		t.Fatal("a session holding an unpushed commit must be held")
	}
	if !strings.Contains(session.Held, "on no remote") {
		t.Fatalf("held reason = %q", session.Held)
	}
	if session.Artifacts[0].Unpushed != 1 {
		t.Fatalf("unpushed = %d, want 1", session.Artifacts[0].Unpushed)
	}
}

// Declaring a session finished is a mark on the lease, never a teardown of a
// process that may still be running.
func TestReleaseMarksTheLeaseAndSkipsTheGrace(t *testing.T) {
	root := t.TempDir()
	createNativeTestRepository(t, root, "owner", "one")
	runtime := nativeTestRuntime(t, root)
	leases := nativeStatePath(runtime, "leases")
	if err := os.MkdirAll(leases, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(leases, "ff99.json")
	if err := writeNativeJSON(path, nativeLease{
		ID:          "ff99",
		Harness:     "claude",
		PID:         os.Getpid(),
		SessionRoot: filepath.Join(root, "sessions", "ff99"),
	}); err != nil {
		t.Fatal(err)
	}
	if err := releaseNativeShadow(runtime, "ff99"); err != nil {
		t.Fatal(err)
	}
	var lease nativeLease
	if err := readNativeJSON(path, &lease); err != nil {
		t.Fatal(err)
	}
	if lease.Released == nil {
		t.Fatal("release did not mark the lease")
	}
	if lease.PID != os.Getpid() {
		t.Fatal("release must not disturb a running session's lease")
	}
	if err := releaseNativeShadow(runtime, "ff99"); err != nil {
		t.Fatalf("releasing twice should be quiet: %v", err)
	}
	if err := releaseNativeShadow(runtime, "nope"); err == nil {
		t.Fatal("releasing an unknown session should refuse")
	}
}

// COI-3427: a closed seat's shadow stayed for the 24-hour grace, because nothing
// declared it finished. Finishing collects it now and still keeps unpushed work.
func TestFinishingCollectsADeadShadowKeepsUnpushedWorkAndSparesALiveOne(t *testing.T) {
	root := t.TempDir()
	repository, _ := createNativeTestRepository(t, root, "owner", "one")
	runtime := nativeTestRuntime(t, root)
	leases := nativeStatePath(runtime, "leases")
	if err := os.MkdirAll(leases, 0o700); err != nil {
		t.Fatal(err)
	}
	const deadPID = 2147000
	sessionRoot := func(id string) string {
		path := filepath.Join(root, "sessions", id)
		if err := os.MkdirAll(filepath.Join(path, "home"), 0o755); err != nil {
			t.Fatal(err)
		}
		return path
	}
	write := func(lease nativeLease) {
		lease.Harness = "claude"
		if err := writeNativeJSON(filepath.Join(leases, lease.ID+".json"), lease); err != nil {
			t.Fatal(err)
		}
	}
	write(nativeLease{ID: "gg11", PID: deadPID, ProcessStart: "gone", SessionRoot: sessionRoot("gg11")})
	write(nativeLease{ID: "jj44", PID: os.Getpid(), ProcessStart: runtime.ProcessStart, SessionRoot: sessionRoot("jj44")})
	keptRoot := sessionRoot("hh22")
	worktree := filepath.Join(keptRoot, "one")
	testGit(t, repository, "worktree", "add", "-b", "aos/claude/hh22", worktree)
	commitFile(t, worktree, "local.txt", "local")
	write(nativeLease{
		ID: "hh22", PID: deadPID, ProcessStart: "gone", SessionRoot: keptRoot,
		Artifacts: []nativeArtifact{{Repository: repository, Worktree: worktree, Branch: "aos/claude/hh22"}},
	})

	everything := func(nativeLease) bool { return true }
	if err := finishNativeShadows(runtime, everything, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(sessionRoot("gg11")); err != nil {
		t.Fatalf("a dry run must remove nothing: %v", err)
	}
	if err := finishNativeShadows(runtime, everything, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "sessions", "gg11")); !os.IsNotExist(err) {
		t.Fatalf("a dead, clean shadow should be gone at once, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(leases, "gg11.json")); !os.IsNotExist(err) {
		t.Fatalf("its lease should go with it, stat err = %v", err)
	}
	for _, kept := range []string{filepath.Join(worktree, "local.txt"), filepath.Join(root, "sessions", "jj44"), filepath.Join(leases, "hh22.json")} {
		if _, err := os.Stat(kept); err != nil {
			t.Fatalf("%s must survive: unpushed work and live sessions are kept: %v", kept, err)
		}
	}
}

// COI-3427: with --release-dead the header said 79 would be released and the footer
// said 0, because the verdicts were read before the release the real run makes.
func TestADryRunWithReleaseDeadReadsTheVerdictsTheRealRunEnforces(t *testing.T) {
	root := t.TempDir()
	runtime := nativeTestRuntime(t, root)
	leases := nativeStatePath(runtime, "leases")
	if err := os.MkdirAll(leases, 0o700); err != nil {
		t.Fatal(err)
	}
	fresh := runtime.Now.Add(-time.Hour)
	for id, lease := range map[string]nativeLease{
		"kk55": {DeadSince: &fresh},
		"mm66": {PID: os.Getpid(), ProcessStart: runtime.ProcessStart},
	} {
		lease.ID, lease.Harness = id, "claude"
		lease.SessionRoot = filepath.Join(root, "sessions", id)
		if err := writeNativeJSON(filepath.Join(leases, id+".json"), lease); err != nil {
			t.Fatal(err)
		}
	}
	verdicts := func(releaseDead bool) map[string]bool {
		report, err := inspectNativeShadowsAs(runtime, releaseDead)
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]bool{}
		for _, session := range report.Sessions {
			out[session.ID] = session.Releasable
		}
		return out
	}
	if got := verdicts(false); got["kk55"] {
		t.Fatalf("without the flag a dead lease inside the grace is held: %v", got)
	}
	if got := verdicts(true); !got["kk55"] || got["mm66"] {
		t.Fatalf("with the flag the dead lease is releasable and the live one is not: %v", got)
	}
}

// COI-3427: the sweep removed clean, pushed worktrees that live seats made inside their
// own shadows, since only lease artifacts counted as live. Their ignored files went too.
func TestTheSweepLeavesWorktreesInsideALiveSessionsShadowAndScratchpad(t *testing.T) {
	root := t.TempDir()
	repository, _ := createNativeTestRepository(t, root, "owner", "one")
	runtime := nativeTestRuntime(t, root)
	liveRoot := filepath.Join(root, "native", "sp59")
	scratch := filepath.Join(root, "claude-502", strings.ReplaceAll(liveRoot, string(os.PathSeparator), "-")+"-projects", "abc", "scratchpad")
	add := func(path, branch string) {
		testGit(t, repository, "worktree", "add", "-b", branch, path, "origin/main")
		testGit(t, path, "push", "-u", "origin", branch)
	}
	inShadow := filepath.Join(liveRoot, "projects", "owner", "one")
	inScratch := filepath.Join(scratch, "deploy-main")
	elsewhere := filepath.Join(root, "stray")
	add(inShadow, "own-shadow")
	add(inScratch, "own-scratch")
	add(elsewhere, "stray-branch")

	live := nativeLiveWorktrees{}
	live.addRoot(liveRoot)
	if err := cleanUnleasedNativeWorktrees(runtime, nativeRepository{Owner: "owner", Name: "one", Path: repository}, live); err != nil {
		t.Fatal(err)
	}
	for _, kept := range []string{inShadow, inScratch} {
		if _, err := os.Stat(kept); err != nil {
			t.Fatalf("%s belongs to a live session and must survive: %v", kept, err)
		}
	}
	if _, err := os.Stat(elsewhere); !os.IsNotExist(err) {
		t.Fatalf("a clean, pushed worktree outside every live session is still swept, stat err = %v", err)
	}
}
