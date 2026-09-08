package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/urfave/cli/v3"
)

// justSummaryTimeout bounds one `just --summary`, so a justfile that blocks on
// a settings expression cannot hang resolution across the whole residency set.
const justSummaryTimeout = 5 * time.Second

type verbOwner struct {
	Identity string
	Path     string
}

// resolveVerbOwners returns every resident repository declaring verb, asking
// `just --summary` so aliases and imports resolve the way just resolves them.
func resolveVerbOwners(ctx context.Context, plan aosRepositoryPlan, verb string) []verbOwner {
	var mutex sync.Mutex
	var owners []verbOwner
	var group sync.WaitGroup
	for _, repository := range plan.Residency {
		justfile := filepath.Join(repository.Path, "justfile")
		if info, err := os.Stat(justfile); err != nil || info.IsDir() {
			continue
		}
		group.Add(1)
		go func(identity, root, justfile string) {
			defer group.Done()
			if !justfileDeclares(ctx, root, justfile, verb) {
				return
			}
			mutex.Lock()
			defer mutex.Unlock()
			owners = append(owners, verbOwner{Identity: identity, Path: root})
		}(repository.Identity, repository.Path, justfile)
	}
	group.Wait()
	sort.Slice(owners, func(i, j int) bool { return owners[i].Identity < owners[j].Identity })
	return owners
}

func justfileDeclares(ctx context.Context, root, justfile, verb string) bool {
	ctx, cancel := context.WithTimeout(ctx, justSummaryTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "just",
		"--justfile", justfile, "--working-directory", root, "--summary")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	for _, candidate := range strings.Fields(string(out)) {
		if candidate == verb {
			return true
		}
	}
	return false
}

// checkoutState is what a caller needs before trusting a run: how far the
// checkout trails its upstream, and whether a pull could even fast-forward.
type checkoutState struct {
	Upstream  string
	Behind    int
	Dirty     bool
	FetchedAt time.Time
	Known     bool
}

func inspectCheckout(root string) checkoutState {
	state := checkoutState{}
	upstream, err := gitOutput(root, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}")
	if err != nil {
		return state
	}
	state.Upstream = upstream
	behind, err := gitOutput(root, "rev-list", "--count", "HEAD..@{upstream}")
	if err != nil {
		return state
	}
	count, err := strconv.Atoi(behind)
	if err != nil {
		return state
	}
	state.Behind = count
	state.Known = true
	if status, err := gitOutput(root, "status", "--porcelain"); err == nil && status != "" {
		state.Dirty = true
	}
	if info, err := os.Stat(filepath.Join(root, ".git", "FETCH_HEAD")); err == nil {
		state.FetchedAt = info.ModTime()
	}
	return state
}

