package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/urfave/cli/v3"
)

const nativeShadowReportFormat = "agentic-os.native-shadows.v1"

type nativeShadowArtifact struct {
	Repository string `json:"repository"`
	Worktree   string `json:"worktree"`
	Branch     string `json:"branch"`
	Present    bool   `json:"present"`
	Dirty      bool   `json:"dirty"`
	Unpushed   int    `json:"unpushed"`
	Releasable bool   `json:"releasable"`
	Held       string `json:"held,omitempty"`
}

type nativeShadowSession struct {
	ID         string                 `json:"id"`
	Harness    string                 `json:"harness"`
	PID        int                    `json:"pid"`
	Live       bool                   `json:"live"`
	Released   bool                   `json:"released"`
	DeadSince  *time.Time             `json:"dead_since,omitempty"`
	Root       string                 `json:"session_root"`
	RootExists bool                   `json:"session_root_exists"`
	Artifacts  []nativeShadowArtifact `json:"artifacts"`
	Releasable bool                   `json:"releasable"`
	Held       string                 `json:"held,omitempty"`
}

type nativeShadowReport struct {
	Format   string                `json:"format"`
	Sessions []nativeShadowSession `json:"sessions"`
}

// inspectNativeShadows reads what a shadow holds without changing any of it.
// See docs/native-shadow.md.
func inspectNativeShadows(runtime nativeRuntime) (nativeShadowReport, error) {
	return inspectNativeShadowsAs(runtime, false)
}

// inspectNativeShadowsAs reads as if --release-dead had already marked every dead lease,
// so a dry run's verdicts match what the real run does.
func inspectNativeShadowsAs(runtime nativeRuntime, releaseDead bool) (nativeShadowReport, error) {
	report := nativeShadowReport{Format: nativeShadowReportFormat}
	leaseDir := nativeStatePath(runtime, "leases")
	entries, err := os.ReadDir(leaseDir)
	if errors.Is(err, fs.ErrNotExist) {
		return report, nil
	}
	if err != nil {
		return report, fmt.Errorf("read native leases: %w", err)
	}
	held := []nativeHeldLease{}
	pids := []int{}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(leaseDir, entry.Name())
		var lease nativeLease
		if err := readNativeJSON(path, &lease); err != nil {
			report.Sessions = append(report.Sessions, nativeShadowSession{
				ID:   strings.TrimSuffix(entry.Name(), ".json"),
				Held: "the lease is unreadable",
			})
			continue
		}
		held = append(held, nativeHeldLease{path: path, lease: lease})
		pids = append(pids, lease.PID)
	}
	probe := probeNativeProcesses(pids)
	for _, entry := range held {
		if releaseDead && entry.lease.Released == nil && !probe.leaseIsLive(entry.lease) {
			released := runtime.Now
			entry.lease.Released = &released
		}
		report.Sessions = append(report.Sessions, inspectNativeShadow(runtime, entry.lease, probe))
	}
	sort.Slice(report.Sessions, func(first, second int) bool {
		return report.Sessions[first].ID < report.Sessions[second].ID
	})
	return report, nil
}

func inspectNativeShadow(
	runtime nativeRuntime,
	lease nativeLease,
	probe nativeProcessProbe,
) nativeShadowSession {
	session := nativeShadowSession{
		ID:        lease.ID,
		Harness:   lease.Harness,
		PID:       lease.PID,
		Live:      probe.leaseIsLive(lease),
		Released:  lease.Released != nil,
		DeadSince: lease.DeadSince,
		Root:      lease.SessionRoot,
	}
	if _, err := os.Stat(lease.SessionRoot); err == nil {
		session.RootExists = true
	}
	for _, artifact := range lease.Artifacts {
		session.Artifacts = append(session.Artifacts, inspectNativeArtifact(artifact))
	}
	if session.Live && !session.Released {
		session.Held = "the session process is still running"
		return session
	}
	// A crash and a clean exit look the same from outside, so a dead lease
	// waits out the grace. A released one already said which it was.
	if lease.Released == nil {
		if lease.DeadSince == nil {
			session.Held = "the first dead reading is not recorded yet"
			return session
		}
		expires := lease.DeadSince.Add(nativeDeadSessionGrace)
		if runtime.Now.Before(expires) {
			session.Held = fmt.Sprintf("within the dead-session grace, %s left",
				expires.Sub(runtime.Now).Round(time.Minute))
			return session
		}
	}
	blocked := []string{}
	for _, artifact := range session.Artifacts {
		if !artifact.Releasable && artifact.Held != "" {
			blocked = append(blocked, artifact.Branch+": "+artifact.Held)
		}
	}
	if len(blocked) > 0 {
		session.Held = strings.Join(blocked, "; ")
		return session
	}
	session.Releasable = true
	return session
}

