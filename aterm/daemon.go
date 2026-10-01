package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
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
	mu         sync.Mutex
	sessions   map[string]*ptySession
	tokens     map[string]*ptySession
	orphans    []*pendingSend
	asks       map[string]*pendingAsk
	askTimeout time.Duration
	clientDir  string
	// holdDir is where each session's holder keeps its socket. A restarted
	// daemon adopts what it finds there.
	holdDir     string
	subscribers map[*conn]bool
	conns       int
	lastActive  time.Time
	processes   func() ([]processEntry, error)
	roster      func(context.Context) (listedRoster, error)
	launch      func(role, seat string) error
	logf        func(string, ...any)
}

func newDaemon(logf func(string, ...any)) *daemon {
	return &daemon{
		sessions:    map[string]*ptySession{},
		tokens:      map[string]*ptySession{},
		asks:        map[string]*pendingAsk{},
		askTimeout:  defaultAskTimeout,
		subscribers: map[*conn]bool{},
		lastActive:  time.Now(),
		processes:   listProcesses,
		roster:      launchableRoster,
		launch:      launchRole,
		logf:        logf,
	}
}

// launchableRoster is `aterm --list --json` for a client that cannot shell
// out. It is read per request, since the read is fast and roles turn over.
func launchableRoster(ctx context.Context) (listedRoster, error) {
	deps := systemDeps()
	deps.notice = nil
	agentCompose, err := requireBinary(deps.lookPath, envOr("AGENT_COMPOSE_BIN", defaultOverlayBin))
	if err != nil {
		return listedRoster{}, err
	}
	document, err := loadRoster(ctx, deps, agentCompose)
	if err != nil {
		return listedRoster{}, err
	}
	return listRoster(document), nil
}

func envOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

// daemonOptions is what the daemon verb takes. An empty address or port turns
// that listener off.
type daemonOptions struct {
	Socket      string
	Websocket   string
	TailnetPort string
	AllowTags   []string
	// AllowOrigins are hosted client pages, like https://coilyco.dev.
	AllowOrigins []string
	ClientDir    string
	Idle         time.Duration
	// EndSessions ends every session when the daemon stops. By default they
	// keep running in their holders for the next daemon to adopt.
	EndSessions bool
	// Stop ends the daemon when closed, as a signal does. A test's handle.
	Stop <-chan struct{}
}

// runDaemon holds the lock, owns the socket, and serves until idle. A second
// daemon finding the lock held exits cleanly, which settles a start race.
func runDaemon(options daemonOptions, stderr io.Writer) error {
	socket, idle := options.Socket, options.Idle
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
	d.holdDir = filepath.Join(dir, "hold")
	logf("serving %s as pid %d, build %s", socket, os.Getpid(), version)
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(stop)
	go func() {
		select {
		case <-stop:
		case <-options.Stop:
		}
		_ = listener.Close()
	}()
	d.adoptHolders()
	d.clientDir = options.ClientDir
	if options.Websocket != "" {
		server, err := d.listenWebsocket(options.Websocket)
		if err != nil {
			logf("no websocket, so browser clients cannot attach: %v", err)
		} else {
			defer server.Close()
		}
	}
	if options.TailnetPort != "" {
		done := make(chan struct{})
		defer close(done)
		go d.serveTailnetWhenUp(done, func() (*http.Server, error) {
			return d.listenTailnet(options.TailnetPort, options.AllowTags, options.AllowOrigins, filepath.Join(dir, "tailnet"))
		})
	}
	go d.watchIdle(listener, idle)
	for {
		raw, err := listener.Accept()
		if err != nil {
			break
		}
		go d.serve(raw)
	}
	_ = os.Remove(socket)
	if options.EndSessions {
		d.endAll()
	} else {
		d.letGoAll()
	}
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

// adoptHolders takes over the sessions a earlier daemon left running. A socket
// nobody answers is a holder that ended, so it is removed.
func (d *daemon) adoptHolders() {
	if err := ensureSocketDir(d.holdDir); err != nil {
		d.logf("no holder directory, so earlier sessions are not adopted: %v", err)
		return
	}
	sockets, _ := filepath.Glob(filepath.Join(d.holdDir, "*.sock"))
	for _, socket := range sockets {
		s, err := d.connectHolder(socket, true)
		if err != nil {
			if errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, os.ErrNotExist) {
				d.logf("dropped %s, no holder answers there", filepath.Base(socket))
				_ = os.Remove(socket)
				_ = os.Remove(strings.TrimSuffix(socket, ".sock") + ".log")
				continue
			}
			d.logf("could not adopt %s: %v", filepath.Base(socket), err)
			continue
		}
		d.mu.Lock()
		if d.sessions[s.name] != nil {
			d.mu.Unlock()
			d.logf("holder %s names a session already adopted, left alone", filepath.Base(socket))
			s.letGo()
			continue
		}
		d.sessions[s.name] = s
		d.tokens[s.token] = s
		d.mu.Unlock()
		d.logf("adopted %s as pid %d", s.name, s.pid)
	}
	d.pushSessions()
}

