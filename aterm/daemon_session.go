package main

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	scrollbackLimit = 1 << 20
	// A TUI reading a paste as a burst takes an Enter that arrives with it as
	// a newline, so the Enter waits. See docs/aterm-daemon.md.
	submitDelay = 300 * time.Millisecond
	// Kai typing inside this window holds a message outright.
	typingHold = 1500 * time.Millisecond
	// An unsent draft holds a message until Kai sends it, clears it, or leaves
	// it this long. Past that the message goes, and lands after her draft.
	draftHold = time.Minute
)

// terminalQuery matches output a terminal answers: status, attribute, version,
// mode, window, color, setting, capability, and kitty graphics queries.
var terminalQuery = regexp.MustCompile("\x1b\\[(?:\\??[0-9;]*n|[>=]?[0-9;]*c|>[0-9;]*q|\\?u|\\??[0-9;]*\\$p|(?:1[13-689]|2[01])t)" +
	"|\x1b\\](?:4;[0-9]+|1[0-9]|52;[a-z]*);\\?(?:\x07|\x1b\\\\)" +
	"|\x1bP[$+]q[^\x1b]*\x1b\\\\" +
	"|\x1b_G[^\x1b]*a=q[^\x1b]*\x1b\\\\")

// withoutQueries is the replay a late client gets. Its terminal would answer
// every query in the history again, and the answers would land as typed input.
func withoutQueries(history []byte) []byte {
	return terminalQuery.ReplaceAll(append([]byte(nil), history...), nil)
}

// decset matches a private mode set or reset. 2004 among its parameters is
// bracketed paste, which a TUI turns on to tell a paste from typing.
var decset = regexp.MustCompile("\x1b\\[\\?([0-9;]*)([hl])")

// degradedMark is agent-compose naming the startup steps a launch went without.
// See docs/aterm-daemon.md.
var degradedMark = regexp.MustCompile("\x1b\\]7750;agent-compose;degraded=([A-Za-z0-9,_-]*)(?:\x07|\x1b\\\\)")

// scanTail is how much of a chunk's end is kept for a sequence split across reads.
const scanTail = 256

type ptySession struct {
	d        *daemon
	name     string
	role     string
	identity string
	seat     string
	token    string
	pid      int
	started  time.Time
	// holder is the connection to the aterm hold process that owns the PTY
	// and the child. It outlives this daemon, which adopts it again.
	holder     *conn
	finishOnce sync.Once

	// writeMu serializes every write to the PTY, so a message typed in never
	// interleaves with a person's keystrokes.
	writeMu sync.Mutex

	mu           sync.Mutex
	clients      map[*conn]bool
	scrollback   []byte
	outputOffset int64
	modeTracker
	lastOutput time.Time
	lastInput  time.Time
	// promptSeen is when the seat's promptMarks text first showed, and promptTail
	// keeps a mark split across two chunks.
	promptSeen time.Time
	promptTail []byte
	draft      int
	keys       keyState
	pending    []*pendingSend
	wake       chan struct{}
	done       chan struct{}
	exitCode   int
	// released is a daemon letting go of a session that keeps running.
	released bool
}

// keyState carries a partly read escape sequence across input chunks.
type keyState struct {
	escape  bool
	csi     bool
	ss3     bool
	params  []byte
	inPaste bool
	// str is an OSC, DCS, APC, PM or SOS string, which runs to BEL or ST.
	str    bool
	strEsc bool
	// skip counts the raw bytes after an X10 mouse report's CSI M.
	skip int
}

