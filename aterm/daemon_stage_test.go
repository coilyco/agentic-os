package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// stageHarness is a daemon whose login shell is /bin/sh, a seat with a token,
// and a subscribed client standing in for the web client that would show the note.
type stageHarness struct {
	t      *testing.T
	seat   *testClient
	token  string
	name   string
	events *conn
}

func newStageHarness(t *testing.T) *stageHarness {
	t.Helper()
	t.Setenv("SHELL", "/bin/sh")
	testDaemon(t)
	h := &stageHarness{t: t, name: "sysadmin-ox"}
	h.seat = dialTest(t)
	h.seat.spawn(h.name, "sysadmin", "Ox", `echo TOKEN=$ATERM_SESSION_TOKEN; sleep 60`)
	h.token = h.seat.token()
	h.events = subscribed(t)
	return h
}

func (h *stageHarness) spawnSeat(name, role, identity string) string {
	h.t.Helper()
	seat := dialTest(h.t)
	seat.spawn(name, role, identity, `echo TOKEN=$ATERM_SESSION_TOKEN; sleep 60`)
	return seat.token()
}

func (h *stageHarness) stage(token string, message frame) frame {
	h.t.Helper()
	message.Type, message.Token = "stage", token
	reply, err := dialTest(h.t).c.request(message)
	if err != nil {
		h.t.Fatalf("stage: %v", err)
	}
	if reply.Type != "stage_result" {
		h.t.Fatalf("stage answered %+v", reply)
	}
	return reply
}

// shown is everything the terminal has drawn so far, read on a fresh attach with
// replay so a quiet read cannot end the connection a later step needs.
func (h *stageHarness) shown(terminal string) string {
	h.t.Helper()
	viewer := dialTest(h.t)
	if _, err := viewer.c.request(frame{Type: "attach", Session: terminal, Replay: true, Rows: 24, Cols: 120}); err != nil {
		h.t.Fatalf("attach %s: %v", terminal, err)
	}
	_ = viewer.c.raw.SetReadDeadline(time.Now().Add(600 * time.Millisecond))
	for {
		message, err := viewer.c.read()
		if err != nil {
			return viewer.output.String()
		}
		if message.Type == "output" {
			viewer.output.Write(message.Data)
		}
	}
}

func (h *stageHarness) terminals() []terminalView {
	h.t.Helper()
	return dialTest(h.t).list().Terminals
}

// press types into a terminal the way Kai's client does.
func (h *stageHarness) press(terminal, keys string) {
	h.t.Helper()
	typist := dialTest(h.t)
	if _, err := typist.c.request(frame{Type: "attach", Session: terminal, Rows: 24, Cols: 120}); err != nil {
		h.t.Fatalf("attach %s: %v", terminal, err)
	}
	typist.type_(terminal, keys)
	time.Sleep(300 * time.Millisecond)
}

func TestStageTypesTheCommandBesideTheSeatAndNeverRunsIt(t *testing.T) {
	h := newStageHarness(t)
	// Short, so the shell echoes the line without wrapping it across rows.
	dir, err := os.MkdirTemp("/tmp", "st")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	marker := filepath.Join(dir, "ran")
	// Arithmetic and a side effect, so neither the echo of the typed text nor a
	// prompt can pass for the command having run.
	command := "touch " + marker + "; echo ok-$((6*7))"

	reply := h.stage(h.token, frame{Body: command, Text: "run this after the deploy"})
	if reply.State != stageStaged || !reply.Opened || !strings.HasPrefix(reply.Session, terminalName) {
		t.Fatalf("a first stage opens the terminal and stages: %+v", reply)
	}
	terminals := h.terminals()
	if len(terminals) != 1 || terminals[0].Name != reply.Session || terminals[0].Label != h.name {
		t.Fatalf("the terminal is labelled with the calling seat, which is how the client pairs it: %+v", terminals)
	}

	event := nextFrame(t, h.events, "staged")
	if event.Session != reply.Session || event.Label != h.name || event.Body != command || event.Text != "run this after the deploy" {
		t.Fatalf("the note rides a staged frame naming the terminal and its seat: %+v", event)
	}

	waitFor(t, "the staged line on the shell's input line", 10*time.Second, func() bool { return strings.Contains(h.shown(reply.Session), command) })
	if _, err := os.Stat(marker); err == nil || strings.Contains(h.shown(reply.Session), "ok-42") {
		t.Fatalf("staged is not run: marker %v, output %q", err, h.shown(reply.Session))
	}
	if strings.Contains(h.shown(reply.Session), "run this after the deploy") {
		t.Fatal("the note is never typed into the shell")
	}

	h.press(reply.Session, "\r")
	waitFor(t, "Enter running the staged line", 10*time.Second, func() bool { return strings.Contains(h.shown(reply.Session), "ok-42") })
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("Enter runs it: %v", err)
	}
}

