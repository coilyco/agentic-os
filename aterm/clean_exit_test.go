package main

import (
	"strings"
	"testing"
	"time"
)

// exitsOnSlashExit is a seat that quits cleanly when it reads /exit and otherwise
// holds, so an exit code of 0 proves the line was typed and no signal was needed.
const exitsOnSlashExit = `echo READY; read line; case "$line" in */exit*) exit 0;; esac; sleep 30`

func TestCloseTypesTheHarnessExitAndWaitsBeforeAnySignal(t *testing.T) {
	rig := newClearRig(t)
	rig.target("exit-claude", "claude", exitsOnSlashExit)
	rig.settle("exit-claude")
	reply, err := closeFrame(t, rig.tc, frame{Target: "exit-claude"})
	if err != nil {
		t.Fatalf("close: %v", err)
	}
	if reply.Code != 0 {
		t.Fatalf("close code = %d, want 0 from the seat's own exit and not 143 from SIGTERM", reply.Code)
	}
}

func TestCloseFallsBackToTheSignalWhenTheTypedExitIsIgnored(t *testing.T) {
	rig := newClearRig(t)
	seat := rig.target("exit-ignored", "claude", rawEcho)
	rig.settle("exit-ignored")
	started := time.Now()
	reply, err := closeFrame(t, rig.tc, frame{Target: "exit-ignored"})
	if err != nil {
		t.Fatalf("close: %v", err)
	}
	if reply.Code == 0 {
		t.Fatalf("close code = 0, want the signal's code from a seat that ignored /exit")
	}
	if waited := time.Since(started); waited < cleanExitGrace {
		t.Fatalf("closed after %s, before the %s typed-exit grace", waited, cleanExitGrace)
	}
	seat.until("/exit^M")
}

func TestCloseNeverTypesExitOverADraft(t *testing.T) {
	rig := newClearRig(t)
	seat := rig.target("exit-drafted", "claude", rawEcho)
	rig.settle("exit-drafted")
	if err := seat.c.write(frame{Type: "input", Session: "exit-drafted", Data: []byte("half a thought")}); err != nil {
		t.Fatalf("type a draft: %v", err)
	}
	waitFor(t, "the draft to register", 5*time.Second, func() bool {
		reply, _ := rig.tc.c.request(frame{Type: "list"})
		for _, view := range reply.Sessions {
			if view.Name == "exit-drafted" {
				return view.Drafted
			}
		}
		return false
	})
	if _, err := closeFrame(t, rig.tc, frame{Target: "exit-drafted", Force: true}); err != nil {
		t.Fatalf("forced close: %v", err)
	}
	if strings.Contains(seat.output.String(), "/exit") {
		t.Fatalf("/exit joined Kai's draft and would have been sent: %q", seat.output.String())
	}
}

func TestCloseTypesNothingForASeatWithNoKnownExit(t *testing.T) {
	rig := newClearRig(t)
	seat := rig.target("exit-codex", "codex", rawEcho)
	rig.settle("exit-codex")
	if _, err := closeFrame(t, rig.tc, frame{Target: "exit-codex"}); err != nil {
		t.Fatalf("close: %v", err)
	}
	if strings.Contains(seat.output.String(), "/exit") {
		t.Fatalf("a seat with no known exit line was typed to: %q", seat.output.String())
	}
}