func gitOutput(root string, args ...string) (string, error) {
	out, err := exec.Command("git", append([]string{"-C", root}, args...)...).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// describeStaleness reports the trailing distance and stamps the reading,
// because a behind-count is only as current as the last fetch.
func describeStaleness(state checkoutState, now time.Time) string {
	if !state.Known {
		return "aos: no upstream for this checkout, so staleness is unknown"
	}
	stamp := "never fetched"
	if !state.FetchedAt.IsZero() {
		stamp = "fetched " + humanizeAge(now.Sub(state.FetchedAt)) + " ago"
	}
	if state.Behind == 0 {
		return fmt.Sprintf("aos: up to date with %s (%s)", state.Upstream, stamp)
	}
	plural := "commits"
	if state.Behind == 1 {
		plural = "commit"
	}
	return fmt.Sprintf("aos: %d %s behind %s (%s)", state.Behind, plural, state.Upstream, stamp)
}

func humanizeAge(age time.Duration) string {
	switch {
	case age < time.Minute:
		return "seconds"
	case age < time.Hour:
		return fmt.Sprintf("%dm", int(age.Minutes()))
	case age < 24*time.Hour:
		return fmt.Sprintf("%dh", int(age.Hours()))
	default:
		return fmt.Sprintf("%dd", int(age.Hours()/24))
	}
}

// handoffScript is the line a person pastes outside the session. The path is
// absolute, because a relative one resolves against a directory they were never in.
func handoffScript(owner verbOwner, state checkoutState, verb string, args []string) string {
	command := "just " + shellJoin(append([]string{verb}, args...))
	if state.Known && state.Behind > 0 && !state.Dirty {
		command = "git pull && " + command
	}
	return fmt.Sprintf("cd %s \\\n      && %s", shellJoin([]string{owner.Path}), command)
}

// shadowNotice explains why a run reaches past the session, or is empty outside
// one. Residency decides: a repository the shadow lacks is touched canonically.
func shadowNotice(env func(string) string, owner verbOwner, handoff bool) string {
	session := strings.TrimSpace(env(nativeSessionEnv))
	if session == "" {
		return ""
	}
	projects := strings.TrimSpace(env(nativeSessionProjectsEnv))
	if projects == "" {
		return ""
	}
	identity := filepath.Join(strings.Split(owner.Identity, "/")...)
	if info, err := os.Stat(filepath.Join(projects, identity)); err == nil && info.IsDir() {
		return ""
	}
	notice := fmt.Sprintf(
		"aos: %s has no checkout in session shadow %s, so this reaches the canonical\n"+
			"  one while HOME is %s. A verb that renders a ~-rooted host path\n"+
			"  writes into the shadow instead.",
		owner.Identity, session, env("HOME"))
	if handoff {
		return notice
	}
	return notice + " Pass --handoff for the line to run outside."
}

// resolveRunPlanPath falls back to the canonical home's plan when a shadow
// carries none, because the plan names canonical paths from either home.
func resolveRunPlanPath(configured string, env func(string) string) string {
	if _, err := os.Stat(configured); err == nil {
		return configured
	}
	canonical := strings.TrimSpace(env(nativeCanonicalHomeEnv))
	if canonical == "" {
		return configured
	}
	fallback := repositoryPlanPath(canonical)
	if _, err := os.Stat(fallback); err != nil {
		return configured
	}
	return fallback
}

func runRun(ctx context.Context, cmd *cli.Command) error {
	args := cmd.Args().Slice()
	if len(args) == 0 {
		return fmt.Errorf("run needs a just verb")
	}
	verb, rest := args[0], args[1:]
	plan, err := loadAOSRepositoryPlan(resolveRunPlanPath(cmd.String("plan"), os.Getenv))
	if err != nil {
		return err
	}
	owners := resolveVerbOwners(ctx, plan, verb)
	if selected := strings.TrimSpace(cmd.String("repo")); selected != "" {
		owners = filterOwners(owners, selected)
		if len(owners) == 0 {
			return fmt.Errorf("no resident repository %q declares %q", selected, verb)
		}
	}
	stderr := cmd.Root().ErrWriter
	if len(owners) > 1 {
		cwd, _ := os.Getwd()
		if inside := ownerContainingCWD(owners, cwd, os.Getenv(nativeSessionProjectsEnv)); inside != nil {
			owners = []verbOwner{*inside}
		}
	}
	switch len(owners) {
	case 0:
		return fmt.Errorf(
			"no resident repository declares %q; run `just` inside one to list its verbs", verb)
	case 1:
	default:
		var lines []string
		for _, owner := range owners {
			lines = append(lines, "  "+owner.Identity)
		}
		return fmt.Errorf("%q is declared by %d repositories:\n%s\nname one with --repo <owner/name>",
			verb, len(owners), strings.Join(lines, "\n"))
	}
	owner := owners[0]
	state := inspectCheckout(owner.Path)
	fmt.Fprintf(stderr, "aos: %s -> %s\n", verb, owner.Identity)
	fmt.Fprintln(stderr, describeStaleness(state, time.Now()))
	if state.Dirty {
		fmt.Fprintln(stderr, "aos: that checkout is dirty, so a pull would not fast-forward")
	}
	if notice := shadowNotice(os.Getenv, owner, cmd.Bool("handoff")); notice != "" {
		fmt.Fprintln(stderr, notice)
	}
	if cmd.Bool("handoff") {
		fmt.Fprint(stderr, "aos: run this in a terminal outside the agent session:\n\n")
		fmt.Fprintf(cmd.Root().Writer, "    %s\n", handoffScript(owner, state, verb, rest))
		return nil
	}
	just := exec.CommandContext(ctx, "just", append([]string{verb}, rest...)...)
	just.Dir = owner.Path
	just.Stdin, just.Stdout, just.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := just.Run(); err != nil {
		if notice := shadowNotice(os.Getenv, owner, true); notice != "" {
			fmt.Fprintf(stderr, "\naos: that failed inside a session shadow. Outside it:\n\n    %s\n",
				handoffScript(owner, state, verb, rest))
		}
		return cli.Exit("", just.ProcessState.ExitCode())
	}
	return nil
}

// ownerContainingCWD picks the candidate the caller stands in, shadow roots
// included: a shadow cwd carries the identity the plan only names canonically.
func ownerContainingCWD(owners []verbOwner, cwd, sessionProjects string) *verbOwner {
	cwd = resolvePath(cwd)
	best := -1
	var winner verbOwner
	for _, owner := range owners {
		roots := []string{owner.Path}
		if sessionProjects != "" {
			roots = append(roots, filepath.Join(sessionProjects,
				filepath.Join(strings.Split(owner.Identity, "/")...)))
		}
		for _, root := range roots {
			root = resolvePath(root)
			relative, err := filepath.Rel(root, cwd)
			if err != nil || relative == ".." ||
				strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
				continue
			}
			if len(root) > best {
				best, winner = len(root), owner
			}
		}
	}
	if best < 0 {
		return nil
	}
	return &winner
}

func resolvePath(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return path
}

func filterOwners(owners []verbOwner, identity string) []verbOwner {
	var kept []verbOwner
	for _, owner := range owners {
		if owner.Identity == identity {
			kept = append(kept, owner)
		}
	}
	return kept
}
