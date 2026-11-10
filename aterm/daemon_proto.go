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

	// MCPApps is on a spawn the launch opted in to the MCP Apps gateway with, which
	// the holder keeps since it drops the environment that said so.
	MCPApps bool `json:"mcp_apps,omitempty"`

	// Origin is on a seat spawn: the launcher says whether it holds a window ("window") or
	// not ("headless"). Anything else is dropped. See references/seat-state.md.
	Origin string `json:"origin,omitempty"`

	// Kind is "terminal" on a spawn for a login shell, absent for a seat, and the
	// input type (mouse, wheel, key, text) on a browser_input. See docs/aterm-daemon.md.
	Kind string `json:"kind,omitempty"`

	// Label is opaque client data on a terminal spawn, echoed unchanged in spawned
	// and on each terminals entry. The daemon attaches no meaning to it.
	Label string `json:"label,omitempty"`

	// attach. Scales says this client renders a PTY larger than its box by panning.
	Replay bool `json:"replay,omitempty"`
	Scales bool `json:"scales,omitempty"`

	// input and output, and the image bytes of an image_upload
	Data   []byte `json:"data,omitempty"`
	Offset int64  `json:"offset,omitempty"`
	// Path is the absolute file an image_uploaded names.
	Path string `json:"path,omitempty"`
	// feedback: Image and Images are screenshot tokens, Screen the client's display text
	// (not "context", which a seat's meter owns), Record the report's id.
	Image  string          `json:"image,omitempty"`
	Images []string        `json:"images,omitempty"`
	Screen *feedbackScreen `json:"screen,omitempty"`
	Record string          `json:"record,omitempty"`
	// Delivered is how many clients an opened reply says took the address.
	Delivered int `json:"delivered,omitempty"`

	// Media: Media is one kept file, MediaID names one, and Length, Size and EOF answer a
	// media_read. EOF is a pointer so a false still rides every chunk. See daemon_media.go.
	Media   *mediaView `json:"media,omitempty"`
	MediaID string     `json:"media_id,omitempty"`
	Length  int        `json:"length,omitempty"`
	Size    int64      `json:"size,omitempty"`
	EOF     *bool      `json:"eof,omitempty"`

	// Artifacts: Artifact is a whole page pushed to the views channel, ArtifactID names
	// one, and Waiting and SeatEnded are set only when they changed. See daemon_artifact.go.
	Artifact   *artifactView `json:"artifact,omitempty"`
	ArtifactID string        `json:"artifact_id,omitempty"`
	Waiting    *bool         `json:"waiting,omitempty"`
	SeatEnded  *bool         `json:"seat_ended,omitempty"`
	Standing   bool          `json:"standing,omitempty"`

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
	// Self closes the caller's own session, named by Token alone and never by Target.
	Self bool `json:"self,omitempty"`
	// Peek asks a claim which name it would grant, without holding it.
	Peek bool `json:"peek,omitempty"`
	// All asks an inbox for the messages already read as well as the unread.
	All bool `json:"all,omitempty"`

	// replies and events
	Message *peerMessage   `json:"message,omitempty"`
	Inbox   []inboxMessage `json:"inbox,omitempty"`

	// github_token: Repo names the OWNER/REPO asked for, GithubToken and Expires
	// answer it. The token is a credential, so it rides only the reply frame.
	Repo        string        `json:"repo,omitempty"`
	Workflows   bool          `json:"workflows,omitempty"`
	GithubToken string        `json:"github_token,omitempty"`
	Expires     string        `json:"expires,omitempty"`
	Sessions    []sessionView `json:"sessions,omitempty"`

	// stage: Body is the command and Text the note. The reply carries State,
	// Opened and Viewers. See docs/aterm-pane.md.
	Opened  bool `json:"opened,omitempty"`
	Viewers int  `json:"viewers,omitempty"`

	// Terminals rides beside Sessions and never inside it, so a consumer that
	// counts or targets seats cannot meet a shell.
	Terminals []terminalView `json:"terminals,omitempty"`

	// Lines is how many screen rows a status asks for, and Status is its answer.
	Lines  int            `json:"lines,omitempty"`
	Status *sessionStatus `json:"status,omitempty"`
	// Reply answers a last_reply request, which carries a target.
	Reply *lastReplyView `json:"reply,omitempty"`
	// ask_choice
	Ask    *choiceAsk    `json:"ask,omitempty"`
	AskID  string        `json:"ask_id,omitempty"`
	Picks  []int         `json:"picks,omitempty"`
	Text   string        `json:"text,omitempty"`
	State  string        `json:"state,omitempty"`
	Answer *choiceAnswer `json:"answer,omitempty"`
	// grant_request carries Grant, grant_check carries Check, and the reply is
	// grant_verdict. See grant.go.
	Grant   *grantSpec    `json:"grant,omitempty"`
	Check   *grantCheck   `json:"check,omitempty"`
	Verdict *grantVerdict `json:"verdict,omitempty"`
	Roster  *listedRoster `json:"roster,omitempty"`
	Code    int           `json:"code,omitempty"`
	Error   string        `json:"error,omitempty"`
	// Passkey ceremonies: EnrollCode is the one-time code, Options and Credential
	// are WebAuthn JSON. See docs/aterm-daemon.md.
	EnrollCode string `json:"enroll_code,omitempty"`
	// Resume is the bearer proof an unlock reply issues and a resume frame presents,
	// ExpiresIn the seconds it outlives a dropped connection. See daemon_resume.go.
	Resume    string `json:"resume,omitempty"`
	ExpiresIn int    `json:"expires_in,omitempty"`
	// Unlock rides the welcome of a brokered channel only, and UnlockToken is the accounts
	// token an unlock frame presents. Token stays a seat's session token.
	Unlock      *unlockOffer    `json:"unlock,omitempty"`
	UnlockToken string          `json:"unlock_token,omitempty"`
	Options     json.RawMessage `json:"options,omitempty"`
	Credential  json.RawMessage `json:"credential,omitempty"`
	// Device key ceremonies. The binary fields are base64url, no padding, and
	// Attestation is an array of DER certificates, leaf first.
	Challenge   string          `json:"challenge,omitempty"`
	PublicKey   string          `json:"public_key,omitempty"`
	KeyID       string          `json:"key_id,omitempty"`
	Signature   string          `json:"signature,omitempty"`
	Attestation json.RawMessage `json:"attestation,omitempty"`
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

	// browser_*. State, Reason, Force and Data carry the shared fields above.
	// Params, shared with the MCP Apps frames, is declared below.
	Driver string `json:"driver,omitempty"`
	URL    string `json:"url,omitempty"`
	Title  string `json:"title,omitempty"`
	// Notify is a `notify` event for clients, and NotifyID the id a `notified` reply names.
	Notify   *notifyEvent    `json:"notify,omitempty"`
	NotifyID string          `json:"notify_id,omitempty"`
	Holder   string          `json:"holder,omitempty"`
	Tab      int             `json:"tab,omitempty"`
	Tabs     int             `json:"tabs,omitempty"`
	Client   string          `json:"client,omitempty"`
	Width    int             `json:"width,omitempty"`
	Height   int             `json:"height,omitempty"`
	Seq      int64           `json:"seq,omitempty"`
	Take     bool            `json:"take,omitempty"`
	Metadata json.RawMessage `json:"metadata,omitempty"`

	// MCP Apps: the gateway and the views channel. See docs/aterm-daemon.md.
	View       *viewFrame      `json:"view,omitempty"`
	ViewID     string          `json:"view_id,omitempty"`
	Server     string          `json:"server,omitempty"`
	Gateway    *gatewaySpec    `json:"gateway,omitempty"`
	Method     string          `json:"method,omitempty"`
	Params     json.RawMessage `json:"params,omitempty"`
	Result     json.RawMessage `json:"result,omitempty"`
	ToolResult json.RawMessage `json:"tool_result,omitempty"`
	Cancelled  string          `json:"cancelled,omitempty"`

	// Hosts answers a hosts request: the tailnet daemons that answered this one.
	Hosts []hostView `json:"hosts,omitempty"`

	// Web Push: Key is the VAPID public key on a push_key reply, and Push is a
	// browser's PushSubscription.toJSON(). See docs/aterm-daemon.md.
	Key  string            `json:"key,omitempty"`
	Push *pushSubscription `json:"push,omitempty"`
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
	// StateSeconds is how long State has lasted since the daemon first saw it, so a restart
	// counts from the restart. A pointer, so 0 is a reading and absence is an old daemon.
	StateSeconds *int `json:"state_seconds,omitempty"`
	// Degraded names the startup steps agent-compose launched without.
	Degraded []string `json:"degraded,omitempty"`
	// Context is how full the seat's context is. Absent until a source has read
	// one, and from a daemon that predates the field.
	Context *contextView `json:"context,omitempty"`
	// LastReply is the first line of the seat's last finished reply. Absent until a
	// transcript has shown one, and from a daemon that predates the field.
	LastReply string `json:"last_reply,omitempty"`
	// LastReplyAt is when that reply finished. The whole text is fetched with last_reply.
	LastReplyAt *time.Time `json:"last_reply_at,omitempty"`
	// Ref names this session and not its name, which is reused. close and clear take
	// `name@ref`. See daemon_names.go.
	Ref string `json:"ref,omitempty"`
	// Activity is the seat's own "what I'm doing" line, at most 57 characters, and
	// ActivityAt when it was set. Absent until set. See references/seat-state.md.
	Activity   string     `json:"activity,omitempty"`
	ActivityAt *time.Time `json:"activity_at,omitempty"`
	// Background counts the seat's background tasks. A pointer, so 0 is a reading and
	// absence is unknown, never zero. See references/seat-state.md.
	Background *int `json:"background,omitempty"`
	// Origin is how the seat was launched, a fact of the launch that attaching never moves.
	// Absent when the launcher or an older daemon did not say. See references/seat-state.md.
	Origin string `json:"origin,omitempty"`
}

