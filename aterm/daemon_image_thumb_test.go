package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// realPNG is a decodable picture of the given size, which a thumbnail has to read.
func realPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	picture := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			picture.SetNRGBA(x, y, color.NRGBA{R: uint8(x), G: uint8(y), B: 200, A: 255})
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, picture); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func (r *imageRig) thumb(path string, full bool) (frame, error) {
	return r.c.request(frame{Type: "image_thumb", Session: "scientist-evie", Path: path, Full: full})
}

func TestImageThumbShrinksAnUploadAndFullReturnsItWhole(t *testing.T) {
	r := newImageRig(t)
	data := realPNG(t, 1200, 600)
	uploaded, err := r.upload(data)
	if err != nil {
		t.Fatal(err)
	}
	reply, err := r.thumb(uploaded.Path, false)
	if err != nil || reply.Type != "image_thumb" || reply.Path != uploaded.Path {
		t.Fatalf("an upload's path should be served: %+v, %v", reply, err)
	}
	small, err := png.Decode(bytes.NewReader(reply.Data))
	if err != nil {
		t.Fatalf("a PNG thumbnail should stay a PNG: %v", err)
	}
	if got := small.Bounds(); got.Dx() != thumbSide || got.Dy() != thumbSide/2 || reply.Width != thumbSide || reply.Height != thumbSide/2 {
		t.Fatalf("1200x600 should fit %d wide and keep its shape: %v, %dx%d", thumbSide, got, reply.Width, reply.Height)
	}
	whole, err := r.thumb(uploaded.Path, true)
	if err != nil || !bytes.Equal(whole.Data, data) {
		t.Fatalf("full should be the bytes as uploaded: %d bytes, %v", len(whole.Data), err)
	}
	if bytes.Contains([]byte(r.log()), []byte(uploaded.Path)) {
		t.Fatalf("no path reaches a log: %s", r.log())
	}
}

func TestImageThumbPassesThroughWhatItCannotScale(t *testing.T) {
	r := newImageRig(t)
	// A WebP the standard library cannot decode goes back as it is, and fits a frame.
	webp := imageBytes([]byte("RIFF\x10\x00\x00\x00WEBPVP8 "), 64)
	uploaded, err := r.upload(webp)
	if err != nil {
		t.Fatal(err)
	}
	reply, err := r.thumb(uploaded.Path, false)
	if err != nil || !bytes.Equal(reply.Data, webp) || reply.Width != 0 {
		t.Fatalf("an undecodable image should come back unchanged: %+v, %v", reply, err)
	}
}

// COI-3800's done-condition: nothing outside the seat's own upload directory is served.
func TestImageThumbRefusesEveryPathOutsideTheSeatsUploadDirectory(t *testing.T) {
	r := newImageRig(t)
	uploaded, err := r.upload(realPNG(t, 8, 8))
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(uploaded.Path)
	outside := filepath.Join(filepath.Dir(r.root), "secret.png")
	if err := os.WriteFile(outside, realPNG(t, 8, 8), 0o600); err != nil {
		t.Fatal(err)
	}
	// Another seat's directory, a file at the image root, and a link out of the seat's own.
	other := filepath.Join(r.root, "other-seat-1", "x.png")
	if err := os.MkdirAll(filepath.Dir(other), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(other, realPNG(t, 8, 8), 0o600); err != nil {
		t.Fatal(err)
	}
	rootFile := filepath.Join(r.root, "loose.png")
	if err := os.WriteFile(rootFile, realPNG(t, 8, 8), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.png")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(dir, "deeper")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, path := range map[string]string{
		"outside the image root": outside,
		"system file":            "/etc/hosts",
		"dot dot":                dir + "/../../secret.png",
		"dot dot back in":        dir + "/../" + filepath.Base(dir) + "/" + filepath.Base(uploaded.Path),
		"another seat":           other,
		"the image root":         rootFile,
		"relative":               filepath.Join("images", filepath.Base(dir), filepath.Base(uploaded.Path)),
		"a symlink out":          link,
		"a directory":            nested,
		"the directory itself":   dir,
		"empty":                  "",
	} {
		for _, full := range []bool{false, true} {
			reply, err := r.thumb(path, full)
			if err == nil || reply.Reason != reasonBadPath || len(reply.Data) != 0 {
				t.Fatalf("%s (full %v) should be %s with no bytes: %+v, %v", name, full, reasonBadPath, reply, err)
			}
		}
	}
	gone := filepath.Join(dir, "missing.png")
	if reply, err := r.thumb(gone, false); err == nil || reply.Reason != reasonImageGone {
		t.Fatalf("a missing upload should be %s: %+v, %v", reasonImageGone, reply, err)
	}
}

func TestImageThumbKeepsTheTypingGuard(t *testing.T) {
	r := newImageRig(t)
	uploaded, err := r.upload(realPNG(t, 8, 8))
	if err != nil {
		t.Fatal(err)
	}
	device := pipeClient(t, r.d, peerStanding{web: true, remote: true})
	nextFrame(t, device, "welcome")
	reply, err := device.request(frame{Type: "image_thumb", Session: "scientist-evie", Path: uploaded.Path})
	if err == nil || reply.Reason != reasonPasskeyRequired || len(reply.Data) != 0 {
		t.Fatalf("a device that has not unlocked should be %s: %+v, %v", reasonPasskeyRequired, reply, err)
	}
}