func TestStageRefusesEveryControlByteAndOpensNothing(t *testing.T) {
	h := newStageHarness(t)
	for name, command := range map[string]string{
		"newline":              "echo hi\nrm x",
		"trailing newline":     "echo hi\n",
		"carriage return":      "echo hi\r",
		"ctrl-c":               "echo hi\x03",
		"escape":               "echo \x1b[A",
		"tab":                  "echo\thi",
		"nul":                  "echo hi\x00",
		"delete":               "echo hi\x7f",
		"c1 control":           "echo \u0085hi",
		"line separator":       "echo  hi",
		"bidi override":        "echo ‮hi",
		"empty":                "   ",
		"over the command cap": strings.Repeat("a", maxStageCommand+1),
	} {
		reply := h.stage(h.token, frame{Body: command})
		if reply.State != stageRefused || reply.Text == "" || reply.Session != "" {
			t.Errorf("%s should be refused with a reason: %+v", name, reply)
		}
	}
	if reply := h.stage(h.token, frame{Body: "echo hi", Text: "two\nlines"}); reply.State != stageRefused {
		t.Errorf("a note with a newline should be refused: %+v", reply)
	}
	if terminals := h.terminals(); len(terminals) != 0 {
		t.Fatalf("a refused stage opens no shell: %+v", terminals)
	}
	// JSON turns invalid UTF-8 into U+FFFD, so only the check itself can refuse it.
	if err := checkStagedCommand("echo \xff"); err == nil {
		t.Fatal("invalid UTF-8 is refused by the check")
	}
	if err := checkStagedCommand("echo 'quoted $VAR' | wc -l && ls -la ~/dir é"); err != nil {
		t.Fatalf("ordinary shell text is accepted: %v", err)
	}
}

func TestStageReachesOnlyTheCallersOwnTerminal(t *testing.T) {
	h := newStageHarness(t)
	otherToken := h.spawnSeat("sysadmin-cow", "sysadmin", "Cow")

	mine := h.stage(h.token, frame{Body: "echo mine-$((1+1))"})
	theirs := h.stage(otherToken, frame{Body: "echo theirs-$((2+2))"})
	if mine.State != stageStaged || theirs.State != stageStaged || mine.Session == theirs.Session {
		t.Fatalf("each seat gets its own terminal: %+v vs %+v", mine, theirs)
	}

	// Every field that could name somewhere else is refused whole, and nothing is typed.
	for name, aimed := range map[string]frame{
		"target a seat":     {Body: "echo hijack", Target: "sysadmin-cow"},
		"session a shell":   {Body: "echo hijack", Session: theirs.Session},
		"label a seat":      {Body: "echo hijack", Label: "sysadmin-cow"},
		"identity of other": {Body: "echo hijack", Identity: "Cow"},
		"role of other":     {Body: "echo hijack", Role: "sysadmin"},
	} {
		if reply := h.stage(h.token, aimed); reply.State != stageRefused || !strings.Contains(reply.Text, "never names") {
			t.Errorf("%s should be refused: %+v", name, reply)
		}
	}
	waitFor(t, "both staged lines", 10*time.Second, func() bool {
		return strings.Contains(h.shown(mine.Session), "mine-$((1+1))") && strings.Contains(h.shown(theirs.Session), "theirs-$((2+2))")
	})
	if strings.Contains(h.shown(theirs.Session), "hijack") || strings.Contains(h.shown(mine.Session), "hijack") {
		t.Fatal("a stage that named another target typed into a terminal")
	}
	if strings.Contains(h.shown(theirs.Session), "mine-") || strings.Contains(h.shown(mine.Session), "theirs-") {
		t.Fatal("a staged line crossed to the other seat's terminal")
	}

	// A token is the only identity: invented, missing, or a terminal's name all fail.
	for _, token := range []string{"", "not-a-token", mine.Session} {
		message := frame{Type: "stage", Token: token, Body: "echo hi"}
		if reply, err := dialTest(t).c.request(message); err == nil {
			t.Errorf("token %q should speak for no seat: %+v", token, reply)
		}
	}
}

func TestStageReportsNoClientAndOpensNothingWhenNobodyCouldSeeIt(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")
	testDaemon(t)
	seat := dialTest(t)
	seat.spawn("sysadmin-ox", "sysadmin", "Ox", `echo TOKEN=$ATERM_SESSION_TOKEN; sleep 60`)
	h := &stageHarness{t: t, token: seat.token()}
	reply := h.stage(h.token, frame{Body: "echo hi"})
	if reply.State != stageNoClient || reply.Session != "" {
		t.Fatalf("no subscribed client means no stage: %+v", reply)
	}
	if terminals := h.terminals(); len(terminals) != 0 {
		t.Fatalf("nothing was opened for nobody: %+v", terminals)
	}
}

func TestStageUsesTheTerminalAClientOpenedForTheSeat(t *testing.T) {
	h := newStageHarness(t)
	opener := dialTest(t)
	spawned, err := opener.c.request(frame{Type: "spawn", Kind: kindTerminal, Label: h.name, Rows: 24, Cols: 120})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	reply := h.stage(h.token, frame{Body: "echo viewed-$((3+4))"})
	if reply.State != stageStaged || reply.Opened || reply.Session != spawned.Session || reply.Viewers != 1 {
		t.Fatalf("the client's own terminal is reused and seen: %+v", reply)
	}
	opener.until("viewed-$((3+4))")
	if len(h.terminals()) != 1 {
		t.Fatalf("a second shell was opened: %+v", h.terminals())
	}
}

