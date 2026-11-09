package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// artifactsFeature is how a client knows the daemon sends `artifact` frames on the
// views channel. See the tooling-aterm-client skill, references/artifacts.md.
const artifactsFeature = "artifacts"

const (
	reasonBadPath        = "bad_path"
	reasonNotText        = "not_text"
	reasonArtifactOwned  = "artifact_owned"
	reasonNoArtifact     = "no_artifact"
	maxArtifactBytes     = maxViewBytes
	maxArtifactsPerSeat  = 16
	maxStandingArtifacts = 16
	maxArtifactTitle     = 200
)

// artifactURL is the one address shape accepted, with the id captured.
var artifactURL = regexp.MustCompile(`^https://claude\.ai/artifact/([A-Za-z0-9_-]{1,64})$`)

// artifactView is what a client draws. The id is the artifact's own, from its URL.
type artifactView struct {
	ID        string `json:"id"`
	URL       string `json:"url"`
	Title     string `json:"title"`
	Waiting   bool   `json:"waiting"`
	Standing  bool   `json:"standing"`
	HTML      string `json:"html"`
	SeatEnded bool   `json:"seat_ended,omitempty"`
}

// artifactEntry is a view plus who posted it. Owner and Started name one seat
// instance, since a pool name recurs.
type artifactEntry struct {
	artifactView
	Owner   string    `json:"owner"`
	Started time.Time `json:"started"`
}

// artifactID reads the id out of an address, or takes the argument as an id.
func artifactID(ref string) string {
	if match := artifactURL.FindStringSubmatch(ref); match != nil {
		return match[1]
	}
	return ref
}

func (e *artifactEntry) ownedBy(s *ptySession) bool {
	return e.Owner == s.name && e.Started.Equal(s.started)
}

// managedBy says whether seat s may change the entry: its owner, or any seat once
// a standing page's owner has ended.
func (e *artifactEntry) managedBy(s *ptySession) bool {
	return e.ownedBy(s) || (e.SeatEnded && e.Standing)
}

// artifactRoots are where a seat may register a source file from. claude-<uid> is
// shared by every claude seat of this user, as one seat's scratchpad is not nameable.
func (s *ptySession) artifactRoots() []string {
	roots := slices.Clone(s.fileRoots)
	roots = append(roots, s.cwd, filepath.Join("/tmp", "claude-"+strconv.Itoa(os.Getuid())))
	var resolved []string
	for _, root := range roots {
		// A root of / would admit every file, so a seat started there gets none from it.
		if real, err := filepath.EvalSymlinks(root); root != "" && err == nil && real != string(filepath.Separator) {
			resolved = append(resolved, real)
		}
	}
	return resolved
}

// envDirs are the values of the named variables in a launch environment, when set.
func envDirs(environ []string, names ...string) []string {
	var dirs []string
	for _, entry := range environ {
		key, value, _ := strings.Cut(entry, "=")
		if slices.Contains(names, key) && value != "" {
			dirs = append(dirs, value)
		}
	}
	return dirs
}

func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// readArtifactSource reads path only if it resolves, symlinks followed, to a regular
// file under one of the seat's roots, within the size cap, and valid text.
func readArtifactSource(s *ptySession, path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", withReason(reasonBadPath, errors.New("the source path must be absolute"))
	}
	real, err := filepath.EvalSymlinks(path)
	if err != nil || !slices.ContainsFunc(s.artifactRoots(), func(root string) bool { return within(root, real) }) {
		return "", withReason(reasonBadPath, errors.New("the source must be a file under this seat's working directory or scratchpad"))
	}
	info, err := os.Stat(real)
	if err != nil || !info.Mode().IsRegular() {
		return "", withReason(reasonBadPath, errors.New("the source is not a regular file"))
	}
	if info.Size() > maxArtifactBytes {
		return "", withReason(reasonTooLarge, fmt.Errorf("the source is over %d bytes", maxArtifactBytes))
	}
	file, err := os.Open(real)
	if err != nil {
		return "", withReason(reasonBadPath, errors.New("the source cannot be read"))
	}
	defer file.Close()
	// One byte past the cap, so a file that grew after the stat is still refused.
	data, err := io.ReadAll(io.LimitReader(file, maxArtifactBytes+1))
	if err != nil {
		return "", withReason(reasonBadPath, errors.New("the source cannot be read"))
	}
	if len(data) > maxArtifactBytes {
		return "", withReason(reasonTooLarge, fmt.Errorf("the source is over %d bytes", maxArtifactBytes))
	}
	if !utf8.Valid(data) {
		return "", withReason(reasonNotText, errors.New("the source is not UTF-8 text"))
	}
	return string(data), nil
}

