package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// handlePagesToken grants a publish token for a daemon proof, the way accounts does.
func (a *fakeAccounts) handlePagesToken(w http.ResponseWriter, r *http.Request) {
	var req struct{ Cert, Proof string }
	if err := decodeStrict(r, &req); err != nil {
		http.Error(w, `{"error":"bad_request"}`, http.StatusBadRequest)
		return
	}
	if _, ok := provenJkt(req.Cert, req.Proof); !ok {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"token": "publish-token", "scope": "publish", "expires_in": 600})
}

// fakePages is the pages API: it takes only the token accounts granted, and keeps what it
// was sent by page id and file path.
type fakePages struct {
	server *httptest.Server
	mu     sync.Mutex
	pages  map[string]map[string]string
	titles map[string]string
}

func newFakePages(t *testing.T) *fakePages {
	p := &fakePages{pages: map[string]map[string]string{}, titles: map[string]string{}}
	p.server = httptest.NewServer(http.HandlerFunc(p.handle))
	t.Cleanup(p.server.Close)
	return p
}

func (p *fakePages) handle(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer publish-token" {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	id, _ := strings.CutPrefix(r.URL.Path, "/v1/pages/")
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/v1/pages":
		var items []pageItem
		for page, files := range p.pages {
			items = append(items, pageItem{Page: page, Title: p.titles[page], Files: len(files)})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"pages": items})
	case r.Method == http.MethodDelete:
		if p.pages[id] == nil {
			http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
			return
		}
		delete(p.pages, id)
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodPost || r.Method == http.MethodPut:
		page := "pg_abcdefghijklmnopqrst"
		if r.Method == http.MethodPut {
			if p.pages[id] == nil {
				http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
				return
			}
			page = id
		}
		mr, err := r.MultipartReader()
		if err != nil {
			http.Error(w, `{"error":"bad_request"}`, http.StatusBadRequest)
			return
		}
		files := map[string]string{}
		for {
			part, err := mr.NextPart()
			if err != nil {
				break
			}
			_, params, _ := mime.ParseMediaType(part.Header.Get("Content-Disposition"))
			body, _ := io.ReadAll(part)
			if name, ok := params["filename"]; ok {
				files[name] = string(body)
			} else if params["name"] == "title" {
				p.titles[page] = string(body)
			}
		}
		p.pages[page] = files
		_ = json.NewEncoder(w).Encode(pageItem{Page: page, Title: p.titles[page], Files: len(files)})
	default:
		http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
	}
}

// pagesRig is an artifact rig whose daemon publishes to a fake pages API, with a
// certificate from a fake accounts.
func pagesRig(t *testing.T) (*artifactRig, *fakePages) {
	t.Helper()
	r := newArtifactRig(t)
	a := newFakeAccounts(t)
	pages := newFakePages(t)
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	link := &brokerLink{key: key, cert: a.daemonCert(jwkThumbprint(&key.PublicKey), time.Hour), post: accountsPoster(a.server.URL)}
	client, err := newPagesClient(pages.server.URL, link)
	if err != nil {
		t.Fatal(err)
	}
	r.d.pages = client
	return r, pages
}

