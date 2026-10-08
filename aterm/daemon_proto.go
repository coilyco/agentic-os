package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"
)

// daemonFormat names the wire contract. A client and daemon that disagree on
// it refuse each other rather than half-talk. See docs/aterm-daemon.md.
const daemonFormat = "aterm.daemon.v1"

const (
	daemonSocketEnv = "ATERM_DAEMON_SOCKET"
	sessionTokenEnv = "ATERM_SESSION_TOKEN"
	sessionNameEnv  = "ATERM_SESSION"
	// Terminal output is bursty, and a JSON line carries it base64, so the
	// scanner's ceiling sits well above one read of the PTY.
	maxFrame = 4 << 20
)

// frame is every message on the wire, one JSON object per line. Fields a type
// does not use stay empty. The same objects ride a websocket later.
type frame struct {
	Type    string `json:"type"`
	ID      string `json:"id,omitempty"`
	Format  string `json:"format,omitempty"`
	Version string `json:"version,omitempty"`
	// Features lists what the daemon answers beyond the format, in welcome.
	Features []string `json:"features,omitempty"`
	Session  string   `json:"session,omitempty"`

	// spawn
	Role     string   `json:"role,omitempty"`
	Identity string   `json:"identity,omitempty"`
	Seat     string   `json:"seat,omitempty"`
	Argv     []string `json:"argv,omitempty"`
	Env      []string `json:"env,omitempty"`
	Cwd      string   `json:"cwd,omitempty"`
	Rows     int      `json:"rows,omitempty"`
	Cols     int      `json:"cols,omitempty"`

	// attach
	Replay bool `json:"replay,omitempty"`

	// input and output
	Data   []byte `json:"data,omitempty"`
	Offset int64  `json:"offset,omitempty"`

	// send
	Token  string `json:"token,omitempty"`
	Target string `json:"target,omitempty"`
	Body   string `json:"body,omitempty"`
	Launch bool   `json:"launch,omitempty"`
	// New opens a fresh instance of the role even when one is live.
	New bool `json:"new,omitempty"`
	// Wait is seconds a send holds its reply for the final state, 3 when absent.
	Wait int `json:"wait,omitempty"`
	// NotifyIdle asks for one notice typed into the sender when the target next goes idle.
	NotifyIdle bool `json:"notify_idle,omitempty"`
	// Force closes a session even while it holds a draft or undelivered messages.
	Force bool `json:"force,omitempty"`
	// Peek asks a claim which name it would grant, without holding it.
	Peek bool `json:"peek,omitempty"`

	// replies and events
	Message  *peerMessage  `json:"message,omitempty"`
	Sessions []sessionView `json:"sessions,omitempty"`
	// Lines is how many screen rows a status asks for, and Status is its answer.
	Lines  int            `json:"lines,omitempty"`
	Status *sessionStatus `json:"status,omitempty"`
	// ask_choice
	Ask    *choiceAsk    `json:"ask,omitempty"`
	AskID  string        `json:"ask_id,omitempty"`
	Picks  []int         `json:"picks,omitempty"`
	Text   string        `json:"text,omitempty"`
	State  string        `json:"state,omitempty"`
	Answer *choiceAnswer `json:"answer,omitempty"`
	Roster *listedRoster `json:"roster,omitempty"`
	Code   int           `json:"code,omitempty"`
	Error  string        `json:"error,omitempty"`
	// Reason is the stable name of a typing refusal on an error, and of a
	// refusal standing on a welcome. See docs/aterm-daemon.md.
	Reason string `json:"reason,omitempty"`
	// Typing is on a welcome to a websocket: whether the peer may type.
	Typing  *typingStanding `json:"typing,omitempty"`
	Channel string          `json:"channel,omitempty"`
	PID     int             `json:"pid,omitempty"`
	// Hold is what a session holder reports on attach, and is not part of the
	// client wire.
	Hold *holdInfo `json:"hold,omitempty"`
}

