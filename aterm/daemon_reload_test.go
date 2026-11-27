package main

import (
	"os"
	"strings"
	"testing"
	"time"
)

// idleClaude draws the prompt mark, then claude's ruled input box, and waits.
const idleClaude = `printf 'Try "x"\r\033[2K────────────────\r\n❯ \r\n────────────────\r\n'; sleep 30`

// reloadRig is a daemon holding one fake claude seat, a ledger that records it, and a
// relaunch that spawns the seat again with the script `back` draws.
func reloadRig(t *testing.T, script, back string, conversation string) (*daemon, *conn, *[]ledgerEntry) {
	t.Helper()
	saved, savedPoll := reloadWait, reloadPoll
	reloadWait, reloadPoll = 10*time.Second, 50*time.Millisecond
	t.Cleanup(func() { reloadWait, reloadPoll = saved, savedPoll })
	var entries []ledgerEntry
	d, c, reopened := reopenRig(t, nil)
	d.entries = func() []ledgerEntry { return entries }
	// A relaunch dials its own connection, as the launch the daemon runs does.
	relaunch := pipeClient(t, d, peerStanding{pids: []int{os.Getpid()}})
	nextFrame(t, relaunch, "welcome")
	spawn := func(on *conn, script string) {
		if _, err := on.request(frame{
			Type: "spawn", Session: "eng-platform-beetle-ox", Role: "eng-platform", Identity: "Beetle-Ox", Seat: "claude",
			Argv: []string{"/bin/sh", "-c", script}, Env: os.Environ(), Cwd: t.TempDir(), Rows: 24, Cols: 80,
		}); err != nil {
			t.Fatalf("spawn: %v", err)
		}
	}
	spawn(c, script)
	s := d.session("eng-platform-beetle-ox")
	if s == nil {
		t.Fatal("the seat was not spawned")
	}
	entries = []ledgerEntry{{Name: s.name, Role: s.role, Identity: "Beetle-Ox", Seat: "claude", Conversation: conversation, Started: s.started}}
	d.reopen = func(entry ledgerEntry) error {
		*reopened = append(*reopened, entry)
		spawn(relaunch, back)
		return nil
	}
	return d, c, reopened
}

// tuiEventually waits for the detector to read the seat as want.
func tuiEventually(t *testing.T, d *daemon, name, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if s := d.session(name); s != nil && s.view().TUI == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s never read %s", name, want)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// COI-3787: an idle seat is closed and resumed on its own conversation under its own
// name, and the reply waits for the detector to read it idle again.
func TestReloadResumesAnIdleSeatOnItsConversationAndWaitsForIdle(t *testing.T) {
	d, c, reopened := reloadRig(t, idleClaude, idleClaude, "c-1")
	tuiEventually(t, d, "eng-platform-beetle-ox", stateIdle)
	before := d.session("eng-platform-beetle-ox").started
	reply, err := c.request(frame{Type: "reload", Target: "eng-platform-beetle-ox"})
	if err != nil || reply.Type != "reloaded" {
		t.Fatalf("reload = %+v, %v, want reloaded", reply, err)
	}
	if len(*reopened) != 1 || (*reopened)[0].Conversation != "c-1" || (*reopened)[0].Identity != "Beetle-Ox" {
		t.Fatalf("reopened %+v, want conversation c-1 under Beetle-Ox", *reopened)
	}
	if reply.Session != "eng-platform-beetle-ox" || reply.State != stateIdle || !strings.Contains(reply.Text, "idle") {
		t.Fatalf("reply = %+v, want the same name back and idle", reply)
	}
	if s := d.session("eng-platform-beetle-ox"); s == nil || !s.started.After(before) {
		t.Fatal("the live seat is not the relaunched one")
	}
}

// A seat that comes back to a screen the detector does not read idle is named as such.
func TestReloadSaysWhenTheSeatIsNotYetIdle(t *testing.T) {
	saved := reloadWait
	d, c, _ := reloadRig(t, idleClaude, `printf 'Try "x"\r\033[2Kstill loading'; sleep 30`, "c-1")
	reloadWait = time.Second
	t.Cleanup(func() { reloadWait = saved })
	tuiEventually(t, d, "eng-platform-beetle-ox", stateIdle)
	reply, err := c.request(frame{Type: "reload", Target: "eng-platform-beetle-ox"})
	if err != nil || reply.State == stateIdle || !strings.Contains(reply.Text, "not yet idle") {
		t.Fatalf("reload = %+v, %v, want it named as not yet idle", reply, err)
	}
}

// Nothing ends unless the seat is idle and can be resumed.
func TestReloadRefusesABusySeatOrOneWithNoConversation(t *testing.T) {
	busy := `printf 'Try "x"\r\033[2K✻ Working… (esc to interrupt)\r\n'; sleep 30`
	d, c, reopened := reloadRig(t, busy, idleClaude, "c-1")
	tuiEventually(t, d, "eng-platform-beetle-ox", stateBusy)
	reply, err := c.request(frame{Type: "reload", Target: "eng-platform-beetle-ox"})
	if err == nil || reply.Code != exitUsage || !strings.Contains(err.Error(), "not idle") {
		t.Fatalf("reload of a busy seat = %+v, %v, want a usage refusal", reply, err)
	}
	d2, c2, reopened2 := reloadRig(t, idleClaude, idleClaude, "")
	tuiEventually(t, d2, "eng-platform-beetle-ox", stateIdle)
	reply, err = c2.request(frame{Type: "reload", Target: "eng-platform-beetle-ox"})
	if err == nil || reply.Code != exitUsage || !strings.Contains(err.Error(), "no recorded conversation") {
		t.Fatalf("reload with no conversation = %+v, %v, want a usage refusal", reply, err)
	}
	if len(*reopened)+len(*reopened2) != 0 || d.session("eng-platform-beetle-ox") == nil || d2.session("eng-platform-beetle-ox") == nil {
		t.Fatal("a refused reload ended or relaunched the seat")
	}
}
