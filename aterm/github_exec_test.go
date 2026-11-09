package main

import (
	"bytes"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGhRepoFromArgsTakesTheLastRepoFlagOrTheAPIPath(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"pr", "view", "1", "--repo", "coilyco/a"}, "coilyco/a"},
		{[]string{"pr", "view", "--repo=coilyco/a", "1"}, "coilyco/a"},
		{[]string{"pr", "view", "--repo", "coilyco/a", "--repo", "coilyco/b"}, "coilyco/b"},
		{[]string{"api", "repos/coilyco/a/pulls", "--jq", ".[]"}, "coilyco/a"},
		{[]string{"api", "/repos/coilyco/a"}, "coilyco/a"},
		{[]string{"api", "user"}, ""},
		{[]string{"repo", "list", "coilyco"}, ""},
		{[]string{"pr", "view", "--repo"}, ""},
		{nil, ""},
	}
	for _, c := range cases {
		if got := ghRepoFromArgs(c.args); got != c.want {
			t.Errorf("%v = %q, want %q", c.args, got, c.want)
		}
	}
}

func TestGhRepoFromRemotesNeedsOneGithubRepo(t *testing.T) {
	both := "forgejo\thttps://forgejo.coilysiren.me/coilyco/x.git (fetch)\ngithub\tgit@github.com:coilyco/x.git (fetch)\ngithub\tgit@github.com:coilyco/x.git (push)\n"
	if got := ghRepoFromRemotes(both); got != "coilyco/x" {
		t.Fatalf("got %q, want the github remote only", got)
	}
	if got := ghRepoFromRemotes("origin\thttps://github.com/coilyco/x (fetch)\nup\thttps://github.com/coilyco/y.git (fetch)\n"); got != "" {
		t.Fatalf("two repos should name none, got %q", got)
	}
	if got := ghRepoFromRemotes("origin\thttps://forgejo.coilysiren.me/coilyco/x.git (fetch)\n"); got != "" {
		t.Fatalf("a forgejo remote is not a github repo, got %q", got)
	}
}

// fakeGh stands in for the real gh: it records what it was given, and the
// marker file proves whether it ran at all.
func fakeGh(t *testing.T) (record string) {
	t.Helper()
	dir := t.TempDir()
	record = filepath.Join(dir, "ran")
	script := "#!/bin/sh\n" +
		"{ echo \"args=$*\"; echo \"token=$GH_TOKEN\"; echo \"github=$GITHUB_TOKEN\"; echo \"cfg=$GH_CONFIG_DIR\";\n" +
		"  ls -A \"$GH_CONFIG_DIR\" | wc -l | tr -d ' ' | sed 's/^/cfgfiles=/'; } > '" + record + "'\n" +
		"exit 3\n"
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return record
}

func brokerDaemon(t *testing.T) {
	t.Helper()
	key, encoded := testApp(t)
	fake := &fakeGithub{t: t, pub: &key.PublicKey, expires: time.Now().Add(time.Hour)}
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	t.Setenv(githubAPIEnv, server.URL)
	t.Setenv(githubAppIDEnv, "12345")
	t.Setenv(githubAppKeyEnv, encoded)
	testDaemon(t)
	seat := dialTest(t)
	seat.spawn("eng-platform-beetle-ox", "eng-platform", "Beetle-Ox", `echo TOKEN=$ATERM_SESSION_TOKEN; sleep 30`)
	t.Setenv(sessionTokenEnv, seat.token())
}

