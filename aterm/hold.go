package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
)

// holdFormat names the wire between the daemon and one holder, kept for the
// holder's life so a newer daemon adopts an older one. See docs/aterm-daemon.md.
const holdFormat = "aterm.hold.v1"

const (
	holdCommand = "hold"
	// An exited session's code and last output wait this long for a daemon
	// that was away, then the holder goes.
	holdLinger = 10 * time.Minute
	// The daemon waits this long for a new holder to say it started.
	holdStartWait = 10 * time.Second
)

// holdInfo is what a holder tells an attaching daemon: who the session is, and
// the state a wrapped ring cannot rebuild.
type holdInfo struct {
	Name     string    `json:"name"`
	Role     string    `json:"role"`
	Identity string    `json:"identity"`
	Seat     string    `json:"seat"`
	Token    string    `json:"token"`
	PID      int       `json:"pid"`
	Started  time.Time `json:"started"`
	// ReplayStart is the stream offset of the first replayed byte, so offsets
	// stay one sequence across daemons.
	ReplayStart int64    `json:"replay_start"`
	Paste       bool     `json:"paste,omitempty"`
	PasteSeen   bool     `json:"paste_seen,omitempty"`
	Degraded    []string `json:"degraded,omitempty"`
	// Rows and Cols are the PTY's size now, so a daemon rebuilding the screen
	// from the replay lays it out the way the program drew it.
	Rows   int  `json:"rows,omitempty"`
	Cols   int  `json:"cols,omitempty"`
	Exited bool `json:"exited,omitempty"`
	Code   int  `json:"code,omitempty"`

	// Kind and Cwd let a daemon adopting this holder tell a terminal from a seat.
	Kind  string `json:"kind,omitempty"`
	Cwd   string `json:"cwd,omitempty"`
	Label string `json:"label,omitempty"`

	// MCPApps and Gateways let an adopting daemon rebuild the gateway. Memory
	// only, since a stdio spec's env can carry credentials.
	MCPApps  bool                   `json:"mcp_apps,omitempty"`
	Gateways map[string]gatewaySpec `json:"gateways,omitempty"`
}

// modeTracker follows what a session's output asked of the terminal. The
// holder and the daemon both keep one, since the holder outlives the daemon.
type modeTracker struct {
	paste     bool
	pasteSeen bool
	degraded  []string
	tail      []byte
}

// scan follows bracketed paste across chunk boundaries by keeping the tail a
// split sequence would start in.
func (m *modeTracker) scan(chunk []byte) {
	combined := append(append([]byte(nil), m.tail...), chunk...)
	for _, match := range decset.FindAllSubmatchIndex(combined, -1) {
		if match[1] <= len(m.tail) {
			continue
		}
		params := strings.Split(string(combined[match[2]:match[3]]), ";")
		if !containsString(params, "2004") {
			continue
		}
		m.paste = combined[match[4]] == 'h'
		if m.paste {
			m.pasteSeen = true
		}
	}
	for _, match := range degradedMark.FindAllSubmatchIndex(combined, -1) {
		if match[1] <= len(m.tail) {
			continue
		}
		m.degraded = nil
		for _, step := range strings.Split(string(combined[match[2]:match[3]]), ",") {
			if step != "" {
				m.degraded = append(m.degraded, step)
			}
		}
	}
	if len(combined) > scanTail {
		combined = combined[len(combined)-scanTail:]
	}
	m.tail = combined
}

// holder owns one session's PTY and child and nothing else, so the daemon that
// keeps names, tokens and delivery can be replaced without ending it.
type holder struct {
	spec    frame
	cmd     *exec.Cmd
	ptmx    *os.File
	started time.Time

	writeMu sync.Mutex

	mu      sync.Mutex
	clients map[*conn]bool
	ring    []byte
	offset  int64
	modes   modeTracker
	exited  bool
	code    int
	// gateways are the servers the daemon registered, by name.
	gateways map[string]gatewaySpec

	released    chan struct{}
	releaseOnce sync.Once
}

// holdSocketPath names a holder socket. A client chooses the name, so it is
// reduced to a safe slug plus a hash that keeps names reducing alike apart.
func holdSocketPath(dir, name string) string { return filepath.Join(dir, fileStem(name)+".sock") }

func fileStem(name string) string {
	slug := regexp.MustCompile(`[^A-Za-z0-9._-]+`).ReplaceAllString(name, "_")
	slug = strings.TrimLeft(slug, ".")
	if len(slug) > 48 {
		slug = slug[:48]
	}
	sum := sha256.Sum256([]byte(name))
	return slug + "-" + hex.EncodeToString(sum[:4])
}

