package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

var (
	mp4Head  = append([]byte("\x00\x00\x00\x18ftypisom\x00\x00\x02\x00"), bytes.Repeat([]byte{'v'}, 4096)...)
	webmHead = append([]byte("\x1a\x45\xdf\xa3\x93\x42\x82\x84webm"), bytes.Repeat([]byte{'w'}, 512)...)
)

// mediaRig is a daemon with a media root, a seat whose working directory is a real temp
// directory, and a local client that is under no session, so it may type.
type mediaRig struct {
	t    *testing.T
	d    *daemon
	seat *ptySession
	cwd  string
	root string
	log  func() string
}

func newMediaRig(t *testing.T) *mediaRig {
	t.Helper()
	d, _ := wsDaemon(t)
	dir, err := os.MkdirTemp("/tmp", "aterm-media-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	d.mediaRoot = filepath.Join(dir, "media")
	var mu sync.Mutex
	logged := &strings.Builder{}
	d.logf = func(format string, args ...any) {
		mu.Lock()
		defer mu.Unlock()
		fmt.Fprintf(logged, format+"\n", args...)
	}
	cwd := filepath.Join(dir, "work")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	s, err := startPTYSession(d, "scientist-evie", frame{
		Role: "scientist", Identity: "evie", Seat: "codex",
		Argv: []string{"/bin/sh", "-c", "cat"}, Env: []string{"PATH=" + os.Getenv("PATH")}, Cwd: cwd,
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	d.mu.Lock()
	d.sessions[s.name] = s
	d.tokens[s.token] = s
	d.mu.Unlock()
	t.Cleanup(func() {
		s.end()
		select {
		case <-s.forgotten:
		case <-time.After(10 * time.Second):
		}
	})
	return &mediaRig{t: t, d: d, seat: s, cwd: cwd, root: d.mediaRoot, log: func() string {
		mu.Lock()
		defer mu.Unlock()
		return logged.String()
	}}
}

// write puts a file in the seat's working directory.
func (r *mediaRig) write(name string, data []byte) string {
	r.t.Helper()
	path := filepath.Join(r.cwd, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		r.t.Fatal(err)
	}
	return path
}

// client is a connection that is under no session. It is welcomed, and the frame is read.
func (r *mediaRig) client(peer peerStanding) *conn {
	r.t.Helper()
	c := pipeClient(r.t, r.d, peer)
	nextFrame(r.t, c, "welcome")
	return c
}

func (r *mediaRig) watcher() *conn {
	r.t.Helper()
	c := r.client(peerStanding{pids: []int{os.Getpid()}})
	if _, err := c.request(frame{Type: "subscribe", Channel: "media"}); err != nil {
		r.t.Fatalf("subscribe: %v", err)
	}
	return c
}

// show asks as the seat, on a connection of its own, since request drops pushes.
func (r *mediaRig) show(path, title string) (frame, error) {
	r.t.Helper()
	return r.client(peerStanding{pids: []int{os.Getpid()}}).request(frame{Type: "media_show", Token: r.seat.token, Path: path, Title: title})
}

// read assembles a kept file in chunks of size, checking each carries size and eof.
func (r *mediaRig) read(c *conn, id string, chunk int) []byte {
	r.t.Helper()
	var got []byte
	for {
		reply, err := c.request(frame{Type: "media_read", MediaID: id, Offset: int64(len(got)), Length: chunk})
		if err != nil || reply.Type != "media_data" {
			r.t.Fatalf("media_read at %d: %+v, %v", len(got), reply, err)
		}
		if reply.Size == 0 || reply.EOF == nil {
			r.t.Fatalf("every chunk carries size and eof: %+v", reply)
		}
		got = append(got, reply.Data...)
		if *reply.EOF {
			if int64(len(got)) != reply.Size {
				r.t.Fatalf("eof at %d bytes, size says %d", len(got), reply.Size)
			}
			return got
		}
		if len(reply.Data) == 0 {
			r.t.Fatalf("a chunk with no data that is not the end: %+v", reply)
		}
	}
}

func TestShowReachesASubscriberAsImageVideoAndFileAndReadsBackTheSameHash(t *testing.T) {
	r := newMediaRig(t)
	watcher := r.watcher()
	reader := r.client(peerStanding{pids: []int{os.Getpid()}})
	cases := []struct {
		name, kind, mime string
		data             []byte
	}{
		{"frame.png", "image", "image/png", imageBytes(pngMagic, 5000)},
		{"clip.mp4", "video", "video/mp4", mp4Head},
		{"notes.bin", "file", "application/octet-stream", []byte("not any known type, but bytes")},
	}
	for _, c := range cases {
		reply, err := r.show(r.write(c.name, c.data), "")
		if err != nil || reply.Type != "media_shown" || reply.Delivered != 1 || reply.Media == nil {
			t.Fatalf("show %s: %+v, %v", c.name, reply, err)
		}
		pushed := nextFrame(t, watcher, "media")
		view := pushed.Media
		if pushed.Session != r.seat.name || view == nil || view.Kind != c.kind || view.Mime != c.mime || view.Name != c.name || view.Size != int64(len(c.data)) || view.At.IsZero() {
			t.Fatalf("%s should arrive as a %s %s named %s: %+v", c.name, c.kind, c.mime, c.name, pushed)
		}
		if view.ID != reply.MediaID {
			t.Fatalf("the push and the reply name one item: %q and %q", view.ID, reply.MediaID)
		}
		for _, chunk := range []int{0, 1000, len(c.data)} {
			if got := r.read(reader, view.ID, chunk); sha256.Sum256(got) != sha256.Sum256(c.data) {
				t.Fatalf("%s read back in chunks of %d is not the shared bytes", c.name, chunk)
			}
		}
	}
}

func TestMediaTypeComesFromTheBytesNotTheName(t *testing.T) {
	r := newMediaRig(t)
	for name, tc := range map[string]struct {
		data       []byte
		kind, mime string
	}{
		"a.txt":  {imageBytes(pngMagic, 64), "image", "image/png"},
		"b.png":  {[]byte("plain text named like a picture"), "file", "application/octet-stream"},
		"c.jpg":  {imageBytes([]byte("\xff\xd8\xff\xe0"), 64), "image", "image/jpeg"},
		"d.gif":  {imageBytes([]byte("GIF89a"), 64), "image", "image/gif"},
		"e.webp": {imageBytes([]byte("RIFF\x10\x00\x00\x00WEBPVP8 "), 64), "image", "image/webp"},
		"f.dat":  {webmHead, "video", "video/webm"},
		"g.mov":  {append([]byte("\x00\x00\x00\x14ftypqt  \x00\x00\x00\x00"), bytes.Repeat([]byte{'q'}, 64)...), "file", "application/octet-stream"},
		"h.heic": {append([]byte("\x00\x00\x00\x18ftypheic\x00\x00\x00\x00"), bytes.Repeat([]byte{'h'}, 64)...), "file", "application/octet-stream"},
	} {
		reply, err := r.show(r.write(name, tc.data), "")
		if err != nil || reply.Media.Kind != tc.kind || reply.Media.Mime != tc.mime {
			t.Errorf("%s should be %s %s: %+v, %v", name, tc.kind, tc.mime, reply.Media, err)
		}
	}
}

func TestShowRefusesWhatIsNotARegularFileUnderTheSeatsRoots(t *testing.T) {
	r := newMediaRig(t)
	outside := filepath.Join(filepath.Dir(r.cwd), "outside.png")
	if err := os.WriteFile(outside, imageBytes(pngMagic, 64), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(r.cwd, "link.png")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(r.cwd, "folder"), 0o755); err != nil {
		t.Fatal(err)
	}
	empty := r.write("empty.png", nil)
	big := filepath.Join(r.cwd, "big.bin")
	if err := os.WriteFile(big, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(big, maxMediaBytes+1); err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct{ path, reason string }{
		"outside the roots":        {outside, reasonBadPath},
		"a symlink out":            {filepath.Join(r.cwd, "link.png"), reasonBadPath},
		"a directory":              {filepath.Join(r.cwd, "folder"), reasonBadPath},
		"a relative path":          {"work/frame.png", reasonBadPath},
		"a file that is not there": {filepath.Join(r.cwd, "nothing.png"), reasonBadPath},
		"an empty file":            {empty, reasonBadPath},
		"over the file cap":        {big, reasonTooLarge},
	} {
		reply, err := r.show(tc.path, "")
		if err == nil || reply.Reason != tc.reason {
			t.Errorf("%s should be refused as %s: %+v, %v", name, tc.reason, reply, err)
		}
	}
	if entries, _ := os.ReadDir(r.root); len(entries) != 0 {
		t.Fatalf("a refused file must leave nothing behind: %v", entries)
	}
}

func TestShowCopiesTheBytesSoAnOverwrittenFileIsANewItem(t *testing.T) {
	r := newMediaRig(t)
	reader := r.client(peerStanding{pids: []int{os.Getpid()}})
	path := r.write("latest.jpg", imageBytes([]byte("\xff\xd8\xff\xe0"), 100))
	first, err := r.show(path, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, imageBytes([]byte("\xff\xd8\xff\xe0"), 200), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := r.read(reader, first.MediaID, 0); len(got) != 100 {
		t.Fatalf("the first frame must keep its own bytes after the file is overwritten: %d", len(got))
	}
	second, err := r.show(path, "")
	if err != nil || second.MediaID == first.MediaID || second.Media.Size != 200 {
		t.Fatalf("an overwritten file is a new item: %+v, %v", second, err)
	}
	info, err := os.Stat(filepath.Join(r.root, imageDirName(r.seat), first.MediaID))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("a kept copy is 0600: %v, %v", info, err)
	}
	if dir, _ := os.Stat(filepath.Join(r.root, imageDirName(r.seat))); dir.Mode().Perm() != 0o700 {
		t.Fatalf("a seat's directory is 0700: %v", dir.Mode())
	}
}

func TestFortyShowsKeepTheLastThirtyTwoAndTellTheClientWhichWentAway(t *testing.T) {
	r := newMediaRig(t)
	watcher := r.watcher()
	reader := r.client(peerStanding{pids: []int{os.Getpid()}})
	var ids []string
	for i := 0; i < 40; i++ {
		reply, err := r.show(r.write(fmt.Sprintf("f%02d.png", i), imageBytes(pngMagic, 64+i)), "")
		if err != nil {
			t.Fatalf("show %d: %v", i, err)
		}
		ids = append(ids, reply.MediaID)
	}
	closed := map[string]bool{}
	for i := 0; i < 8; i++ {
		closed[nextFrame(t, watcher, "media_closed").MediaID] = true
	}
	for i, id := range ids {
		if want := i < 8; closed[id] != want {
			t.Fatalf("item %d evicted = %v, want %v", i, closed[id], want)
		}
	}
	if reply, err := reader.request(frame{Type: "media_read", MediaID: ids[0]}); err == nil || reply.Reason != reasonMediaUnknown {
		t.Fatalf("an evicted item reads as media_unknown: %+v, %v", reply, err)
	}
	if got := r.read(reader, ids[39], 0); len(got) != 64+39 {
		t.Fatalf("the newest item is kept: %d bytes", len(got))
	}
	// A client that subscribes now is replayed what is kept, oldest first.
	late := r.client(peerStanding{pids: []int{os.Getpid()}})
	if _, err := late.request(frame{Type: "subscribe", Channel: "media"}); err != nil {
		t.Fatal(err)
	}
	for i := 8; i < 40; i++ {
		if got := nextFrame(t, late, "media").Media.ID; got != ids[i] {
			t.Fatalf("replay position %d is %s, want %s (oldest first)", i-8, got, ids[i])
		}
	}
}

func TestTheSeatByteCapEvictsTheOldestToMakeRoom(t *testing.T) {
	r := newMediaRig(t)
	earlier := maxMediaSeatBytes
	t.Cleanup(func() { maxMediaSeatBytes = earlier })
	maxMediaSeatBytes = 1000
	var ids []string
	for i := 0; i < 4; i++ {
		reply, err := r.show(r.write(fmt.Sprintf("b%d.png", i), imageBytes(pngMagic, 400)), "")
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, reply.MediaID)
	}
	if kept := r.seat.mediaItems(); len(kept) != 2 || kept[0].ID != ids[2] || kept[1].ID != ids[3] {
		t.Fatalf("1000 bytes holds the last two 400 byte items: %+v", kept)
	}
	if entries, _ := os.ReadDir(filepath.Join(r.root, imageDirName(r.seat))); len(entries) != 2 {
		t.Fatalf("an evicted copy is removed from disk: %v", entries)
	}
}

func TestAClientThatMayNotTypeCannotSubscribeOrRead(t *testing.T) {
	r := newMediaRig(t)
	shown, err := r.show(r.write("frame.png", imageBytes(pngMagic, 64)), "")
	if err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct {
		peer   peerStanding
		reason string
	}{
		"a process inside a session": {peerStanding{pids: []int{r.seat.pid}}, reasonSessionDescendant},
		"a locked remote device":     {peerStanding{web: true, remote: true}, reasonPasskeyRequired},
	} {
		c := r.client(tc.peer)
		if reply, err := c.request(frame{Type: "subscribe", Channel: "media"}); err == nil || reply.Reason != tc.reason {
			t.Errorf("%s: subscribe should be refused as %s: %+v, %v", name, tc.reason, reply, err)
		}
		if reply, err := c.request(frame{Type: "media_read", MediaID: shown.MediaID}); err == nil || reply.Reason != tc.reason {
			t.Errorf("%s: media_read should be refused as %s: %+v, %v", name, tc.reason, reply, err)
		}
	}
}

func TestOnlyTheSeatsOwnTokenCanShow(t *testing.T) {
	r := newMediaRig(t)
	path := r.write("frame.png", imageBytes(pngMagic, 64))
	c := r.client(peerStanding{pids: []int{os.Getpid()}})
	if reply, err := c.request(frame{Type: "media_show", Token: "not-a-token", Path: path}); err == nil {
		t.Fatalf("a token that names no seat must be refused: %+v", reply)
	}
	if reply, err := c.request(frame{Type: "media_show", Path: path}); err == nil {
		t.Fatalf("no token must be refused: %+v", reply)
	}
}

func TestEndingTheSeatRemovesItsFilesAndTellsSubscribers(t *testing.T) {
	r := newMediaRig(t)
	watcher := r.watcher()
	var ids []string
	for i := 0; i < 3; i++ {
		reply, err := r.show(r.write(fmt.Sprintf("e%d.png", i), imageBytes(pngMagic, 64)), "")
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, reply.MediaID)
	}
	dir := filepath.Join(r.root, imageDirName(r.seat))
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("the seat's directory exists while it lives: %v", err)
	}
	r.seat.end()
	select {
	case <-r.seat.forgotten:
	case <-time.After(10 * time.Second):
		t.Fatal("the seat did not end")
	}
	seen := map[string]bool{}
	for range ids {
		seen[nextFrame(t, watcher, "media_closed").MediaID] = true
	}
	for _, id := range ids {
		if !seen[id] {
			t.Fatalf("subscribers should be told %s is gone: %v", id, seen)
		}
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("ending the seat removes its directory: %v", err)
	}
	if _, _, err := r.seat.keepMedia(r.root, nil, "file", "x", "x"); err == nil {
		t.Fatal("an ended seat refuses later shows")
	}
}

func TestShowsPastTheWindowLimitAreRefused(t *testing.T) {
	r := newMediaRig(t)
	path := r.write("f.png", imageBytes(pngMagic, 64))
	for i := 0; i < maxShowsPerWindow; i++ {
		if _, err := r.show(path, ""); err != nil {
			t.Fatalf("show %d should be allowed: %v", i, err)
		}
	}
	if reply, err := r.show(path, ""); err == nil || reply.Reason != reasonTooMany {
		t.Fatalf("one more in the window is too_many: %+v, %v", reply, err)
	}
}

func TestMediaLogsNeitherNamesNorPaths(t *testing.T) {
	r := newMediaRig(t)
	if _, err := r.show(r.write("secret-plans.png", imageBytes(pngMagic, 64)), "a private title"); err != nil {
		t.Fatal(err)
	}
	logged := r.log()
	for _, leak := range []string{"secret-plans", "a private title", r.cwd, "aterm-media-"} {
		if strings.Contains(logged, leak) {
			t.Fatalf("the log must not carry %q: %s", leak, logged)
		}
	}
	if !strings.Contains(logged, "image of 64 bytes") {
		t.Fatalf("the log says what happened, by kind and size: %q", logged)
	}
}

func TestMediaNameIsTheTitleOrTheFilesOwnNameMadeSafe(t *testing.T) {
	if name, err := mediaName("", "/a/b/frame 1.png"); err != nil || name != "frame 1.png" {
		t.Fatalf("the file's own name: %q, %v", name, err)
	}
	if name, _ := mediaName("", "/a/b/bad\nname.png"); name != "bad_name.png" {
		t.Fatalf("a control character is replaced: %q", name)
	}
	if name, _ := mediaName("", "/a/.."); name != "file" {
		t.Fatalf("a name with nothing in it is file: %q", name)
	}
	if _, err := mediaName("tab\there", "/a/b"); err == nil {
		t.Fatal("a title with a control character is refused")
	}
	if name, _ := mediaName("", "/a/"+strings.Repeat("é", 300)); len(name) > maxMediaName || !strings.HasPrefix(name, "é") {
		t.Fatalf("a long name is cut on a rune boundary: %d bytes", len(name))
	}
}

func TestMediaIsNotOfferedWithoutARoot(t *testing.T) {
	d, _ := wsDaemon(t)
	c := pipeClient(t, d, peerStanding{pids: []int{os.Getpid()}})
	welcome := nextFrame(t, c, "welcome")
	for _, feature := range welcome.Features {
		if feature == mediaFeature {
			t.Fatalf("no media root, no %s feature: %v", mediaFeature, welcome.Features)
		}
	}
	if _, err := c.request(frame{Type: "subscribe", Channel: "media"}); err == nil {
		t.Fatal("no media root, no media channel")
	}
}

func TestWelcomeOffersMediaWithARoot(t *testing.T) {
	r := newMediaRig(t)
	c := pipeClient(t, r.d, peerStanding{pids: []int{os.Getpid()}})
	welcome := nextFrame(t, c, "welcome")
	for _, feature := range welcome.Features {
		if feature == mediaFeature {
			return
		}
	}
	t.Fatalf("the welcome should offer %s: %v", mediaFeature, welcome.Features)
}

func TestSweepMediaRemovesWhatAnEarlierDaemonLeft(t *testing.T) {
	r := newMediaRig(t)
	stale := filepath.Join(r.root, "old-seat-1", "deadbeef")
	if err := os.MkdirAll(filepath.Dir(stale), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	r.d.sweepMedia()
	if entries, _ := os.ReadDir(r.root); len(entries) != 0 {
		t.Fatalf("a restart sweeps every kept file, since the ring is memory only: %v", entries)
	}
}