// startPTYSession starts a holder for the spawn and attaches to it. The
// session's environment goes to the holder over a pipe, never to disk.
func startPTYSession(d *daemon, name string, message frame) (*ptySession, error) {
	if len(message.Argv) == 0 {
		return nil, withExit(exitUsage, errors.New("spawn needs an argv"))
	}
	if d.holdDir == "" {
		return nil, errors.New("this daemon has no directory for session holders")
	}
	if err := ensureSocketDir(d.holdDir); err != nil {
		return nil, err
	}
	token := randomID(24)
	spec := frame{
		Type: "spawn", Session: name, Role: message.Role, Identity: message.Identity, Seat: message.Seat,
		Argv: message.Argv, Env: sessionEnv(message.Env, name, token), Cwd: message.Cwd,
		Rows: message.Rows, Cols: message.Cols, Token: token,
	}
	socket := holdSocketPath(d.holdDir, name)
	if _, err := startHolder(socket, spec); err != nil {
		return nil, err
	}
	s, err := d.connectHolder(socket, false)
	if err != nil {
		return nil, fmt.Errorf("attach to the holder of %s: %w", name, err)
	}
	return s, nil
}

// connectHolder attaches to a holder with replay. An adopted session's typing
// state is unknown, so a message waits out the typing hold.
func (d *daemon) connectHolder(socket string, adopted bool) (*ptySession, error) {
	raw, err := net.DialTimeout("unix", socket, 2*time.Second)
	if err != nil {
		return nil, err
	}
	c := newConn(raw)
	_ = raw.SetDeadline(time.Now().Add(holdStartWait))
	fail := func(err error) (*ptySession, error) {
		_ = c.Close()
		return nil, err
	}
	if err := c.write(frame{Type: "hello", Format: holdFormat, Version: version}); err != nil {
		return fail(err)
	}
	welcome, err := c.read()
	if err != nil {
		return fail(err)
	}
	if welcome.Type != "welcome" || welcome.Format != holdFormat {
		return fail(fmt.Errorf("holder speaks %q, this daemon speaks %s: %s", welcome.Format, holdFormat, welcome.Error))
	}
	if err := c.write(frame{Type: "attach", Replay: true}); err != nil {
		return fail(err)
	}
	attached, err := c.read()
	if err != nil {
		return fail(err)
	}
	if attached.Type != "attached" || attached.Hold == nil {
		return fail(fmt.Errorf("holder answered %q to an attach", attached.Type))
	}
	_ = raw.SetDeadline(time.Time{})
	info := attached.Hold
	now := time.Now()
	s := &ptySession{
		d:            d,
		name:         info.Name,
		role:         info.Role,
		identity:     info.Identity,
		seat:         info.Seat,
		token:        info.Token,
		pid:          info.PID,
		started:      info.Started,
		holder:       c,
		clients:      map[*conn]bool{},
		outputOffset: info.ReplayStart,
		lastOutput:   now,
		wake:         make(chan struct{}, 1),
		done:         make(chan struct{}),
	}
	s.paste, s.pasteSeen, s.degraded = info.Paste, info.PasteSeen, append([]string(nil), info.Degraded...)
	if adopted {
		s.lastInput = now
	}
	go s.readHold()
	go s.deliverLoop()
	return s, nil
}

// readHold feeds the holder's output into the session until it exits. A
// holder that goes without an exit frame took its child with it.
func (s *ptySession) readHold() {
	for {
		message, err := s.holder.read()
		if err != nil {
			s.mu.Lock()
			released := s.released
			s.mu.Unlock()
			if !released {
				s.finish(1)
			}
			return
		}
		switch message.Type {
		case "output":
			s.output(message.Data)
		case "exit":
			s.finish(message.Code)
			return
		}
	}
}

// writePTY types bytes into the session through its holder.
func (s *ptySession) writePTY(data []byte) error {
	if s.holder == nil {
		return errors.New("the session has no holder")
	}
	return s.holder.write(frame{Type: "input", Data: data})
}

// sessionEnv replaces rather than appends, so a window opened from inside a
// session cannot carry the opener's token into the new one.
func sessionEnv(environ []string, name, token string) []string {
	kept := make([]string, 0, len(environ)+2)
	for _, entry := range environ {
		key, _, _ := strings.Cut(entry, "=")
		if key == sessionTokenEnv || key == sessionNameEnv {
			continue
		}
		kept = append(kept, entry)
	}
	return append(kept, sessionTokenEnv+"="+token, sessionNameEnv+"="+name)
}