// runHold is the holder verb. The spawn arrives as one stdin line, never argv or
// disk, since its environment holds credentials. The reply is one line.
func runHold(socket string, in io.Reader, out io.Writer) error {
	var spec frame
	if err := json.NewDecoder(in).Decode(&spec); err != nil {
		return replyHold(out, 0, fmt.Errorf("read the spawn: %w", err))
	}
	if len(spec.Argv) == 0 {
		return replyHold(out, exitUsage, errors.New("spawn needs an argv"))
	}
	program, err := lookPathIn(spec.Argv[0], spec.Env)
	if err != nil {
		return replyHold(out, exitMissing, fmt.Errorf("start %s: %w", spec.Argv[0], err))
	}
	_ = os.Remove(socket)
	listener, err := net.Listen("unix", socket)
	if err != nil {
		return replyHold(out, 0, err)
	}
	if err := os.Chmod(socket, 0o600); err != nil {
		_ = listener.Close()
		return replyHold(out, 0, err)
	}
	command := exec.Command(program, spec.Argv[1:]...)
	command.Args[0] = spec.Argv[0]
	command.Env = spec.Env
	command.Dir = spec.Cwd
	size := &pty.Winsize{Rows: uint16(clampSize(spec.Rows, 24)), Cols: uint16(clampSize(spec.Cols, 80))}
	ptmx, err := pty.StartWithSize(command, size)
	if err != nil {
		_ = listener.Close()
		_ = os.Remove(socket)
		return replyHold(out, 0, fmt.Errorf("start %s: %w", spec.Argv[0], err))
	}
	h := &holder{
		spec:     spec,
		cmd:      command,
		ptmx:     ptmx,
		started:  time.Now(),
		clients:  map[*conn]bool{},
		released: make(chan struct{}),
	}
	h.spec.Env = nil
	// A terminal hangup on the launching shell must not reach a holder, and a
	// stop request ends the session the way closing its terminal would.
	signal.Ignore(syscall.SIGHUP)
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-stop
		_ = syscall.Kill(-command.Process.Pid, syscall.SIGTERM)
	}()
	readDone := make(chan struct{})
	go h.pump(readDone)
	go h.waitExit(readDone)
	go func() {
		for {
			raw, err := listener.Accept()
			if err != nil {
				return
			}
			go h.serve(newConn(raw))
		}
	}()
	if err := replyHold(out, 0, nil, command.Process.Pid); err != nil {
		return err
	}
	<-h.released
	_ = listener.Close()
	_ = os.Remove(socket)
	_ = os.Remove(strings.TrimSuffix(socket, ".sock") + ".log")
	return nil
}

// replyHold writes the one startup line, and returns the failure it reports.
func replyHold(out io.Writer, code int, failure error, pid ...int) error {
	reply := frame{Type: "ready", Code: code}
	if failure != nil {
		reply.Error = failure.Error()
	}
	if len(pid) > 0 {
		reply.PID = pid[0]
	}
	encoded, err := json.Marshal(reply)
	if err != nil {
		return err
	}
	if _, err := out.Write(append(encoded, '\n')); err != nil {
		return err
	}
	return failure
}

func (h *holder) release() { h.releaseOnce.Do(func() { close(h.released) }) }

func (h *holder) pump(readDone chan struct{}) {
	defer close(readDone)
	buffer := make([]byte, 32<<10)
	for {
		n, err := h.ptmx.Read(buffer)
		if n > 0 {
			chunk := append([]byte(nil), buffer[:n]...)
			h.mu.Lock()
			offset := h.offset
			h.offset += int64(n)
			h.modes.scan(chunk)
			h.ring = append(h.ring, chunk...)
			if over := len(h.ring) - scrollbackLimit; over > 0 {
				h.ring = append([]byte(nil), h.ring[over:]...)
			}
			clients := h.clientList()
			h.mu.Unlock()
			h.sendTo(clients, frame{Type: "output", Session: h.spec.Session, Data: chunk, Offset: offset})
		}
		if err != nil {
			return
		}
	}
}

func (h *holder) waitExit(readDone chan struct{}) {
	err := h.cmd.Wait()
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
	h.mu.Lock()
	h.exited, h.code = true, code
	clients := h.clientList()
	h.mu.Unlock()
	_ = h.ptmx.Close()
	h.sendTo(clients, frame{Type: "exit", Session: h.spec.Session, Code: code})
	time.AfterFunc(holdLinger, h.release)
}

// clientList is the clients to fan out to. Caller holds mu.
func (h *holder) clientList() []*conn {
	clients := make([]*conn, 0, len(h.clients))
	for c := range h.clients {
		clients = append(clients, c)
	}
	return clients
}

func (h *holder) sendTo(clients []*conn, message frame) {
	for _, c := range clients {
		if err := c.write(message); err != nil {
			// The client's connection stays open on its side and gets nothing
			// more, so leave a trace in the holder log (COI-2515).
			fmt.Fprintf(os.Stderr, "aterm hold: dropped a client after a failed %s write: %v\n", message.Type, err)
			h.mu.Lock()
			delete(h.clients, c)
			h.mu.Unlock()
		}
	}
}

