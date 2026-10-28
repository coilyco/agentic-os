package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"
)

// ghExecCommand is the verb the guarded gh replacement execs, after umbra has
// validated the argv: `aterm gh-exec <gh args>`.
const ghExecCommand = "gh-exec"

// ghCredentialEnv are the variables gh reads a login from. They are dropped, so
// nothing the seat exported can stand in for the broker token.
var ghCredentialEnv = []string{"GH_TOKEN", "GITHUB_TOKEN", "GH_ENTERPRISE_TOKEN", "GITHUB_ENTERPRISE_TOKEN", "GH_CONFIG_DIR"}

var (
	apiRepoPath = regexp.MustCompile(`^/?repos/([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+)(/|$)`)
	githubURL   = regexp.MustCompile(`github\.com[:/]([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+?)(\.git)?/?$`)
)

// ghRepoFromArgs is the repo a gh call names: the last --repo, as gh takes the
// last, else the first `repos/OWNER/NAME` of a gh api call.
func ghRepoFromArgs(args []string) string {
	repo := ""
	for index := 0; index < len(args); index++ {
		switch value, ok := strings.CutPrefix(args[index], "--repo="); {
		case ok:
			repo = value
		case args[index] == "--repo" && index+1 < len(args):
			index++
			repo = args[index]
		}
	}
	if repo != "" || len(args) == 0 || args[0] != "api" {
		return repo
	}
	for _, arg := range args[1:] {
		if found := apiRepoPath.FindStringSubmatch(arg); found != nil {
			return found[1] + "/" + found[2]
		}
	}
	return ""
}

// ghRepoFromRemotes is what gh would resolve from the checkout: the one repo
// every github.com remote points at, and none when they disagree.
func ghRepoFromRemotes(remotes string) string {
	repo := ""
	for _, line := range strings.Split(remotes, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		found := githubURL.FindStringSubmatch(fields[1])
		if found == nil {
			continue
		}
		candidate := found[1] + "/" + found[2]
		if repo != "" && repo != candidate {
			return ""
		}
		repo = candidate
	}
	return repo
}

// ghReadVerbs are the gh verbs that run anonymously for a repo outside coilyco.
// The guard already limits `gh api` to a GET, so it is listed whole.
var ghReadVerbs = map[string][]string{
	"pr":       {"view", "list", "checks", "diff", "status"},
	"issue":    {"view", "list", "status"},
	"run":      {"view", "list"},
	"repo":     {"view", "list"},
	"release":  {"view", "list"},
	"workflow": {"view", "list"},
	"search":   nil,
	"api":      nil,
}

// ghVerbWords is the leading words of a call, before its first flag.
func ghVerbWords(args []string) []string {
	words := []string{}
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") || len(words) == 2 {
			break
		}
		words = append(words, arg)
	}
	return words
}

func ghReadVerb(args []string) bool {
	words := ghVerbWords(args)
	if len(words) == 0 {
		return false
	}
	subs, known := ghReadVerbs[words[0]]
	if !known {
		return false
	}
	return subs == nil || (len(words) == 2 && slices.Contains(subs, words[1]))
}

func ghExecRefuse(stderr io.Writer, format string, args ...any) int {
	fmt.Fprintf(stderr, "aterm gh-exec: "+format+"\n", args...)
	return exitUsage
}

// runGhExec returns the real gh's exit code, or a refusal. Every refusal runs
// before gh starts, and none falls back to the host login.
func runGhExec(args []string, stdout, stderr io.Writer) int {
	repo := ghRepoFromArgs(args)
	if repo == "" {
		if out, err := exec.Command("git", "remote", "-v").Output(); err == nil {
			repo = ghRepoFromRemotes(string(out))
		}
	}
	token := ""
	if owner, _, _ := strings.Cut(repo, "/"); owner == githubOwner {
		minted, err := requestGithubToken(repo)
		if err != nil {
			fmt.Fprintf(stderr, "aterm gh-exec: cannot mint a token for %s: %v\naterm gh-exec: refusing, and not falling back to the host login\n", repo, err)
			return exitCodeFor(err)
		}
		token = minted
	} else if !ghReadVerb(args) {
		// Anonymous is read-only: a write has no token to carry it.
		return ghExecRefuse(stderr, "`gh %s` is refused. It is not a read verb, and a write needs a coilyco repo to mint a token for (this call names %q)", strings.Join(ghVerbWords(args), " "), repo)
	}
	gh, err := exec.LookPath("gh")
	if err != nil {
		return ghExecRefuse(stderr, "no gh on PATH to run")
	}
	// An empty config dir hides the host login: gh finds the keyring user through
	// hosts.yml, which this directory does not have.
	config, err := os.MkdirTemp("", "aterm-gh-")
	if err != nil {
		return ghExecRefuse(stderr, "cannot make a private gh config dir: %v", err)
	}
	defer os.RemoveAll(config)
	env := []string{}
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if !slices.Contains(ghCredentialEnv, name) {
			env = append(env, entry)
		}
	}
	child := exec.Command(gh, args...)
	child.Env = append(env, "GH_CONFIG_DIR="+config)
	if token != "" {
		child.Env = append(child.Env, "GH_TOKEN="+token)
	}
	child.Stdin, child.Stdout, child.Stderr = os.Stdin, stdout, stderr
	err = child.Run()
	var exited *exec.ExitError
	switch {
	case err == nil:
		return 0
	case errors.As(err, &exited):
		return exited.ExitCode()
	}
	return ghExecRefuse(stderr, "could not run gh: %v", err)
}
