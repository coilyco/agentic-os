package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func trustRuntime(t *testing.T) nativeRuntime {
	t.Helper()
	return nativeRuntime{StateRoot: t.TempDir(), Now: time.Date(2026, 11, 4, 23, 30, 0, 0, time.UTC)}
}

func TestTheTrustLogRecordsWhatWasSeededAndWhetherItHeld(t *testing.T) {
	runtime := trustRuntime(t)
	plan := nativeTrustPlan{Config: "/c", Paths: []string{"/a/projects", "/a/seat"}, External: true}
	logNativeClaudeTrust(runtime, "staged", plan, nativeSeedResult{Wrote: true, Repairs: 1, Clean: true}, nil)
	logNativeClaudeTrust(runtime, "pre-exec", plan, nativeSeedResult{}, os.ErrPermission)
	raw, err := os.ReadFile(filepath.Join(runtime.StateRoot, "claude-trust.log"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 2 ||
		lines[0] != "2026-11-04T23:30:00Z staged external=true wrote=true repairs=1 clean=true paths=/a/projects,/a/seat err=-" ||
		!strings.Contains(lines[1], "pre-exec") || !strings.Contains(lines[1], "err=permission denied") {
		t.Fatalf("the log should name the stage, what held, the paths and any error: %q", lines)
	}
}

func TestARecheckBeforeTheHandoffWritesAgainASeedErasedSinceStaging(t *testing.T) {
	runtime := trustRuntime(t)
	config := filepath.Join(t.TempDir(), ".claude.json")
	project := filepath.Join(t.TempDir(), "projects")
	plan := nativeTrustPlan{Config: config, Paths: []string{project}, External: true}
	if _, err := ensureNativeClaudeTrust(config, plan.Paths, true, nil); err != nil {
		t.Fatal(err)
	}
	// Another claude's write erases it between staging and the hand-off.
	if err := os.WriteFile(config, []byte(`{"projects":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	saved := nativeTrustRecheckChecks
	nativeTrustRecheckChecks = []time.Duration{10 * time.Millisecond}
	t.Cleanup(func() { nativeTrustRecheckChecks = saved })
	recheckNativeClaudeTrust(runtime, plan)
	if entry := projectEntry(t, config, project); string(entry["hasClaudeMdExternalIncludesApproved"]) != "true" {
		t.Fatalf("the recheck should have written the seed again: %v", entry)
	}
	raw, _ := os.ReadFile(filepath.Join(runtime.StateRoot, "claude-trust.log"))
	if !strings.Contains(string(raw), "pre-exec external=true wrote=true") {
		t.Fatalf("a seed found erased at the hand-off is the line the log exists for: %q", raw)
	}
	recheckNativeClaudeTrust(runtime, nativeTrustPlan{})
	if after, _ := os.ReadFile(filepath.Join(runtime.StateRoot, "claude-trust.log")); string(after) != string(raw) {
		t.Fatal("a launch with no Claude plan should log nothing")
	}
}

// COI-3343: one launch in six lost the seed to another claude's stale write, some of them
// after the hand-off. The guard keeps repairing for the whole window.
func TestTheGuardRepairsASeedErasedAgainAndAgainAfterTheHandoff(t *testing.T) {
	config := filepath.Join(t.TempDir(), ".claude.json")
	project := filepath.Join(t.TempDir(), "projects")
	plan := nativeTrustPlan{Config: config, Paths: []string{project}, External: true}
	if err := seedNativeClaudeTrust(config, plan.Paths, true); err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	var erased sync.WaitGroup
	erased.Add(1)
	go func() {
		defer erased.Done()
		for {
			select {
			case <-stop:
				return
			case <-time.After(25 * time.Millisecond):
				_ = writeNativeClaudeJSON(config, map[string]json.RawMessage{"projects": json.RawMessage("{}")})
			}
		}
	}()
	repairs, err := guardNativeClaudeTrust(plan, 300*time.Millisecond, 5*time.Millisecond)
	close(stop)
	erased.Wait()
	if err != nil || repairs < 2 {
		t.Fatalf("repairs = %d, err = %v, want the seed repaired after each erasure", repairs, err)
	}
	if _, err := guardNativeClaudeTrust(plan, 20*time.Millisecond, 5*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	entry := projectEntry(t, config, project)
	if string(entry["hasTrustDialogAccepted"]) != "true" || string(entry["hasClaudeMdExternalIncludesApproved"]) != "true" {
		t.Fatalf("the seed should be in place once the erasing stops: %v", entry)
	}
}
