package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var jpegMagic = []byte("\xff\xd8\xff\xe0")

type feedbackRig struct {
	d        *daemon
	c        *conn
	root     string
	launches *atomic.Int32
	log      func() string
}

func newFeedbackRig(t *testing.T) *feedbackRig {
	t.Helper()
	d, _ := wsDaemon(t)
	d.feedbackDir = filepath.Join(t.TempDir(), "feedback")
	launches := &atomic.Int32{}
	d.launch = func(role, seat string) error { launches.Add(1); return nil }
	// The real lister shells out for every typing guard, about a second apiece.
	d.processes = func() ([]processEntry, error) { return []processEntry{{PID: os.Getpid(), PPID: 1}}, nil }
	var mu sync.Mutex
	logged := &strings.Builder{}
	d.logf = func(format string, args ...any) {
		mu.Lock()
		defer mu.Unlock()
		fmt.Fprintf(logged, format+"\n", args...)
	}
	grace := boxGrace
	t.Cleanup(func() { boxGrace = grace })
	boxGrace = 300 * time.Millisecond
	c := pipeClient(t, d, peerStanding{pids: []int{os.Getpid()}})
	nextFrame(t, c, "welcome")
	return &feedbackRig{d: d, c: c, root: d.feedbackDir, launches: launches, log: func() string {
		mu.Lock()
		defer mu.Unlock()
		return logged.String()
	}}
}

func (r *feedbackRig) stage(data []byte) (frame, error) {
	return r.c.request(frame{Type: "feedback_image", Data: data})
}

func (r *feedbackRig) send(text string, images ...string) (frame, error) {
	screen := &feedbackScreen{View: "seat sprite", Host: "kai-server", Viewport: feedbackViewport{Width: 360, Height: 800, Ratio: 3}, Build: "abc1234", App: true, At: "2026-11-04T21:00:00Z"}
	return r.c.request(frame{Type: "feedback", Text: text, Screen: screen, Images: images})
}

func (r *feedbackRig) reports(t *testing.T) []string {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join(r.root, "*", feedbackReportFile))
	return files
}

// feedbackSeat opens a frontend-eng seat through spawn, the path a launched seat takes,
// and returns a reader of everything typed into it.
func feedbackSeat(t *testing.T, d *daemon) func(want string) {
	t.Helper()
	script := `printf 'Try "\r\033[2K'; printf 'UP\033[?2004h\n'; cat`
	s, err := d.spawn(frame{
		Session: "frontend-eng-sprite", Role: "frontend-eng", Identity: "Sprite", Seat: "claude",
		Argv: []string{"/bin/sh", "-c", script}, Env: os.Environ(), Cwd: "/", Rows: 24, Cols: 200,
	})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	t.Cleanup(s.end)
	watcher := pipeClient(t, d, peerStanding{pids: []int{os.Getpid()}})
	nextFrame(t, watcher, "welcome")
	if _, err := watcher.request(frame{Type: "attach", Session: s.name, Rows: 24, Cols: 200, Replay: true}); err != nil {
		t.Fatalf("attach: %v", err)
	}
	var seen bytes.Buffer
	return func(want string) {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for !strings.Contains(seen.String(), want) {
			if time.Now().After(deadline) {
				t.Fatalf("the seat never received %q:\n%q", want, seen.String())
			}
			message := nextFrame(t, watcher, "output")
			seen.Write(message.Data)
		}
	}
}

func readReport(t *testing.T, path string) feedbackReport {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var report feedbackReport
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatal(err)
	}
	return report
}

