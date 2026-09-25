package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	// A daemon with nothing to serve exits, so an upgraded binary takes over
	// at the next launch rather than an old one serving forever.
	daemonIdle = 5 * time.Minute
	// A launch-and-deliver message waits this long for its target to open.
	launchWait = 3 * time.Minute
	// A client that cannot take output this long is dropped, not waited on.
	clientWriteTimeout = 5 * time.Second
)

type daemon struct {
	mu          sync.Mutex
	sessions    map[string]*ptySession
	tokens      map[string]*ptySession
	orphans     []*pendingSend
	subscribers map[*conn]bool
	conns       int
	lastActive  time.Time
	processes   func() ([]processEntry, error)
	logf        func(string, ...any)
}

func newDaemon(logf func(string, ...any)) *daemon {
	return &daemon{
		sessions:    map[string]*ptySession{},
		tokens:      map[string]*ptySession{},
		subscribers: map[*conn]bool{},
		lastActive:  time.Now(),
		processes:   listProcesses,
		logf:        logf,
	}
}

// runDaemon holds the lock, owns the socket, and serves until idle. A second
// daemon finding the lock held exits cleanly, which settles a start race.
func runDaemon(socket string, idle time.Duration, stderr io.Writer) error {
	dir := filepath.Dir(socket)
	if err := ensureSocketDir(dir); err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(dir, filepath.Base(socket)+".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		fmt.Fprintf(stderr, "aterm daemon: another daemon holds %s\n", socket)
		return nil
	}
	_ = os.Remove(socket)
	listener, err := net.Listen("unix", socket)
	if err != nil {
		return err
	}
	if err := os.Chmod(socket, 0o600); err != nil {
		_ = listener.Close()
		return err
	}
	logf := func(format string, args ...any) {
		fmt.Fprintf(stderr, "%s aterm daemon: %s\n", time.Now().UTC().Format(time.RFC3339), fmt.Sprintf(format, args...))
	}
	d := newDaemon(logf)
	logf("serving %s as pid %d, build %s", socket, os.Getpid(), version)
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-stop
		_ = listener.Close()
	}()
	go d.watchIdle(listener, idle)
	for {
		raw, err := listener.Accept()
		if err != nil {
			break
		}
		go d.serve(raw)
	}
	_ = os.Remove(socket)
	d.endAll()
	logf("stopped")
	return nil
}

func (d *daemon) watchIdle(listener net.Listener, idle time.Duration) {
	for range time.Tick(time.Second) {
		d.expireOrphans(time.Now())
		d.mu.Lock()
		quiet := len(d.sessions) == 0 && d.conns == 0 && time.Since(d.lastActive) > idle
		d.mu.Unlock()
		if quiet {
			d.logf("idle for %s with no session, exiting", idle)
			_ = listener.Close()
			return
		}
	}
}

func (d *daemon) endAll() {
	d.mu.Lock()
	sessions := make([]*ptySession, 0, len(d.sessions))
	for _, s := range d.sessions {
		sessions = append(sessions, s)
	}
	d.mu.Unlock()
	for _, s := range sessions {
		s.end()
	}
}

// client is one connection's standing: which sessions it spawned, which it is
// attached to, and whether it may type.
type client struct {
	c        *conn
	peerPID  int
	owned    map[string]bool
	attached map[string]*ptySession
}