// lookPathIn resolves against the client's PATH, since exec would use the
// daemon's, and the daemon may have started from a different shell.
func lookPathIn(name string, environ []string) (string, error) {
	if strings.Contains(name, "/") {
		return name, nil
	}
	path := ""
	for _, entry := range environ {
		if value, ok := strings.CutPrefix(entry, "PATH="); ok {
			path = value
		}
	}
	for _, dir := range filepath.SplitList(path) {
		if dir == "" {
			continue
		}
		candidate := filepath.Join(dir, name)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("%q is not on the session's PATH", name)
}

func clampSize(value, fallback int) int {
	if value <= 0 || value > 1000 {
		return fallback
	}
	return value
}

func (s *ptySession) output(chunk []byte) {
	s.mu.Lock()
	offset := s.outputOffset
	s.outputOffset += int64(len(chunk))
	s.lastOutput = time.Now()
	pasteBefore := s.paste
	degradedBefore := len(s.degraded)
	s.scanModes(chunk)
	promptBefore := s.promptSeen.IsZero()
	s.scanPrompt(chunk, s.lastOutput)
	promptShown := promptBefore && !s.promptSeen.IsZero()
	s.scrollback = append(s.scrollback, chunk...)
	if over := len(s.scrollback) - scrollbackLimit; over > 0 {
		s.scrollback = append([]byte(nil), s.scrollback[over:]...)
	}
	clients := s.clientList()
	pasteChanged := pasteBefore != s.paste
	degradedChanged := degradedBefore != len(s.degraded)
	s.mu.Unlock()
	s.sendTo(clients, frame{Type: "output", Session: s.name, Data: chunk, Offset: offset})
	if pasteChanged || promptShown {
		s.nudge()
	}
	if pasteChanged || degradedChanged {
		s.d.pushSessions()
	}
}

func (s *ptySession) scanModes(chunk []byte) { s.modeTracker.scan(chunk) }

// scanPrompt notes when the seat's prompt mark first shows. Caller holds mu.
func (s *ptySession) scanPrompt(chunk []byte, now time.Time) {
	mark, marked := promptMarks[s.seat]
	if !marked || !s.promptSeen.IsZero() {
		return
	}
	combined := append(append([]byte(nil), s.promptTail...), chunk...)
	if bytes.Contains(combined, mark) {
		s.promptSeen = now
		s.promptTail = nil
		return
	}
	if keep := len(mark) - 1; len(combined) > keep {
		combined = combined[len(combined)-keep:]
	}
	s.promptTail = combined
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// finish records the exit once and tells the holder it can go.
func (s *ptySession) finish(code int) {
	s.finishOnce.Do(func() {
		s.mu.Lock()
		s.exitCode = code
		clients := s.clientList()
		pending := s.pending
		s.pending = nil
		s.mu.Unlock()
		close(s.done)
		_ = s.holder.write(frame{Type: "release"})
		_ = s.holder.Close()
		s.sendTo(clients, frame{Type: "exit", Session: s.name, Code: code})
		for _, p := range pending {
			p.setState("failed", s.name+" ended before it was delivered")
		}
		s.d.logf("session %s (pid %d) exited %d", s.name, s.pid, code)
		s.d.forget(s)
	})
}

// letGo detaches from a session that keeps running, as a daemon stopping does.
func (s *ptySession) letGo() {
	s.mu.Lock()
	s.released = true
	s.mu.Unlock()
	_ = s.holder.Close()
}

// end stops the session the way closing its terminal would, then harder.
func (s *ptySession) end() {
	_ = syscall.Kill(-s.pid, syscall.SIGTERM)
	select {
	case <-s.done:
		return
	case <-time.After(terminateGrace):
	}
	_ = syscall.Kill(-s.pid, syscall.SIGKILL)
	select {
	case <-s.done:
	case <-time.After(time.Second):
		s.d.logf("session %s (pid %d) would not end", s.name, s.pid)
	}
}

func (s *ptySession) clientList() []*conn {
	clients := make([]*conn, 0, len(s.clients))
	for c := range s.clients {
		clients = append(clients, c)
	}
	return clients
}

func (s *ptySession) sendTo(clients []*conn, message frame) {
	for _, c := range clients {
		if err := c.write(message); err != nil {
			s.detach(c)
		}
	}
}

// attach registers a client and replays the scrollback. From is a stream offset
// a client already drew, so a reconnecting window gets only what it missed.
func (s *ptySession) attach(c *conn, replay bool, from int64) {
	s.mu.Lock()
	s.clients[c] = true
	var history []byte
	var offset int64
	if replay {
		history, offset = replayFrom(s, from)
	}
	s.mu.Unlock()
	if len(history) > 0 {
		s.sendTo([]*conn{c}, frame{Type: "output", Session: s.name, Data: history, Offset: offset})
	}
}

// replayFrom is the scrollback after offset from, without terminal queries, and
// the stream offset it ends at. Caller holds mu.
func replayFrom(s *ptySession, from int64) ([]byte, int64) {
	held := s.scrollback
	if start := s.outputOffset - int64(len(held)); from > start {
		if from >= s.outputOffset {
			held = nil
		} else {
			held = held[from-start:]
		}
	}
	history := withoutQueries(held)
	return history, s.outputOffset - int64(len(history))
}

func (s *ptySession) detach(c *conn) {
	s.mu.Lock()
	delete(s.clients, c)
	s.mu.Unlock()
}

func (s *ptySession) resize(rows, cols int) {
	if rows <= 0 || cols <= 0 {
		return
	}
	if s.holder != nil {
		_ = s.holder.write(frame{Type: "resize", Rows: clampSize(rows, 24), Cols: clampSize(cols, 80)})
	}
}

// typeInput is a person at a client. It is the only path that is not stamped.
func (s *ptySession) typeInput(data []byte) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	err := s.writePTY(data)
	s.mu.Lock()
	s.lastInput = time.Now()
	drafting := s.draft > 0
	s.trackDraft(data)
	flipped := drafting != (s.draft > 0)
	s.mu.Unlock()
	s.nudge()
	if flipped {
		s.d.pushSessions()
	}
	return err
}

// trackDraft counts what Kai has typed and not sent. Escape sequences and a
// terminal's replies to queries are not text. Caller holds mu.
func (s *ptySession) trackDraft(data []byte) {
	keys := &s.keys
	for _, b := range data {
		switch {
		case keys.skip > 0:
			keys.skip--
		case keys.str:
			keys.str = !(b == 0x07 || (keys.strEsc && b == '\\'))
			keys.strEsc = b == 0x1b
		case keys.csi:
			keys.params = append(keys.params, b)
			if b >= 0x40 && b <= 0x7e {
				switch string(keys.params) {
				case "200~":
					keys.inPaste = true
				case "201~":
					keys.inPaste = false
				case "M":
					keys.skip = 3
				}
				keys.csi, keys.params = false, nil
			}
		case keys.ss3:
			keys.ss3 = false
		case keys.escape:
			keys.escape = false
			keys.csi = b == '['
			keys.ss3 = b == 'O'
			keys.str = b == ']' || b == 'P' || b == '_' || b == '^' || b == 'X'
		case b == 0x1b:
			keys.escape = true
		case keys.inPaste:
			s.draft++
		case b == '\r' || b == '\n' || b == 0x03 || b == 0x15:
			s.draft = 0
		case b == 0x7f || b == 0x08:
			if s.draft > 0 {
				s.draft--
			}
		case b >= 0x20 && (b < 0x80 || b >= 0xc0):
			s.draft++
		}
	}
}

func (s *ptySession) enqueue(p *pendingSend) {
	p.mu.Lock()
	p.msg.Session = s.name
	p.mu.Unlock()
	s.mu.Lock()
	s.pending = append(s.pending, p)
	s.mu.Unlock()
	p.setState("queued", "")
	s.nudge()
	s.d.pushSessions()
}

func (s *ptySession) nudge() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *ptySession) deliverLoop() {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-s.done:
			return
		case <-s.wake:
		case <-ticker.C:
		}
		s.deliverNext(time.Now())
	}
}

