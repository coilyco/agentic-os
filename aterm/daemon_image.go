package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
)

// imageUploadFeature is how a client knows the daemon answers `image_upload`.
// A daemon without it answers an unknown frame. See references/image-upload.md.
const imageUploadFeature = "image-upload"

const (
	// maxImageBytes is the most that fits one frame once base64 makes it 4/3 as
	// long, since a frame over maxFrame drops the connection. See the docs page.
	maxImageBytes = 3_000_000
	// maxImagesPerSession bounds the files one seat's directory holds.
	maxImagesPerSession = 16
)

// The reasons an image_upload refusal names, beside passkey_required and
// session_descendant from the typing guard.
const (
	reasonNotAnImage     = "not_an_image"
	reasonTooLarge       = "too_large"
	reasonTooMany        = "too_many"
	reasonSessionUnknown = "session_unknown"
)

const (
	imageDirPerm         = 0o700
	imageFilePerm        = 0o600
	imageRandomNameBytes = 12
)

// sniffImage names the extension for the bytes' own signature. The client's
// claimed type is never read, since it decides nothing about what the file is.
func sniffImage(data []byte) (string, bool) {
	switch {
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		return ".png", true
	case bytes.HasPrefix(data, []byte("\xff\xd8\xff")):
		return ".jpg", true
	case bytes.HasPrefix(data, []byte("GIF87a")), bytes.HasPrefix(data, []byte("GIF89a")):
		return ".gif", true
	case len(data) >= 12 && bytes.Equal(data[:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")):
		return ".webp", true
	}
	return "", false
}

// imageDirName is the session's directory under the image root. The start time
// keeps a recycled pool name from inheriting or deleting an older seat's files.
func imageDirName(s *ptySession) string {
	return fileStem(s.name) + "-" + strconv.FormatInt(s.started.Unix(), 10)
}

// uploadImage writes an image_upload's bytes to a file the daemon names and answers
// its path. The client types the path, so the frame has the standing of typing.
func (d *daemon) uploadImage(cl *client, message frame) error {
	if d.imageRoot == "" {
		return fmt.Errorf("unknown frame type %q", message.Type)
	}
	s := cl.attached[message.Session]
	if s == nil || s.kind == kindTerminal {
		return withReason(reasonSessionUnknown, fmt.Errorf("not attached to a seat named %q", message.Session))
	}
	if !cl.owned[s.name] {
		if err := d.typingRefusal(cl, "a process inside an aterm session cannot hand a seat an image, write a file and name its path"); err != nil {
			return err
		}
	}
	// Checked before anything is written, on the bytes the frame already holds.
	if len(message.Data) > maxImageBytes {
		return withReason(reasonTooLarge, fmt.Errorf("the image is over %d bytes (3 MB), the most one upload carries", maxImageBytes))
	}
	extension, ok := sniffImage(message.Data)
	if !ok {
		return withReason(reasonNotAnImage, errors.New("the bytes are not a PNG, JPEG, GIF or WebP image"))
	}
	path, err := s.saveImage(d.imageRoot, message.Data, extension)
	if err != nil {
		return err
	}
	// Name, type and size only. The bytes never reach a log.
	d.logf("%s took a %s image of %d bytes", s.name, extension[1:], len(message.Data))
	return cl.c.write(frame{Type: "image_uploaded", ID: message.ID, Session: s.name, Path: path})
}

// saveImage creates the file 0600 under a name the daemon chose, in a 0700
// directory of this session's own.
func (s *ptySession) saveImage(root string, data []byte, extension string) (string, error) {
	s.imageMu.Lock()
	defer s.imageMu.Unlock()
	if s.imagesClosed {
		return "", withReason(reasonSessionUnknown, errors.New(s.name+" has ended"))
	}
	if err := ensureSocketDir(root); err != nil {
		return "", fmt.Errorf("prepare the image directory: %w", err)
	}
	dir := filepath.Join(root, imageDirName(s))
	if err := os.MkdirAll(dir, imageDirPerm); err != nil {
		return "", fmt.Errorf("prepare the image directory: %w", err)
	}
	if err := ensureSocketDir(dir); err != nil {
		return "", err
	}
	held, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	if len(held) >= maxImagesPerSession {
		return "", withReason(reasonTooMany, fmt.Errorf("%s already holds %d images, which is the most a seat keeps", s.name, len(held)))
	}
	path := filepath.Join(dir, randomID(imageRandomNameBytes)+extension)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, imageFilePerm)
	if err != nil {
		return "", fmt.Errorf("write the image: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return "", fmt.Errorf("write the image: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("write the image: %w", err)
	}
	return path, nil
}

// dropImages removes the session's directory and refuses later uploads to it.
func (s *ptySession) dropImages(root string) {
	s.imageMu.Lock()
	defer s.imageMu.Unlock()
	s.imagesClosed = true
	if root != "" {
		_ = os.RemoveAll(filepath.Join(root, imageDirName(s)))
	}
}

// sweepImages removes every image directory no live session owns. A restart adopts
// its sessions first, so what it keeps is theirs, and what it removes is left over.
func (d *daemon) sweepImages() {
	if d.imageRoot == "" {
		return
	}
	entries, err := os.ReadDir(d.imageRoot)
	if err != nil {
		return
	}
	keep := map[string]bool{}
	d.mu.Lock()
	for _, s := range d.sessions {
		keep[imageDirName(s)] = true
	}
	d.mu.Unlock()
	for _, entry := range entries {
		if !keep[entry.Name()] {
			_ = os.RemoveAll(filepath.Join(d.imageRoot, entry.Name()))
		}
	}
}