func (d *daemon) serve(raw net.Conn) {
	c := newConn(raw)
	defer c.Close()
	hello, err := c.read()
	if err != nil || hello.Type != "hello" {
		return
	}
	if hello.Format != daemonFormat {
		_ = c.write(frame{Type: "welcome", Format: daemonFormat, Error: "unsupported format " + hello.Format})
		return
	}
	if err := c.write(frame{Type: "welcome", Format: daemonFormat, Version: version, PID: os.Getpid()}); err != nil {
		return
	}
	d.mu.Lock()
	d.conns++
	d.lastActive = time.Now()
	d.mu.Unlock()
	pid, _ := peerPID(raw)
	cl := &client{c: c, peerPID: pid, owned: map[string]bool{}, attached: map[string]*ptySession{}}
	defer func() {
		for _, s := range cl.attached {
			s.detach(c)
		}
		d.mu.Lock()
		d.conns--
		d.lastActive = time.Now()
		delete(d.subscribers, c)
		d.mu.Unlock()
		d.pushSessions()
	}()
	for {
		message, err := c.read()
		if err != nil {
			return
		}
		if err := d.handle(cl, message); err != nil {
			_ = c.write(frame{Type: "error", ID: message.ID, Error: err.Error(), Code: exitCodeFor(err)})
		}
	}
}

func (d *daemon) handle(cl *client, message frame) error {
	switch message.Type {
	case "spawn":
		s, err := d.spawn(message)
		if err != nil {
			return err
		}
		cl.owned[s.name] = true
		cl.attached[s.name] = s
		// The reply goes first, since a client reads up to it and no further.
		// The replay then carries whatever the child printed in between.
		if err := cl.c.write(frame{Type: "spawned", ID: message.ID, Session: s.name, PID: s.pid}); err != nil {
			return err
		}
		s.attach(cl.c, true)
		d.pushSessions()
		return nil
	case "attach":
		s := d.session(message.Session)
		if s == nil {
			return withExit(exitOffRoster, fmt.Errorf("no live session named %q", message.Session))
		}
		cl.attached[s.name] = s
		if err := cl.c.write(frame{Type: "attached", ID: message.ID, Session: s.name, PID: s.pid}); err != nil {
			return err
		}
		s.attach(cl.c, message.Replay)
		s.resize(message.Rows, message.Cols)
		d.pushSessions()
		return nil
	case "detach":
		if s := cl.attached[message.Session]; s != nil {
			s.detach(cl.c)
			delete(cl.attached, message.Session)
			d.pushSessions()
		}
		return nil
	case "input":
		s := cl.attached[message.Session]
		if s == nil {
			return fmt.Errorf("not attached to %q", message.Session)
		}
		if !cl.owned[s.name] && d.insideSession(cl.peerPID) {
			return errors.New("a process inside an aterm session cannot type into one, use `aterm send`")
		}
		return s.typeInput(message.Data)
	case "resize":
		if s := cl.attached[message.Session]; s != nil {
			s.resize(message.Rows, message.Cols)
		}
		return nil
	case "send":
		return d.send(cl.c, message)
	case "list":
		return cl.c.write(frame{Type: "sessions", ID: message.ID, Sessions: d.views()})
	case "whoami":
		s := d.byToken(message.Token)
		if s == nil {
			return withExit(exitUsage, errors.New("the token names no live session"))
		}
		return cl.c.write(frame{Type: "sessions", ID: message.ID, Sessions: []sessionView{s.view()}})
	case "subscribe":
		if message.Channel != "sessions" {
			return fmt.Errorf("no channel named %q", message.Channel)
		}
		d.mu.Lock()
		d.subscribers[cl.c] = true
		d.mu.Unlock()
		return cl.c.write(frame{Type: "sessions", ID: message.ID, Channel: "sessions", Sessions: d.views()})
	}
	return fmt.Errorf("unknown frame type %q", message.Type)
}

func (d *daemon) session(name string) *ptySession {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.sessions[name]
}