func TestFeedbackKeepsANoteAndTwoImagesAsAPrivateRecord(t *testing.T) {
	r := newFeedbackRig(t)
	png, jpeg := imageBytes(pngMagic, 2048), imageBytes(jpegMagic, 1024)
	first, err := r.stage(png)
	if err != nil || first.Type != "feedback_image_saved" || first.Image == "" {
		t.Fatalf("a PNG should be staged: %+v, %v", first, err)
	}
	second, err := r.stage(jpeg)
	if err != nil || second.Image == first.Image {
		t.Fatalf("each image gets its own token: %+v, %v", second, err)
	}
	reply, err := r.send("the sheet covers the composer", first.Image, second.Image)
	if err != nil || reply.Type != "feedback_saved" || reply.Record == "" {
		t.Fatalf("the report should be kept: %+v, %v", reply, err)
	}
	dir := filepath.Join(r.root, reply.Record)
	if filepath.Dir(dir) != r.d.feedbackDir {
		t.Fatalf("the record belongs under the feedback directory: %q", dir)
	}
	report := readReport(t, filepath.Join(dir, feedbackReportFile))
	if report.ID != reply.Record || report.Text != "the sheet covers the composer" || len(report.Images) != 2 ||
		report.Screen.View != "seat sprite" || report.Screen.Viewport.Width != 360 || !report.Screen.App {
		t.Fatalf("report.json should carry the note, the screen and two images: %+v", report)
	}
	for index, want := range [][]byte{png, jpeg} {
		got, err := os.ReadFile(filepath.Join(dir, report.Images[index]))
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("image %d should hold the uploaded bytes: %v", index+1, err)
		}
	}
	if filepath.Ext(report.Images[0]) != ".png" || filepath.Ext(report.Images[1]) != ".jpg" {
		t.Fatalf("the signature names the extension: %v", report.Images)
	}
	for path, want := range map[string]os.FileMode{
		r.root: 0o700, dir: 0o700, filepath.Join(dir, feedbackReportFile): 0o600,
		filepath.Join(dir, report.Images[0]): 0o600, filepath.Join(dir, report.Images[1]): 0o600,
	} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != want {
			t.Fatalf("%s should be %v: %v, %v", path, want, info, err)
		}
	}
	if left, _ := os.ReadDir(filepath.Join(r.root, feedbackStagingDir)); len(left) != 0 {
		t.Fatalf("a closed report leaves nothing staged: %v", left)
	}
	if _, err := r.send("again", first.Image); err == nil {
		t.Fatal("a token is spent once it closes a report")
	}
}

func TestFeedbackTakesANoteWithNoImageAndAnImageWithNoNote(t *testing.T) {
	r := newFeedbackRig(t)
	if reply, err := r.send("only words"); err != nil || reply.Record == "" {
		t.Fatalf("a note alone is a report: %+v, %v", reply, err)
	}
	staged, err := r.stage(imageBytes(pngMagic, 64))
	if err != nil {
		t.Fatal(err)
	}
	if reply, err := r.send("", staged.Image); err != nil || reply.Record == "" {
		t.Fatalf("a screenshot alone is a report: %+v, %v", reply, err)
	}
	if reply, err := r.send("  "); err == nil {
		t.Fatalf("a report with neither is refused: %+v", reply)
	}
}

func TestFeedbackRefusalsNameTheirReason(t *testing.T) {
	r := newFeedbackRig(t)
	t.Run("not an image", func(t *testing.T) {
		for name, data := range map[string][]byte{"text": []byte("#!/bin/sh\n"), "empty": nil, "html": []byte("<html>")} {
			reply, err := r.stage(data)
			if err == nil || reply.Reason != reasonNotAnImage {
				t.Fatalf("%s should be %s: %+v, %v", name, reasonNotAnImage, reply, err)
			}
		}
	})
	t.Run("too many in one report", func(t *testing.T) {
		tokens := make([]string, 0, maxFeedbackImages+1)
		for range maxFeedbackImages + 1 {
			staged, err := r.stage(imageBytes(pngMagic, 64))
			if err != nil {
				t.Fatal(err)
			}
			tokens = append(tokens, staged.Image)
		}
		reply, err := r.send("five", tokens...)
		if err == nil || reply.Reason != reasonTooMany {
			t.Fatalf("five images should be %s: %+v, %v", reasonTooMany, reply, err)
		}
		if reply, err := r.send("four", tokens[:maxFeedbackImages]...); err != nil || reply.Record == "" {
			t.Fatalf("a refusal keeps the tokens, so the retry with four works: %+v, %v", reply, err)
		}
	})
	t.Run("too many waiting", func(t *testing.T) {
		var last error
		var reply frame
		for range maxStagedImages + 1 {
			reply, last = r.stage(imageBytes(pngMagic, 64))
		}
		if last == nil || reply.Reason != reasonTooMany {
			t.Fatalf("a connection that never closes its report is held to %d screenshots: %+v, %v", maxStagedImages, reply, last)
		}
	})
	t.Run("text too long", func(t *testing.T) {
		long := strings.Repeat("é", maxFeedbackText+1)
		reply, err := r.send(long)
		if err == nil || reply.Reason != reasonTextTooLong {
			t.Fatalf("over %d characters should be %s: %+v, %v", maxFeedbackText, reasonTextTooLong, reply, err)
		}
		if reply, err := r.send(strings.Repeat("é", maxFeedbackText)); err != nil || reply.Record == "" {
			t.Fatalf("%d characters is the limit and fits, though it is %d bytes: %+v, %v", maxFeedbackText, 2*maxFeedbackText, reply, err)
		}
	})
	t.Run("an unknown token", func(t *testing.T) {
		reply, err := r.send("x", "not-a-token")
		if err == nil || reply.Reason != reasonImageExpired {
			t.Fatalf("a token the daemon never gave should be %s: %+v, %v", reasonImageExpired, reply, err)
		}
	})
	t.Run("another connection's token", func(t *testing.T) {
		r := newFeedbackRig(t)
		staged, err := r.stage(imageBytes(pngMagic, 64))
		if err != nil {
			t.Fatal(err)
		}
		other := pipeClient(t, r.d, peerStanding{pids: []int{os.Getpid()}})
		nextFrame(t, other, "welcome")
		reply, err := other.request(frame{Type: "feedback", Text: "x", Images: []string{staged.Image}})
		if err == nil || reply.Reason != reasonImageExpired {
			t.Fatalf("a token belongs to the connection that staged it: %+v, %v", reply, err)
		}
	})
	t.Run("a lapsed token", func(t *testing.T) {
		r := newFeedbackRig(t)
		staged, err := r.stage(imageBytes(pngMagic, 64))
		if err != nil {
			t.Fatal(err)
		}
		r.d.feedback.mu.Lock()
		image := r.d.feedback.staged[staged.Image]
		image.expires = time.Now().Add(-time.Second)
		r.d.feedback.staged[staged.Image] = image
		r.d.feedback.mu.Unlock()
		reply, err := r.send("x", staged.Image)
		if err == nil || reply.Reason != reasonImageExpired {
			t.Fatalf("a token is good ten minutes: %+v, %v", reply, err)
		}
		if _, err := os.Stat(image.path); !os.IsNotExist(err) {
			t.Fatalf("a lapsed screenshot is removed: %v", err)
		}
	})
}

