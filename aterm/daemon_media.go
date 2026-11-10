package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"
)

// mediaFeature says the daemon serves the `media` channel and media_read.
// See references/media.md in the tooling-aterm-client skill.
const mediaFeature = "media"

const (
	reasonMediaUnknown = "media_unknown"
	// maxMediaBytes is one file, and the seat's ring holds at most maxMediaItems of them
	// within maxMediaSeatBytes, the oldest evicted to make room.
	maxMediaBytes     = 64 << 20
	maxMediaItems     = 32
	maxMediaSeatBytes = 256 << 20
	// maxMediaChunk is one media_data. Base64 makes it 4/3 larger, under the 4 MiB frame.
	maxMediaChunk = 2_000_000
	maxMediaName  = 200
	// Each show copies a file, so a loop cannot churn the disk.
	maxShowsPerWindow = 30
	showWindow        = time.Minute
	mediaFilePerm     = 0o600
	mediaIDBytes      = 8
	mediaSniffBytes   = 512
)

// mediaGuard is the refusal a client that may not type gets, since a seat's files
// can hold secrets. It is the rule `message` text follows.
const mediaGuard = "a process inside an aterm session cannot read a seat's shared files"

// mediaView is what a client lists. The path never leaves the daemon.
type mediaView struct {
	ID   string    `json:"id"`
	Name string    `json:"name"`
	Mime string    `json:"mime"`
	Kind string    `json:"kind"`
	Size int64     `json:"size"`
	At   time.Time `json:"at"`
}

// mediaItem is a view plus the daemon's own copy of the bytes.
type mediaItem struct {
	mediaView
	path string
}

// mp4Brands are the ftyp brands a browser plays as MP4. HEIC, AVIF and QuickTime carry
// the same box and are not video here.
var mp4Brands = []string{"isom", "iso2", "iso4", "iso5", "iso6", "mp41", "mp42", "avc1", "dash", "MSNV", "M4V ", "f4v "}

// classifyMedia reads the kind and type from a file's first bytes, never its name.
func classifyMedia(head []byte) (kind, mime string) {
	if extension, ok := sniffImage(head); ok {
		return "image", map[string]string{".png": "image/png", ".jpg": "image/jpeg", ".gif": "image/gif", ".webp": "image/webp"}[extension]
	}
	if len(head) >= 12 && string(head[4:8]) == "ftyp" && slices.Contains(mp4Brands, string(head[8:12])) {
		return "video", "video/mp4"
	}
	if bytes.HasPrefix(head, []byte{0x1a, 0x45, 0xdf, 0xa3}) && bytes.Contains(head[:min(len(head), 64)], []byte("webm")) {
		return "video", "video/webm"
	}
	return "file", "application/octet-stream"
}

func checkMediaTitle(title string) error {
	if strings.TrimSpace(title) == "" || len(title) > maxMediaName || strings.ContainsFunc(title, func(r rune) bool { return r < ' ' || r == 0x7f }) {
		return withExit(exitUsage, fmt.Errorf("a name is 1 to %d bytes with no control characters", maxMediaName))
	}
	return nil
}

// mediaName is the title the seat gave, or the file's own name made safe to show.
func mediaName(title, path string) (string, error) {
	if title != "" {
		return title, checkMediaTitle(title)
	}
	name := strings.Map(func(r rune) rune {
		if r < ' ' || r == 0x7f {
			return '_'
		}
		return r
	}, filepath.Base(path))
	for len(name) > maxMediaName {
		_, width := utf8.DecodeLastRuneInString(name)
		name = name[:len(name)-width]
	}
	if strings.Trim(name, ". ") == "" {
		return "file", nil
	}
	return name, nil
}

// allowShow counts one show against the seat's window and says whether it may.
func (s *ptySession) allowShow(now time.Time) bool {
	s.showMu.Lock()
	defer s.showMu.Unlock()
	s.shown = slices.DeleteFunc(s.shown, func(at time.Time) bool { return now.Sub(at) > showWindow })
	if len(s.shown) >= maxShowsPerWindow {
		return false
	}
	s.shown = append(s.shown, now)
	return true
}

