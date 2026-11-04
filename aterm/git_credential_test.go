package main

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func runCredential(t *testing.T, op, input string, mint func(string) (string, error)) (code int, out, errOut string) {
	t.Helper()
	var stdout, stderr strings.Builder
	code = runGitCredential([]string{op}, strings.NewReader(input), &stdout, &stderr, mint)
	return code, stdout.String(), stderr.String()
}

func TestCredentialHelperAnswersACoilycoRepoWithTheMintedToken(t *testing.T) {
	var asked []string
	mint := func(repo string) (string, error) {
		asked = append(asked, repo)
		return "ghs_minted", nil
	}
	for _, path := range []string{"coilyco/treehouse-servers.git", "coilyco/treehouse-servers", "/coilyco/treehouse-servers.git/"} {
		code, out, errOut := runCredential(t, "get", "protocol=https\nhost=github.com\npath="+path+"\n\n", mint)
		if code != 0 || out != "username=x-access-token\npassword=ghs_minted\n" || errOut != "" {
			t.Fatalf("path %q: code %d out %q err %q", path, code, out, errOut)
		}
	}
	for _, repo := range asked {
		if repo != "coilyco/treehouse-servers" {
			t.Fatalf("minted for %q, want the repo without .git", repo)
		}
	}
}

func TestCredentialHelperLeavesEveryOtherRequestToTheNextHelper(t *testing.T) {
	mint := func(repo string) (string, error) {
		t.Fatalf("minted for %q, which is not a coilyco github repo", repo)
		return "", nil
	}
	for name, input := range map[string]string{
		"another owner":  "protocol=https\nhost=github.com\npath=coilysiren/website.git\n\n",
		"another host":   "protocol=https\nhost=gist.github.com\npath=coilyco/x.git\n\n",
		"not https":      "protocol=ssh\nhost=github.com\npath=coilyco/x.git\n\n",
		"owner only":     "protocol=https\nhost=github.com\npath=coilyco\n\n",
		"nested path":    "protocol=https\nhost=github.com\npath=coilyco/a/b.git\n\n",
		"empty request":  "\n",
		"forgejo is out": "protocol=https\nhost=forgejo.example\npath=coilyco/x.git\n\n",
	} {
		if code, out, _ := runCredential(t, "get", input, mint); code != 0 || out != "" {
			t.Fatalf("%s: code %d out %q, want nothing and exit 0", name, code, out)
		}
	}
}

func TestCredentialHelperSaysSoWhenGitSendsNoPath(t *testing.T) {
	mint := func(string) (string, error) { t.Fatal("minted without a repo"); return "", nil }
	code, out, errOut := runCredential(t, "get", "protocol=https\nhost=github.com\n\n", mint)
	if code != 0 || out != "" || !strings.Contains(errOut, "credential.useHttpPath") {
		t.Fatalf("code %d out %q err %q, want a useHttpPath hint and no credential", code, out, errOut)
	}
}

func TestCredentialHelperFailsLoudlyWithoutPrintingACredential(t *testing.T) {
	mint := func(string) (string, error) { return "", errors.New("GitHub answered 422") }
	code, out, errOut := runCredential(t, "get", "protocol=https\nhost=github.com\npath=coilyco/x.git\n\n", mint)
	if code == 0 || out != "" || !strings.Contains(errOut, "next credential helper") || !strings.Contains(errOut, "422") {
		t.Fatalf("code %d out %q err %q", code, out, errOut)
	}
}

func TestCredentialHelperStoreAndEraseDoNothing(t *testing.T) {
	mint := func(string) (string, error) { t.Fatal("minted on store or erase"); return "", nil }
	for _, op := range []string{"store", "erase"} {
		if code, out, errOut := runCredential(t, op, "protocol=https\nhost=github.com\npath=coilyco/x.git\nusername=u\npassword=p\n\n", mint); code != 0 || out != "" || errOut != "" {
			t.Fatalf("%s: code %d out %q err %q", op, code, out, errOut)
		}
	}
	var stderr strings.Builder
	if code := runGitCredential(nil, strings.NewReader(""), &strings.Builder{}, &stderr, mint); code != exitUsage {
		t.Fatalf("no operation: code %d, want exitUsage", code)
	}
}

func TestBrokerAsksForWorkflowsOnlyWhenToldAndCachesThemApart(t *testing.T) {
	key, _ := testApp(t)
	now := time.Now()
	fake := &fakeGithub{t: t, pub: &key.PublicKey, expires: now.Add(time.Hour)}
	broker := testBroker(t, fake, key, &now)

	if _, _, err := broker.token(t.Context(), "coilyco/bluesky-mcp", false); err != nil {
		t.Fatalf("plain mint: %v", err)
	}
	if perms, _ := fake.lastBody["permissions"].(map[string]any); perms["workflows"] != nil {
		t.Fatalf("a plain mint asked for workflows: %v", perms)
	}
	if _, _, err := broker.token(t.Context(), "coilyco/bluesky-mcp", true); err != nil {
		t.Fatalf("workflows mint: %v", err)
	}
	perms, _ := fake.lastBody["permissions"].(map[string]any)
	if perms["workflows"] != "write" || perms["contents"] != "write" || len(perms) != 4 {
		t.Fatalf("permissions = %v, want the default three plus workflows=write", perms)
	}
	if fake.mints != 2 {
		t.Fatalf("mints = %d, want the workflows token not served from the plain one's cache", fake.mints)
	}
	if len(defaultGithubPermissions) != 3 {
		t.Fatalf("the workflows request leaked into the shared default: %v", defaultGithubPermissions)
	}
	if _, _, err := broker.token(t.Context(), "coilyco/bluesky-mcp", true); err != nil || fake.mints != 2 {
		t.Fatalf("a repeat workflows mint should hit the cache: %v mints %d", err, fake.mints)
	}
}
