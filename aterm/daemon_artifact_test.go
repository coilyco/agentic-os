package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

const testArtifactURL = "https://claude.ai/artifact/J9c4fpaxzmdaZeuuTk27Zr"

// artifactRig is a daemon with a persistence directory, seats whose working
// directories are real temp directories, and a client watching the views channel.
type artifactRig struct {
	t       *testing.T
	d       *daemon
	watcher *conn
}

func newArtifactRig(t *testing.T) *artifactRig {
	t.Helper()
	d, _ := wsDaemon(t)
	d.artifactDir = filepath.Join(t.TempDir(), "artifacts")
	r := &artifactRig{t: t, d: d}
	r.watcher = pipeClient(t, d, peerStanding{pids: []int{os.Getpid()}})
	nextFrame(t, r.watcher, "welcome")
	if _, err := r.watcher.request(frame{Type: "subscribe", Channel: "views"}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	return r
}

// seat starts a session and returns its working directory. With no TMPDIR or shadow
// root in its launch, its only roots are that directory and claude's scratchpad.
func (r *artifactRig) seat(name string) (*ptySession, string) {
	r.t.Helper()
	cwd, err := os.MkdirTemp("/tmp", "aterm-art-")
	if err != nil {
		r.t.Fatal(err)
	}
	r.t.Cleanup(func() { _ = os.RemoveAll(cwd) })
	s, err := startPTYSession(r.d, name, frame{
		Role: "scientist", Identity: name, Seat: "codex",
		Argv: []string{"/bin/sh", "-c", "cat"}, Env: []string{"PATH=" + os.Getenv("PATH")}, Cwd: cwd,
	})
	if err != nil {
		r.t.Fatalf("start: %v", err)
	}
	r.d.mu.Lock()
	r.d.sessions[s.name] = s
	r.d.tokens[s.token] = s
	r.d.mu.Unlock()
	// Ending a seat rewrites its standing pages, so teardown waits for it.
	r.t.Cleanup(func() {
		s.end()
		select {
		case <-s.forgotten:
		case <-time.After(10 * time.Second):
		}
	})
	return s, cwd
}

func (r *artifactRig) call(s *ptySession, request frame) (frame, error) {
	r.t.Helper()
	c := pipeClient(r.t, r.d, peerStanding{pids: []int{os.Getpid()}})
	nextFrame(r.t, c, "welcome")
	request.Token = s.token
	return c.request(request)
}

func (r *artifactRig) add(s *ptySession, cwd, content string, standing bool) (frame, error) {
	r.t.Helper()
	path := filepath.Join(cwd, "page.html")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		r.t.Fatal(err)
	}
	return r.call(s, frame{Type: "artifact_add", URL: testArtifactURL, Title: "Desk", Path: path, Standing: standing})
}

func TestArtifactSourceReachesAWatcherAndReplaysToALateOne(t *testing.T) {
	r := newArtifactRig(t)
	s, cwd := r.seat("scientist-evie")
	source := "<!doctype html><script>if (a < b && c) {}</script>\n"
	reply, err := r.add(s, cwd, source, false)
	if err != nil || reply.Type != "artifact_added" || reply.ArtifactID != "J9c4fpaxzmdaZeuuTk27Zr" || reply.Delivered != 1 {
		t.Fatalf("added: %+v, %v", reply, err)
	}
	pushed := nextFrame(t, r.watcher, "artifact")
	if pushed.Session != "scientist-evie" || pushed.Artifact.HTML != source || pushed.Artifact.Title != "Desk" ||
		pushed.Artifact.URL != testArtifactURL || pushed.Artifact.Waiting || pushed.Artifact.Standing || pushed.Token != "" {
		t.Fatalf("the watcher should get the page labeled with the seat: %+v", pushed)
	}
	late := pipeClient(t, r.d, peerStanding{pids: []int{os.Getpid()}})
	nextFrame(t, late, "welcome")
	if err := late.write(frame{Type: "subscribe", ID: "s", Channel: "views"}); err != nil {
		t.Fatal(err)
	}
	if replay := nextFrame(t, late, "artifact"); replay.Artifact.HTML != source {
		t.Fatalf("a late watcher gets the page on subscribe: %+v", replay)
	}
}

