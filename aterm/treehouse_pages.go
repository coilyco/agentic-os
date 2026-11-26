package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// pagesFeature is in welcome when this daemon can publish to a Treehouse pages host.
const pagesFeature = "pages"

// The pages host's own caps, checked here first so a seat hears which file broke one
// before anything is sent.
const (
	maxPageBytes = 8 << 20
	maxPageFiles = 64
)

const (
	reasonNoPages      = "no_pages"
	reasonSecretInPage = "secret_in_page"
	reasonPagesRefused = "pages_refused"
)

var pagePathPattern = regexp.MustCompile(`^[A-Za-z0-9_-][A-Za-z0-9._-]*(/[A-Za-z0-9_-][A-Za-z0-9._-]*)*$`)

// pageItem is one page as the pages host lists it, treehouse.pages.publish.v1.
type pageItem struct {
	Page    string `json:"page"`
	Title   string `json:"title"`
	Files   int    `json:"files"`
	Bytes   int64  `json:"bytes"`
	Created string `json:"created"`
	Updated string `json:"updated"`
}

type pageFile struct {
	path string
	data []byte
}

// pagesClient speaks to the pages host as this daemon, with a publish token accounts
// mints for the daemon's certificate and a fresh proof. A seat never holds the token.
type pagesClient struct {
	base string
	link *brokerLink
	http *http.Client

	mu      sync.Mutex
	token   string
	expires time.Time
}

func newPagesClient(base string, link *brokerLink) (*pagesClient, error) {
	base = strings.TrimRight(base, "/")
	if !strings.HasPrefix(base, "https://") && !isLoopbackURL(base) {
		return nil, errors.New("the pages host must be https")
	}
	return &pagesClient{base: base, link: link, http: &http.Client{Timeout: 2 * time.Minute}}, nil
}

// publishToken returns a cached token, or trades the certificate for a new one a minute
// before the cached one lapses.
func (p *pagesClient) publishToken(ctx context.Context) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.token != "" && time.Until(p.expires) > time.Minute {
		return p.token, nil
	}
	proof, err := signDaemonProof(p.link.key, "treehouse-accounts", "", time.Now())
	if err != nil {
		return "", err
	}
	var grant struct {
		Token     string `json:"token"`
		Scope     string `json:"scope"`
		ExpiresIn int    `json:"expires_in"`
	}
	if err := p.link.post(ctx, "/v1/pages/token", map[string]string{"cert": p.link.currentCert(), "proof": proof}, &grant); err != nil {
		return "", withReason(reasonPagesRefused, err)
	}
	if grant.Scope != "publish" || grant.Token == "" {
		return "", withReason(reasonPagesRefused, errors.New("accounts did not grant a publish token"))
	}
	p.token, p.expires = grant.Token, time.Now().Add(time.Duration(grant.ExpiresIn)*time.Second)
	return p.token, nil
}

