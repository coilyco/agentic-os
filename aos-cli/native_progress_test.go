package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type progressClock struct {
	at time.Time
}

func (clock *progressClock) now() time.Time {
	return clock.at
}

func (clock *progressClock) advance(elapsed time.Duration) {
	clock.at = clock.at.Add(elapsed)
}

func newTestProgress(t *testing.T, level string) (*nativeProgress, *progressClock, *bytes.Buffer) {
	t.Helper()
	t.Setenv(nativeProgressEnv, level)
	clock := &progressClock{at: time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)}
	out := &bytes.Buffer{}
	return newNativeProgress(out, clock.now), clock, out
}

func TestNativeProgressNarratesEveryStep(t *testing.T) {
	progress, clock, out := newTestProgress(t, "")
	progress.Begin("claude", []string{"agent-compose", "launch"})
	step := progress.Step("fleet pass over %d repositories", 3)
	progress.Item("fetch", 1, 3, "owner/repo")
	clock.advance(2 * time.Second)
	step.Done("")

	text := out.String()
	for _, want := range []string{
		"aos: launch   native claude startup",
		"aos: start    fleet pass over 3 repositories",
		"aos: fetch    1/3 owner/repo",
		"aos: done     fleet pass over 3 repositories 2.00s",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in:\n%s", want, text)
		}
	}
	if strings.Contains(text, "aos: command") {
		t.Fatalf("default level leaked debug detail:\n%s", text)
	}
}

func TestNativeProgressAttributesTheSlowestItem(t *testing.T) {
	progress, clock, out := newTestProgress(t, "")
	step := progress.Step("fleet pass")
	step.Track("owner/fast", 100*time.Millisecond)
	step.Track("owner/slow", 3*time.Second)
	clock.advance(4 * time.Second)
	step.Done("%d fetched", 2)

	want := "aos: done     fleet pass 4.00s (2 fetched, slowest owner/slow 3.00s)"
	if !strings.Contains(out.String(), want) {
		t.Fatalf("missing %q in:\n%s", want, out.String())
	}
}

func TestNativeProgressReadyDropsInstantSteps(t *testing.T) {
	progress, clock, out := newTestProgress(t, "")
	progress.Step("converge environment").Done("")
	clock.advance(time.Second)
	progress.Ready()

	if !strings.Contains(out.String(), "aos: ready    native startup 1.00s\n") {
		t.Fatalf("ready line named an instant step:\n%s", out.String())
	}
}

func TestNativeProgressReadyRanksSlowestSteps(t *testing.T) {
	progress, clock, out := newTestProgress(t, "")
	converge := progress.Step("converge environment")
	clock.advance(time.Second)
	converge.Done("")
	fleet := progress.Step("fleet pass")
	clock.advance(5 * time.Second)
	fleet.Done("")
	clock.advance(6 * time.Second)
	progress.Ready()

	text := out.String()
	ready := ""
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "aos: ready") {
			ready = line
		}
	}
	want := "aos: ready    native startup 12.0s " +
		"(fleet pass 5.00s, converge environment 1.00s)"
	if ready != want {
		t.Fatalf("ready line = %q, want %q\nfull output:\n%s", ready, want, text)
	}
}

func TestNativeProgressOffStaysSilent(t *testing.T) {
	progress, clock, out := newTestProgress(t, "off")
	progress.Begin("claude", nil)
	step := progress.Step("fleet pass")
	clock.advance(time.Second)
	step.Done("")
	progress.Ready()
	if out.Len() != 0 {
		t.Fatalf("off level wrote %q", out.String())
	}
}

func TestNativeProgressSummaryKeepsOnlyTheTotal(t *testing.T) {
	progress, clock, out := newTestProgress(t, "summary")
	progress.Begin("claude", nil)
	step := progress.Step("fleet pass")
	clock.advance(time.Second)
	step.Done("")
	progress.Ready()

	text := out.String()
	if strings.Contains(text, "aos: start") || strings.Contains(text, "aos: done") {
		t.Fatalf("summary level narrated steps:\n%s", text)
	}
	if !strings.Contains(text, "aos: ready") {
		t.Fatalf("summary level dropped the total:\n%s", text)
	}
}

func TestNativeProgressDebugAddsDetail(t *testing.T) {
	progress, _, out := newTestProgress(t, "debug")
	progress.Begin("claude", []string{"agent-compose", "launch", "platform"})
	progress.Note("plan %s", "missing")

	text := out.String()
	if !strings.Contains(text, "agent-compose launch platform") {
		t.Fatalf("debug level dropped the command:\n%s", text)
	}
	if !strings.Contains(text, "aos: note     plan missing") {
		t.Fatalf("debug level dropped the note:\n%s", text)
	}
}