func (d *daemon) byToken(token string) *ptySession {
	if token == "" {
		return nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.tokens[token]
}

func (d *daemon) views() []sessionView {
	d.mu.Lock()
	sessions := make([]*ptySession, 0, len(d.sessions))
	for _, s := range d.sessions {
		sessions = append(sessions, s)
	}
	d.mu.Unlock()
	views := make([]sessionView, 0, len(sessions))
	for _, s := range sessions {
		views = append(views, s.view())
	}
	sort.Slice(views, func(i, j int) bool { return views[i].Name < views[j].Name })
	return views
}

// spawn ends any live session of the same name first, which is how a second
// launch of a role replaces the first. The name is who answers, not a harness.
func (d *daemon) spawn(message frame) (*ptySession, error) {
	if len(message.Argv) == 0 {
		return nil, withExit(exitUsage, errors.New("spawn needs an argv"))
	}
	name := strings.TrimSpace(message.Session)
	if name == "" {
		name = "session-" + randomID(3)
	}
	if earlier := d.session(name); earlier != nil {
		d.logf("ending earlier session %s (pid %d) for a new launch", name, earlier.pid)
		earlier.end()
	}
	s, err := startPTYSession(d, name, message)
	if err != nil {
		return nil, err
	}
	d.mu.Lock()
	d.sessions[name] = s
	d.tokens[s.token] = s
	var adopted []*pendingSend
	kept := d.orphans[:0]
	for _, orphan := range d.orphans {
		if orphan.target == s.role {
			adopted = append(adopted, orphan)
			continue
		}
		kept = append(kept, orphan)
	}
	d.orphans = kept
	d.lastActive = time.Now()
	d.mu.Unlock()
	for _, orphan := range adopted {
		s.enqueue(orphan)
	}
	d.logf("spawned %s as pid %d: %s", name, s.pid, strings.Join(message.Argv, " "))
	return s, nil
}

// forget drops an ended session, unless a newer one already took its name.
func (d *daemon) forget(s *ptySession) {
	d.mu.Lock()
	if d.sessions[s.name] == s {
		delete(d.sessions, s.name)
	}
	delete(d.tokens, s.token)
	d.lastActive = time.Now()
	d.mu.Unlock()
	d.pushSessions()
}

// send resolves the sender from its token and never from anything it says.
// The reply waits briefly so a message delivered at once reports delivered.
func (d *daemon) send(c *conn, message frame) error {
	sender := d.byToken(message.Token)
	if sender == nil {
		return withExit(exitUsage, errors.New(
			"aterm send speaks for a session aterm launched, and this token names no live one"))
	}
	if strings.TrimSpace(message.Body) == "" {
		return withExit(exitUsage, errors.New("the message is empty"))
	}
	from := sender.role + " " + sender.identity
	pending := &pendingSend{
		msg: peerMessage{
			ID:       randomID(6),
			From:     from,
			Target:   message.Target,
			Accepted: time.Now().UTC(),
		},
		text: envelope(sender.role, sender.identity, message.Body),
		done: make(chan struct{}),
		d:    d,
	}
	targets := d.resolve(message.Target)
	if slices.Contains(targets, sender) {
		return withExit(exitUsage, fmt.Errorf("%s is this session", message.Target))
	}
	switch {
	case len(targets) > 1:
		names := make([]string, 0, len(targets))
		for _, target := range targets {
			names = append(names, target.name)
		}
		return withExit(exitUsage, fmt.Errorf("%q matches %s, name one", message.Target, strings.Join(names, ", ")))
	case len(targets) == 1:
		targets[0].enqueue(pending)
	case message.Launch && safeRoleSlug(message.Target):
		pending.target = message.Target
		pending.setState("launching", "no live session, one was asked to open")
		d.mu.Lock()
		d.orphans = append(d.orphans, pending)
		d.mu.Unlock()
	default:
		return withExit(exitOffRoster, fmt.Errorf("no live session answers to %q. Live: %s",
			message.Target, d.liveNames()))
	}
	go func() {
		select {
		case <-pending.done:
		case <-time.After(3 * time.Second):
		}
		snapshot := pending.snapshot()
		_ = c.write(frame{Type: "sent", ID: message.ID, Message: &snapshot})
	}()
	return nil
}

func (d *daemon) liveNames() string {
	views := d.views()
	if len(views) == 0 {
		return "none"
	}
	names := make([]string, 0, len(views))
	for _, view := range views {
		names = append(names, view.Name)
	}
	return strings.Join(names, ", ")
}

// resolve takes the first tier that matches anything: the session name, then
// the role, then the identity, then the harness.
func (d *daemon) resolve(target string) []*ptySession {
	target = strings.TrimSpace(target)
	d.mu.Lock()
	defer d.mu.Unlock()
	tiers := []func(*ptySession) bool{
		func(s *ptySession) bool { return s.name == target },
		func(s *ptySession) bool { return s.role == target },
		func(s *ptySession) bool { return slugify(s.identity) == slugify(target) && slugify(target) != "" },
		func(s *ptySession) bool { return s.seat == target },
	}
	for _, match := range tiers {
		var found []*ptySession
		for _, s := range d.sessions {
			if match(s) {
				found = append(found, s)
			}
		}
		if len(found) > 0 {
			sort.Slice(found, func(i, j int) bool { return found[i].name < found[j].name })
			return found
		}
	}
	return nil
}

func (d *daemon) expireOrphans(now time.Time) {
	d.mu.Lock()
	var expired []*pendingSend
	kept := d.orphans[:0]
	for _, orphan := range d.orphans {
		if now.Sub(orphan.msg.Accepted) > launchWait {
			expired = append(expired, orphan)
			continue
		}
		kept = append(kept, orphan)
	}
	d.orphans = kept
	d.mu.Unlock()
	for _, orphan := range expired {
		orphan.setState("failed", fmt.Sprintf("no %s session opened within %s", orphan.target, launchWait))
	}
}

func (d *daemon) pushSessions() {
	d.broadcast(frame{Type: "sessions", Channel: "sessions", Sessions: d.views()})
}

func (d *daemon) broadcast(message frame) {
	d.mu.Lock()
	subscribers := make([]*conn, 0, len(d.subscribers))
	for c := range d.subscribers {
		subscribers = append(subscribers, c)
	}
	d.mu.Unlock()
	for _, c := range subscribers {
		_ = c.raw.SetWriteDeadline(time.Now().Add(clientWriteTimeout))
		if err := c.write(message); err != nil {
			d.mu.Lock()
			delete(d.subscribers, c)
			d.mu.Unlock()
		}
	}
}

// insideSession reports whether pid runs under a session this daemon started,
// an unreadable pid counting as inside. See docs/aterm-daemon.md.
func (d *daemon) insideSession(pid int) bool {
	if pid <= 0 {
		return true
	}
	d.mu.Lock()
	roots := map[int]bool{}
	for _, s := range d.sessions {
		roots[s.pid] = true
	}
	d.mu.Unlock()
	entries, err := d.processes()
	if err != nil {
		return true
	}
	parents := map[int]int{}
	for _, entry := range entries {
		parents[entry.PID] = entry.PPID
	}
	seen := map[int]bool{}
	for current := pid; current > 1 && !seen[current]; current = parents[current] {
		if roots[current] {
			return true
		}
		seen[current] = true
	}
	return false
}

// pendingSend is a message on its way. Its state is read by the sender's
// reply, the sessions channel, and the delivering session, so it locks itself.
type pendingSend struct {
	mu     sync.Mutex
	msg    peerMessage
	text   string
	target string
	done   chan struct{}
	once   sync.Once
	d      *daemon
}

func (p *pendingSend) setState(state, reason string) {
	p.mu.Lock()
	changed := p.msg.State != state || p.msg.Reason != reason
	p.msg.State, p.msg.Reason = state, reason
	snapshot := p.msg
	p.mu.Unlock()
	if state == "delivered" || state == "failed" {
		p.once.Do(func() { close(p.done) })
	}
	if changed && p.d != nil {
		p.d.broadcast(frame{Type: "message", Message: &snapshot})
	}
}

func (p *pendingSend) snapshot() peerMessage {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.msg
}

func randomID(size int) string {
	raw := make([]byte, size)
	if _, err := rand.Read(raw); err != nil {
		panic(err)
	}
	return hex.EncodeToString(raw)
}
