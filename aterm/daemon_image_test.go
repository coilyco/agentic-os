package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
)

var pngMagic = []byte("\x89PNG\r\n\x1a\n")

// imageBytes is a signature followed by filler, which is all the daemon reads.
func imageBytes(signature []byte, size int) []byte {
	return append(append([]byte(nil), signature...), bytes.Repeat([]byte{'x'}, size-len(signature))...)
}

// imageRig is a daemon with an image root, one seat, and a local connection
// attached to it that did not spawn it, so every guard is judged.
type imageRig struct {
	d    *daemon
	c    *conn
	root string
	log  func() string
}

func newImageRig(t *testing.T) *imageRig {
	t.Helper()
	d, _ := wsDaemon(t)
	dir, err := os.MkdirTemp("/tmp", "aterm-img-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	// Absent until the first upload, since the daemon makes it.
	d.imageRoot = filepath.Join(dir, "images")
	var mu sync.Mutex
	logged := &strings.Builder{}
	read := func() string {
		mu.Lock()
		defer mu.Unlock()
		return logged.String()
	}
	d.logf = func(format string, args ...any) {
		mu.Lock()
		defer mu.Unlock()
		fmt.Fprintf(logged, format+"\n", args...)
	}
	wsSession(t, d, false)
	c := pipeClient(t, d, peerStanding{pids: []int{os.Getpid()}})
	nextFrame(t, c, "welcome")
	if _, err := c.request(frame{Type: "attach", Session: "scientist-evie", Rows: 24, Cols: 80}); err != nil {
		t.Fatalf("attach: %v", err)
	}
	return &imageRig{d: d, c: c, root: d.imageRoot, log: read}
}

func (r *imageRig) upload(data []byte) (frame, error) {
	return r.c.request(frame{Type: "image_upload", Session: "scientist-evie", Data: data})
}

func TestImageUploadWritesAPrivateFileTheDaemonNames(t *testing.T) {
	r := newImageRig(t)
	data := imageBytes(pngMagic, 2048)
	reply, err := r.upload(data)
	if err != nil || reply.Type != "image_uploaded" {
		t.Fatalf("a PNG should be taken: %+v, %v", reply, err)
	}
	if !filepath.IsAbs(reply.Path) || filepath.Dir(filepath.Dir(reply.Path)) != r.root || filepath.Ext(reply.Path) != ".png" {
		t.Fatalf("the path should be absolute, two levels under the image root, and end in .png: %q", reply.Path)
	}
	written, err := os.ReadFile(reply.Path)
	if err != nil || !bytes.Equal(written, data) {
		t.Fatalf("the file should hold the bytes: %v", err)
	}
	for path, want := range map[string]os.FileMode{r.root: 0o700, filepath.Dir(reply.Path): 0o700, reply.Path: 0o600} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != want {
			t.Fatalf("%s should be %v: %v, %v", path, want, info, err)
		}
	}
	other, err := r.upload(data)
	if err != nil || other.Path == reply.Path {
		t.Fatalf("each upload gets its own name: %q then %q, %v", reply.Path, other.Path, err)
	}
}

func TestImageUploadNamesTheTypeFromTheBytesNotTheClaim(t *testing.T) {
	for name, tc := range map[string]struct {
		data []byte
		ext  string
	}{
		"jpeg":  {imageBytes([]byte("\xff\xd8\xff\xe0"), 64), ".jpg"},
		"gif87": {imageBytes([]byte("GIF87a"), 64), ".gif"},
		"gif89": {imageBytes([]byte("GIF89a"), 64), ".gif"},
		"webp":  {imageBytes([]byte("RIFF\x10\x00\x00\x00WEBPVP8 "), 64), ".webp"},
	} {
		t.Run(name, func(t *testing.T) {
			r := newImageRig(t)
			// The frame struct has no mime field, so the claim goes in as raw JSON.
			line := `{"type":"image_upload","id":"m","session":"scientist-evie","mime":"image/png","data":"` +
				base64.StdEncoding.EncodeToString(tc.data) + `"}`
			if err := r.c.writeLine([]byte(line)); err != nil {
				t.Fatal(err)
			}
			reply := nextFrame(t, r.c, "image_uploaded", "error")
			if reply.Type != "image_uploaded" || filepath.Ext(reply.Path) != tc.ext {
				t.Fatalf("a %s should be saved as %s whatever the client claims: %+v", name, tc.ext, reply)
			}
		})
	}
}