func TestGhExecRunsGhOnTheBrokerTokenInAnEmptyConfigDir(t *testing.T) {
	record := fakeGh(t)
	brokerDaemon(t)
	t.Setenv("GH_TOKEN", "host-login")
	t.Setenv("GITHUB_TOKEN", "host-login-2")
	t.Setenv("GH_CONFIG_DIR", "/the/host/config")
	var stdout, stderr bytes.Buffer

	code := runGhExec([]string{"pr", "view", "1", "--repo", "coilyco/bluesky-mcp"}, &stdout, &stderr)

	if code != 3 {
		t.Fatalf("exit = %d, want gh's own 3 passed through. stderr: %s", code, stderr.String())
	}
	ran, err := os.ReadFile(record)
	if err != nil {
		t.Fatalf("gh never ran: %v", err)
	}
	for _, want := range []string{"args=pr view 1 --repo coilyco/bluesky-mcp", "token=ghs_fake", "github=\n", "cfgfiles=0"} {
		if !strings.Contains(string(ran), want) {
			t.Errorf("gh saw %q, want it to contain %q", ran, want)
		}
	}
	if strings.Contains(string(ran), "host-login") || strings.Contains(string(ran), "/the/host/config") {
		t.Errorf("a host credential reached gh: %s", ran)
	}
	if _, err := os.Stat(strings.TrimPrefix(strings.Split(string(ran), "\n")[3], "cfg=")); err == nil {
		t.Errorf("the private config dir outlived the call")
	}
}

func TestGhExecRefusesWithoutRunningGhAndNeverFallsBack(t *testing.T) {
	record := fakeGh(t)
	brokerDaemon(t)
	t.Chdir(t.TempDir())
	cases := map[string][]string{
		"a write naming no repo outside a checkout": {"pr", "merge", "1"},
		"a write on another owner":                  {"pr", "merge", "1", "--repo", "coilysiren/website"},
		"a non-read verb on another owner":          {"workflow", "run", "x", "--repo", "other/x"},
	}
	for name, args := range cases {
		var stdout, stderr bytes.Buffer
		if code := runGhExec(args, &stdout, &stderr); code == 0 || code == 3 {
			t.Errorf("%s: exit %d, want a refusal", name, code)
		}
		if _, err := os.Stat(record); err == nil {
			t.Errorf("%s: gh ran", name)
		}
	}

	t.Setenv(sessionTokenEnv, "")
	var stdout, stderr bytes.Buffer
	code := runGhExec([]string{"pr", "view", "1", "--repo", "coilyco/bluesky-mcp"}, &stdout, &stderr)
	if code == 0 || code == 3 || !strings.Contains(stderr.String(), "not falling back to the host login") {
		t.Fatalf("exit %d, stderr %q: a broker failure should refuse and say so", code, stderr.String())
	}
	if _, err := os.Stat(record); err == nil {
		t.Fatal("gh ran after the broker failed")
	}
}

func TestGhExecRunsReadVerbsAnonymouslyOutsideCoilyco(t *testing.T) {
	record := fakeGh(t)
	brokerDaemon(t)
	t.Setenv("GH_TOKEN", "host-login")
	t.Chdir(t.TempDir())
	for _, args := range [][]string{
		{"pr", "view", "1", "--repo", "coilysiren/website"},
		{"run", "view", "9", "--repo", "other/x", "--log-failed"},
		{"api", "repos/cli/cli", "--jq", ".name"},
		{"repo", "list", "coilysiren"},
	} {
		os.Remove(record)
		var stdout, stderr bytes.Buffer
		if code := runGhExec(args, &stdout, &stderr); code != 3 {
			t.Fatalf("%v: exit %d, want gh's own 3. stderr: %s", args, code, stderr.String())
		}
		ran, err := os.ReadFile(record)
		if err != nil {
			t.Fatalf("%v: gh never ran: %v", args, err)
		}
		if !strings.Contains(string(ran), "token=\n") || !strings.Contains(string(ran), "cfgfiles=0") || strings.Contains(string(ran), "host-login") {
			t.Errorf("%v: anonymous run saw a credential or a populated config: %s", args, ran)
		}
	}
}

func TestGhReadVerbIsAnAllowlistOfLeadingWords(t *testing.T) {
	for args, want := range map[string]bool{
		"pr view 1": true, "pr list": true, "run view 1 --log-failed": true, "search repos x": true, "api repos/a/b": true,
		"pr merge 1": false, "pr create": false, "release create v1": false, "workflow run x": false, "repo delete x": false, "auth token": false, "": false,
	} {
		if got := ghReadVerb(strings.Fields(args)); got != want {
			t.Errorf("%q = %v, want %v", args, got, want)
		}
	}
}