func checkArtifactTitle(title string) error {
	if strings.TrimSpace(title) == "" || len(title) > maxArtifactTitle || strings.ContainsFunc(title, func(r rune) bool { return r < ' ' || r == 0x7f }) {
		return withExit(exitUsage, fmt.Errorf("a title is 1 to %d bytes with no control characters", maxArtifactTitle))
	}
	return nil
}

// artifactAdd registers or replaces a seat's artifact and pushes it to the views channel.
func (d *daemon) artifactAdd(cl *client, message frame) error {
	s := d.byToken(message.Token)
	if s == nil {
		return withExit(exitUsage, errors.New("aterm artifact speaks for a session aterm launched, and this token names no live one"))
	}
	match := artifactURL.FindStringSubmatch(message.URL)
	if match == nil {
		return withReason(reasonBadURL, errors.New("an artifact address is exactly https://claude.ai/artifact/<id>"))
	}
	if err := checkArtifactTitle(message.Title); err != nil {
		return err
	}
	source, err := readArtifactSource(s, message.Path)
	if err != nil {
		return err
	}
	entry := &artifactEntry{
		artifactView: artifactView{ID: match[1], URL: message.URL, Title: message.Title, Standing: message.Standing, HTML: source},
		Owner:        s.name, Started: s.started,
	}
	if message.Waiting != nil {
		entry.Waiting = *message.Waiting
	}
	d.apps.mu.Lock()
	defer d.apps.mu.Unlock()
	earlier := d.apps.artifacts[entry.ID]
	if earlier != nil && !earlier.managedBy(s) {
		return withReason(reasonArtifactOwned, errors.New("another live seat posted this artifact"))
	}
	if err := d.artifactRoomLocked(entry, s); err != nil {
		return err
	}
	if d.apps.artifacts == nil {
		d.apps.artifacts = map[string]*artifactEntry{}
	}
	d.apps.artifacts[entry.ID] = entry
	d.persistArtifact(entry)
	d.apps.emit(frame{Type: "artifact", Session: entry.Owner, Artifact: &entry.artifactView})
	d.logf("%s posted an artifact, shown to %d clients", s.name, len(d.apps.watchers))
	if entry.Waiting && (earlier == nil || !earlier.Waiting) {
		d.notifyArtifactWaiting(s, entry.Title)
	}
	return cl.c.write(frame{Type: "artifact_added", ID: message.ID, Session: s.name, ArtifactID: entry.ID, Delivered: len(d.apps.watchers)})
}

// artifactRoomLocked refuses a page that would go past the per-seat or standing limit.
// Replacing a page of the same id never adds one.
func (d *daemon) artifactRoomLocked(entry *artifactEntry, s *ptySession) error {
	mine, standing := 0, 0
	for id, other := range d.apps.artifacts {
		if id == entry.ID {
			continue
		}
		if other.ownedBy(s) {
			mine++
		}
		if other.Standing {
			standing++
		}
	}
	if mine >= maxArtifactsPerSeat {
		return withReason(reasonTooMany, fmt.Errorf("%s already holds %d artifacts", s.name, maxArtifactsPerSeat))
	}
	if entry.Standing && standing >= maxStandingArtifacts {
		return withReason(reasonTooMany, fmt.Errorf("the daemon already holds %d standing artifacts", maxStandingArtifacts))
	}
	return nil
}

// notifyArtifactWaiting rides the ask path, since the broker's kind enum is closed.
// The caller holds apps.mu, which NotifyAsk never takes. references/web-push.md
func (d *daemon) notifyArtifactWaiting(s *ptySession, title string) {
	d.users.NotifyAsk(choiceAsk{Session: s.name, From: s.role + " " + s.identity, Question: title})
}

// artifactChange is `artifact_waiting` and `artifact_remove`.
func (d *daemon) artifactChange(cl *client, message frame) error {
	s := d.byToken(message.Token)
	if s == nil {
		return withExit(exitUsage, errors.New("aterm artifact speaks for a session aterm launched, and this token names no live one"))
	}
	id := artifactID(message.ArtifactID)
	d.apps.mu.Lock()
	defer d.apps.mu.Unlock()
	entry := d.apps.artifacts[id]
	if entry == nil {
		return withReason(reasonNoArtifact, fmt.Errorf("no artifact %q", id))
	}
	if !entry.managedBy(s) {
		return withReason(reasonArtifactOwned, errors.New("another live seat posted this artifact"))
	}
	if message.Type == "artifact_remove" {
		d.dropArtifactLocked(entry)
		return cl.c.write(frame{Type: "artifact_removed", ID: message.ID, ArtifactID: id})
	}
	if message.Waiting == nil {
		return withExit(exitUsage, errors.New("artifact_waiting needs waiting true or false"))
	}
	wasWaiting := entry.Waiting
	entry.Waiting = *message.Waiting
	d.persistArtifact(entry)
	d.apps.emit(frame{Type: "artifact_update", Session: entry.Owner, ArtifactID: id, Waiting: message.Waiting})
	if entry.Waiting && !wasWaiting {
		d.notifyArtifactWaiting(s, entry.Title)
	}
	return cl.c.write(frame{Type: "artifact_waiting", ID: message.ID, ArtifactID: id, Waiting: message.Waiting})
}