// terminalView is one plain terminal as a client sees it. Cwd is where the shell
// started, not where it is now.
type terminalView struct {
	Name    string    `json:"name"`
	PID     int       `json:"pid"`
	Started time.Time `json:"started"`
	Clients int       `json:"clients"`
	Cwd     string    `json:"cwd,omitempty"`
	Label   string    `json:"label,omitempty"`
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
	// Text is set by broadcastMessage alone, never on a `sent` reply or receipt.
	Text    string `json:"text,omitempty"`
	TextCut bool   `json:"text_cut,omitempty"`
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
	// outbox, gone and outMu serve the daemon's broadcasts. See daemon_outbox.go.
	outMu  sync.Mutex
	outbox chan frame
	gone   chan struct{}
}

// sendNewFeature is how a client knows the daemon reads `new` on a send. A
// daemon predating it ignores the field and delivers to the live session.
const sendNewFeature = "send-new"

// sendWaitFeature is how a client knows the daemon reads `wait` on a send.
const sendWaitFeature = "send-wait"

// sendIdleFeature is how a client knows the daemon reads `notify_idle` on a send.
const sendIdleFeature = "send-idle"

// askRelayFeature is how a client knows the daemon reads `relay` on an ask and stamps
// `card_seat`. A daemon predating it would show the card on the asker, unrelayed.
const askRelayFeature = "ask-relay"

