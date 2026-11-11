package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestNativeSessionIDMatchesDictatableContract(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "agentic_os", "agent_id_vectors.json"))
	if err != nil {
		t.Fatal(err)
	}
	var contract struct {
		Letters string `json:"id_letters"`
		Digits  string `json:"id_digits"`
		Length  int    `json:"id_len"`
	}
	if err := json.Unmarshal(raw, &contract); err != nil {
		t.Fatal(err)
	}
	if nativeIDLetters != contract.Letters || nativeIDDigits != contract.Digits {
		t.Fatalf(
			"native ID alphabets = %q/%q, want %q/%q",
			nativeIDLetters,
			nativeIDDigits,
			contract.Letters,
			contract.Digits,
		)
	}
	runtime := nativeTestRuntime(t, t.TempDir())
	runtime.Random = bytes.NewReader([]byte{0, 1, 2, 3})
	id, err := nativeSessionID(runtime)
	if err != nil {
		t.Fatal(err)
	}
	if id != "ab67" || len(id) != contract.Length {
		t.Fatalf("native session ID = %q, want four-character dictatable ID ab67", id)
	}
}

func TestNativeLaunchRetriesOccupiedShortSessionIDs(t *testing.T) {
	root := t.TempDir()
	repository, _ := createNativeTestRepository(t, root, "owner", "one")
	runtime := nativeTestRuntime(t, root)
	runtime.Random = bytes.NewReader([]byte{
		0, 0, 0, 0,
		1, 1, 1, 1,
		2, 2, 2, 2,
	})
	writeNativeTestPlan(t, runtime.PlanFile, "one")
	writeNativeTestList(t, runtime.FleetFile, "owner")
	if err := os.MkdirAll(filepath.Join(runtime.SessionsRoot, "aa44"), 0o700); err != nil {
		t.Fatal(err)
	}
	// A pushed branch is what makes an ID taken. A bare local one no longer is:
	// the sweep reaps a landed session branch nothing leases, agentic-os#1260.
	testGit(t, repository, "branch", "aos/codex/bb55")
	testGit(t, repository, "push", "origin", "aos/codex/bb55")

	if _, err := prepareNativeLaunch(runtime, "codex"); err != nil {
		t.Fatal(err)
	}

	_, lease := onlyNativeLease(t, runtime)
	if lease.ID != "cc66" {
		t.Fatalf("native session ID = %q, want collision retry cc66", lease.ID)
	}
	if got := filepath.Base(lease.SessionRoot); got != lease.ID {
		t.Fatalf("native session root suffix = %q, want %q", got, lease.ID)
	}
	if len(lease.Artifacts) != 1 || lease.Artifacts[0].Branch != "aos/codex/cc66" {
		t.Fatalf("native artifacts = %#v, want short collision-free branch", lease.Artifacts)
	}
}

func TestNativeLaunchTakesTheRequestedSessionID(t *testing.T) {
	root := t.TempDir()
	createNativeTestRepository(t, root, "owner", "one")
	runtime := nativeTestRuntime(t, root)
	runtime.RequestedID = "ab84"
	runtime.Random = bytes.NewReader([]byte{0, 0, 0, 0})
	writeNativeTestPlan(t, runtime.PlanFile, "one")
	writeNativeTestList(t, runtime.FleetFile, "owner")

	if _, err := prepareNativeLaunch(runtime, "claude"); err != nil {
		t.Fatal(err)
	}

	if _, lease := onlyNativeLease(t, runtime); lease.ID != "ab84" {
		t.Fatalf("native session ID = %q, want the requested ab84", lease.ID)
	}
}

func TestNativeLaunchDrawsFreshWhenALiveLeaseHoldsTheRequestedID(t *testing.T) {
	root := t.TempDir()
	createNativeTestRepository(t, root, "owner", "one")
	runtime := nativeTestRuntime(t, root)
	runtime.RequestedID = "ab84"
	runtime.Random = bytes.NewReader([]byte{2, 2, 2, 2})
	writeNativeTestPlan(t, runtime.PlanFile, "one")
	writeNativeTestList(t, runtime.FleetFile, "owner")
	if err := os.MkdirAll(filepath.Join(runtime.SessionsRoot, "ab84"), 0o700); err != nil {
		t.Fatal(err)
	}
	holder := nativeLease{Format: "agentic-os.native-lease.v1", ID: "ab84", Harness: "claude", PID: os.Getpid()}
	if err := writeNativeJSON(nativeStatePath(runtime, "leases", "ab84.json"), holder); err != nil {
		t.Fatal(err)
	}

	if _, err := prepareNativeLaunch(runtime, "claude"); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(nativeStatePath(runtime, "leases"))
	if err != nil || len(entries) != 2 {
		t.Fatalf("leases = %v, err = %v, want the holder's and one fresh", entries, err)
	}
	var held nativeLease
	if err := readNativeJSON(nativeStatePath(runtime, "leases", "ab84.json"), &held); err != nil || held.Harness != "claude" {
		t.Fatalf("the holder's lease was rewritten: %+v, %v", held, err)
	}
}