// openMediaSource opens path only if it resolves, symlinks followed, to a regular file
// under one of the seat's roots, as an artifact source does, and is the file it checked.
func openMediaSource(s *ptySession, path string) (*os.File, error) {
	if !filepath.IsAbs(path) {
		return nil, withReason(reasonBadPath, errors.New("the path must be absolute"))
	}
	real, err := filepath.EvalSymlinks(path)
	if err != nil || !slices.ContainsFunc(s.artifactRoots(), func(root string) bool { return within(root, real) }) {
		return nil, withReason(reasonBadPath, errors.New("the file must be under this seat's working directory or scratchpad"))
	}
	before, err := os.Stat(real)
	if err != nil || !before.Mode().IsRegular() {
		return nil, withReason(reasonBadPath, errors.New("the path is not a regular file"))
	}
	if before.Size() > maxMediaBytes {
		return nil, withReason(reasonTooLarge, fmt.Errorf("the file is over %d bytes", maxMediaBytes))
	}
	// O_NONBLOCK so a file swapped for a FIFO after the stat cannot hold the daemon.
	file, err := os.OpenFile(real, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, withReason(reasonBadPath, errors.New("the file cannot be read"))
	}
	after, err := file.Stat()
	if err != nil || !after.Mode().IsRegular() || !os.SameFile(before, after) {
		_ = file.Close()
		return nil, withReason(reasonBadPath, errors.New("the file changed while it was opened"))
	}
	return file, nil
}

// showMedia is a seat's request, judged by its token like `send`. It copies the file
// into the seat's own directory, so a file the seat overwrites later is a new item.
func (d *daemon) showMedia(cl *client, message frame) error {
	if d.mediaRoot == "" {
		return fmt.Errorf("unknown frame type %q", message.Type)
	}
	s := d.byToken(message.Token)
	if s == nil || s.kind == kindTerminal {
		return withExit(exitUsage, errors.New("aterm show speaks for a session aterm launched, and this token names no live one"))
	}
	name, err := mediaName(message.Title, message.Path)
	if err != nil {
		return err
	}
	if !s.allowShow(time.Now()) {
		return withReason(reasonTooMany, fmt.Errorf("%s showed more than %d files in a minute", s.name, maxShowsPerWindow))
	}
	source, err := openMediaSource(s, message.Path)
	if err != nil {
		return err
	}
	defer source.Close()
	head := make([]byte, mediaSniffBytes)
	n, _ := io.ReadFull(source, head)
	kind, mime := classifyMedia(head[:n])
	if _, err := source.Seek(0, io.SeekStart); err != nil {
		return withReason(reasonBadPath, errors.New("the file cannot be read"))
	}
	item, evicted, err := s.keepMedia(d.mediaRoot, source, kind, mime, name)
	if err != nil {
		return err
	}
	for _, old := range evicted {
		d.pushMedia(frame{Type: "media_closed", Session: s.name, MediaID: old.ID})
	}
	view := item.mediaView
	taken := d.pushMedia(frame{Type: "media", Session: s.name, Media: &view})
	// Kind and size only. A name or a path can say what a seat is working on.
	d.logf("%s showed a %s of %d bytes, taken by %d clients", s.name, kind, item.Size, taken)
	return cl.c.write(frame{Type: "media_shown", ID: message.ID, Session: s.name, MediaID: item.ID, Media: &view, Delivered: taken})
}

