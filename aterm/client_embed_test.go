package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"testing/fstest"
)

func serveClientAt(t *testing.T, d *daemon, path string) (int, string) {
	t.Helper()
	recorder := httptest.NewRecorder()
	d.serveClient(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	body, _ := io.ReadAll(recorder.Result().Body)
	return recorder.Code, string(body)
}

func TestClientComesFromTheBinaryUnlessADirIsNamed(t *testing.T) {
	d := newDaemon(func(string, ...any) {})
	d.clientFS = fstest.MapFS{"index.html": {Data: []byte("embedded page")}}

	if code, body := serveClientAt(t, d, "/"); code != http.StatusOK || body != "embedded page" {
		t.Fatalf("embedded client: %d %q", code, body)
	}

	d.clientDir = t.TempDir()
	if err := os.WriteFile(filepath.Join(d.clientDir, "index.html"), []byte("dir page"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, body := serveClientAt(t, d, "/"); body != "dir page" {
		t.Fatalf("a named dir must win over the embedded client, got %q", body)
	}

	d.clientDir = filepath.Join(d.clientDir, "gone")
	if code, _ := serveClientAt(t, d, "/"); code != http.StatusNotFound {
		t.Fatalf("a named dir that is missing must not fall back to the embedded client: %d", code)
	}
}

func TestClientIsNotFoundWhenTheBuildEmbedsNone(t *testing.T) {
	d := newDaemon(func(string, ...any) {})
	d.clientFS = nil
	if code, body := serveClientAt(t, d, "/"); code != http.StatusNotFound || body == "" {
		t.Fatalf("no client: %d %q", code, body)
	}
}

func TestDefaultClientDirIsOnlyTheEnvironmentOverride(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv(clientDirEnv, "")
	if dir := defaultClientDir(); dir != "" {
		t.Fatalf("no override must mean the embedded client, got %q", dir)
	}
	t.Setenv(clientDirEnv, " /srv/client ")
	if dir := defaultClientDir(); dir != "/srv/client" {
		t.Fatalf("override: %q", dir)
	}
}

// The release gate sets ATERM_REQUIRE_EMBEDDED_CLIENT, so an empty embed fails
// there and only skips in a plain `go test`.
func TestServedIndexIsTheEmbeddedBuild(t *testing.T) {
	d := newDaemon(func(string, ...any) {})
	if d.clientFS == nil {
		if os.Getenv("ATERM_REQUIRE_EMBEDDED_CLIENT") != "" {
			t.Fatal("this build embeds no client: run just aterm-client-embed before the Go build")
		}
		t.Skip("no client embedded; run just aterm-client-embed to exercise this")
	}

	code, served := serveClientAt(t, d, "/")
	if code != http.StatusOK {
		t.Fatalf("GET /: %d", code)
	}
	embedded, err := os.ReadFile(filepath.Join("clientdist", "index.html"))
	if err != nil || string(embedded) != served {
		t.Fatalf("served index differs from clientdist/index.html (read err %v)", err)
	}

	built, err := os.ReadFile(filepath.Join("..", "aterm-client", "dist", "index.html"))
	switch {
	case err == nil && string(built) != served:
		t.Fatal("served index differs from aterm-client/dist/index.html, so the embedded client is not the latest build")
	case err != nil && os.Getenv("ATERM_REQUIRE_EMBEDDED_CLIENT") != "":
		t.Fatalf("the release gate needs aterm-client/dist to compare against: %v", err)
	}

	// The page must name a bundle the daemon can actually serve.
	script := regexp.MustCompile(`src="(/assets/[^"]+\.js)"`).FindStringSubmatch(served)
	if script == nil {
		t.Fatal("index.html names no /assets script")
	}
	if code, body := serveClientAt(t, d, script[1]); code != http.StatusOK || body == "" {
		t.Fatalf("GET %s: %d, %d bytes", script[1], code, len(body))
	}
}