func (p *pagesClient) do(ctx context.Context, method, path, ctype string, body io.Reader, out any) error {
	token, err := p.publishToken(ctx)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, p.base+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	resp, err := p.http.Do(req)
	if err != nil {
		return withReason(reasonPagesRefused, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode/100 != 2 {
		var e struct{ Error, Message string }
		_ = json.Unmarshal(data, &e)
		reason := reasonPagesRefused
		if e.Error == "too_large" || e.Error == "quota" {
			reason = reasonTooLarge
		}
		return withReason(reason, fmt.Errorf("the pages host answered %d %s: %s", resp.StatusCode, e.Error, e.Message))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(data, out)
}

// publish creates a page, or replaces every file of page when it is set.
func (p *pagesClient) publish(ctx context.Context, title, page string, files []pageFile) (pageItem, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if err := mw.WriteField("title", title); err != nil {
		return pageItem{}, err
	}
	for _, f := range files {
		h := textproto.MIMEHeader{}
		h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, f.path))
		h.Set("Content-Type", "application/octet-stream")
		w, err := mw.CreatePart(h)
		if err != nil {
			return pageItem{}, err
		}
		if _, err := w.Write(f.data); err != nil {
			return pageItem{}, err
		}
	}
	if err := mw.Close(); err != nil {
		return pageItem{}, err
	}
	method, path := http.MethodPost, "/v1/pages"
	if page != "" {
		method, path = http.MethodPut, "/v1/pages/"+page
	}
	var item pageItem
	err := p.do(ctx, method, path, mw.FormDataContentType(), &buf, &item)
	return item, err
}

func (p *pagesClient) list(ctx context.Context) ([]pageItem, error) {
	var reply struct {
		Pages []pageItem `json:"pages"`
	}
	err := p.do(ctx, http.MethodGet, "/v1/pages", "", nil, &reply)
	return reply.Pages, err
}

func (p *pagesClient) remove(ctx context.Context, page string) error {
	return p.do(ctx, http.MethodDelete, "/v1/pages/"+page, "", nil, nil)
}

// readPageFiles reads one HTML file, sent as index.html, or a directory with index.html,
// from the seat's roots. Dot files stay home and a symlink is refused.
func readPageFiles(s *ptySession, path string) ([]pageFile, error) {
	if !filepath.IsAbs(path) {
		return nil, withReason(reasonBadPath, errors.New("the page path must be absolute"))
	}
	real, err := filepath.EvalSymlinks(path)
	roots := s.artifactRoots()
	inRoots := func(p string) bool {
		for _, root := range roots {
			if within(root, p) {
				return true
			}
		}
		return false
	}
	if err != nil || !inRoots(real) {
		return nil, withReason(reasonBadPath, errors.New("the page must be under this seat's working directory or scratchpad"))
	}
	info, err := os.Stat(real)
	if err != nil {
		return nil, withReason(reasonBadPath, errors.New("the page cannot be read"))
	}
	if info.Mode().IsRegular() {
		if ext := strings.ToLower(filepath.Ext(real)); ext != ".html" && ext != ".htm" {
			return nil, withReason(reasonBadPath, errors.New("a single-file page is an .html file"))
		}
		data, err := readCapped(real, maxPageBytes)
		if err != nil {
			return nil, err
		}
		return []pageFile{{path: "index.html", data: data}}, nil
	}
	var files []pageFile
	var total int64
	err = filepath.WalkDir(real, func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == real {
			return nil
		}
		if strings.HasPrefix(e.Name(), ".") {
			if e.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if e.Type()&fs.ModeSymlink != 0 {
			return withReason(reasonBadPath, fmt.Errorf("%s is a symlink", e.Name()))
		}
		if e.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(real, p)
		rel = filepath.ToSlash(rel)
		if len(rel) > 200 || !pagePathPattern.MatchString(rel) {
			return withReason(reasonBadPath, fmt.Errorf("%q is not a page path", rel))
		}
		if len(files) == maxPageFiles {
			return withReason(reasonTooLarge, fmt.Errorf("a page has at most %d files", maxPageFiles))
		}
		data, err := readCapped(p, maxPageBytes-total)
		if err != nil {
			return err
		}
		total += int64(len(data))
		files = append(files, pageFile{path: rel, data: data})
		return nil
	})
	if err != nil {
		if reasonFor(err) != "" {
			return nil, err
		}
		return nil, withReason(reasonBadPath, err)
	}
	for _, f := range files {
		if f.path == "index.html" {
			return files, nil
		}
	}
	return nil, withReason(reasonBadPath, errors.New("a page directory needs an index.html at its top"))
}

func readCapped(path string, room int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, withReason(reasonBadPath, errors.New("a page file cannot be read"))
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, room+1))
	if err != nil {
		return nil, withReason(reasonBadPath, errors.New("a page file cannot be read"))
	}
	if int64(len(data)) > room {
		return nil, withReason(reasonTooLarge, fmt.Errorf("a page is at most %d bytes", maxPageBytes))
	}
	return data, nil
}

// leaksToken says whether any file holds a live session token. A page is served to a
// browser, so a token pasted into one would leave the host.
func (d *daemon) leaksToken(files []pageFile) bool {
	d.mu.Lock()
	tokens := make([][]byte, 0, len(d.tokens))
	for token := range d.tokens {
		tokens = append(tokens, []byte(token))
	}
	d.mu.Unlock()
	for _, f := range files {
		for _, token := range tokens {
			if len(token) > 0 && bytes.Contains(f.data, token) {
				return true
			}
		}
	}
	return false
}

func (d *daemon) pagePublish(cl *client, message frame) error {
	s := d.byToken(message.Token)
	if s == nil {
		return withExit(exitUsage, errors.New("aterm page speaks for a session aterm launched, and this token names no live one"))
	}
	if d.pages == nil {
		return withReason(reasonNoPages, errors.New("this daemon has no Treehouse pages host. Start it with --treehouse-pages"))
	}
	if err := checkArtifactTitle(message.Title); err != nil {
		return err
	}
	if message.PageID != "" && !pageIDPattern.MatchString(message.PageID) {
		return withExit(exitUsage, errors.New("a page id is pg_ and 20 lowercase base32 characters"))
	}
	files, err := readPageFiles(s, message.Path)
	if err != nil {
		return err
	}
	if d.leaksToken(files) {
		return withReason(reasonSecretInPage, errors.New("a page file holds a session token, so it is not sent"))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	item, err := d.pages.publish(ctx, message.Title, message.PageID, files)
	if err != nil {
		return err
	}
	d.logf("%s published page %s, %d files", s.name, item.Page, item.Files)
	return cl.c.write(frame{Type: "page_published", ID: message.ID, Page: &item})
}

var pageIDPattern = regexp.MustCompile(`^pg_[a-z2-7]{20}$`)

func (d *daemon) pageCall(cl *client, message frame) error {
	if d.byToken(message.Token) == nil {
		return withExit(exitUsage, errors.New("aterm page speaks for a session aterm launched, and this token names no live one"))
	}
	if d.pages == nil {
		return withReason(reasonNoPages, errors.New("this daemon has no Treehouse pages host. Start it with --treehouse-pages"))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if message.Type == "page_list" {
		pages, err := d.pages.list(ctx)
		if err != nil {
			return err
		}
		if pages == nil {
			pages = []pageItem{}
		}
		return cl.c.write(frame{Type: "page_listed", ID: message.ID, Pages: pages})
	}
	if !pageIDPattern.MatchString(message.PageID) {
		return withExit(exitUsage, errors.New("a page id is pg_ and 20 lowercase base32 characters"))
	}
	if err := d.pages.remove(ctx, message.PageID); err != nil {
		return err
	}
	return cl.c.write(frame{Type: "page_removed", ID: message.ID, PageID: message.PageID})
}