func TestImageUploadRefusesWhatIsNotAnImageAndWritesNothing(t *testing.T) {
	r := newImageRig(t)
	for name, data := range map[string][]byte{
		"text":          []byte("#!/bin/sh\necho owned\n"),
		"empty":         nil,
		"html":          []byte("<html><img src=x></html>"),
		"riff not webp": imageBytes([]byte("RIFF\x10\x00\x00\x00WAVEfmt "), 64),
		"png sig cut":   pngMagic[:5],
	} {
		reply, err := r.upload(data)
		if err == nil || reply.Reason != reasonNotAnImage {
			t.Fatalf("%s should be %s: %+v, %v", name, reasonNotAnImage, reply, err)
		}
	}
	if _, err := os.Stat(r.root); !os.IsNotExist(err) {
		t.Fatalf("a refusal must write nothing, not even the directory: %v", err)
	}
}

func TestImageUploadChecksTheSizeBeforeAnythingIsWritten(t *testing.T) {
	r := newImageRig(t)
	reply, err := r.upload(imageBytes(pngMagic, maxImageBytes+1))
	if err == nil || reply.Reason != reasonTooLarge {
		t.Fatalf("one byte over the cap should be %s: %+v, %v", reasonTooLarge, reply, err)
	}
	if _, err := os.Stat(r.root); !os.IsNotExist(err) {
		t.Fatalf("an oversize image must not reach the disk: %v", err)
	}
	if reply, err := r.upload(imageBytes(pngMagic, maxImageBytes)); err != nil || reply.Type != "image_uploaded" {
		t.Fatalf("exactly the cap should be taken: %+v, %v", reply, err)
	}
}

// A frame over maxFrame drops the connection before the daemon can answer, so the
// cap has to leave the whole encoded frame room.
func TestImageCapFitsAFrame(t *testing.T) {
	encoded, err := json.Marshal(frame{Type: "image_upload", ID: randomID(6), Session: strings.Repeat("a", 200), Data: make([]byte, maxImageBytes)})
	if err != nil || len(encoded) > maxFrame {
		t.Fatalf("a full-size upload is %d bytes and maxFrame is %d: %v", len(encoded), maxFrame, err)
	}
	if want := base64.StdEncoding.EncodedLen(maxImageBytes); len(encoded) < want {
		t.Fatalf("the frame cannot be shorter than its data: %d < %d", len(encoded), want)
	}
}

func TestImageUploadRefusesPastTheSessionLimit(t *testing.T) {
	r := newImageRig(t)
	for i := 0; i < maxImagesPerSession; i++ {
		if _, err := r.upload(imageBytes(pngMagic, 64)); err != nil {
			t.Fatalf("upload %d should fit: %v", i, err)
		}
	}
	reply, err := r.upload(imageBytes(pngMagic, 64))
	if err == nil || reply.Reason != reasonTooMany {
		t.Fatalf("one past the limit should be %s: %+v, %v", reasonTooMany, reply, err)
	}
}

func TestImageUploadNeedsAnAttachedSeat(t *testing.T) {
	r := newImageRig(t)
	reply, err := r.c.request(frame{Type: "image_upload", Session: "no-such-seat", Data: imageBytes(pngMagic, 64)})
	if err == nil || reply.Reason != reasonSessionUnknown {
		t.Fatalf("an unknown session should be %s: %+v, %v", reasonSessionUnknown, reply, err)
	}
	if _, err := os.Stat(r.root); !os.IsNotExist(err) {
		t.Fatalf("nothing is written for a session that is not there: %v", err)
	}
}

func TestImageUploadKeepsTheTypingGuards(t *testing.T) {
	t.Run("a locked remote device", func(t *testing.T) {
		r := newImageRig(t)
		device := pipeClient(t, r.d, peerStanding{web: true, remote: true})
		nextFrame(t, device, "welcome")
		reply, err := device.request(frame{Type: "image_upload", Session: "scientist-evie", Data: imageBytes(pngMagic, 64)})
		if err == nil || reply.Reason != reasonPasskeyRequired {
			t.Fatalf("a device that has not unlocked should be %s: %+v, %v", reasonPasskeyRequired, reply, err)
		}
		if _, err := os.Stat(r.root); !os.IsNotExist(err) {
			t.Fatalf("a refused device must write nothing: %v", err)
		}
	})
	t.Run("a process inside a session", func(t *testing.T) {
		d, _ := wsDaemon(t)
		dir, err := os.MkdirTemp("/tmp", "aterm-img-")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(dir) })
		d.imageRoot = filepath.Join(dir, "images")
		wsSession(t, d, true)
		c := pipeClient(t, d, peerStanding{pids: []int{os.Getpid()}})
		nextFrame(t, c, "welcome")
		if _, err := c.request(frame{Type: "attach", Session: "scientist-evie", Rows: 24, Cols: 80}); err != nil {
			t.Fatal(err)
		}
		reply, err := c.request(frame{Type: "image_upload", Session: "scientist-evie", Data: imageBytes(pngMagic, 64)})
		if err == nil || reply.Reason != reasonSessionDescendant {
			t.Fatalf("a caller inside a session should be %s: %+v, %v", reasonSessionDescendant, reply, err)
		}
		if _, err := os.Stat(d.imageRoot); !os.IsNotExist(err) {
			t.Fatalf("a refused caller must write nothing: %v", err)
		}
	})
}

