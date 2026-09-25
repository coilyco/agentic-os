package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
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

// decset matches a private mode set or reset. 2004 among its parameters is
// bracketed paste, which a TUI turns on to tell a paste from typing.
var decset = regexp.MustCompile("\x1b\\[\\?([0-9;]*)([hl])")

type ptySession struct {
	d        *daemon
	name     string
	role     string
	identity string
	seat     string
	token    string
	pid      int
	started  time.Time
	cmd      *exec.Cmd
	ptmx     *os.File

	// writeMu serializes every write to the PTY, so a message typed in never
	// interleaves with a person's keystrokes.
	writeMu sync.Mutex

	mu           sync.Mutex
	clients      map[*conn]bool
	scrollback   []byte
	outputOffset int64
	paste        bool
	pasteSeen    bool
	tail         []byte
	lastOutput   time.Time
	lastInput    time.Time
	draft        int
	keys         keyState
	pending      []*pendingSend
	wake         chan struct{}
	done         chan struct{}
	exitCode     int
}

// keyState carries a partly read escape sequence across input chunks.
type keyState struct {
	escape  bool
	csi     bool
	ss3     bool
	params  []byte
	inPaste bool
}

func startPTYSession(d *daemon, name string, message frame) (*ptySession, error) {
	program, err := lookPathIn(message.Argv[0], message.Env)
	if err != nil {
		return nil, withExit(exitMissing, fmt.Errorf("start %s: %w", message.Argv[0], err))
	}
	token := randomID(24)
	command := exec.Command(program, message.Argv[1:]...)
	command.Args[0] = message.Argv[0]
	command.Env = sessionEnv(message.Env, name, token)
	command.Dir = message.Cwd
	size := &pty.Winsize{Rows: uint16(clampSize(message.Rows, 24)), Cols: uint16(clampSize(message.Cols, 80))}
	ptmx, err := pty.StartWithSize(command, size)
	if err != nil {
		return nil, fmt.Errorf("start %s: %w", message.Argv[0], err)
	}
	now := time.Now()
	s := &ptySession{
		d:          d,
		name:       name,
		role:       message.Role,
		identity:   message.Identity,
		seat:       message.Seat,
		token:      token,
		pid:        command.Process.Pid,
		started:    now,
		cmd:        command,
		ptmx:       ptmx,
		clients:    map[*conn]bool{},
		lastOutput: now,
		wake:       make(chan struct{}, 1),
		done:       make(chan struct{}),
	}
	readDone := make(chan struct{})
	go s.readOutput(readDone)
	go s.waitExit(readDone)
	go s.deliverLoop()
	return s, nil
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

func (s *ptySession) readOutput(readDone chan struct{}) {
	defer close(readDone)
	buffer := make([]byte, 32<<10)
	for {
		n, err := s.ptmx.Read(buffer)
		if n > 0 {
			s.output(append([]byte(nil), buffer[:n]...))
		}
		if err != nil {
			return
		}
	}
}

func (s *ptySession) output(chunk []byte) {
	s.mu.Lock()
	offset := s.outputOffset
	s.outputOffset += int64(len(chunk))
	s.lastOutput = time.Now()
	pasteBefore := s.paste
	s.scanModes(chunk)
	s.scrollback = append(s.scrollback, chunk...)
	if over := len(s.scrollback) - scrollbackLimit; over > 0 {
		s.scrollback = append([]byte(nil), s.scrollback[over:]...)
	}
	clients := s.clientList()
	pasteChanged := pasteBefore != s.paste
	s.mu.Unlock()
	s.sendTo(clients, frame{Type: "output", Session: s.name, Data: chunk, Offset: offset})
	if pasteChanged {
		s.nudge()
		s.d.pushSessions()
	}
}

// scanModes follows bracketed paste across chunk boundaries by keeping the
// tail a split sequence would start in. Caller holds mu.
func (s *ptySession) scanModes(chunk []byte) {
	combined := append(append([]byte(nil), s.tail...), chunk...)
	for _, match := range decset.FindAllSubmatchIndex(combined, -1) {
		if match[1] <= len(s.tail) {
			continue
		}
		params := strings.Split(string(combined[match[2]:match[3]]), ";")
		if !containsString(params, "2004") {
			continue
		}
		s.paste = combined[match[4]] == 'h'
		if s.paste {
			s.pasteSeen = true
		}
	}
	if len(combined) > 32 {
		combined = combined[len(combined)-32:]
	}
	s.tail = combined
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func (s *ptySession) waitExit(readDone chan struct{}) {
	err := s.cmd.Wait()
	code := 0
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code = exitErr.ExitCode()
		if code < 0 {
			code = 1
		}
	} else if err != nil {
		code = 1
	}
	// A background child still holding the terminal keeps the reader open,
	// so the drain is bounded.
	select {
	case <-readDone:
	case <-time.After(500 * time.Millisecond):
	}
	s.mu.Lock()
	s.exitCode = code
	clients := s.clientList()
	pending := s.pending
	s.pending = nil
	s.mu.Unlock()
	close(s.done)
	_ = s.ptmx.Close()
	s.sendTo(clients, frame{Type: "exit", Session: s.name, Code: code})
	for _, p := range pending {
		p.setState("failed", s.name+" ended before it was delivered")
	}
	s.d.logf("session %s (pid %d) exited %d", s.name, s.pid, code)
	s.d.forget(s)
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
		_ = c.raw.SetWriteDeadline(time.Now().Add(clientWriteTimeout))
		if err := c.write(message); err != nil {
			s.detach(c)
		}
	}
}

func (s *ptySession) attach(c *conn, replay bool) {
	s.mu.Lock()
	s.clients[c] = true
	var history []byte
	if replay {
		history = append([]byte(nil), s.scrollback...)
	}
	offset := s.outputOffset - int64(len(history))
	s.mu.Unlock()
	if len(history) > 0 {
		s.sendTo([]*conn{c}, frame{Type: "output", Session: s.name, Data: history, Offset: offset})
	}
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
	_ = pty.Setsize(s.ptmx, &pty.Winsize{Rows: uint16(clampSize(rows, 24)), Cols: uint16(clampSize(cols, 80))})
}

// typeInput is a person at a client. It is the only path that is not stamped.
func (s *ptySession) typeInput(data []byte) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err := s.ptmx.Write(data)
	s.mu.Lock()
	s.lastInput = time.Now()
	s.trackDraft(data)
	s.mu.Unlock()
	s.nudge()
	return err
}

// trackDraft counts what Kai has typed and not sent. Escape sequences are
// keys, not text. Caller holds mu.
func (s *ptySession) trackDraft(data []byte) {
	keys := &s.keys
	for _, b := range data {
		switch {
		case keys.csi:
			keys.params = append(keys.params, b)
			if b >= 0x40 && b <= 0x7e {
				switch string(keys.params) {
				case "200~":
					keys.inPaste = true
				case "201~":
					keys.inPaste = false
				}
				keys.csi, keys.params = false, nil
			}
		case keys.ss3:
			keys.ss3 = false
		case keys.escape:
			keys.escape = false
			keys.csi = b == '['
			keys.ss3 = b == 'O'
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

// ready is whether the program can take a message yet. Caller holds mu.
func (s *ptySession) ready(now time.Time) bool {
	age := now.Sub(s.started)
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
	if _, err := s.ptmx.Write([]byte(body)); err != nil {
		return err
	}
	time.Sleep(submitDelay)
	_, err := s.ptmx.Write([]byte("\r"))
	return err
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
	}
}
