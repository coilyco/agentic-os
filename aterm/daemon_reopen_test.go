package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// reopenRig is a daemon whose ledger and relaunch are the test's, and the entries it
// was asked to reopen.
func reopenRig(t *testing.T, entries []ledgerEntry) (*daemon, *conn, *[]ledgerEntry) {
	t.Helper()
	d, _ := wsDaemon(t)
	d.processes = func() ([]processEntry, error) { return []processEntry{{PID: os.Getpid(), PPID: 1}}, nil }
	var reopened []ledgerEntry
	d.entries = func() []ledgerEntry { return entries }
	d.reopen = func(entry ledgerEntry) error { reopened = append(reopened, entry); return nil }
	c := pipeClient(t, d, peerStanding{pids: []int{os.Getpid()}})
	nextFrame(t, c, "welcome")
	return d, c, &reopened
}

func TestReopenResumesTheLatestConversationOfAClosedSeat(t *testing.T) {
	older := time.Date(2026, 11, 8, 9, 0, 0, 0, time.UTC)
	entries := []ledgerEntry{
		{Name: "eng-platform-beetle-ox", Role: "eng-platform", Seat: "claude", Conversation: "c-old", Started: older},
		{Name: "eng-platform-beetle-ox", Role: "eng-platform", Seat: "claude", Conversation: "c-new", Started: older.Add(24 * time.Hour)},
	}
	_, c, reopened := reopenRig(t, entries)
	reply, err := c.request(frame{Type: "reopen", Target: "eng-platform-beetle-ox"})
	if err != nil || reply.Type != "reopened" || reply.Role != "eng-platform" || reply.Seat != "claude" {
		t.Fatalf("reopen = %+v, %v, want reopened eng-platform claude", reply, err)
	}
	if len(*reopened) != 1 || (*reopened)[0].Conversation != "c-new" {
		t.Fatalf("reopened %+v, want the latest conversation c-new", *reopened)
	}
}

func TestReopenWithARefPicksThatLaunchOfAReusedName(t *testing.T) {
	older := time.Date(2026, 11, 8, 9, 0, 0, 0, time.UTC)
	entries := []ledgerEntry{
		{Name: "eng-platform-beetle-ox", Role: "eng-platform", Seat: "claude", Conversation: "c-old", Started: older},
		{Name: "eng-platform-beetle-ox", Role: "eng-platform", Seat: "claude", Conversation: "c-new", Started: older.Add(24 * time.Hour)},
	}
	_, c, reopened := reopenRig(t, entries)
	target := "eng-platform-beetle-ox@" + sessionRef("eng-platform-beetle-ox", older)
	if _, err := c.request(frame{Type: "reopen", Target: target}); err != nil {
		t.Fatalf("reopen %s: %v", target, err)
	}
	if len(*reopened) != 1 || (*reopened)[0].Conversation != "c-old" {
		t.Fatalf("reopened %+v, want the launch the ref names, c-old", *reopened)
	}
}

func TestReopenRefusesASeatWithNothingToResume(t *testing.T) {
	entries := []ledgerEntry{{Name: "codex-seat", Role: "eng-platform", Seat: "codex"}}
	_, c, reopened := reopenRig(t, entries)
	for _, target := range []string{"codex-seat", "never-ran", ""} {
		reply, err := c.request(frame{Type: "reopen", Target: target})
		if err == nil || reply.Code != exitUsage {
			t.Fatalf("reopen %q = %+v, %v, want a usage refusal", target, reply, err)
		}
	}
	if len(*reopened) != 0 {
		t.Fatalf("a refused reopen still launched %+v", *reopened)
	}
	if _, err := c.request(frame{Type: "reopen", Target: "never-ran"}); err == nil || !strings.Contains(err.Error(), "a new launch of its role is the only path") {
		t.Fatalf("the refusal should say a new launch is the only path, got %v", err)
	}
}