func TestImageBytesNeverReachTheLog(t *testing.T) {
	r := newImageRig(t)
	marker := "UNIQUE-PIXEL-MARKER"
	data := append(append([]byte(nil), pngMagic...), []byte(marker)...)
	if _, err := r.upload(data); err != nil {
		t.Fatal(err)
	}
	if _, err := r.upload([]byte(marker)); err == nil {
		t.Fatal("not an image")
	}
	log := r.log()
	if strings.Contains(log, marker) || strings.Contains(log, base64.StdEncoding.EncodeToString(data)) {
		t.Fatalf("the daemon log must hold no image bytes:\n%s", log)
	}
	if !strings.Contains(log, "scientist-evie took a png image of") {
		t.Fatalf("the log should still name the upload: %q", log)
	}
}

func TestDaemonWithoutAnImageRootDoesNotOfferUpload(t *testing.T) {
	d, _ := wsDaemon(t)
	wsSession(t, d, false)
	c := pipeClient(t, d, peerStanding{pids: []int{os.Getpid()}})
	welcome := nextFrame(t, c, "welcome")
	if slices.Contains(welcome.Features, imageUploadFeature) {
		t.Fatalf("no image root, so no %s: %v", imageUploadFeature, welcome.Features)
	}
	if _, err := c.request(frame{Type: "attach", Session: "scientist-evie", Rows: 24, Cols: 80}); err != nil {
		t.Fatal(err)
	}
	if reply, err := c.request(frame{Type: "image_upload", Session: "scientist-evie", Data: imageBytes(pngMagic, 64)}); err == nil || reply.Reason != "" {
		t.Fatalf("a daemon without the build answers an unknown frame: %+v, %v", reply, err)
	}
}

func TestSweepKeepsLiveSessionsImagesAndRemovesTheRest(t *testing.T) {
	r := newImageRig(t)
	reply, err := r.upload(imageBytes(pngMagic, 64))
	if err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(r.root, "gone-session-1")
	if err := os.MkdirAll(stale, 0o700); err != nil {
		t.Fatal(err)
	}
	stray := filepath.Join(r.root, "stray.png")
	if err := os.WriteFile(stray, pngMagic, 0o600); err != nil {
		t.Fatal(err)
	}
	r.d.sweepImages()
	if _, err := os.Stat(reply.Path); err != nil {
		t.Fatalf("a live session's image must survive a sweep: %v", err)
	}
	for _, path := range []string{stale, stray} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("%s is left over and should be swept: %v", path, err)
		}
	}
}

// Through runDaemon and a real close, which is the path a seat's window takes.
func TestImageUploadThroughARunningDaemonEndsWithTheSession(t *testing.T) {
	socket := testDaemon(t)
	tc := dialTest(t)
	if !slices.Contains(tc.c.features, imageUploadFeature) {
		t.Fatalf("welcome should offer %q: %v", imageUploadFeature, tc.c.features)
	}
	tc.spawn("scientist-evie", "scientist", "Evie", "sleep 30")
	reply, err := tc.c.request(frame{Type: "image_upload", Session: "scientist-evie", Data: imageBytes(pngMagic, 512)})
	if err != nil {
		t.Fatalf("the spawning window may upload: %v", err)
	}
	if !strings.HasPrefix(reply.Path, filepath.Join(filepath.Dir(socket), "images")+string(filepath.Separator)) {
		t.Fatalf("the file belongs under the daemon's own directory: %q", reply.Path)
	}
	if _, err := os.Stat(reply.Path); err != nil {
		t.Fatal(err)
	}
	if _, err := tc.c.request(frame{Type: "close", Target: "scientist-evie", Force: true}); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(reply.Path)); !os.IsNotExist(err) {
		t.Fatalf("closing the session should remove its images: %v", err)
	}
}

func TestImageUploadRefusesATerminal(t *testing.T) {
	tc := shellDaemon(t)
	terminal := tc.spawnTerminal("/")
	reply, err := tc.c.request(frame{Type: "image_upload", Session: terminal.Session, Data: imageBytes(pngMagic, 64)})
	if err == nil || reply.Reason != reasonSessionUnknown {
		t.Fatalf("a terminal is not a seat, so %s: %+v, %v", reasonSessionUnknown, reply, err)
	}
}
