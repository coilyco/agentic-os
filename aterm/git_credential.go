package main

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// gitCredentialCommand is `aterm git-credential <get|store|erase>`, a git credential
// helper that answers a coilyco request with a one-repo App token. See the skill.
const gitCredentialCommand = "git-credential"

// readCredentialAttrs reads the key=value lines git writes to a helper, up to
// the blank line or EOF.
func readCredentialAttrs(r io.Reader) map[string]string {
	attrs := map[string]string{}
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			break
		}
		if key, value, ok := strings.Cut(line, "="); ok {
			attrs[key] = value
		}
	}
	return attrs
}

// credentialRepo is the coilyco OWNER/REPO a request names, and false for any
// request this helper should leave to the next helper.
func credentialRepo(attrs map[string]string) (string, bool) {
	if attrs["protocol"] != "https" || attrs["host"] != "github.com" {
		return "", false
	}
	path := strings.TrimSuffix(strings.Trim(attrs["path"], "/"), ".git")
	owner, name, _ := strings.Cut(path, "/")
	if _, _, err := parseGithubRepo(owner + "/" + name); err != nil {
		return "", false
	}
	return owner + "/" + name, true
}

// runGitCredential returns the exit code. Only get acts, since a token lives an hour,
// and every refusal leaves stdout empty so git asks the next helper.
func runGitCredential(args []string, stdin io.Reader, stdout, stderr io.Writer, mint func(repo string) (string, error)) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "aterm git-credential: want exactly one of get, store, erase")
		return exitUsage
	}
	if args[0] != "get" {
		return 0
	}
	attrs := readCredentialAttrs(stdin)
	repo, ok := credentialRepo(attrs)
	if !ok {
		if attrs["host"] == "github.com" && attrs["path"] == "" {
			// Without the path git sends only the host, so no repo can be named.
			fmt.Fprintln(stderr, "aterm git-credential: git sent no repository path, so the App token is skipped. Set credential.useHttpPath=true for https://github.com/coilyco")
		}
		return 0
	}
	token, err := mint(repo)
	if err != nil {
		fmt.Fprintf(stderr, "aterm git-credential: no App token for %s, leaving it to the next credential helper: %v\n", repo, err)
		return exitCodeFor(err)
	}
	// x-access-token is the username GitHub takes for an installation token.
	fmt.Fprintf(stdout, "username=x-access-token\npassword=%s\n", token)
	return 0
}