func (h *holder) serve(c *conn) {
	defer c.Close()
	hello, err := c.read()
	if err != nil || hello.Type != "hello" {
		return
	}
	if hello.Format != holdFormat {
		_ = c.write(frame{Type: "welcome", Format: holdFormat, Error: "unsupported format " + hello.Format})
		return
	}
	if err := c.write(frame{Type: "welcome", Format: holdFormat, Version: version, PID: os.Getpid()}); err != nil {
		return
	}
	defer func() {
		h.mu.Lock()
		delete(h.clients, c)
		h.mu.Unlock()
	}()
	for {
		message, err := c.read()
		if err != nil {
			return
		}
		switch message.Type {
		case "attach":
			if err := h.attach(c, message); err != nil {
				return
			}
		case "input":
			h.writeMu.Lock()
			_, err := h.ptmx.Write(message.Data)
			h.writeMu.Unlock()
			if err != nil {
				_ = c.write(frame{Type: "error", ID: message.ID, Error: err.Error()})
			}
		case "resize":
			if message.Rows > 0 && message.Cols > 0 {
				_ = pty.Setsize(h.ptmx, &pty.Winsize{Rows: uint16(clampSize(message.Rows, 24)), Cols: uint16(clampSize(message.Cols, 80))})
			}
		case "gateway_set":
			if message.Gateway != nil && message.Server != "" {
				h.mu.Lock()
				if h.gateways == nil {
					h.gateways = map[string]gatewaySpec{}
				}
				h.gateways[message.Server] = *message.Gateway
				h.mu.Unlock()
			}
		case "release":
			h.release()
		}
	}
}

// attach registers the client and replays under one lock, so no byte reaches
// it twice or out of order.
func (h *holder) attach(c *conn, message frame) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[c] = true
	var history []byte
	if message.Replay {
		history = append([]byte(nil), h.ring...)
	}
	rows, cols, _ := pty.Getsize(h.ptmx)
	info := holdInfo{
		Name:        h.spec.Session,
		Role:        h.spec.Role,
		Identity:    h.spec.Identity,
		Seat:        h.spec.Seat,
		Token:       h.spec.Token,
		PID:         h.cmd.Process.Pid,
		Started:     h.started.UTC(),
		ReplayStart: h.offset - int64(len(history)),
		Paste:       h.modes.paste,
		PasteSeen:   h.modes.pasteSeen,
		Degraded:    append([]string(nil), h.modes.degraded...),
		Rows:        rows,
		Cols:        cols,
		Exited:      h.exited,
		Code:        h.code,
		Kind:        h.spec.Kind,
		Cwd:         h.spec.Cwd,
		Label:       h.spec.Label,
		MCPApps:     h.spec.MCPApps,
		Gateways:    maps.Clone(h.gateways),
	}
	if err := c.write(frame{Type: "attached", ID: message.ID, Session: info.Name, PID: info.PID, Hold: &info}); err != nil {
		return err
	}
	if len(history) > 0 {
		if err := c.write(frame{Type: "output", Session: info.Name, Data: history, Offset: info.ReplayStart}); err != nil {
			return err
		}
	}
	if h.exited {
		return c.write(frame{Type: "exit", Session: info.Name, Code: h.code})
	}
	return nil
}

// startHolder runs the hold verb detached and returns once the session runs.
// The holder leads its own session, so the daemon ending signals nothing in it.
func startHolder(socket string, spec frame) (int, error) {
	self, err := os.Executable()
	if err != nil {
		return 0, err
	}
	log, err := os.OpenFile(strings.TrimSuffix(socket, ".sock")+".log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return 0, err
	}
	defer log.Close()
	command := exec.Command(self, holdCommand, "--socket", socket)
	command.Stderr = log
	command.Dir = "/"
	command.SysProcAttr = detachAttr()
	stdin, err := command.StdinPipe()
	if err != nil {
		return 0, err
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		return 0, err
	}
	if err := command.Start(); err != nil {
		return 0, fmt.Errorf("start the session holder: %w", err)
	}
	encoded, err := json.Marshal(spec)
	if err == nil {
		_, err = stdin.Write(append(encoded, '\n'))
	}
	_ = stdin.Close()
	type startup struct {
		reply frame
		err   error
	}
	started := make(chan startup, 1)
	go func() {
		var reply frame
		err := json.NewDecoder(stdout).Decode(&reply)
		started <- startup{reply, err}
	}()
	var result startup
	if err != nil {
		_ = command.Process.Kill()
		result.err = err
	} else {
		select {
		case result = <-started:
		case <-time.After(holdStartWait):
			_ = command.Process.Kill()
			result.err = errors.New("the session holder did not start in time")
		}
	}
	// Wait reaps the holder while this daemon lives. After the daemon goes,
	// launchd reaps it.
	go func() { _ = command.Wait() }()
	if result.err != nil {
		return 0, fmt.Errorf("the session holder did not report: %w", result.err)
	}
	if result.reply.Error != "" {
		failure := errors.New(result.reply.Error)
		if result.reply.Code != 0 {
			return 0, withExit(result.reply.Code, failure)
		}
		return 0, failure
	}
	return result.reply.PID, nil
}

// runHoldVerb is `aterm hold --socket <path>`, started by a daemon and never
// by a person, so it has no entry in the command tree.
func runHoldVerb(args []string) int {
	if len(args) != 2 || args[0] != "--socket" {
		fmt.Fprintln(os.Stderr, "aterm hold: started by the daemon, takes --socket <path>")
		return exitUsage
	}
	if err := runHold(args[1], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "aterm hold:", err)
		return exitCodeFor(err)
	}
	return 0
}