func TestReopenRefusesAnOpenSeat(t *testing.T) {
	entries := []ledgerEntry{{Name: "live-seat", Role: "eng-platform", Seat: "claude", Conversation: "c1"}}
	d, c, reopened := reopenRig(t, entries)
	d.sessions["live-seat"] = &ptySession{name: "live-seat"}
	reply, err := c.request(frame{Type: "reopen", Target: "live-seat"})
	if err == nil || reply.Code != exitUsage || len(*reopened) != 0 {
		t.Fatalf("reopen of a live seat = %+v, %v, reopened %+v, want a refusal and no launch", reply, err, *reopened)
	}
}

func TestReopenReportsAFailedRelaunch(t *testing.T) {
	entries := []ledgerEntry{{Name: "gone", Role: "eng-platform", Seat: "claude", Conversation: "c1"}}
	d, c, _ := reopenRig(t, entries)
	d.reopen = func(ledgerEntry) error { return os.ErrPermission }
	reply, err := c.request(frame{Type: "reopen", Target: "gone"})
	if err == nil || reply.Code != exitSpawn {
		t.Fatalf("a failed relaunch = %+v, %v, want exit %d", reply, err, exitSpawn)
	}
}

func TestWelcomeNamesTheFeaturesAClientBranchesOn(t *testing.T) {
	d, _ := wsDaemon(t)
	d.processes = func() ([]processEntry, error) { return []processEntry{{PID: os.Getpid(), PPID: 1}}, nil }
	welcome := nextFrame(t, pipeClient(t, d, peerStanding{pids: []int{os.Getpid()}}), "welcome")
	for _, feature := range []string{backgroundFeature, closeUnpushedFeature, reopenFeature} {
		if !slices.Contains(welcome.Features, feature) {
			t.Errorf("welcome features %v lack %q", welcome.Features, feature)
		}
	}
}

// spawnAt starts a fake claude seat in cwd.
func (tc *testClient) spawnAt(name, cwd, script string) {
	tc.t.Helper()
	script = `printf '` + string(promptMarks["claude"]) + `\r\033[2K'; ` + script
	_, err := tc.c.request(frame{
		Type: "spawn", Session: name, Role: "eng-platform", Identity: "Beetle-Ox", Seat: "claude",
		Argv: []string{"/bin/sh", "-c", script}, Env: os.Environ(), Cwd: cwd, Rows: 24, Cols: 200,
	})
	if err != nil {
		tc.t.Fatalf("spawn %s: %v", name, err)
	}
}

func TestCloseRefusesASeatHoldingUnpushedWorkUntilForced(t *testing.T) {
	testDaemon(t)
	project := t.TempDir()
	repo := filepath.Join(project, "agentic-os")
	pushedRepo(t, repo)
	runGit(t, repo, "commit", "-q", "--allow-empty", "-m", "local only")
	room := dialTest(t)
	room.spawnAt("eng-platform-beetle-ox", project, `echo UP; sleep 30`)
	room.until("UP")
	closer := dialTest(t)
	reply, err := closeFrame(t, closer, frame{Target: "eng-platform-beetle-ox"})
	if err == nil || reply.Reason != reasonUnpushedWork || reply.Code != exitUsage {
		t.Fatalf("close = %+v, %v, want an unpushed_work refusal with exit %d", reply, err, exitUsage)
	}
	if !strings.Contains(err.Error(), "1 commit(s) on no remote") || !strings.Contains(err.Error(), repo) {
		t.Fatalf("the refusal should name the checkout and the commit, got %v", err)
	}
	if names := liveNamesEventually(t, closer, "none"); len(names) != 1 {
		t.Fatalf("a refused close must leave the seat live, got %v", names)
	}
	reply, err = closeFrame(t, closer, frame{Target: "eng-platform-beetle-ox", Force: true})
	if err != nil || reply.Type != "closed" {
		t.Fatalf("a forced close = %+v, %v, want closed", reply, err)
	}
}