func TestArtifactRefusesASourceOutsideTheSeatsDirectories(t *testing.T) {
	r := newArtifactRig(t)
	s, cwd := r.seat("scientist-evie")
	outside, err := os.MkdirTemp("/tmp", "aterm-art-out-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(outside) })
	secret := filepath.Join(outside, "secret.html")
	if err := os.WriteFile(secret, []byte("<p>secret</p>"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(cwd, "link.html")
	if err := os.Symlink(secret, link); err != nil {
		t.Fatal(err)
	}
	for name, path := range map[string]string{
		"outside":   secret,
		"symlink":   link,
		"dotdot":    filepath.Join(cwd, "..", filepath.Base(outside), "secret.html"),
		"relative":  "page.html",
		"directory": cwd,
		"missing":   filepath.Join(cwd, "nope.html"),
	} {
		reply, err := r.call(s, frame{Type: "artifact_add", URL: testArtifactURL, Title: "T", Path: path})
		if err == nil || reply.Reason != reasonBadPath {
			t.Fatalf("%s should be %s: %+v, %v", name, reasonBadPath, reply, err)
		}
	}
	select {
	case m := <-readAhead(r.watcher):
		t.Fatalf("a refusal must push nothing, got %+v", m)
	case <-time.After(150 * time.Millisecond):
	}
}

// readAhead reads one frame off c, so a test can assert that none arrived.
func readAhead(c *conn) <-chan frame {
	out := make(chan frame, 1)
	go func() {
		if m, err := c.read(); err == nil {
			out <- m
		}
	}()
	return out
}

func TestArtifactChecksSizeTextAndTitleBeforeShowingAnything(t *testing.T) {
	r := newArtifactRig(t)
	s, cwd := r.seat("scientist-evie")
	if reply, err := r.add(s, cwd, strings.Repeat("x", maxArtifactBytes), false); err != nil || reply.Type != "artifact_added" {
		t.Fatalf("exactly the cap is taken: %+v, %v", reply, err)
	}
	reply, err := r.add(s, cwd, strings.Repeat("x", maxArtifactBytes+1), false)
	if err == nil || reply.Reason != reasonTooLarge {
		t.Fatalf("one byte over is %s: %+v, %v", reasonTooLarge, reply, err)
	}
	reply, err = r.add(s, cwd, "ok\xff\xfe", false)
	if err == nil || reply.Reason != reasonNotText {
		t.Fatalf("bytes that are not UTF-8 are %s: %+v, %v", reasonNotText, reply, err)
	}
	path := filepath.Join(cwd, "page.html")
	_ = os.WriteFile(path, []byte("<p>ok</p>"), 0o600)
	for _, title := range []string{"", "   ", "a\nb", strings.Repeat("t", maxArtifactTitle+1)} {
		if reply, err := r.call(s, frame{Type: "artifact_add", URL: testArtifactURL, Title: title, Path: path}); err == nil {
			t.Fatalf("title %q should be refused: %+v", title, reply)
		}
	}
}

func TestArtifactRefusesAnAddressThatIsNotAClaudeArtifact(t *testing.T) {
	r := newArtifactRig(t)
	s, cwd := r.seat("scientist-evie")
	path := filepath.Join(cwd, "page.html")
	_ = os.WriteFile(path, []byte("<p>ok</p>"), 0o600)
	for _, address := range []string{
		"https://example.com/artifact/abc",
		"http://claude.ai/artifact/abc",
		"https://claude.ai.evil.example/artifact/abc",
		"https://claude.ai/artifact/",
		"https://claude.ai/artifact/abc/",
		"https://claude.ai/artifact/abc?x=1",
		"https://claude.ai/artifact/abc#top",
		"https://user@claude.ai/artifact/abc",
		"https://claude.ai/artifacts/abc",
		"javascript:alert(1)",
		"",
	} {
		reply, err := r.call(s, frame{Type: "artifact_add", URL: address, Title: "T", Path: path})
		if err == nil || reply.Reason != reasonBadURL {
			t.Fatalf("%q should be %s: %+v, %v", address, reasonBadURL, reply, err)
		}
	}
}

func TestArtifactNeedsALiveSeatToken(t *testing.T) {
	r := newArtifactRig(t)
	c := pipeClient(t, r.d, peerStanding{pids: []int{os.Getpid()}})
	nextFrame(t, c, "welcome")
	for _, token := range []string{"", "not-a-token"} {
		if reply, err := c.request(frame{Type: "artifact_add", Token: token, URL: testArtifactURL, Title: "T", Path: "/tmp/x"}); err == nil {
			t.Fatalf("token %q names no seat: %+v", token, reply)
		}
	}
}

func TestArtifactAddAgainReplacesAndWaitingAndRemoveFollow(t *testing.T) {
	r := newArtifactRig(t)
	s, cwd := r.seat("scientist-evie")
	_, _ = r.add(s, cwd, "<p>one</p>", false)
	nextFrame(t, r.watcher, "artifact")
	if _, err := r.add(s, cwd, "<p>two</p>", false); err != nil {
		t.Fatal(err)
	}
	if again := nextFrame(t, r.watcher, "artifact"); again.Artifact.HTML != "<p>two</p>" {
		t.Fatalf("the same address replaces the page: %+v", again)
	}
	r.d.apps.mu.Lock()
	held := len(r.d.apps.artifacts)
	r.d.apps.mu.Unlock()
	if held != 1 {
		t.Fatalf("a replace must not add a second page: %d", held)
	}
	yes, no := true, false
	for _, waiting := range []*bool{&yes, &no} {
		if _, err := r.call(s, frame{Type: "artifact_waiting", ArtifactID: testArtifactURL, Waiting: waiting}); err != nil {
			t.Fatal(err)
		}
		update := nextFrame(t, r.watcher, "artifact_update")
		if update.ArtifactID != "J9c4fpaxzmdaZeuuTk27Zr" || update.Waiting == nil || *update.Waiting != *waiting || update.Session != "scientist-evie" {
			t.Fatalf("waiting %v: %+v", *waiting, update)
		}
	}
	if _, err := r.call(s, frame{Type: "artifact_remove", ArtifactID: "J9c4fpaxzmdaZeuuTk27Zr"}); err != nil {
		t.Fatal(err)
	}
	if closed := nextFrame(t, r.watcher, "artifact_closed"); closed.ArtifactID != "J9c4fpaxzmdaZeuuTk27Zr" {
		t.Fatalf("%+v", closed)
	}
	if reply, err := r.call(s, frame{Type: "artifact_remove", ArtifactID: "J9c4fpaxzmdaZeuuTk27Zr"}); err == nil || reply.Reason != reasonNoArtifact {
		t.Fatalf("a removed page is gone: %+v, %v", reply, err)
	}
}

func TestArtifactIsOnlyChangedByItsOwnerWhileThatSeatLives(t *testing.T) {
	r := newArtifactRig(t)
	owner, cwd := r.seat("scientist-evie")
	other, otherCwd := r.seat("scientist-beth")
	_, _ = r.add(owner, cwd, "<p>mine</p>", true)
	no := false
	for name, request := range map[string]frame{
		"waiting": {Type: "artifact_waiting", ArtifactID: testArtifactURL, Waiting: &no},
		"remove":  {Type: "artifact_remove", ArtifactID: testArtifactURL},
	} {
		if reply, err := r.call(other, request); err == nil || reply.Reason != reasonArtifactOwned {
			t.Fatalf("%s by another live seat is %s: %+v, %v", name, reasonArtifactOwned, reply, err)
		}
	}
	if reply, err := r.add(other, otherCwd, "<p>theirs</p>", true); err == nil || reply.Reason != reasonArtifactOwned {
		t.Fatalf("a replace by another live seat is %s: %+v, %v", reasonArtifactOwned, reply, err)
	}
}

func TestArtifactWhenItsSeatEndsStandingStaysAndOthersClose(t *testing.T) {
	r := newArtifactRig(t)
	s, cwd := r.seat("scientist-evie")
	successor, successorCwd := r.seat("scientist-beth")
	if _, err := r.add(s, cwd, "<p>desk</p>", true); err != nil {
		t.Fatal(err)
	}
	nextFrame(t, r.watcher, "artifact")
	path := filepath.Join(cwd, "plain.html")
	_ = os.WriteFile(path, []byte("<p>plain</p>"), 0o600)
	if _, err := r.call(s, frame{Type: "artifact_add", URL: "https://claude.ai/artifact/plain", Title: "Plain", Path: path}); err != nil {
		t.Fatal(err)
	}
	nextFrame(t, r.watcher, "artifact")

	s.end()
	var sawClosed, sawEnded bool
	for i := 0; i < 2; i++ {
		m := nextFrame(t, r.watcher, "artifact_closed", "artifact_update")
		switch {
		case m.Type == "artifact_closed" && m.ArtifactID == "plain":
			sawClosed = true
		case m.Type == "artifact_update" && m.ArtifactID == "J9c4fpaxzmdaZeuuTk27Zr" && m.SeatEnded != nil && *m.SeatEnded:
			sawEnded = true
		default:
			t.Fatalf("unexpected: %+v", m)
		}
	}
	if !sawClosed || !sawEnded {
		t.Fatalf("a plain page closes and a standing one is marked ended: closed=%v ended=%v", sawClosed, sawEnded)
	}
	r.d.apps.mu.Lock()
	kept := r.d.apps.artifacts[artifactID(testArtifactURL)]
	r.d.apps.mu.Unlock()
	if kept == nil || kept.Owner != "scientist-evie" || !kept.SeatEnded {
		t.Fatalf("the standing page keeps the posting seat's name as provenance: %+v", kept)
	}

	if _, err := r.add(successor, successorCwd, "<p>adopted</p>", true); err != nil {
		t.Fatalf("a later seat takes over a standing page: %v", err)
	}
	adopted := nextFrame(t, r.watcher, "artifact")
	if adopted.Session != "scientist-beth" || adopted.Artifact.SeatEnded || adopted.Artifact.HTML != "<p>adopted</p>" {
		t.Fatalf("the takeover is a full page labeled with the new seat: %+v", adopted)
	}
}

func TestArtifactAnyLiveSeatMayRemoveAStandingPageWhoseSeatEnded(t *testing.T) {
	r := newArtifactRig(t)
	s, cwd := r.seat("scientist-evie")
	other, _ := r.seat("scientist-beth")
	_, _ = r.add(s, cwd, "<p>desk</p>", true)
	s.end()
	if _, err := r.call(other, frame{Type: "artifact_remove", ArtifactID: testArtifactURL}); err != nil {
		t.Fatalf("an ended owner leaves the page to any seat: %v", err)
	}
}

func TestArtifactLimitsPagesPerSeatAndStandingPagesInAll(t *testing.T) {
	r := newArtifactRig(t)
	s, cwd := r.seat("scientist-evie")
	path := filepath.Join(cwd, "page.html")
	_ = os.WriteFile(path, []byte("<p>x</p>"), 0o600)
	add := func(i int, standing bool) (frame, error) {
		return r.call(s, frame{Type: "artifact_add", URL: fmt.Sprintf("https://claude.ai/artifact/p%d", i), Title: "T", Path: path, Standing: standing})
	}
	for i := 0; i < maxArtifactsPerSeat; i++ {
		if _, err := add(i, i < maxStandingArtifacts); err != nil {
			t.Fatalf("page %d should fit: %v", i, err)
		}
	}
	if reply, err := add(99, false); err == nil || reply.Reason != reasonTooMany {
		t.Fatalf("one past the per-seat limit is %s: %+v, %v", reasonTooMany, reply, err)
	}
	if _, err := add(0, true); err != nil {
		t.Fatalf("replacing a page never counts as another: %v", err)
	}
	other, otherCwd := r.seat("scientist-beth")
	_ = os.WriteFile(filepath.Join(otherCwd, "page.html"), []byte("<p>x</p>"), 0o600)
	reply, err := r.call(other, frame{Type: "artifact_add", URL: "https://claude.ai/artifact/extra", Title: "T", Path: filepath.Join(otherCwd, "page.html"), Standing: true})
	if err == nil || reply.Reason != reasonTooMany {
		t.Fatalf("the daemon-wide standing limit is %s: %+v, %v", reasonTooMany, reply, err)
	}
}

func TestStandingArtifactsSurviveARestartAndOthersDoNot(t *testing.T) {
	r := newArtifactRig(t)
	s, cwd := r.seat("scientist-evie")
	_, _ = r.add(s, cwd, "<p>desk</p>", true)
	path := filepath.Join(cwd, "plain.html")
	_ = os.WriteFile(path, []byte("<p>plain</p>"), 0o600)
	_, _ = r.call(s, frame{Type: "artifact_add", URL: "https://claude.ai/artifact/plain", Title: "Plain", Path: path})

	for path, want := range map[string]os.FileMode{r.d.artifactDir: 0o700, r.d.artifactFile("J9c4fpaxzmdaZeuuTk27Zr"): 0o600} {
		if info, err := os.Stat(path); err != nil || info.Mode().Perm() != want {
			t.Fatalf("%s should be %v: %v, %v", path, want, info, err)
		}
	}
	junk := filepath.Join(r.d.artifactDir, "junk.json")
	_ = os.WriteFile(junk, []byte("not json"), 0o600)

	restart := func(live *ptySession) *daemon {
		d := newDaemon(func(string, ...any) {})
		d.artifactDir = r.d.artifactDir
		if live != nil {
			d.sessions[live.name] = live
		}
		d.restoreArtifacts()
		return d
	}
	adopted := restart(s)
	if got := adopted.apps.artifacts["J9c4fpaxzmdaZeuuTk27Zr"]; got == nil || got.SeatEnded || adopted.apps.artifacts["plain"] == nil {
		t.Fatalf("pages of a seat the new daemon adopted come back as posted: %+v", adopted.apps.artifacts)
	}
	orphaned := restart(nil)
	if got := orphaned.apps.artifacts["J9c4fpaxzmdaZeuuTk27Zr"]; got == nil || !got.SeatEnded || got.HTML != "<p>desk</p>" {
		t.Fatalf("a standing page whose seat is gone comes back ended: %+v", got)
	}
	if orphaned.apps.artifacts["plain"] != nil {
		t.Fatal("a plain page does not outlive its seat's restart")
	}
	if _, err := os.Stat(r.d.artifactFile("plain")); !os.IsNotExist(err) {
		t.Fatalf("the dropped page's file should go: %v", err)
	}
	if _, err := os.Stat(junk); err != nil {
		t.Fatalf("a file the daemon did not write is left alone: %v", err)
	}
	if !slices.Contains(welcomeFeatures(t), artifactsFeature) {
		t.Fatalf("welcome should offer %q", artifactsFeature)
	}
}

func welcomeFeatures(t *testing.T) []string {
	t.Helper()
	d, _ := wsDaemon(t)
	c := pipeClient(t, d, peerStanding{pids: []int{os.Getpid()}})
	return nextFrame(t, c, "welcome").Features
}

func TestArtifactSourceNeverReachesTheLog(t *testing.T) {
	r := newArtifactRig(t)
	var logged strings.Builder
	var mu = make(chan struct{}, 1)
	r.d.logf = func(format string, args ...any) {
		mu <- struct{}{}
		defer func() { <-mu }()
		fmt.Fprintf(&logged, format+"\n", args...)
	}
	s, cwd := r.seat("scientist-evie")
	marker := "UNIQUE-ARTIFACT-MARKER"
	if _, err := r.add(s, cwd, "<p>"+marker+"</p>", true); err != nil {
		t.Fatal(err)
	}
	mu <- struct{}{}
	text := logged.String()
	<-mu
	if strings.Contains(text, marker) || strings.Contains(text, cwd) {
		t.Fatalf("the log must hold neither the source nor its path:\n%s", text)
	}
}

func TestArtifactTurningToWaitingPushesOnceAsAnAsk(t *testing.T) {
	r := newArtifactRig(t)
	r.d.roster = flaggedRoster(t, "scientist")
	store, _ := loadPushStore("")
	if err := r.d.local().setupPush(mustKey(t), store); err != nil {
		t.Fatal(err)
	}
	sub, _, _ := testSubscription(t, "https://fcm.googleapis.com/fcm/send/phone")
	_ = store.add(sub)
	sent := make(chan pushPayload, 8)
	r.d.local().push.send = func(_ context.Context, _ *vapidKey, _ pushSubscription, payload []byte) (int, error) {
		var got pushPayload
		if err := json.Unmarshal(payload, &got); err != nil {
			t.Errorf("payload: %v", err)
		}
		sent <- got
		return http.StatusCreated, nil
	}
	brokered := make(chan waitEvent, 8)
	r.d.local().brokerEvent = func(e waitEvent) { brokered <- e }
	none := func(step string) {
		t.Helper()
		select {
		case got := <-sent:
			t.Fatalf("%s should push nothing: %+v", step, got)
		case got := <-brokered:
			t.Fatalf("%s should tell the broker nothing: %+v", step, got)
		case <-time.After(150 * time.Millisecond):
		}
	}
	s, cwd := r.seat("scientist-evie")
	path := filepath.Join(cwd, "page.html")
	if err := os.WriteFile(path, []byte("<p>q</p>"), 0o600); err != nil {
		t.Fatal(err)
	}
	yes, no := true, false
	add := func(waiting *bool) {
		t.Helper()
		if _, err := r.call(s, frame{Type: "artifact_add", URL: testArtifactURL, Title: "Desk", Path: path, Waiting: waiting}); err != nil {
			t.Fatal(err)
		}
	}
	flip := func(waiting *bool) {
		t.Helper()
		if _, err := r.call(s, frame{Type: "artifact_waiting", ArtifactID: testArtifactURL, Waiting: waiting}); err != nil {
			t.Fatal(err)
		}
	}

	add(&no)
	none("a page that does not wait on Kai")
	add(&yes)
	select {
	case got := <-sent:
		if got.Kind != "ask" || got.Session != s.name || got.Title != "scientist-evie is asking" || got.Body != "Desk" || got.Tag != "aterm-"+s.name {
			t.Fatalf("a page added waiting pushes as an ask naming its title: %+v", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a page added waiting on Kai never pushed")
	}
	if got := <-brokered; got.Kind != "ask" || got.Session != s.name || got.Identity != "scientist-evie" || got.Question != "Desk" {
		t.Fatalf("the broker hears the same ask, its question gated by the owner's setting later: %+v", got)
	}
	add(&yes)
	none("replacing a page that already waits")
	flip(&yes)
	none("marking a waiting page waiting again")
	flip(&no)
	none("clearing waiting")
	flip(&yes)
	select {
	case got := <-sent:
		if got.Kind != "ask" || got.Body != "Desk" {
			t.Fatalf("flipping a page to waiting pushes: %+v", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a page flipped to waiting never pushed")
	}
}