// A deleted pod never runs the shadow cleanup, so a resume finds its directory and
// branch with no lease. The code is reused, which keeps the directory goose recorded.
func TestNativeLaunchReusesARequestedIDWhoseDirectoryAnEndedSessionLeftBehind(t *testing.T) {
	root := t.TempDir()
	createNativeTestRepository(t, root, "owner", "one")
	runtime := nativeTestRuntime(t, root)
	runtime.RequestedID = "ab84"
	runtime.Random = bytes.NewReader([]byte{2, 2, 2, 2})
	writeNativeTestPlan(t, runtime.PlanFile, "one")
	writeNativeTestList(t, runtime.FleetFile, "owner")
	if _, err := prepareNativeLaunch(runtime, "claude"); err != nil {
		t.Fatal(err)
	}
	path, first := onlyNativeLease(t, runtime)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	left := filepath.Join(first.SessionRoot, "left-behind")
	if err := os.WriteFile(left, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := prepareNativeLaunch(runtime, "claude"); err != nil {
		t.Fatal(err)
	}

	_, lease := onlyNativeLease(t, runtime)
	if lease.ID != "ab84" || lease.SessionRoot != first.SessionRoot {
		t.Fatalf("lease = %s in %s, want ab84 in %s", lease.ID, lease.SessionRoot, first.SessionRoot)
	}
	if _, err := os.Stat(left); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("the ended session's leftovers were kept: %v", err)
	}
}

func TestNativeLaunchKeepsAnEndedSessionsUnpushedWorkAndDrawsFreshInstead(t *testing.T) {
	root := t.TempDir()
	createNativeTestRepository(t, root, "owner", "one")
	runtime := nativeTestRuntime(t, root)
	runtime.RequestedID = "ab84"
	runtime.Random = bytes.NewReader([]byte{2, 2, 2, 2})
	writeNativeTestPlan(t, runtime.PlanFile, "one")
	writeNativeTestList(t, runtime.FleetFile, "owner")
	if _, err := prepareNativeLaunch(runtime, "claude"); err != nil {
		t.Fatal(err)
	}
	path, first := onlyNativeLease(t, runtime)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	worktree := first.Artifacts[0].Worktree
	if err := os.WriteFile(filepath.Join(worktree, "work.txt"), []byte("mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	testGit(t, worktree, "add", "work.txt")
	testGit(t, worktree, "commit", "-m", "unpushed")

	if _, err := prepareNativeLaunch(runtime, "claude"); err != nil {
		t.Fatal(err)
	}

	if _, lease := onlyNativeLease(t, runtime); lease.ID == "ab84" {
		t.Fatalf("native session ID = %q, which would have discarded unpushed work", lease.ID)
	}
	if _, err := os.Stat(filepath.Join(worktree, "work.txt")); err != nil {
		t.Fatalf("the unpushed work is gone: %v", err)
	}
}

func TestNativeValidSessionIDFollowsTheContract(t *testing.T) {
	for id, want := range map[string]bool{
		"ab84": true, "zz99": true, "AB81": false, "ab1": false,
		"ai84": false, "ab31": false, "../x": false, "ab841": false,
	} {
		if got := nativeValidSessionID(id); got != want {
			t.Errorf("nativeValidSessionID(%q) = %v, want %v", id, got, want)
		}
	}
}

// The host wrapper converges on `_native-shadow` calls, so minting must stay a
// root verb of its own or a launcher's metadata read waits on Ansible.
func TestSessionIDIsItsOwnRootVerb(t *testing.T) {
	if !isRootSubcommand("_session-id") {
		t.Fatal("_session-id should dispatch as a root subcommand")
	}
}