// Alone, because a frame this size is slow under -race and a late reply would
// otherwise answer the next request on the same connection.
func TestFeedbackChecksTheSizeBeforeAnythingIsWritten(t *testing.T) {
	r := newFeedbackRig(t)
	reply, err := r.stage(imageBytes(pngMagic, maxImageBytes+1))
	if err == nil || reply.Reason != reasonTooLarge {
		t.Fatalf("one byte over the cap should be %s: %+v, %v", reasonTooLarge, reply, err)
	}
	if _, err := os.Stat(r.root); !os.IsNotExist(err) {
		t.Fatalf("a refusal must write nothing, not even the directory: %v", err)
	}
	if reply, err := r.stage(imageBytes(pngMagic, maxImageBytes)); err != nil || reply.Type != "feedback_image_saved" {
		t.Fatalf("exactly the cap should be taken: %+v, %v", reply, err)
	}
}

func TestFeedbackKeepsTheTypingGuards(t *testing.T) {
	t.Run("a locked remote device", func(t *testing.T) {
		r := newFeedbackRig(t)
		device := pipeClient(t, r.d, peerStanding{web: true, remote: true})
		nextFrame(t, device, "welcome")
		for _, message := range []frame{
			{Type: "feedback_image", Data: imageBytes(pngMagic, 64)},
			{Type: "feedback", Text: "hello"},
		} {
			reply, err := device.request(message)
			if err == nil || reply.Reason != reasonPasskeyRequired {
				t.Fatalf("%s from a device that has not unlocked should be %s: %+v, %v", message.Type, reasonPasskeyRequired, reply, err)
			}
		}
		if _, err := os.Stat(r.root); !os.IsNotExist(err) {
			t.Fatalf("a refused device must write nothing: %v", err)
		}
	})
	t.Run("a process inside a session", func(t *testing.T) {
		r := newFeedbackRig(t)
		wsSession(t, r.d, true)
		for _, message := range []frame{
			{Type: "feedback_image", Data: imageBytes(pngMagic, 64)},
			{Type: "feedback", Text: "hello"},
		} {
			reply, err := r.c.request(message)
			if err == nil || reply.Reason != reasonSessionDescendant {
				t.Fatalf("%s from inside a session should be %s: %+v, %v", message.Type, reasonSessionDescendant, reply, err)
			}
		}
		if _, err := os.Stat(r.root); !os.IsNotExist(err) {
			t.Fatalf("a refused caller must write nothing: %v", err)
		}
	})
}