func TestNativeProgressNilStaysSilent(t *testing.T) {
	var progress *nativeProgress
	progress.Begin("claude", nil)
	step := progress.Step("fleet pass")
	step.Track("owner/repo", time.Second)
	progress.Item("fetch", 1, 1, "owner/repo")
	progress.Skip("fleet pass", "not due")
	progress.Wait("locked")
	progress.Note("detail")
	step.Done("")
	step.Fail(errors.New("boom"))
	progress.Exec([]string{"claude"})
	progress.Ready()
}

func TestNativeSweepNarratesEveryFetch(t *testing.T) {
	root := t.TempDir()
	first, _ := createNativeTestRepository(t, root, "owner", "one")
	second, _ := createNativeTestRepository(t, root, "owner", "two")
	runtime := nativeTestRuntime(t, root)
	writeNativeTestPlan(t, runtime.PlanFile, "one", "two")
	writeNativeTestList(t, runtime.FleetFile, "owner")
	progress, _, out := newTestProgress(t, "")
	runtime.Progress = progress

	repositories := []nativeRepository{
		{Owner: "owner", Name: "one", Path: first},
		{Owner: "owner", Name: "two", Path: second},
	}
	err := runNativeWorkspaceSweep(runtime, repositories, nativeExpected{
		Full:          map[string]bool{filepath.Join("owner", "one"): true, filepath.Join("owner", "two"): true},
		FleetOrgs:     map[string]bool{"owner": true},
		Authoritative: true,
	}, nativeLiveWorktrees{}, nativeSweepState{Candidates: map[string]nativeCandidate{}})
	if err != nil {
		t.Fatal(err)
	}

	text := out.String()
	for _, want := range []string{
		"aos: start    fleet pass over 2 repositories",
		"aos: fetch    1/2 owner/one",
		"aos: fetch    2/2 owner/two",
		"aos: start    scan for unexpected clones",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in:\n%s", want, text)
		}
	}
}

func TestStartupLockReclaimsAnAbandonedHolder(t *testing.T) {
	root := t.TempDir()
	runtime := nativeTestRuntime(t, root)
	progress, _, out := newTestProgress(t, "")
	runtime.Progress = progress
	lock := filepath.Join(runtime.StateRoot, "startup.lock")
	if err := os.MkdirAll(lock, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeNativeJSON(nativeLockOwnerPath(lock), nativeLockOwner{
		PID: 999999, ProcessStart: "gone", Acquired: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	ran := false
	began := time.Now()
	if err := withNativeStartupLock(runtime, func() error {
		ran = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	if !ran {
		t.Fatal("action never ran behind an abandoned lock")
	}
	if waited := time.Since(began); waited > 5*time.Second {
		t.Fatalf("reclaim waited %s, want an immediate reclaim", waited)
	}
	if !strings.Contains(out.String(), "reclaiming startup lock abandoned by pid 999999") {
		t.Fatalf("reclaim was not narrated:\n%s", out.String())
	}
	if _, err := os.Stat(lock); !os.IsNotExist(err) {
		t.Fatalf("lock survived the run: %v", err)
	}
}

func TestStartupLockLeavesALiveHolderAlone(t *testing.T) {
	root := t.TempDir()
	runtime := nativeTestRuntime(t, root)
	lock := filepath.Join(runtime.StateRoot, "startup.lock")
	if err := os.MkdirAll(lock, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeNativeJSON(nativeLockOwnerPath(lock), nativeLockOwner{
		PID: runtime.PID, ProcessStart: runtime.ProcessStart, Acquired: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	holder, live := inspectNativeStartupLock(lock)
	if !live {
		t.Fatal("a running holder was reported dead")
	}
	if holder.PID != runtime.PID {
		t.Fatalf("holder pid = %d, want %d", holder.PID, runtime.PID)
	}
}

func TestFormatNativeDurationSwitchesPrecision(t *testing.T) {
	cases := map[time.Duration]string{
		0:                        "0.00s",
		250 * time.Millisecond:   "0.25s",
		9500 * time.Millisecond:  "9.50s",
		12300 * time.Millisecond: "12.3s",
	}
	for elapsed, want := range cases {
		if got := formatNativeDuration(elapsed); got != want {
			t.Fatalf("formatNativeDuration(%s) = %s, want %s", elapsed, got, want)
		}
	}
}