// pasteSeats were watched turning bracketed paste on at a live prompt, so for
// them a quiet screen is not ready. See docs/aterm-daemon.md.
var pasteSeats = map[string]bool{"claude": true, "codex": true}

// promptMarks is text a harness paints once its prompt takes input. opencode turns
// paste on two seconds before that and drops one until then. See docs/aterm-daemon.md.
var promptMarks = map[string][]byte{"opencode": []byte("Ask anything")}

const (
	// promptSettle is the beat after the mark, so the input is mounted.
	promptSettle = 250 * time.Millisecond
	// promptWait is how long a seat waits for its mark before trying anyway, for
	// a version that words it differently or a session adopted long after it.
	promptWait = 20 * time.Second
)

// ready is whether the program can take a message yet. Caller holds mu.
func (s *ptySession) ready(now time.Time) bool {
	age := now.Sub(s.started)
	if _, marked := promptMarks[s.seat]; marked {
		if s.promptSeen.IsZero() {
			return age > promptWait
		}
		return now.Sub(s.promptSeen) > promptSettle
	}
	if s.pasteSeen {
		return age > 1500*time.Millisecond
	}
	return !pasteSeats[s.seat] && age > 30*time.Second && now.Sub(s.lastOutput) > 2*time.Second
}

// holdReason is why a person at the keyboard outranks a message. Caller holds mu.
func (s *ptySession) holdReason(now time.Time) string {
	since := now.Sub(s.lastInput)
	switch {
	case s.lastInput.IsZero():
		return ""
	case since < typingHold:
		return "Kai is typing in " + s.name
	case s.draft > 0 && since < draftHold:
		return "Kai has an unsent draft in " + s.name
	}
	return ""
}