func (d *daemon) dropArtifactLocked(entry *artifactEntry) {
	delete(d.apps.artifacts, entry.ID)
	d.forgetArtifact(entry.ID)
	d.apps.emit(frame{Type: "artifact_closed", Session: entry.Owner, ArtifactID: entry.ID})
}

// endArtifacts runs when a seat ends. A standing page stays and says its seat is
// gone, and any other page closes with the seat.
func (d *daemon) endArtifacts(s *ptySession) {
	d.apps.mu.Lock()
	defer d.apps.mu.Unlock()
	ended := true
	for _, entry := range slices.Collect(mapValues(d.apps.artifacts)) {
		if !entry.ownedBy(s) {
			continue
		}
		if !entry.Standing {
			d.dropArtifactLocked(entry)
			continue
		}
		entry.SeatEnded = true
		d.persistArtifact(entry)
		d.apps.emit(frame{Type: "artifact_update", Session: entry.Owner, ArtifactID: entry.ID, SeatEnded: &ended})
	}
}

func mapValues[K comparable, V any](m map[K]V) func(func(V) bool) {
	return func(yield func(V) bool) {
		for _, v := range m {
			if !yield(v) {
				return
			}
		}
	}
}

// replayArtifacts sends a new views watcher every page, with the apps lock held.
func (d *daemon) replayArtifacts(c *conn) error {
	for _, entry := range d.apps.artifacts {
		if err := c.write(frame{Type: "artifact", Session: entry.Owner, Artifact: &entry.artifactView}); err != nil {
			return err
		}
	}
	return nil
}

func (d *daemon) artifactFile(id string) string {
	return filepath.Join(d.artifactDir, id+".json")
}

// persistArtifact keeps a page across a daemon restart. A failure is logged, and
// the page still shows until the daemon stops.
func (d *daemon) persistArtifact(entry *artifactEntry) {
	if d.artifactDir == "" {
		return
	}
	encoded, err := json.Marshal(entry)
	if err == nil {
		err = os.MkdirAll(d.artifactDir, 0o700)
	}
	if err == nil {
		temporary := d.artifactFile(entry.ID) + ".tmp"
		if err = os.WriteFile(temporary, encoded, 0o600); err == nil {
			err = os.Rename(temporary, d.artifactFile(entry.ID))
		}
	}
	if err != nil {
		d.logf("could not keep artifact %s across a restart: %v", entry.ID, err)
	}
}

func (d *daemon) forgetArtifact(id string) {
	if d.artifactDir != "" {
		_ = os.Remove(d.artifactFile(id))
	}
}

// restoreArtifacts loads what an earlier daemon kept: adopted seats' pages as posted,
// standing pages of gone seats as ended, and any other page dropped.
func (d *daemon) restoreArtifacts() {
	if d.artifactDir == "" {
		return
	}
	files, _ := filepath.Glob(filepath.Join(d.artifactDir, "*.json"))
	d.apps.mu.Lock()
	defer d.apps.mu.Unlock()
	for _, file := range files {
		raw, err := os.ReadFile(file)
		var entry artifactEntry
		if err == nil {
			err = json.Unmarshal(raw, &entry)
		}
		if err != nil || !artifactURL.MatchString(entry.URL) || entry.ID != artifactID(entry.URL) {
			d.logf("left %s alone, it is not an artifact this daemon wrote", filepath.Base(file))
			continue
		}
		d.mu.Lock()
		live := d.sessions[entry.Owner]
		d.mu.Unlock()
		owned := live != nil && live.started.Equal(entry.Started)
		switch {
		case owned:
			entry.SeatEnded = false
		case entry.Standing:
			entry.SeatEnded = true
		default:
			_ = os.Remove(file)
			continue
		}
		if d.apps.artifacts == nil {
			d.apps.artifacts = map[string]*artifactEntry{}
		}
		d.apps.artifacts[entry.ID] = &entry
	}
}