func inspectNativeArtifact(artifact nativeArtifact) nativeShadowArtifact {
	reading := nativeShadowArtifact{
		Repository: artifact.Repository,
		Worktree:   artifact.Worktree,
		Branch:     artifact.Branch,
	}
	if _, err := os.Stat(artifact.Worktree); err == nil {
		reading.Present = true
	}
	if reading.Present {
		if humanWorkdir(artifact.Worktree) {
			reading.Held = "human workdir is outside automation"
			return reading
		}
		clean, err := nativeWorktreeClean(artifact.Worktree, false)
		if err != nil {
			reading.Held = "the worktree cannot be read"
			return reading
		}
		reading.Dirty = !clean
		if reading.Dirty {
			reading.Held = "the worktree has uncommitted changes"
			return reading
		}
	}
	reading.Unpushed = nativeUnpushedCommits(artifact)
	if reading.Unpushed > 0 && !nativeBranchLanded(artifact.Repository, artifact.Branch) {
		reading.Held = fmt.Sprintf("%d commit(s) exist on no remote", reading.Unpushed)
		// Landed-only is the release rule, so a branch whose hunks main
		// rewrote waits for a person, who is told that much. #8277
		if nativeBranchSubsumed(artifact.Repository, artifact.Branch) {
			reading.Held += "; main has them except hunks it rewrote, so a person decides whether to delete the branch"
		}
		return reading
	}
	reading.Releasable = true
	return reading
}

func nativeUnpushedCommits(artifact nativeArtifact) int {
	reference := "refs/heads/" + artifact.Branch
	if artifact.Branch == "" {
		return 0
	}
	output, err := nativeGit(artifact.Repository,
		"rev-list", "--count", reference, "--not", "--remotes=origin")
	if err != nil {
		return 0
	}
	count := 0
	if _, err := fmt.Sscanf(output, "%d", &count); err != nil {
		return 0
	}
	return count
}

// writeClaudeCredentialHealth prints state only. docs/native-claude-credentials.md.
func writeClaudeCredentialHealth(health claudeCredentialHealth, asJSON bool) error {
	if asJSON {
		encoded, err := json.MarshalIndent(health, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal the credential health: %w", err)
		}
		_, err = fmt.Fprintf(os.Stdout, "%s\n", encoded)
		return err
	}
	_, err := fmt.Fprintln(os.Stdout, health.Line())
	return err
}

func writeNativeShadowReport(runtime nativeRuntime, report nativeShadowReport, asJSON bool) error {
	if asJSON {
		encoded, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal the shadow report: %w", err)
		}
		_, err = fmt.Fprintf(os.Stdout, "%s\n", encoded)
		return err
	}
	if len(report.Sessions) == 0 {
		_, err := fmt.Fprintln(os.Stdout, "no native session shadows")
		return err
	}
	for _, session := range report.Sessions {
		state := "dead"
		switch {
		case session.Live && session.Released:
			state = "live, released"
		case session.Live:
			state = "live"
		case session.Released:
			state = "released"
		}
		verdict := "held"
		if session.Releasable {
			verdict = "releasable"
		}
		fmt.Fprintf(os.Stdout, "%s // %s // %s // %s // %d worktree(s)\n",
			session.ID, session.Harness, state, verdict, len(session.Artifacts))
		if session.Held != "" {
			fmt.Fprintf(os.Stdout, "  held: %s\n", session.Held)
		}
		for _, artifact := range session.Artifacts {
			presence := "present"
			if !artifact.Present {
				presence = "purged"
			}
			fmt.Fprintf(os.Stdout, "  %s // %s // %d unpushed\n",
				artifact.Branch, presence, artifact.Unpushed)
		}
	}
	return nil
}

// releaseNativeShadow is how a session declares itself finished. It never tears
// down a running session: it marks the lease and the next sweep collects it.
func releaseNativeShadow(runtime nativeRuntime, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		id = strings.TrimSpace(os.Getenv(nativeSessionEnv))
	}
	if id == "" {
		return fmt.Errorf("--release needs a session id, and %s is not set", nativeSessionEnv)
	}
	path := nativeStatePath(runtime, "leases", id+".json")
	var lease nativeLease
	if err := readNativeJSON(path, &lease); err != nil {
		return fmt.Errorf("read the lease for session %s: %w", id, err)
	}
	if lease.Released != nil {
		fmt.Fprintf(os.Stdout, "session %s was already released\n", id)
		return nil
	}
	released := runtime.Now
	lease.Released = &released
	if err := writeNativeJSON(path, lease); err != nil {
		return fmt.Errorf("record the release of session %s: %w", id, err)
	}
	fmt.Fprintf(os.Stdout,
		"session %s released. Its worktrees go on the next sweep, once the process exits.\n", id)
	return nil
}