// letGoAll detaches from every session and leaves each one running.
func (d *daemon) letGoAll() {
	d.mu.Lock()
	sessions := make([]*ptySession, 0, len(d.sessions))
	for _, s := range d.sessions {
		sessions = append(sessions, s)
	}
	d.mu.Unlock()
	for _, s := range sessions {
		s.letGo()
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
// attached to, and whether it may type. A browser has no pid to walk.
type client struct {
	c        *conn
	peerPID  int
	browser  bool
	owned    map[string]bool
	attached map[string]*ptySession
	asks     []string
}

func (d *daemon) serve(raw net.Conn) {
	pid, _ := peerPID(raw)
	d.serveConn(newConn(raw), pid, false)
}

func (d *daemon) serveConn(c *conn, pid int, browser bool) {
	defer c.Close()
	hello, err := c.read()
	if err != nil || hello.Type != "hello" {
		return
	}
	if hello.Format != daemonFormat {
		_ = c.write(frame{Type: "welcome", Format: daemonFormat, Error: "unsupported format " + hello.Format})
		return
	}
	if err := c.write(frame{Type: "welcome", Format: daemonFormat, Version: version, PID: os.Getpid(), Features: []string{sendNewFeature, closeFeature}}); err != nil {
		return
	}
	d.mu.Lock()
	d.conns++
	d.lastActive = time.Now()
	d.mu.Unlock()
	cl := &client{c: c, peerPID: pid, browser: browser, owned: map[string]bool{}, attached: map[string]*ptySession{}}
	defer func() {
		for _, s := range cl.attached {
			s.detach(c)
		}
		asked := cl.asks
		d.settleWhere(func(ask choiceAsk) bool { return slices.Contains(asked, ask.ID) }, "the asking call ended")
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
		if !cl.owned[s.name] && !cl.browser && d.insideSession(cl.peerPID) {
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
	case "close":
		return d.closeSession(cl, message)
	case "list":
		return cl.c.write(frame{Type: "sessions", ID: message.ID, Sessions: d.views()})
	case "launch":
		if !safeRoleSlug(message.Role) || (message.Seat != "" && !isNativeHarness(message.Seat)) {
			return withExit(exitUsage, fmt.Errorf("launch needs a role slug and, optionally, a native seat"))
		}
		if err := d.launch(message.Role, message.Seat); err != nil {
			return withExit(exitSpawn, err)
		}
		return cl.c.write(frame{Type: "launched", ID: message.ID, Role: message.Role, Seat: message.Seat})
	case "roster":
		roster, err := d.roster(context.Background())
		if err != nil {
			return err
		}
		return cl.c.write(frame{Type: "roster", ID: message.ID, Roster: &roster})
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
		if err := cl.c.write(frame{Type: "sessions", ID: message.ID, Channel: "sessions", Sessions: d.views()}); err != nil {
			return err
		}
		for _, ask := range d.pendingAsks() {
			if err := cl.c.write(frame{Type: "ask", Ask: &ask}); err != nil {
				return err
			}
		}
		return nil
	case "ask":
		return d.ask(cl, message)
	case "answer":
		return d.answer(cl, message)
	case "cancel_ask":
		if !d.settle(message.AskID, choiceAnswer{State: "cancelled", Reason: "a client dismissed it"}) {
			return withExit(exitOffRoster, fmt.Errorf("no pending ask %q", message.AskID))
		}
		return cl.c.write(frame{Type: "asked", ID: message.ID, AskID: message.AskID, State: "cancelled"})
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

// spawn refuses a name a live session holds rather than ending it, since two
// sessions of one role run side by side. The name is who answers, not a harness.
func (d *daemon) spawn(message frame) (*ptySession, error) {
	if len(message.Argv) == 0 {
		return nil, withExit(exitUsage, errors.New("spawn needs an argv"))
	}
	name := strings.TrimSpace(message.Session)
	if name == "" {
		name = "session-" + randomID(3)
	}
	if earlier := d.session(name); earlier != nil {
		return nil, withExit(exitUsage, fmt.Errorf(
			"session %s is already running as pid %d, so this launch would take its name", name, earlier.pid))
	}
	s, err := startPTYSession(d, name, message)
	if err != nil {
		return nil, err
	}
	d.mu.Lock()
	d.sessions[name] = s
	d.tokens[s.token] = s
	// A fresh instance answers one new-instance send, so two asked for at once
	// open two sessions rather than both landing in the first.
	var adopted []*pendingSend
	tookFresh := false
	kept := d.orphans[:0]
	for _, orphan := range d.orphans {
		if orphan.target == s.role && (!orphan.fresh || !tookFresh) {
			tookFresh = tookFresh || orphan.fresh
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
	d.settleWhere(func(ask choiceAsk) bool { return ask.Session == s.name }, s.name+" ended")
	d.pushSessions()
}

// send resolves the sender from its token and never from anything it says.
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
	if message.New {
		if !safeRoleSlug(message.Target) || d.session(message.Target) != nil {
			return withExit(exitUsage, fmt.Errorf("new opens an instance of a role, and %q is not a role slug", message.Target))
		}
		pending.target, pending.fresh = message.Target, true
		pending.setState("launching", "a new instance was asked to open")
		d.mu.Lock()
		d.orphans = append(d.orphans, pending)
		d.mu.Unlock()
		go d.reportSent(c, message.ID, pending)
		return nil
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
	go d.reportSent(c, message.ID, pending)
	return nil
}

// reportSent waits briefly so a message delivered at once reports delivered.
// A launching one answers at once, since the sender launches after the reply.
func (d *daemon) reportSent(c *conn, id string, pending *pendingSend) {
	if pending.snapshot().State != "launching" {
		select {
		case <-pending.done:
		case <-time.After(3 * time.Second):
		}
	}
	snapshot := pending.snapshot()
	_ = c.write(frame{Type: "sent", ID: id, Message: &snapshot})
}

// closeSession ends a live session and drops it, which closing its window does
// not. It never ends the caller or a session the caller runs inside.
func (d *daemon) closeSession(cl *client, message frame) error {
	targets := d.resolve(message.Target)
	switch {
	case len(targets) == 0:
		return withExit(exitOffRoster, fmt.Errorf("no live session answers to %q. Live: %s",
			message.Target, d.liveNames()))
	case len(targets) > 1:
		names := make([]string, 0, len(targets))
		for _, target := range targets {
			names = append(names, target.name)
		}
		return withExit(exitUsage, fmt.Errorf("%q matches %s, name one", message.Target, strings.Join(names, ", ")))
	}
	s := targets[0]
	caller := d.byToken(message.Token)
	if caller == s {
		return withExit(exitUsage, fmt.Errorf("%s is this session", s.name))
	}
	if !cl.browser && d.descendsFrom(cl.peerPID, map[int]bool{s.pid: true}) {
		return withExit(exitUsage, fmt.Errorf("this caller runs inside %s, so closing it would end the caller too", s.name))
	}
	if view := s.view(); !message.Force && (view.Drafted || view.Pending > 0) {
		held := "Kai's unsent draft"
		if !view.Drafted {
			held = fmt.Sprintf("%d undelivered message(s)", view.Pending)
		}
		return withExit(exitUsage, fmt.Errorf("%s holds %s, so it stays open unless forced", s.name, held))
	}
	by := "a client outside every session"
	if caller != nil {
		by = caller.name
	}
	d.logf("closing session %s (pid %d) for %s", s.name, s.pid, by)
	s.end()
	select {
	case <-s.done:
	default:
		return fmt.Errorf("%s (pid %d) did not end after SIGKILL", s.name, s.pid)
	}
	s.mu.Lock()
	code := s.exitCode
	s.mu.Unlock()
	return cl.c.write(frame{Type: "closed", ID: message.ID, Session: s.name, Code: code})
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
	d.mu.Lock()
	roots := map[int]bool{}
	for _, s := range d.sessions {
		roots[s.pid] = true
	}
	d.mu.Unlock()
	return d.descendsFrom(pid, roots)
}

// descendsFrom reports whether pid is one of roots or runs under one. An
// unreadable pid or process table counts as under, the cautious answer.
func (d *daemon) descendsFrom(pid int, roots map[int]bool) bool {
	if pid <= 0 {
		return true
	}
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
	fresh  bool
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