func TestPublishPageSendsAFileOrADirectoryAsTheDaemon(t *testing.T) {
	r, pages := pagesRig(t)
	s, cwd := r.seat("eng-platform-test")

	single := filepath.Join(cwd, "answer.html")
	if err := os.WriteFile(single, []byte("<h1>one</h1>"), 0o600); err != nil {
		t.Fatal(err)
	}
	reply, err := r.call(s, frame{Type: "page_publish", Title: "One", Path: single})
	if err != nil || reply.Type != "page_published" || reply.Page == nil || reply.Page.Page != "pg_abcdefghijklmnopqrst" {
		t.Fatalf("publish a file: %+v %v", reply, err)
	}
	if got := pages.pages["pg_abcdefghijklmnopqrst"]; got["index.html"] != "<h1>one</h1>" || len(got) != 1 {
		t.Fatalf("a single file is sent as index.html: %v", got)
	}

	dir := filepath.Join(cwd, "site")
	for path, body := range map[string]string{"index.html": "<p>i</p>", "assets/app.js": "1", ".git/HEAD": "x", ".env": "SECRET=1"} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, path)), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, path), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	reply, err = r.call(s, frame{Type: "page_publish", Title: "Site", Path: dir, PageID: "pg_abcdefghijklmnopqrst"})
	if err != nil || reply.Page.Files != 2 {
		t.Fatalf("replace with a directory: %+v %v", reply, err)
	}
	got := pages.pages["pg_abcdefghijklmnopqrst"]
	if got["assets/app.js"] != "1" || got["index.html"] != "<p>i</p>" || len(got) != 2 {
		t.Fatalf("dot files stay home and paths keep their directories: %v", got)
	}

	listed, err := r.call(s, frame{Type: "page_list"})
	if err != nil || len(listed.Pages) != 1 || listed.Pages[0].Title != "Site" {
		t.Fatalf("list: %+v %v", listed, err)
	}
	if _, err := r.call(s, frame{Type: "page_remove", PageID: "pg_abcdefghijklmnopqrst"}); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if len(pages.pages) != 0 {
		t.Fatalf("remove left %v", pages.pages)
	}
}

func TestPublishPageRefusesWhatShouldNotLeaveTheHost(t *testing.T) {
	r, pages := pagesRig(t)
	s, cwd := r.seat("eng-platform-test")
	write := func(name, body string) string {
		path := filepath.Join(cwd, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	outside := filepath.Join(t.TempDir(), "elsewhere.html")
	if err := os.WriteFile(outside, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(cwd, "linked")
	write("linked/index.html", "<p>")
	if err := os.Symlink(outside, filepath.Join(linked, "leak.html")); err != nil {
		t.Fatal(err)
	}
	cases := map[string]struct {
		path, reason string
	}{
		"outside the roots": {outside, reasonBadPath},
		"a symlink inside":  {linked, reasonBadPath},
		"no index":          {filepath.Dir(write("noindex/other.html", "x")), reasonBadPath},
		"not html":          {write("notes.txt", "x"), reasonBadPath},
		"a session token":   {write("leak/index.html", "token "+s.token), reasonSecretInPage},
		"over the byte cap": {write("big.html", strings.Repeat("a", maxPageBytes+1)), reasonTooLarge},
		"a bad file name":   {filepath.Dir(write("odd/sp ace.html", "x")), reasonBadPath},
	}
	write("odd/index.html", "x")
	for name, c := range cases {
		reply, err := r.call(s, frame{Type: "page_publish", Title: "T", Path: c.path})
		if err == nil || reply.Reason != c.reason {
			t.Errorf("%s: %+v %v, want reason %s", name, reply, err, c.reason)
		}
	}
	if len(pages.pages) != 0 {
		t.Fatalf("a refused page reached the host: %v", pages.pages)
	}
}

func TestPagesNeedAConfiguredHostAndALiveSeat(t *testing.T) {
	r := newArtifactRig(t)
	s, cwd := r.seat("eng-platform-test")
	path := filepath.Join(cwd, "p.html")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if reply, err := r.call(s, frame{Type: "page_publish", Title: "T", Path: path}); err == nil || reply.Reason != reasonNoPages {
		t.Fatalf("no pages host: %+v %v", reply, err)
	}
	c := pipeClient(t, r.d, peerStanding{pids: []int{os.Getpid()}})
	welcome := nextFrame(t, c, "welcome")
	for _, f := range welcome.Features {
		if f == pagesFeature {
			t.Fatal("a daemon with no pages host advertises pages")
		}
	}
	if _, err := c.request(frame{Type: "page_list", Token: "not-a-token"}); err == nil {
		t.Fatal("a request with no live seat's token was answered")
	}
	if _, err := newPagesClient("http://pages.example", nil); err == nil {
		t.Fatal("a plain-http pages host was accepted")
	}
}