// keepMedia copies src into the seat's 0700 directory under a name the daemon chose,
// then evicts the oldest items until the ring fits. It returns what it evicted.
func (s *ptySession) keepMedia(root string, src *os.File, kind, mime, name string) (mediaItem, []mediaItem, error) {
	s.mediaMu.Lock()
	defer s.mediaMu.Unlock()
	if s.mediaClosed {
		return mediaItem{}, nil, withReason(reasonSessionUnknown, errors.New(s.name+" has ended"))
	}
	if err := ensureSocketDir(root); err != nil {
		return mediaItem{}, nil, fmt.Errorf("prepare the media directory: %w", err)
	}
	dir := filepath.Join(root, imageDirName(s))
	if err := os.MkdirAll(dir, imageDirPerm); err != nil {
		return mediaItem{}, nil, fmt.Errorf("prepare the media directory: %w", err)
	}
	if err := ensureSocketDir(dir); err != nil {
		return mediaItem{}, nil, err
	}
	id := randomID(mediaIDBytes)
	path := filepath.Join(dir, id)
	dst, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, mediaFilePerm)
	if err != nil {
		return mediaItem{}, nil, fmt.Errorf("keep the file: %w", err)
	}
	// One byte past the cap, so a file that grew after the stat is still refused.
	copied, err := io.Copy(dst, io.LimitReader(src, maxMediaBytes+1))
	if closeErr := dst.Close(); err == nil {
		err = closeErr
	}
	switch {
	case err != nil:
		_ = os.Remove(path)
		return mediaItem{}, nil, withReason(reasonBadPath, errors.New("the file cannot be read"))
	case copied > maxMediaBytes:
		_ = os.Remove(path)
		return mediaItem{}, nil, withReason(reasonTooLarge, fmt.Errorf("the file is over %d bytes", maxMediaBytes))
	case copied == 0:
		_ = os.Remove(path)
		return mediaItem{}, nil, withReason(reasonBadPath, errors.New("the file is empty"))
	}
	item := mediaItem{mediaView: mediaView{ID: id, Name: name, Mime: mime, Kind: kind, Size: copied, At: time.Now().UTC()}, path: path}
	s.media = append(s.media, item)
	s.mediaBytes += copied
	var evicted []mediaItem
	for len(s.media) > 1 && (len(s.media) > maxMediaItems || s.mediaBytes > maxMediaSeatBytes) {
		oldest := s.media[0]
		s.media = s.media[1:]
		s.mediaBytes -= oldest.Size
		_ = os.Remove(oldest.path)
		evicted = append(evicted, oldest)
	}
	return item, evicted, nil
}

// mediaItems is the seat's kept items, oldest first.
func (s *ptySession) mediaItems() []mediaItem {
	s.mediaMu.Lock()
	defer s.mediaMu.Unlock()
	return slices.Clone(s.media)
}

// dropMedia removes the seat's directory, refuses later shows, and tells subscribers each
// item is gone.
func (d *daemon) dropMedia(s *ptySession) {
	s.mediaMu.Lock()
	s.mediaClosed = true
	items := s.media
	s.media, s.mediaBytes = nil, 0
	s.mediaMu.Unlock()
	if d.mediaRoot != "" {
		_ = os.RemoveAll(filepath.Join(d.mediaRoot, imageDirName(s)))
	}
	for _, item := range items {
		d.pushMedia(frame{Type: "media_closed", Session: s.name, MediaID: item.ID})
	}
}

// sweepMedia removes every kept file at start. The ring lives in memory, so after a
// restart nothing can name what is left.
func (d *daemon) sweepMedia() {
	if d.mediaRoot == "" {
		return
	}
	entries, err := os.ReadDir(d.mediaRoot)
	if err != nil {
		return
	}
	for _, entry := range entries {
		_ = os.RemoveAll(filepath.Join(d.mediaRoot, entry.Name()))
	}
}

