package main

import (
	"bytes"
	"io"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// A forced close at 1 or 143 held the window on "Press Enter to close" (COI-3799). Each
// round runs the window's own attach loop and window end, ten in a row.
func TestTenForcedClosesInARowEachCloseTheirWindow(t *testing.T) {
	testDaemon(t)
	scripts := []string{
		`echo UP; exec sleep 30`,                                   // SIGTERM ends it at 143
		`trap 'exit 1' TERM; echo UP; while :; do sleep 0.1; done`, // a harness that exits 1 on TERM
	}
	for round := range 10 {
		name := "close-window-" + strconv.Itoa(round)
		window := dialTest(t)
		window.spawnSeat(name, "close-window-role", "Seat-"+strconv.Itoa(round), "codex", scripts[round%2])
		window.until("UP")
		ended := make(chan attachResult, 1)
		go func() {
			var next int64
			stdout, err := os.CreateTemp(t.TempDir(), "window-*")
			if err != nil {
				t.Errorf("stdout: %v", err)
				ended <- attachResult{}
				return
			}
			defer stdout.Close()
			ended <- attachLoop(window.c, name, startPump(blockingReader(t)), stdout, false, &next)
		}()
		reply, err := closeFrame(t, dialTest(t), frame{Target: name, Force: true})
		if err != nil {
			t.Fatalf("round %d close: %v", round, err)
		}
		var result attachResult
		select {
		case result = <-ended:
		case <-time.After(15 * time.Second):
			t.Fatalf("round %d: the window never heard %s end", round, name)
		}
		if result.endedBy != endedByClose || result.code != reply.Code {
			t.Fatalf("round %d: window got %+v, want ended by close with code %d", round, result, reply.Code)
		}
		if result.code == 0 {
			t.Fatalf("round %d: the seat exited 0, so this round did not exercise the failing code", round)
		}
		// The window's pump never yields a line, so a held window would block here.
		returned := make(chan int, 1)
		var screen bytes.Buffer
		go func() { returned <- windowEnd(startPump(blockingReader(t)), &screen, result, false) }()
		select {
		case <-returned:
		case <-time.After(2 * time.Second):
			t.Fatalf("round %d: the window held on exit %d after a close: %q", round, result.code, screen.String())
		}
	}
}

func TestAFailureStillHoldsTheWindow(t *testing.T) {
	reader, writer := io.Pipe()
	t.Cleanup(func() { _ = writer.Close() })
	var screen bytes.Buffer
	returned := make(chan int, 1)
	go func() { returned <- windowEnd(startPump(reader), &screen, attachResult{code: 1}, false) }()
	select {
	case <-returned:
		t.Fatal("a failure closed the window before anyone read it")
	case <-time.After(200 * time.Millisecond):
	}
	_, _ = writer.Write([]byte("\n"))
	select {
	case code := <-returned:
		if code != 1 || !strings.Contains(screen.String(), "Session failed (exit 1)") {
			t.Fatalf("code %d, screen %q, want the failure shown and exit 1", code, screen.String())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Enter did not close a held window")
	}
}

func TestAReloadClosesTheWindowAndALostHolderDoesNot(t *testing.T) {
	if !endedOnPurpose(endedByReload) || !endedOnPurpose(endedByClose) {
		t.Fatal("a close and a reload are ends someone asked for")
	}
	if endedOnPurpose(endedByLost) || endedOnPurpose("") {
		t.Fatal("a lost holder or an unknown end is a failure, and the window shows it")
	}
}

// blockingReader never yields, the stdin of a window nobody types into.
func blockingReader(t *testing.T) io.Reader {
	reader, writer := io.Pipe()
	t.Cleanup(func() { _ = writer.Close() })
	return reader
}

// holderOf reads the PARENT= line a seat's script prints, which is its holder's pid.
func holderOf(t *testing.T, tc *testClient) int {
	t.Helper()
	tc.untilMatch("a PARENT= line", func(output string) bool {
		_, rest, found := strings.Cut(output, "PARENT=")
		return found && strings.Contains(rest, "\n")
	})
	_, rest, _ := strings.Cut(tc.output.String(), "PARENT=")
	line, _, _ := strings.Cut(rest, "\n")
	pid, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil {
		t.Fatalf("holder pid %q: %v", line, err)
	}
	return pid
}

func TestKillingAHolderMarksItsSeatDeadAndOffersItToReopen(t *testing.T) {
	testDaemon(t)
	const name, conversation = "dead-holder-seat", "0b6c3f8e-6c1e-4f43-9a51-2a1d3e4f5a6b"
	window := dialTest(t)
	_, err := window.c.request(frame{
		Type: "spawn", Session: name, Role: "dead-role", Identity: "Dead-Seat", Seat: "claude",
		Argv: []string{"/bin/sh", "-c", `printf '` + string(promptMarks["claude"]) + `\r\033[2K'; echo PARENT=$PPID; exec sleep 30`, "sh", "--session-id", conversation},
		Env:  os.Environ(), Cwd: "/", Rows: 24, Cols: 200,
	})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	holder := holderOf(t, window)
	if err := syscall.Kill(holder, syscall.SIGKILL); err != nil {
		t.Fatalf("kill holder %d: %v", holder, err)
	}
	exit := window.frameOfType("exit")
	if exit.EndedBy != endedByLost {
		t.Fatalf("exit = %+v, want ended by lost", exit)
	}
	// The exit frame goes out before the ledger is stamped, so both are waited on.
	deadline := time.Now().Add(time.Minute)
	var record ledgerEntry
	for !record.Lost || record.Ended == nil {
		if time.Now().After(deadline) {
			t.Fatalf("record = %+v, want it ended and lost", record)
		}
		time.Sleep(50 * time.Millisecond)
		for _, entry := range readLedger(ledgerDir()) {
			if entry.Name == name {
				record = entry
			}
		}
	}
	for {
		listed := dialTest(t).list()
		for _, seat := range listed.Dead {
			if seat.Name == name {
				if seat.Ref != sessionRef(record.Name, record.Started) || seat.Role != "dead-role" {
					t.Fatalf("dead seat = %+v, want the record's ref and role", seat)
				}
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s never listed dead: %+v", name, listed.Dead)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
