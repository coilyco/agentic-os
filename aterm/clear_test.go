package main

import (
	"strings"
	"testing"
	"time"
)

// rawEcho turns bracketed paste on, then shows every typed byte, so a typed
// command and its Enter read back as `/clear^M`.
const rawEcho = `printf '\033[?2004h'; stty raw -echo; echo READY; exec cat -v`

// clearRig is a daemon with a director, a peer of another role, and a target.
type clearRig struct {
	t        *testing.T
	tc       *testClient
	director string
	peer     string
}

func newClearRig(t *testing.T) *clearRig {
	t.Helper()
	t.Setenv(sessionTokenEnv, "")
	testDaemon(t)
	rig := &clearRig{t: t, tc: dialTest(t)}
	for _, who := range []struct {
		name, role string
		token      *string
	}{{"clear-director", "prod-director", &rig.director}, {"clear-peer", "eng-platform", &rig.peer}} {
		client := dialTest(t)
		client.spawn(who.name, who.role, "Someone", `echo TOKEN=$ATERM_SESSION_TOKEN; exec cat`)
		*who.token = client.token()
	}
	return rig
}

func (r *clearRig) target(name, seat, script string) *testClient {
	r.t.Helper()
	client := dialTest(r.t)
	client.spawnSeat(name, "frontend-eng", "Imp", seat, script)
	if _, err := client.c.request(frame{Type: "attach", Session: name, Replay: true}); err != nil {
		r.t.Fatalf("attach: %v", err)
	}
	client.until("READY")
	return client
}

// settle waits for a target to read as idle, which takes the quiet window.
func (r *clearRig) settle(name string) {
	r.t.Helper()
	waitFor(r.t, name+" to read idle", 12*time.Second, func() bool {
		reply, err := r.tc.c.request(frame{Type: "list"})
		if err != nil {
			r.t.Fatalf("list: %v", err)
		}
		for _, view := range reply.Sessions {
			if view.Name == name {
				return view.State == stateIdle
			}
		}
		return false
	})
}

func (r *clearRig) clear(token, target string, force bool) (frame, error) {
	return r.tc.c.request(frame{Type: "clear", Token: token, Target: target, Force: force})
}

func TestClearTypesAnUnstampedCommandForKaiAndForTheDirector(t *testing.T) {
	rig := newClearRig(t)
	for _, caller := range []struct{ who, token string }{{"Kai's client", ""}, {"the director", rig.director}} {
		target := rig.target("clear-target", "claude", rawEcho)
		rig.settle("clear-target")
		reply, err := rig.clear(caller.token, "clear-target", false)
		if err != nil || reply.Type != "cleared" || reply.Text != "/clear" {
			t.Fatalf("%s: %+v %v", caller.who, reply, err)
		}
		target.until("/clear^M")
		if strings.Contains(target.output.String(), "[from") {
			t.Fatalf("the command must be unstamped: %q", target.output.String())
		}
		_, _ = rig.tc.c.request(frame{Type: "close", Target: "clear-target", Force: true})
	}
}

func TestClearRefusesWhoMayNotAndWhatItMustNot(t *testing.T) {
	rig := newClearRig(t)
	rig.target("clear-target", "claude", rawEcho)
	rig.settle("clear-target")
	cases := []struct {
		name, token, target string
		force               bool
		want                string
	}{
		{"a peer of another role", rig.peer, "clear-target", false, "may not clear"},
		{"the caller's own session", rig.director, "clear-director", false, "is this session"},
		{"an unknown target", "", "nobody-here", false, "no live session"},
	}
	for _, c := range cases {
		if _, err := rig.clear(c.token, c.target, c.force); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: want a refusal naming %q, got %v", c.name, c.want, err)
		}
	}
}

func TestClearRefusesASeatItHasNoCommandFor(t *testing.T) {
	rig := newClearRig(t)
	rig.target("clear-other-harness", "opencode", rawEcho)
	if _, err := rig.clear("", "clear-other-harness", true); err == nil || !strings.Contains(err.Error(), "no clear command") {
		t.Fatalf("got %v", err)
	}
}

func TestClearNeverAnswersAPromptNotEvenForced(t *testing.T) {
	rig := newClearRig(t)
	rig.target("clear-prompt", "claude", `printf '\033[?2004h'; stty raw -echo; echo READY; printf 'Do you want to proceed?\r\n1. Yes\r\nEsc to cancel\r\n'; exec cat -v`)
	waitFor(t, "a prompt state", 8*time.Second, func() bool {
		status, err := sessionStatusOf("clear-prompt", 0)
		return err == nil && status.State == statePrompt
	})
	if _, err := rig.clear("", "clear-prompt", true); err == nil || !strings.Contains(err.Error(), "sits on a prompt") {
		t.Fatalf("got %v", err)
	}
}

func TestClearRefusesABusySessionUnlessForced(t *testing.T) {
	rig := newClearRig(t)
	rig.target("clear-busy", "claude", `printf '\033[?2004h'; stty raw -echo; echo READY; while :; do echo tick; sleep 0.2; done`)
	waitFor(t, "the session to be ready and busy", 8*time.Second, func() bool {
		status, _ := sessionStatusOf("clear-busy", 0)
		return status.State == stateBusy && status.Ready
	})
	if _, err := rig.clear("", "clear-busy", false); err == nil || !strings.Contains(err.Error(), "is busy") {
		t.Fatalf("a busy session should refuse: %v", err)
	}
	if reply, err := rig.clear("", "clear-busy", true); err != nil || reply.Type != "cleared" {
		t.Fatalf("--force should clear a busy session: %+v %v", reply, err)
	}
}

func TestClearRespectsKaisDraftUnlessForced(t *testing.T) {
	rig := newClearRig(t)
	target := rig.target("clear-draft", "claude", rawEcho)
	rig.settle("clear-draft")
	if err := target.c.write(frame{Type: "input", Session: "clear-draft", Data: []byte("half a thought")}); err != nil {
		t.Fatal(err)
	}
	target.until("half a thought")
	waitFor(t, "the draft to register", 5*time.Second, func() bool {
		status, _ := sessionStatusOf("clear-draft", 0)
		return status.Drafted
	})
	if _, err := rig.clear("", "clear-draft", false); err == nil || !strings.Contains(err.Error(), "draft") {
		t.Fatalf("a held draft should refuse: %v", err)
	}
	if reply, err := rig.clear("", "clear-draft", true); err != nil || reply.Type != "cleared" {
		t.Fatalf("--force should clear over a draft: %+v %v", reply, err)
	}
}