func (s *ptySession) deliverNext(now time.Time) {
	s.mu.Lock()
	if len(s.pending) == 0 {
		s.mu.Unlock()
		return
	}
	next := s.pending[0]
	ready := s.ready(now)
	hold := s.holdReason(now)
	paste := s.paste
	s.mu.Unlock()
	switch {
	case !ready:
		next.setState("queued", s.name+" is still starting")
		return
	case hold != "":
		next.setState("held", hold)
		return
	}
	s.writeMu.Lock()
	s.mu.Lock()
	// Kai may have started typing while this waited for the lock.
	if hold := s.holdReason(time.Now()); hold != "" {
		s.mu.Unlock()
		s.writeMu.Unlock()
		next.setState("held", hold)
		return
	}
	s.mu.Unlock()
	err := s.inject(next.text, paste)
	s.mu.Lock()
	if len(s.pending) > 0 && s.pending[0] == next {
		s.pending = s.pending[1:]
	}
	s.mu.Unlock()
	s.writeMu.Unlock()
	if err != nil {
		next.setState("failed", "writing to "+s.name+": "+err.Error())
		return
	}
	next.setState("delivered", "")
	s.d.pushSessions()
}

// inject types a message and submits it. Without bracketed paste a newline
// would submit early, so the lines join. Caller holds writeMu.
func (s *ptySession) inject(text string, paste bool) error {
	body := strings.Join(strings.Split(text, "\n"), " ")
	if paste {
		body = "\x1b[200~" + text + "\x1b[201~"
	}
	if err := s.writePTY([]byte(body)); err != nil {
		return err
	}
	time.Sleep(submitDelay)
	return s.writePTY([]byte("\r"))
}

func (s *ptySession) view() sessionView {
	s.mu.Lock()
	defer s.mu.Unlock()
	return sessionView{
		Name:     s.name,
		Role:     s.role,
		Identity: s.identity,
		Seat:     s.seat,
		PID:      s.pid,
		Started:  s.started.UTC(),
		Clients:  len(s.clients),
		Paste:    s.paste,
		Ready:    s.ready(time.Now()),
		Drafted:  s.draft > 0,
		Pending:  len(s.pending),
		Degraded: append([]string(nil), s.degraded...),
	}
}