func TestCloseLeavesASeatWithNoUnpushedWorkClosable(t *testing.T) {
	testDaemon(t)
	project := t.TempDir()
	pushedRepo(t, filepath.Join(project, "agentic-os"))
	room := dialTest(t)
	room.spawnAt("eng-platform-beetle-ox", project, `echo UP; sleep 30`)
	room.until("UP")
	reply, err := closeFrame(t, dialTest(t), frame{Target: "eng-platform-beetle-ox"})
	if err != nil || reply.Type != "closed" {
		t.Fatalf("close of a clean seat = %+v, %v, want closed", reply, err)
	}
}

func TestASessionViewCarriesTheBackgroundCountItsFooterShows(t *testing.T) {
	testDaemon(t)
	room := dialTest(t)
	room.spawnAt("eng-platform-beetle-ox", "/", `printf '\n  ⏵⏵ bypass permissions on · 1 shell · ← 1 agent\n'; sleep 30`)
	room.until("1 shell")
	room.spawnSeat("scientist-evie", "scientist", "Evie", "codex", `printf '\n  ⏵⏵ bypass permissions on · 1 shell\n'; sleep 30`)
	room.until("bypass permissions")
	reply, err := dialTest(t).c.request(frame{Type: "list"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, view := range reply.Sessions {
		switch view.Name {
		case "eng-platform-beetle-ox":
			if view.Background == nil || *view.Background != 1 {
				t.Fatalf("a claude footer reading 1 shell gave background %v, want 1", view.Background)
			}
		case "scientist-evie":
			if view.Background != nil {
				t.Fatalf("another harness gave background %d, want absent", *view.Background)
			}
		}
	}
}

func TestASessionViewCarriesTheOriginItsLauncherSaid(t *testing.T) {
	testDaemon(t)
	room := dialTest(t)
	for name, origin := range map[string]string{
		"seat-window": originWindow, "seat-headless": originHeadless, "seat-bogus": "the-app", "seat-unsaid": "",
	} {
		_, err := room.c.request(frame{
			Type: "spawn", Session: name, Role: "eng-platform", Identity: "Beetle-Ox", Seat: "claude",
			Argv: []string{"/bin/sh", "-c", "sleep 30"}, Env: os.Environ(), Cwd: "/", Rows: 24, Cols: 80, Origin: origin,
		})
		if err != nil {
			t.Fatalf("spawn %s: %v", name, err)
		}
	}
	got := map[string]string{}
	for _, view := range room.list().Sessions {
		got[view.Name] = view.Origin
	}
	want := map[string]string{"seat-window": originWindow, "seat-headless": originHeadless, "seat-bogus": "", "seat-unsaid": ""}
	for name, origin := range want {
		if got[name] != origin {
			t.Errorf("%s origin = %q, want %q", name, got[name], origin)
		}
	}
}

func TestOriginSurvivesADaemonRestart(t *testing.T) {
	first := newDaemon(func(string, ...any) {})
	first.holdDir = testHoldDir(t)
	s, err := first.spawn(frame{
		Type: "spawn", Session: "eng-platform-beetle-ox", Role: "eng-platform", Identity: "Beetle-Ox", Seat: "claude",
		Argv: []string{"/bin/sh", "-c", "sleep 30"}, Env: os.Environ(), Cwd: "/", Rows: 24, Cols: 80, Origin: originHeadless,
	})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	t.Cleanup(s.end)
	first.letGoAll()

	second := newDaemon(func(string, ...any) {})
	second.holdDir = first.holdDir
	second.adoptHolders()
	t.Cleanup(func() {
		for _, adopted := range second.everySession() {
			adopted.end()
		}
	})
	views := second.views()
	if len(views) != 1 || views[0].Origin != originHeadless {
		t.Fatalf("the adopted seat should keep origin %q: %+v", originHeadless, views)
	}
}