// finishNativeShadows releases every dead lease pick accepts, skipping the crash grace,
// and collects them unless dryRun. Unpushed or dirty worktrees stay (COI-3427).
func finishNativeShadows(runtime nativeRuntime, pick func(nativeLease) bool, dryRun bool) error {
	leaseDir := nativeStatePath(runtime, "leases")
	entries, err := os.ReadDir(leaseDir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read native leases: %w", err)
	}
	type found struct {
		path  string
		lease nativeLease
	}
	var all []found
	var pids []int
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(leaseDir, entry.Name())
		var lease nativeLease
		if readNativeJSON(path, &lease) != nil || !pick(lease) {
			continue
		}
		all = append(all, found{path, lease})
		pids = append(pids, lease.PID)
	}
	probe := probeNativeProcesses(pids)
	ids := map[string]bool{}
	for _, item := range all {
		if probe.leaseIsLive(item.lease) {
			continue
		}
		ids[item.lease.ID] = true
		if item.lease.Released != nil || dryRun {
			continue
		}
		released := runtime.Now
		item.lease.Released = &released
		if err := writeNativeJSON(item.path, item.lease); err != nil {
			return fmt.Errorf("record the release of session %s: %w", item.lease.ID, err)
		}
	}
	verb := "marked released"
	if dryRun {
		verb = "would be marked released"
	}
	fmt.Fprintf(os.Stdout, "%d dead session(s) %s, grace skipped\n", len(ids), verb)
	if dryRun || len(ids) == 0 {
		return nil
	}
	return withNativeStartupLock(runtime, func() error {
		_, err := cleanNativeSessionsWhere(runtime, func(lease nativeLease) bool { return ids[lease.ID] })
		return err
	})
}

// reapNativeShadows runs the sweep an operator would otherwise have to trigger
// by launching another session.
func reapNativeShadows(runtime nativeRuntime, dryRun, releaseDead bool) error {
	if dryRun {
		report, err := inspectNativeShadowsAs(runtime, releaseDead)
		if err != nil {
			return err
		}
		releasable := 0
		for _, session := range report.Sessions {
			if session.Releasable {
				releasable++
			}
		}
		if err := writeNativeShadowReport(runtime, report, false); err != nil {
			return err
		}
		_, err = fmt.Fprintf(os.Stdout, "\n%d of %d session(s) would be released\n",
			releasable, len(report.Sessions))
		return err
	}
	if err := os.MkdirAll(runtime.StateRoot, 0o700); err != nil {
		return fmt.Errorf("create native state root: %w", err)
	}
	return withNativeStartupLock(runtime, func() error {
		live, err := cleanDeadNativeSessions(runtime)
		if err != nil {
			return err
		}
		projection, err := resolveExpectedRepositories(runtime)
		if err != nil {
			return err
		}
		_, state := nativeSweepDue(runtime)
		return runNativeWorkspaceSweep(
			runtime, projection.Resident, projection.Expected, live, state)
	})
}

// nativeShadowLifecycleVerb splits the read-and-reclaim verbs off the launch
// path, which is the one that needs a harness and a command.
func nativeShadowLifecycleVerb(cmd *cli.Command) (func(nativeRuntime) error, bool) {
	switch {
	case cmd.Bool("list"):
		asJSON := cmd.Bool("json")
		return func(runtime nativeRuntime) error {
			report, err := inspectNativeShadows(runtime)
			if err != nil {
				return err
			}
			return writeNativeShadowReport(runtime, report, asJSON)
		}, true
	case strings.TrimSpace(cmd.String("release")) != "" || cmd.IsSet("release"):
		id := cmd.String("release")
		return func(runtime nativeRuntime) error {
			return releaseNativeShadow(runtime, id)
		}, true
	case cmd.String("trust-guard") != "":
		plan := nativeTrustPlan{
			Config: cmd.String("trust-guard"), External: cmd.Bool("trust-external"),
			Paths: strings.Split(cmd.String("trust-paths"), "\n"),
		}
		return func(runtime nativeRuntime) error { return runNativeTrustGuard(runtime, plan) }, true
	case cmd.Int("finished") > 0:
		pid := int(cmd.Int("finished"))
		return func(runtime nativeRuntime) error {
			return finishNativeShadows(runtime, func(lease nativeLease) bool { return lease.PID == pid }, false)
		}, true
	case cmd.Bool("reap"):
		dryRun, releaseDead := cmd.Bool("dry-run"), cmd.Bool("release-dead")
		return func(runtime nativeRuntime) error {
			if releaseDead {
				if err := finishNativeShadows(runtime, func(nativeLease) bool { return true }, dryRun); err != nil {
					return err
				}
			}
			return reapNativeShadows(runtime, dryRun, releaseDead)
		}, true
	case cmd.Bool("credential"):
		asJSON := cmd.Bool("json")
		return func(runtime nativeRuntime) error {
			return writeClaudeCredentialHealth(
				inspectCanonicalClaudeCredential(runtime.Home, runtime.Now), asJSON)
		}, true
	}
	return nil, false
}
