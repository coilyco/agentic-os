package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// runFixture lays out a projects root whose repositories carry real justfiles,
// because resolution shells out to `just --summary` rather than parsing them.
func runFixture(t *testing.T, verbs map[string][]string) aosRepositoryPlan {
	t.Helper()
	root := t.TempDir()
	plan := aosRepositoryPlan{
		Format: agentComposeRepositoryPlanYAMLFormat, ProjectsRoot: root,
	}
	identities := make([]string, 0, len(verbs))
	for identity := range verbs {
		identities = append(identities, identity)
	}
	for _, identity := range identities {
		dir := filepath.Join(root, filepath.FromSlash(identity))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if recipes := verbs[identity]; recipes != nil {
			body := ""
			for _, recipe := range recipes {
				body += recipe + ":\n    @echo " + recipe + "\n\n"
			}
			if err := os.WriteFile(filepath.Join(dir, "justfile"), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		plan.Residency = append(plan.Residency, aosRepositorySelection{
			Identity: identity, Path: dir,
			Source: "test", Scope: "role-union", Reason: "test selection",
		})
	}
	return plan
}

func requireJust(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("just"); err != nil {
		t.Skip("no just on PATH, so verb resolution cannot be exercised")
	}
}

func TestResolveVerbOwnersFindsEveryDeclaringRepository(t *testing.T) {
	requireJust(t)
	plan := runFixture(t, map[string][]string{
		"owner/alpha": {"test", "build"},
		"owner/beta":  {"test"},
		"owner/gamma": {"deploy"},
		"owner/delta": nil,
	})
	owners := resolveVerbOwners(context.Background(), plan, "test")
	var got []string
	for _, owner := range owners {
		got = append(got, owner.Identity)
	}
	if strings.Join(got, ",") != "owner/alpha,owner/beta" {
		t.Fatalf("resolution is wrong or unsorted: %v", got)
	}
	if owners := resolveVerbOwners(context.Background(), plan, "deploy"); len(owners) != 1 {
		t.Fatalf("a single declaring repository must resolve alone: %v", owners)
	}
	if owners := resolveVerbOwners(context.Background(), plan, "absent"); len(owners) != 0 {
		t.Fatalf("an undeclared verb must resolve to nothing: %v", owners)
	}
}

func TestResolveVerbOwnersIgnoresARepositoryWithoutAJustfile(t *testing.T) {
	requireJust(t)
	plan := runFixture(t, map[string][]string{"owner/alpha": nil})
	if owners := resolveVerbOwners(context.Background(), plan, "test"); len(owners) != 0 {
		t.Fatalf("a repository with no justfile declares nothing: %v", owners)
	}
}

func TestOwnerContainingCWDPrefersTheRepositoryYouStandIn(t *testing.T) {
	owners := []verbOwner{
		{Identity: "owner/alpha", Path: "/projects/owner/alpha"},
		{Identity: "owner/beta", Path: "/projects/owner/beta"},
	}
	if inside := ownerContainingCWD(owners, "/projects/owner/beta/internal", ""); inside == nil ||
		inside.Identity != "owner/beta" {
		t.Fatalf("a nested directory must select its own repository: %v", inside)
	}
	if inside := ownerContainingCWD(owners, "/projects", ""); inside != nil {
		t.Fatalf("an elevated cwd selects nothing: %v", inside)
	}
	// The shadow carries the same identity under a root the plan never names.
	if inside := ownerContainingCWD(owners, "/tmp/shadow/owner/alpha", "/tmp/shadow"); inside == nil ||
		inside.Identity != "owner/alpha" {
		t.Fatalf("a shadow cwd must map to its canonical identity: %v", inside)
	}
	if inside := ownerContainingCWD(owners, "/elsewhere", "/tmp/shadow"); inside != nil {
		t.Fatalf("an unrelated cwd selects nothing: %v", inside)
	}
}

func TestDescribeStalenessStampsTheReading(t *testing.T) {
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	fetched := now.Add(-90 * time.Minute)
	for name, testCase := range map[string]struct {
		state checkoutState
		want  string
	}{
		"behind one": {
			checkoutState{Upstream: "origin/main", Behind: 1, FetchedAt: fetched, Known: true},
			"aos: 1 commit behind origin/main (fetched 1h ago)",
		},
		"behind many": {
			checkoutState{Upstream: "origin/main", Behind: 4, FetchedAt: fetched, Known: true},
			"aos: 4 commits behind origin/main (fetched 1h ago)",
		},
		"current": {
			checkoutState{Upstream: "origin/main", FetchedAt: fetched, Known: true},
			"aos: up to date with origin/main (fetched 1h ago)",
		},
		"never fetched": {
			checkoutState{Upstream: "origin/main", Behind: 2, Known: true},
			"aos: 2 commits behind origin/main (never fetched)",
		},
		"no upstream": {
			checkoutState{},
			"aos: no upstream for this checkout, so staleness is unknown",
		},
	} {
		t.Run(name, func(t *testing.T) {
			if got := describeStaleness(testCase.state, now); got != testCase.want {
				t.Fatalf("got %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestHandoffScriptNamesAnAbsolutePathAndPullsOnlyWhenItCan(t *testing.T) {
	owner := verbOwner{Identity: "owner/infra", Path: "/Users/kai/projects/owner/infra"}
	behind := checkoutState{Upstream: "origin/main", Behind: 1, Known: true}
	script := handoffScript(owner, behind, "ansible-sync", []string{"apply", "tags=agent-compose"})
	if !strings.Contains(script, "cd /Users/kai/projects/owner/infra") {
		t.Fatalf("handoff must name an absolute path: %s", script)
	}
	if !strings.Contains(script, "git pull && just ansible-sync apply tags=agent-compose") {
		t.Fatalf("a behind checkout must pull first: %s", script)
	}
	dirty := behind
	dirty.Dirty = true
	if strings.Contains(handoffScript(owner, dirty, "test", nil), "git pull") {
		t.Fatal("a dirty checkout must not be told to pull, because it would not fast-forward")
	}
	current := checkoutState{Upstream: "origin/main", Known: true}
	if strings.Contains(handoffScript(owner, current, "test", nil), "git pull") {
		t.Fatal("a current checkout needs no pull")
	}
}

func TestHandoffScriptQuotesAnArgumentTheShellWouldReinterpret(t *testing.T) {
	owner := verbOwner{Identity: "owner/alpha", Path: "/projects/owner/alpha"}
	script := handoffScript(owner, checkoutState{}, "test", []string{"-k", "name and other"})
	if !strings.Contains(script, "'name and other'") {
		t.Fatalf("a spaced argument must survive the handoff quoted: %s", script)
	}
}

func TestShadowNoticeFiresOnlyForARepositoryTheShadowLacks(t *testing.T) {
	shadow := t.TempDir()
	present := verbOwner{Identity: "owner/alpha", Path: "/projects/owner/alpha"}
	if err := os.MkdirAll(filepath.Join(shadow, "owner", "alpha"), 0o755); err != nil {
		t.Fatal(err)
	}
	env := func(values map[string]string) func(string) string {
		return func(key string) string { return values[key] }
	}
	inShadow := env(map[string]string{
		nativeSessionEnv:         "xu66",
		nativeSessionProjectsEnv: shadow,
		"HOME":                   "/tmp/shadow/home",
	})
	if notice := shadowNotice(inShadow, present, false); notice != "" {
		t.Fatalf("a shadowed repository needs no notice: %s", notice)
	}
	absent := verbOwner{Identity: "owner/beta", Path: "/projects/owner/beta"}
	notice := shadowNotice(inShadow, absent, false)
	if !strings.Contains(notice, "owner/beta") || !strings.Contains(notice, "/tmp/shadow/home") {
		t.Fatalf("the notice must name the repository and the shadow HOME: %s", notice)
	}
	if !strings.Contains(notice, "--handoff") {
		t.Fatalf("the notice must point at the escape hatch: %s", notice)
	}
	if strings.Contains(shadowNotice(inShadow, absent, true), "--handoff") {
		t.Fatal("the notice must not advertise --handoff to a caller who already passed it")
	}
	outside := env(map[string]string{"HOME": "/Users/kai"})
	if notice := shadowNotice(outside, absent, false); notice != "" {
		t.Fatalf("outside a shadow there is nothing to warn about: %s", notice)
	}
}

func TestFilterOwnersSelectsOneIdentity(t *testing.T) {
	owners := []verbOwner{{Identity: "owner/alpha"}, {Identity: "owner/beta"}}
	if kept := filterOwners(owners, "owner/beta"); len(kept) != 1 || kept[0].Identity != "owner/beta" {
		t.Fatalf("--repo must narrow to the named identity: %v", kept)
	}
	if kept := filterOwners(owners, "owner/absent"); len(kept) != 0 {
		t.Fatalf("an unmatched --repo must narrow to nothing: %v", kept)
	}
}

func TestResolveRunPlanPathFallsBackToTheCanonicalHome(t *testing.T) {
	canonical := t.TempDir()
	plan := filepath.Join(canonical, ".agent-compose", "repository-plan.yaml")
	if err := os.MkdirAll(filepath.Dir(plan), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(plan, []byte("format: test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	env := func(key string) string {
		if key == nativeCanonicalHomeEnv {
			return canonical
		}
		return ""
	}
	absent := filepath.Join(t.TempDir(), "missing.yaml")
	if got := resolveRunPlanPath(absent, env); got != plan {
		t.Fatalf("a shadow without a plan must fall back to the canonical one: %s", got)
	}
	if got := resolveRunPlanPath(plan, env); got != plan {
		t.Fatalf("an existing plan must be used as configured: %s", got)
	}
	bare := func(string) string { return "" }
	if got := resolveRunPlanPath(absent, bare); got != absent {
		t.Fatalf("with no canonical home the configured path stands: %s", got)
	}
}