// sessionView is one live session as a client sees it. It is the roster the
// daemon pushes on the sessions channel.
type sessionView struct {
	Name     string    `json:"name"`
	Role     string    `json:"role"`
	Identity string    `json:"identity"`
	Seat     string    `json:"seat"`
	PID      int       `json:"pid"`
	Started  time.Time `json:"started"`
	Clients  int       `json:"clients"`
	// Paste is whether the program asked for bracketed paste, which decides
	// how a message is typed into it.
	Paste   bool `json:"bracketed_paste"`
	Ready   bool `json:"ready"`
	Drafted bool `json:"kai_drafting"`
	Pending int  `json:"pending"`
	// State is what the screen shows: starting, busy, idle, or prompt. Both fields
	// are absent from a daemon that predates them. See docs/aterm-daemon.md.
	State        string `json:"state,omitempty"`
	QuietSeconds int    `json:"quiet_seconds"`
	// Degraded names the startup steps agent-compose launched without.
	Degraded []string `json:"degraded,omitempty"`
	// Context is how full the seat's context is. Absent until a source has read
	// one, and from a daemon that predates the field.
	Context *contextView `json:"context,omitempty"`
}

// peerMessage is one send and where it stands: queued, held, launching,
// delivered, or failed. See docs/aterm-daemon.md.
type peerMessage struct {
	ID       string    `json:"id"`
	From     string    `json:"from"`
	Target   string    `json:"target"`
	Session  string    `json:"session,omitempty"`
	State    string    `json:"state"`
	Reason   string    `json:"reason,omitempty"`
	Accepted time.Time `json:"accepted"`
}

// conn is one side of a framed connection, over the unix socket or a
// websocket. Writes are serialized because events come from several goroutines.
type conn struct {
	raw       net.Conn
	readLine  func() ([]byte, error)
	writeLine func([]byte) error
	closer    func() error
	mu        sync.Mutex
	features  []string
	// peerVersion is the build the daemon reported in its welcome.
	peerVersion string
}

// sendNewFeature is how a client knows the daemon reads `new` on a send. A
// daemon predating it ignores the field and delivers to the live session.
const sendNewFeature = "send-new"

// sendWaitFeature is how a client knows the daemon reads `wait` on a send.
const sendWaitFeature = "send-wait"

// sendIdleFeature is how a client knows the daemon reads `notify_idle` on a send.
const sendIdleFeature = "send-idle"

// closeFeature is how a client knows the daemon answers a close frame.
const closeFeature = "close"

// statusFeature is how a client knows the daemon answers a status frame.
const statusFeature = "status"

// clearFeature is how a client knows the daemon answers a clear frame.
const clearFeature = "clear"

// claimFeature is how a client knows the daemon grants pool names atomically. A
// client facing one without it picks from a list, which two launches at once can race.
const claimFeature = "claim"

// typingGuardFeature is how a client knows a refusal carries a `reason` and a
// welcome to a websocket carries `typing`. A daemon without it let any browser type.
const typingGuardFeature = "typing-guard"

// holdFeature is how a client knows sessions live in holders, so stopping the
// daemon leaves them running. A daemon without it ends every session when it stops.
const holdFeature = "holders"

// newConn frames a stream as one JSON object per line.
func newConn(raw net.Conn) *conn {
	scanner := bufio.NewScanner(raw)
	scanner.Buffer(make([]byte, 64<<10), maxFrame)
	return &conn{
		raw: raw,
		readLine: func() ([]byte, error) {
			if !scanner.Scan() {
				if err := scanner.Err(); err != nil {
					return nil, err
				}
				return nil, errors.New("connection closed")
			}
			return scanner.Bytes(), nil
		},
		writeLine: func(line []byte) error {
			_ = raw.SetWriteDeadline(time.Now().Add(clientWriteTimeout))
			_, err := raw.Write(append(line, '\n'))
			return err
		},
		closer: raw.Close,
	}
}

func (c *conn) write(message frame) error {
	encoded, err := json.Marshal(message)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.writeLine(encoded)
}