// watchMedia subscribes a client that can show media and replays what each seat kept,
// oldest first. A client that may not type is refused, as it is for `message` text.
func (d *daemon) watchMedia(cl *client, message frame) error {
	if d.mediaRoot == "" {
		return fmt.Errorf("no channel named %q", message.Channel)
	}
	if err := d.typingRefusal(cl, mediaGuard); err != nil {
		return err
	}
	if err := cl.c.write(frame{Type: "subscribed", ID: message.ID, Channel: "media"}); err != nil {
		return err
	}
	// Registered before the replay, so an item shown meanwhile may arrive twice.
	d.mu.Lock()
	if d.mediaWatchers == nil {
		d.mediaWatchers = map[*conn]*client{}
	}
	d.mediaWatchers[cl.c] = cl
	sessions := make([]*ptySession, 0, len(d.sessions))
	for _, s := range d.sessions {
		if s.kind != kindTerminal {
			sessions = append(sessions, s)
		}
	}
	d.mu.Unlock()
	slices.SortFunc(sessions, func(a, b *ptySession) int { return strings.Compare(a.name, b.name) })
	for _, s := range sessions {
		for _, item := range s.mediaItems() {
			view := item.mediaView
			if err := cl.c.write(frame{Type: "media", Session: s.name, Media: &view}); err != nil {
				return err
			}
		}
	}
	return nil
}

// pushMedia offers the frame to each subscriber that may still type and counts those it
// reached. A full outbox drops the client, as broadcast does.
func (d *daemon) pushMedia(message frame) int {
	under := d.descender(d.sessionRoots())
	d.mu.Lock()
	watchers := make(map[*conn]*client, len(d.mediaWatchers))
	for c, cl := range d.mediaWatchers {
		watchers[c] = cl
	}
	d.mu.Unlock()
	taken := 0
	for c, cl := range watchers {
		if d.typingRefusalVia(cl, "", under) != nil {
			continue
		}
		if c.offer(message) {
			taken++
			continue
		}
		d.mu.Lock()
		delete(d.mediaWatchers, c)
		d.mu.Unlock()
		_ = c.Close()
	}
	return taken
}

// findMedia is the kept item an id names, in whichever seat holds it.
func (d *daemon) findMedia(id string) (mediaItem, bool) {
	d.mu.Lock()
	sessions := make([]*ptySession, 0, len(d.sessions))
	for _, s := range d.sessions {
		sessions = append(sessions, s)
	}
	d.mu.Unlock()
	for _, s := range sessions {
		for _, item := range s.mediaItems() {
			if item.ID == id {
				return item, true
			}
		}
	}
	return mediaItem{}, false
}

// readMedia answers one chunk of a kept file. Every chunk carries the whole size and
// whether it is the last, so a client assembles without a second request.
func (d *daemon) readMedia(cl *client, message frame) error {
	if d.mediaRoot == "" {
		return fmt.Errorf("unknown frame type %q", message.Type)
	}
	if err := d.typingRefusal(cl, mediaGuard); err != nil {
		return err
	}
	if message.Offset < 0 || message.Length < 0 {
		return withExit(exitUsage, errors.New("media_read takes a non-negative offset and length"))
	}
	length := message.Length
	if length == 0 || length > maxMediaChunk {
		length = maxMediaChunk
	}
	item, ok := d.findMedia(message.MediaID)
	if !ok {
		return withReason(reasonMediaUnknown, fmt.Errorf("no shared file %q", message.MediaID))
	}
	var data []byte
	if remaining := item.Size - message.Offset; remaining > 0 {
		file, err := os.OpenFile(item.path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
		if err != nil {
			// Evicted between the lookup and the open.
			return withReason(reasonMediaUnknown, fmt.Errorf("no shared file %q", message.MediaID))
		}
		defer file.Close()
		data = make([]byte, min(int64(length), remaining))
		got, err := file.ReadAt(data, message.Offset)
		if err != nil && !errors.Is(err, io.EOF) {
			return fmt.Errorf("read the shared file: %w", err)
		}
		data = data[:got]
	}
	eof := message.Offset+int64(len(data)) >= item.Size
	return cl.c.write(frame{Type: "media_data", ID: message.ID, MediaID: item.ID, Offset: message.Offset, Size: item.Size, EOF: &eof, Data: data})
}