func TestFeedbackIsOfferedOnlyByADaemonThatKeepsIt(t *testing.T) {
	d, _ := wsDaemon(t)
	c := pipeClient(t, d, peerStanding{pids: []int{os.Getpid()}})
	if welcome := nextFrame(t, c, "welcome"); slices.Contains(welcome.Features, feedbackFeature) {
		t.Fatalf("no feedback directory, so no %s: %v", feedbackFeature, welcome.Features)
	}
	if reply, err := c.request(frame{Type: "feedback", Text: "x"}); err == nil || reply.Reason != "" {
		t.Fatalf("a daemon without the build answers an unknown frame: %+v, %v", reply, err)
	}
	d.feedbackDir = filepath.Join(t.TempDir(), "feedback")
	offered := pipeClient(t, d, peerStanding{pids: []int{os.Getpid()}})
	if welcome := nextFrame(t, offered, "welcome"); !slices.Contains(welcome.Features, feedbackFeature) {
		t.Fatalf("welcome should offer %q: %v", feedbackFeature, welcome.Features)
	}
}

// Through runDaemon, where the directory is derived from the state directory and so
// sits beside the session ledger and never beside the socket.
func TestRunningDaemonKeepsFeedbackInTheStateDirectoryNotBesideTheSocket(t *testing.T) {
	state := filepath.Join(t.TempDir(), "state")
	t.Setenv(stateDirEnv, filepath.Join(state, "sessions"))
	socket := testDaemon(t)
	tc := dialTest(t)
	if !slices.Contains(tc.c.features, feedbackFeature) {
		t.Fatalf("welcome should offer %q: %v", feedbackFeature, tc.c.features)
	}
	// Only a role with no live seat would launch one, so a seat is up before the send.
	tc.spawnSeat("frontend-eng-sprite", "frontend-eng", "Sprite", "claude", `printf 'UP\033[?2004h\n'; cat`)
	staged, err := tc.c.request(frame{Type: "feedback_image", Data: imageBytes(pngMagic, 256)})
	if err != nil {
		t.Fatal(err)
	}
	saved, err := tc.c.request(frame{Type: "feedback", Text: "kept", Images: []string{staged.Image}})
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(state, "feedback", saved.Record, feedbackReportFile)
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("the record belongs in the state directory: %v", err)
	}
	if strings.HasPrefix(want, filepath.Dir(socket)) {
		t.Fatalf("the record must not live beside the socket %s: %s", socket, want)
	}
	tc.until(saved.Record)
	tc.until(filepath.Join(state, "feedback", saved.Record, "image-1.png"))
}

func TestFeedbackSurvivesARestartAndIsHandedToTheSeatThatOpens(t *testing.T) {
	r := newFeedbackRig(t)
	staged, err := r.stage(imageBytes(pngMagic, 512))
	if err != nil {
		t.Fatal(err)
	}
	// A staged screenshot that never closed a report, and a write that never finished.
	if _, err := r.stage(imageBytes(pngMagic, 64)); err != nil {
		t.Fatal(err)
	}
	reply, err := r.send("after the restart", staged.Image)
	if err != nil {
		t.Fatal(err)
	}
	unfinished := filepath.Join(r.root, "20260101-000000-abc")
	if err := os.Mkdir(unfinished, 0o700); err != nil {
		t.Fatal(err)
	}

	// A second daemon over the same directory is a restart: nothing of the first is shared.
	restarted := newDaemon(func(string, ...any) {})
	restarted.holdDir = testHoldDir(t)
	restarted.feedbackDir = r.root
	restarted.launch = func(role, seat string) error { t.Error("a start must not launch a seat for old reports"); return nil }
	restarted.sweepFeedback()
	restarted.handWaitingFeedback()
	report := readReport(t, filepath.Join(r.root, reply.Record, feedbackReportFile))
	if report.Text != "after the restart" || report.Handed != "" {
		t.Fatalf("the report outlives the daemon, still unhanded: %+v", report)
	}
	if _, err := os.Stat(filepath.Join(r.root, reply.Record, report.Images[0])); err != nil {
		t.Fatalf("its image outlives the daemon: %v", err)
	}
	for _, gone := range []string{filepath.Join(r.root, feedbackStagingDir), unfinished} {
		if _, err := os.Stat(gone); !os.IsNotExist(err) {
			t.Fatalf("%s is left over from the last daemon and should be swept: %v", gone, err)
		}
	}

	// The first frontend-eng seat to open is handed it, with the screenshot's absolute path.
	seat := feedbackSeat(t, restarted)
	seat(reply.Record)
	seat(filepath.Join(r.root, reply.Record, report.Images[0]))
	seat("after the restart")
	waitFor(t, "the report marked handed", 10*time.Second, func() bool {
		return readReport(t, filepath.Join(r.root, reply.Record, feedbackReportFile)).Handed != ""
	})
	if handed := readReport(t, filepath.Join(r.root, reply.Record, feedbackReportFile)); handed.Seat != "frontend-eng-sprite" {
		t.Fatalf("the record should name the seat that took it: %+v", handed)
	}
}

