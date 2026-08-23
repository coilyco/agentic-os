package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func writeIssuePinConfig(t *testing.T, path, baseURL string, failClosed bool, maxBytes int, freshness string) {
	t.Helper()
	body := "roles:\n" +
		"  exec:\n" +
		"    forgejo:\n" +
		"      base_url: " + baseURL + "\n" +
		"      owner: owner\n" +
		"      repo: inbox\n" +
		"      collection: pinned\n" +
		"      freshness: " + freshness + "\n" +
		"      max_bytes: " + strconv.Itoa(maxBytes) + "\n"
	if failClosed {
		body += "      fail_closed: true\n"
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestPrepareIssuePinLaunchContextPreservesPinsAndExcludesToken(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		gotAuth = request.Header.Get("Authorization")
		if request.URL.Path != "/api/v1/repos/owner/inbox/issues/pinned" {
			t.Fatalf("path = %s", request.URL.Path)
		}
		issues := []issuePinIssue{
			{
				Number: 302, Title: "Campaign", HTMLURL: serverIssueURL("302"),
				UpdatedAt: time.Date(2026, 8, 6, 1, 0, 0, 0, time.UTC),
				Body:      "campaign body\nwith exact lines",
			},
			{
				Number: 303, Title: "Product", HTMLURL: serverIssueURL("303"),
				UpdatedAt: time.Date(2026, 8, 6, 2, 0, 0, 0, time.UTC),
				Body:      "product body",
			},
		}
		if err := json.NewEncoder(response).Encode(issues); err != nil {
			t.Fatal(err)
		}
	}))
	defer server.Close()
	config := filepath.Join(t.TempDir(), "issue-pin-context.yaml")
	writeIssuePinConfig(t, config, server.URL, false, 4096, "5m")
	t.Setenv("AOS_ISSUE_PIN_CONTEXT", config)
	t.Setenv("AOS_ISSUE_PIN_CACHE_DIR", t.TempDir())
	t.Setenv("FORGEJO_TOKEN", "secret-token")

	launchContext, err := prepareIssuePinLaunchContext(context.Background(), "exec")
	if err != nil {
		t.Fatal(err)
	}
	defer launchContext.Close()
	data, err := os.ReadFile(launchContext.HostPath)
	if err != nil {
		t.Fatal(err)
	}
	rendered := string(data)
	for _, want := range []string{
		"## Pin 1: #302 Campaign",
		"campaign body\nwith exact lines",
		"## Pin 2: #303 Product",
		"Snapshot digest: " + launchContext.Digest,
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered issue pins missing %q:\n%s", want, rendered)
		}
	}
	if strings.Index(rendered, "#302") > strings.Index(rendered, "#303") {
		t.Fatalf("pin order changed:\n%s", rendered)
	}
	if strings.Contains(rendered, "secret-token") {
		t.Fatal("rendered issue pins exposed the Forgejo token")
	}
	if gotAuth != "token secret-token" {
		t.Fatalf("authorization header = %q", gotAuth)
	}
}

func TestIssuePinHydratorCacheFreshnessStaleFallbackAndFailClosed(t *testing.T) {
	now := time.Date(2026, 8, 6, 1, 0, 0, 0, time.UTC)
	status := http.StatusOK
	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		hits++
		response.WriteHeader(status)
		if status == http.StatusOK {
			_ = json.NewEncoder(response).Encode([]issuePinIssue{{
				Number: 302, Title: "Campaign", HTMLURL: serverIssueURL("302"),
				UpdatedAt: now, Body: "cached body",
			}})
		}
	}))
	defer server.Close()
	config := resolvedIssuePinConfig{
		Role: "tpm", BaseURL: server.URL, Owner: "owner", Repo: "inbox",
		MaxBytes: 4096, Freshness: time.Hour,
	}
	hydrator := issuePinHydrator{
		now:      func() time.Time { return now },
		client:   server.Client(),
		cacheDir: t.TempDir(),
	}
	first, err := hydrator.hydrate(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	if hits != 1 || !strings.Contains(first.Markdown, "cached body") {
		t.Fatalf("initial hydrate hits=%d rendered=%s", hits, first.Markdown)
	}
	hydrator.now = func() time.Time { return now.Add(30 * time.Minute) }
	status = http.StatusInternalServerError
	fresh, err := hydrator.hydrate(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	if hits != 1 || strings.Contains(fresh.Markdown, "Stale: true") {
		t.Fatalf("fresh cache was not reused hits=%d rendered=%s", hits, fresh.Markdown)
	}
	hydrator.now = func() time.Time { return now.Add(2 * time.Hour) }
	stale, err := hydrator.hydrate(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	if hits != 2 || !strings.Contains(stale.Markdown, "Stale: true") {
		t.Fatalf("stale fallback missing hits=%d rendered=%s", hits, stale.Markdown)
	}
	config.FailClosed = true
	if _, err := hydrator.hydrate(context.Background(), config); err == nil {
		t.Fatal("fail-closed stale refresh succeeded")
	}
}

func TestIssuePinHydratorClipsBodiesAndSkipsRoleMismatch(t *testing.T) {
	snapshot := issuePinSnapshot{
		Format: issuePinSnapshotFormat, Role: "tpm", BaseURL: "https://forgejo.example.test",
		Owner: "owner", Repo: "inbox", HydratedAt: time.Date(2026, 8, 6, 1, 0, 0, 0, time.UTC),
		Issues: []issuePinIssue{{
			Number: 302, Title: "Campaign", HTMLURL: serverIssueURL("302"),
			UpdatedAt: time.Date(2026, 8, 6, 1, 0, 0, 0, time.UTC),
			Body:      strings.Repeat("body ", 100),
		}},
	}
	snapshot.Digest = digestIssuePinSnapshot(snapshot)
	rendered := renderIssuePinSnapshot(snapshot, 220, false, snapshot.HydratedAt, nil)
	if !strings.Contains(rendered.Markdown, "[clipped: context cap exceeded]") {
		t.Fatalf("clipped marker missing:\n%s", rendered.Markdown)
	}
	config := filepath.Join(t.TempDir(), "issue-pin-context.yaml")
	writeIssuePinConfig(t, config, "https://forgejo.example.test", false, 4096, "5m")
	t.Setenv("AOS_ISSUE_PIN_CONTEXT", config)
	if _, ok, err := loadIssuePinConfigForRole("platform"); err != nil || ok {
		t.Fatalf("role mismatch config = ok %v err %v", ok, err)
	}
}

func TestStageHydratedIssuePinContextAppendsSelectedInstruction(t *testing.T) {
	home := t.TempDir()
	source := filepath.Join(t.TempDir(), "issue-pins.md")
	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".codex", "AGENTS.md"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("exact issue body\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	digest := strings.Repeat("a", 64)
	if err := stageHydratedIssuePinContext("codex", home, source, digest); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(home, ".codex", "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	rendered := string(data)
	for _, want := range []string{"base", "Snapshot digest: " + digest, "exact issue body"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("instruction missing %q:\n%s", want, rendered)
		}
	}
	if err := validateStagedHome(home, "codex"); err != nil {
		t.Fatal(err)
	}
}

func serverIssueURL(id string) string {
	return "https://forgejo.example.test/owner/inbox/issues/" + id
}