// stateAgeFeature is how a client knows a view carries `state_seconds` and a `sessions`
// frame is pushed when a state flips. See references/seat-state.md.
const stateAgeFeature = "state-age"

// lastReplyFeature is how a client knows the daemon answers last_reply and stamps
// `last_reply_at` on a session view.
const lastReplyFeature = "last-reply"

// closeFeature is how a client knows the daemon answers a close frame.
const closeFeature = "close"

// closeSelfFeature is how a client knows a close frame may carry Self.
const closeSelfFeature = "close-self"

// backgroundFeature is how a client knows a view may carry `background`.
const backgroundFeature = "background"

// closeUnpushedFeature is how a client knows a close refuses a seat holding unpushed
// work with reason `unpushed_work`, and takes `force` to close it anyway.
const closeUnpushedFeature = "close-unpushed"

// Launch origins, the closed set a spawn and a session view carry.
const (
	originWindow   = "window"
	originHeadless = "headless"
)

// originFeature is how a client knows a view may carry `origin`.
const originFeature = "origin"

// notifyFeature is how a client knows the daemon sends `notify` events and answers a
// `notify` frame from a coordinator seat.
const notifyFeature = "notify"

// reopenFeature is how a client knows the daemon answers a reopen frame.
const reopenFeature = "reopen"

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

// terminalsFeature is how a client knows the daemon spawns a `kind: "terminal"`
// and lists it under `terminals`. A daemon without it would start the spawn as a seat.
const terminalsFeature = "terminals"

// terminalLabelFeature is how a client knows the daemon keeps a `label` on a
// terminal spawn. A daemon without it drops the field, so the entry comes back bare.
const terminalLabelFeature = "terminal-label"

// maxTerminalLabel bounds a label in bytes. It is a name a client chose, not a payload.
const maxTerminalLabel = 256

// kindTerminal is the one spawn kind besides a seat.
const kindTerminal = "terminal"

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

func (c *conn) Close() error {
	c.outMu.Lock()
	if c.gone != nil {
		select {
		case <-c.gone:
		default:
			close(c.gone)
		}
	}
	c.outMu.Unlock()
	return c.closer()
}

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