func TestStageRefusesToRunTogetherWithAnUnsentLine(t *testing.T) {
	h := newStageHarness(t)
	first := h.stage(h.token, frame{Body: "echo first"})
	second := h.stage(h.token, frame{Body: "echo second"})
	if second.State != stageRefused || !strings.Contains(second.Text, "unsent line") || second.Opened {
		t.Fatalf("a second stage would run together with the first: %+v", second)
	}
	waitFor(t, "the first line", 10*time.Second, func() bool { return strings.Contains(h.shown(first.Session), "echo first") })
	if strings.Contains(h.shown(first.Session), "second") {
		t.Fatal("the refused line was typed")
	}
	// Kai clearing the line, as Ctrl-U or Enter does, lets the next one through.
	h.press(first.Session, "\x15")
	third := h.stage(h.token, frame{Body: "echo third"})
	if third.State != stageStaged || third.Session != first.Session {
		t.Fatalf("a cleared line takes the next stage in the same shell: %+v", third)
	}
	// Kai's own half-typed line holds a stage off the same way.
	h.press(first.Session, "\x15")
	h.press(first.Session, "ls ")
	if held := h.stage(h.token, frame{Body: "echo fourth"}); held.State != stageRefused {
		t.Fatalf("Kai's draft holds a stage off: %+v", held)
	}
}

func TestStageCommandFromTheCLIPathReportsTheStates(t *testing.T) {
	h := newStageHarness(t)
	t.Setenv(sessionTokenEnv, h.token)
	result, err := stageCommand("echo cli-$((5+5))", "a note")
	if err != nil || result.State != stageStaged || !result.Opened {
		t.Fatalf("staged: %+v, %v", result, err)
	}
	if err := stageExit(result); err != nil {
		t.Fatalf("staged exits 0: %v", err)
	}
	refused, err := stageCommand("echo hi\n", "")
	if err != nil || refused.State != stageRefused || exitCodeFor(stageExit(refused)) != exitUsage {
		t.Fatalf("refused exits %d: %+v, %v", exitUsage, refused, err)
	}
	if exitCodeFor(stageExit(stageResult{State: stageNoClient})) != exitNoClient {
		t.Fatal("no client has its own exit code")
	}
	t.Setenv(sessionTokenEnv, "")
	if _, err := stageCommand("echo hi", ""); exitCodeFor(err) != exitUsage {
		t.Fatalf("outside a seat there is nobody to stage beside: %v", err)
	}
}

func TestJoinStagedQuotesSeveralWordsAndKeepsALoneLine(t *testing.T) {
	for _, c := range []struct {
		words []string
		want  string
	}{
		{[]string{"ls -la | wc -l"}, "ls -la | wc -l"},
		{[]string{"echo", "a b", "it's"}, `echo 'a b' 'it'\''s'`},
		{[]string{"kubectl", "get", "pods", "-n", "kube-system"}, "kubectl get pods -n kube-system"},
		{[]string{"echo", "$HOME", "x;y"}, `echo '$HOME' 'x;y'`},
	} {
		if got := joinStaged(c.words); got != c.want {
			t.Errorf("joinStaged(%q) = %q, want %q", c.words, got, c.want)
		}
	}
}

func TestStageVerbThroughTheRootCommandStagesAndReportsRefusals(t *testing.T) {
	h := newStageHarness(t)
	t.Setenv(sessionTokenEnv, h.token)
	var out strings.Builder
	root := newCommand(systemDeps())
	root.Writer = &out
	if err := root.Run(t.Context(), []string{"aterm", "stage", "--note", "why", "--", "echo", "word one", "$((2*3))"}); err != nil {
		t.Fatalf("stage verb: %v", err)
	}
	if !strings.Contains(out.String(), "staged in terminal-") || !strings.Contains(out.String(), "Nothing ran") {
		t.Fatalf("the verb says what happened and that nothing ran: %q", out.String())
	}
	event := nextFrame(t, h.events, "staged")
	if event.Body != `echo 'word one' '$((2*3))'` || event.Text != "why" {
		t.Fatalf("several words are quoted into one line, the note stays a frame: %+v", event)
	}

	err := newCommand(systemDeps()).Run(t.Context(), []string{"aterm", "stage", "--", "echo hi\nrm x"})
	if exitCodeFor(err) != exitUsage || err == nil || !strings.Contains(err.Error(), "refused") {
		t.Fatalf("a newline exits %d as refused: %v", exitUsage, err)
	}
	if err := newCommand(systemDeps()).Run(t.Context(), []string{"aterm", "stage"}); exitCodeFor(err) != exitUsage {
		t.Fatalf("no command exits %d: %v", exitUsage, err)
	}
}