func (c *conn) read() (frame, error) {
	line, err := c.readLine()
	if err != nil {
		return frame{}, err
	}
	var message frame
	if err := json.Unmarshal(line, &message); err != nil {
		return frame{}, fmt.Errorf("malformed frame: %w", err)
	}
	return message, nil
}

func (c *conn) Close() error { return c.closer() }

// daemonSocket is keyed by uid rather than HOME, because a session shadow
// moves HOME and every seat on the host must reach the same daemon.
func daemonSocket() string {
	if override := os.Getenv(daemonSocketEnv); override != "" {
		return override
	}
	return defaultDaemonSocket()
}

// defaultDaemonSocket is where the launchd agent's daemon listens, so only a
// caller on it asks launchd to start one.
func defaultDaemonSocket() string {
	return filepath.Join("/tmp", "aterm-"+strconv.Itoa(os.Getuid()), "daemon.sock")
}

// ensureSocketDir refuses a directory another user owns or others can enter,
// since the socket's permission is the whole of local client auth.
func ensureSocketDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !info.IsDir() || (ok && int(stat.Uid) != os.Getuid()) {
		return fmt.Errorf("%s is not a directory this user owns", dir)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("%s is open to other users (%v)", dir, info.Mode().Perm())
	}
	return nil
}

// dialDaemon connects and exchanges hellos. With start set, a daemon that is
// not answering is started and waited for.
func dialDaemon(start bool) (*conn, error) {
	socket := daemonSocket()
	raw, err := net.Dial("unix", socket)
	if err != nil && start {
		if startErr := startDaemon(socket); startErr != nil {
			return nil, startErr
		}
		for waited := time.Duration(0); waited < 5*time.Second; waited += 50 * time.Millisecond {
			if raw, err = net.Dial("unix", socket); err == nil {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("the aterm daemon at %s is not answering: %w", socket, err)
	}
	c := newConn(raw)
	if err := c.write(frame{Type: "hello", Format: daemonFormat, Version: version}); err != nil {
		_ = c.Close()
		return nil, err
	}
	reply, err := c.read()
	if err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("the aterm daemon did not answer its hello: %w", err)
	}
	if reply.Type != "welcome" || reply.Format != daemonFormat {
		_ = c.Close()
		return nil, fmt.Errorf("the aterm daemon speaks %q, this aterm speaks %s: %s",
			reply.Format, daemonFormat, reply.Error)
	}
	c.features = reply.Features
	c.peerVersion = reply.Version
	return c, nil
}

// daemonStartArgs are extra flags for a daemon a client starts, which a test
// uses to keep it off the host's listeners.
var daemonStartArgs []string

// startDaemon runs this binary's daemon verb detached, logging beside the
// socket. A second start loses the lock race and exits on its own.
func startDaemon(socket string) error {
	if startViaLaunchd(socket) {
		return nil
	}
	dir := filepath.Dir(socket)
	if err := ensureSocketDir(dir); err != nil {
		return fmt.Errorf("prepare the daemon directory: %w", err)
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	log, err := os.OpenFile(filepath.Join(dir, "daemon.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer log.Close()
	command := exec.Command(self, append([]string{"daemon", "--socket", socket}, daemonStartArgs...)...)
	command.Stdout = log
	command.Stderr = log
	command.Dir = "/"
	command.SysProcAttr = detachAttr()
	if err := command.Start(); err != nil {
		return fmt.Errorf("start the aterm daemon: %w", err)
	}
	go func() { _ = command.Wait() }()
	return nil
}

// request sends one frame and returns the first reply carrying its id.
func (c *conn) request(message frame) (frame, error) {
	if message.ID == "" {
		message.ID = randomID(6)
	}
	if err := c.write(message); err != nil {
		return frame{}, err
	}
	for {
		reply, err := c.read()
		if err != nil {
			return frame{}, err
		}
		if reply.ID != message.ID {
			continue
		}
		if reply.Type == "error" {
			return reply, errors.New(reply.Error)
		}
		return reply, nil
	}
}