func TestFeedbackWithNoSeatAwakeIsKeptAndHandedToALaterOne(t *testing.T) {
	r := newFeedbackRig(t)
	reply, err := r.send("nobody home")
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "a seat to be asked for", 5*time.Second, func() bool { return r.launches.Load() == 1 })
	if len(r.reports(t)) != 1 {
		t.Fatalf("the report is on disk before any seat exists: %v", r.reports(t))
	}
	// The launch window passes with no seat, which fails the message and keeps the record.
	r.d.expireOrphans(time.Now().Add(launchWait + time.Minute))
	waitFor(t, "the failed hand-off to release", 5*time.Second, func() bool {
		r.d.feedback.mu.Lock()
		defer r.d.feedback.mu.Unlock()
		return !r.d.feedback.inflight[reply.Record]
	})
	if handed := readReport(t, r.reports(t)[0]); handed.Handed != "" {
		t.Fatalf("nothing took it, so it stays unhanded: %+v", handed)
	}
	seat := feedbackSeat(t, r.d)
	seat("nobody home")
	seat(reply.Record)
	waitFor(t, "the report marked handed", 10*time.Second, func() bool { return readReport(t, r.reports(t)[0]).Handed != "" })
	if r.launches.Load() != 1 {
		t.Fatalf("a later seat is handed the report without another launch: %d", r.launches.Load())
	}
}

func TestFeedbackReachesASeatThatAlreadyAnswersToTheRole(t *testing.T) {
	r := newFeedbackRig(t)
	seat := feedbackSeat(t, r.d)
	staged, err := r.stage(imageBytes(pngMagic, 128))
	if err != nil {
		t.Fatal(err)
	}
	reply, err := r.send("live seat", staged.Image)
	if err != nil {
		t.Fatal(err)
	}
	seat("[from aterm feedback]")
	seat(filepath.Join(r.root, reply.Record, "image-1.png"))
	seat("attach each screenshot")
	if r.launches.Load() != 0 {
		t.Fatalf("a live seat needs no launch: %d", r.launches.Load())
	}
}

func TestFeedbackNoteAndBytesNeverReachTheLog(t *testing.T) {
	r := newFeedbackRig(t)
	note, marker := "SECRET-NOTE-MARKER", "UNIQUE-PIXEL-MARKER"
	data := append(append([]byte(nil), pngMagic...), []byte(marker)...)
	staged, err := r.stage(data)
	if err != nil {
		t.Fatal(err)
	}
	reply, err := r.send(note, staged.Image)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.stage([]byte(marker)); err == nil {
		t.Fatal("not an image")
	}
	log := r.log()
	for _, leaked := range []string{note, marker} {
		if strings.Contains(log, leaked) {
			t.Fatalf("the daemon log must hold no note text and no image bytes:\n%s", log)
		}
	}
	for _, want := range []string{reply.Record, "a note of 18 bytes and 1 images"} {
		if !strings.Contains(log, want) {
			t.Fatalf("the log should still name the record and the sizes (%q):\n%s", want, log)
		}
	}
}

func TestFeedbackScreenIsClippedAndStrippedOfControlBytes(t *testing.T) {
	r := newFeedbackRig(t)
	dirty := &feedbackScreen{
		View: "seat\x1b[2J\nsprite", Host: strings.Repeat("h", 500), Build: "b\x07",
		Viewport: feedbackViewport{Width: -5, Height: 1e12, Ratio: 2},
	}
	reply, err := r.c.request(frame{Type: "feedback", Text: "x", Screen: dirty})
	if err != nil {
		t.Fatal(err)
	}
	screen := readReport(t, filepath.Join(r.root, reply.Record, feedbackReportFile)).Screen
	if strings.ContainsAny(screen.View, "\x1b\n") || strings.ContainsRune(screen.Build, '\x07') {
		t.Fatalf("control bytes must not survive: %+v", screen)
	}
	if len([]rune(screen.Host)) != feedbackFieldMax {
		t.Fatalf("a field is clipped to %d characters: %d", feedbackFieldMax, len([]rune(screen.Host)))
	}
	if screen.Viewport.Width != 0 || screen.Viewport.Height != 0 || screen.Viewport.Ratio != 2 {
		t.Fatalf("an absurd number is zeroed: %+v", screen.Viewport)
	}
}

func TestFeedbackFrameFitsTheWire(t *testing.T) {
	encoded, err := json.Marshal(frame{Type: "feedback_image", ID: randomID(6), Data: make([]byte, maxImageBytes)})
	if err != nil || len(encoded) > maxFrame {
		t.Fatalf("a full-size screenshot frame is %d bytes and maxFrame is %d: %v", len(encoded), maxFrame, err)
	}
}
