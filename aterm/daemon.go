package main

import (
	"context"
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
	mu       sync.Mutex
	sessions map[string]*ptySession
	// terminals are plain login shells. They sit in their own map so every walk
	// of sessions (send targets, roster, counts) skips them without a filter.
	terminals  map[string]*ptySession
	tokens     map[string]*ptySession
	orphans    []*pendingSend
	asks       map[string]*pendingAsk
	askTimeout time.Duration
	clientDir  string
	// holdDir is where each session's holder keeps its socket. A restarted
	// daemon adopts what it finds there.
	holdDir string
	tailnet tailnetState
	// loopbackAddr is where the loopback listener bound, empty while it is not
	// serving. A session's Playwright reaches its browser there.
	loopbackAddr string
	// agents are the CDP connections sessions' Playwright hold to their browsers.
	agents cdpProxies
	// discovery answers the hosts frame.
	discovery *hostDiscovery
	// capture sends panics to Sentry, nil without a DSN. Set before goroutines start.
	capture *sentryCapture
	// ledgerDir holds a record per session for `aterm resume`. Empty keeps none.
	ledgerDir string
	// ledgerMu keeps a record's write and its end stamp in one order, however
	// fast the child exits.
	ledgerMu sync.Mutex
	// ended remembers how recent sessions exited, so a window that was
	// disconnected when its session ended still learns the code.
	ended map[string]endedSession
	// claims holds a granted pool name until its spawn arrives or the hold lapses,
	// so two launches at once never settle on one name.
	claims      map[string]time.Time
	subscribers map[*conn]bool
	conns       int
	lastActive  time.Time
	processes   func() ([]processEntry, error)
	passkeys    *passkeyStore
	// peerLookup names the processes holding a local TCP peer's end.
	peerLookup func(remote, local net.Addr) ([]int, error)
	roster     func(context.Context) (listedRoster, error)
	launch     func(role, seat string) error
	// proxyFetch reads Agent Proxy's session usage. Nil when no proxy is configured.
	proxyFetch proxyFetcher
	// browsers holds a session's streamed Chromium. browserLaunch replaces the
	// real one in tests.
	browsers      browserHub
	browserLaunch browserLauncher
	logf          func(string, ...any)

	// apps is the per-session MCP Apps gateway and the views it captured.
	apps appsState

	// push sends Web Push to a browser that is not connected. See daemon_push.go.
	push pushState
}

func newDaemon(logf func(string, ...any)) *daemon {
	return &daemon{
		sessions:    map[string]*ptySession{},
		terminals:   map[string]*ptySession{},
		tokens:      map[string]*ptySession{},
		asks:        map[string]*pendingAsk{},
		ended:       map[string]endedSession{},
		claims:      map[string]time.Time{},
		askTimeout:  defaultAskTimeout,
		subscribers: map[*conn]bool{},
		lastActive:  time.Now(),
		processes:   listProcesses,
		peerLookup:  tcpPeerPIDs,
		passkeys:    defaultPasskeys(),
		roster:      launchableRoster,
		launch:      launchRole,
		discovery:   newHostDiscovery(),
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
	// PeerPorts maps a peer name to the port its daemon listens on, for host discovery.
	PeerPorts map[string]string
	AllowTags []string
	// AllowOrigins are hosted client pages, like https://coilyco.dev.
	AllowOrigins []string
	ClientDir    string
	Idle         time.Duration
	// EndSessions ends every session when the daemon stops. By default they
	// keep running in their holders for the next daemon to adopt.
	EndSessions bool
	// SentryDSN turns on check-ins and panic capture, empty for none. Never logged.
	SentryDSN string
	// AgentProxy is the proxy base URL for proxy-backed seats' context, empty for none.
	AgentProxy string
	// VAPIDKey turns on Web Push, empty for none. Never logged.
	VAPIDKey string
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
	if options.SentryDSN != "" {
		if d.capture, err = newSentryCapture(options.SentryDSN, hostName(), version); err != nil {
			logf("no Sentry error capture: %v", err)
		} else {
			defer d.capture.flush()
			defer d.guard("daemon")
		}
	}
	if d.passkeys, err = newPasskeyStore(passkeyStatePath(), passkeyRPID, []string{passkeyOrigin}); err != nil {
		logf("no stored passkeys, so no remote device can type: %v", err)
		d.passkeys = defaultPasskeys()
	}
	d.holdDir = filepath.Join(dir, "hold")
	d.ledgerDir = ledgerDir()
	pruneLedger(d.ledgerDir, time.Now())
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
	if options.AgentProxy != "" {
		if d.proxyFetch, err = newProxyFetcher(options.AgentProxy); err != nil {
			logf("no context for proxy-backed seats: %v", err)
		}
	}
	contextDone := make(chan struct{})
	defer close(contextDone)
	go d.watchContext(contextDone, contextEvery)
	if store, err := loadPushStore(pushStatePath()); err != nil {
		logf("no stored push subscriptions, so no browser is notified while closed: %v", err)
	} else if err := d.setupPush(options.VAPIDKey, store); err != nil {
		logf("no Web Push: %v", err)
	} else if d.push.enabled() {
		go d.watchWaiting(contextDone, pushWatchEvery)
	}
	d.adoptHolders()
	d.clientDir = options.ClientDir
	d.discovery.port = func() string { return options.TailnetPort }
	d.discovery.peerPorts = options.PeerPorts
	if options.Websocket != "" {
		server, err := d.listenWebsocket(options.Websocket)
		if err != nil {
			logf("no websocket, so browser clients cannot attach: %v", err)
		} else {
			defer server.Close()
		}
	}
	checkInDone := make(chan struct{})
	defer close(checkInDone)
	if options.SentryDSN != "" {
		if cron, err := newSentryCron(options.SentryDSN, sentryMonitorSlug(hostName())); err != nil {
			logf("no Sentry check-ins: %v", err)
		} else {
			go d.sentryCheckIns(checkInDone, cron)
		}
	}
	if options.TailnetPort != "" {
		d.tailnet.want()
		done := make(chan struct{})
		defer close(done)
		go d.serveTailnetWhenUp(done, func() (*tailnetServing, error) {
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
	d.browsers.closeAll()
	d.closeApps()
	if options.EndSessions {
		d.endAll()
	} else {
		d.letGoAll()
	}
	logf("stopped")
	return nil
}

func (d *daemon) watchIdle(listener net.Listener, idle time.Duration) {
	defer d.guard("idle")
	for range time.Tick(time.Second) {
		d.expireOrphans(time.Now())
		d.mu.Lock()
		quiet := idle > 0 && len(d.sessions) == 0 && len(d.terminals) == 0 && d.conns == 0 && time.Since(d.lastActive) > idle
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
	d.reapBrowsers()
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
		if d.sessions[s.name] != nil || d.terminals[s.name] != nil {
			d.mu.Unlock()
			d.logf("holder %s names a session already adopted, left alone", filepath.Base(socket))
			s.letGo()
			continue
		}
		if s.kind == kindTerminal {
			d.terminals[s.name] = s
		} else {
			d.sessions[s.name] = s
			d.tokens[s.token] = s
		}
		d.mu.Unlock()
		d.restoreGateways(s)
		d.logf("adopted %s as pid %d", s.name, s.pid)
	}
	d.pushSessions()
}

// letGoAll detaches from every session and leaves each one running.
func (d *daemon) letGoAll() {
	for _, s := range d.everySession() {
		s.letGo()
	}
}

func (d *daemon) endAll() {
	// Together, since each may spend its typed-exit grace before the signal.
	var group sync.WaitGroup
	for _, s := range d.everySession() {
		group.Go(s.end)
	}
	group.Wait()
}

// peerStanding is the other end before any frame: web or unix, another device
// or this host, and the pids holding it. A remote device has none to walk.
type peerStanding struct {
	web, remote bool
	pids        []int
}

// client is one connection's standing: which sessions it spawned, which it is
// attached to, and whether it may type.
type client struct {
	c *conn
	peerStanding
	owned    map[string]bool
	attached map[string]*ptySession
	asks     []string
	// asserted is a passkey assertion on this connection, which a remote device needs.
	asserted bool
	ceremony *passkeyCeremony
}

func (d *daemon) serve(raw net.Conn) {
	pid, _ := peerPID(raw)
	d.serveConn(newConn(raw), peerStanding{pids: []int{pid}})
}

func (d *daemon) serveConn(c *conn, peer peerStanding) {
	defer d.guard("connection")
	defer c.Close()
	hello, err := c.read()
	if err != nil || hello.Type != "hello" {
		return
	}
	if hello.Format != daemonFormat {
		_ = c.write(frame{Type: "welcome", Format: daemonFormat, Error: "unsupported format " + hello.Format})
		return
	}
	cl := &client{c: c, peerStanding: peer, owned: map[string]bool{}, attached: map[string]*ptySession{}}
	welcome := frame{Type: "welcome", Format: daemonFormat, Version: version, PID: os.Getpid(), Features: []string{sendNewFeature, sendWaitFeature, sendIdleFeature, closeFeature, holdFeature, statusFeature, clearFeature, contextFeature, claimFeature, typingGuardFeature, passkeyFeature, terminalsFeature, terminalLabelFeature, mcpAppsFeature, inboxFeature}}
	if d.browserServed() {
		welcome.Features = append(welcome.Features, browserFeature)
	}
	if d.push.enabled() {
		welcome.Features = append(welcome.Features, pushFeature)
	}
	if peer.web {
		welcome.Typing = d.typingStanding(cl)
	}
	if err := c.write(welcome); err != nil {
		return
	}
	d.mu.Lock()
	d.conns++
	d.lastActive = time.Now()
	d.mu.Unlock()
	defer func() {
		for _, s := range cl.attached {
			s.detach(c)
		}
		asked := cl.asks
		d.settleWhere(func(ask choiceAsk) bool { return slices.Contains(asked, ask.ID) }, "the asking call ended")
		d.browsers.leave(c)
		d.mu.Lock()
		d.conns--
		d.lastActive = time.Now()
		delete(d.subscribers, c)
		d.mu.Unlock()
		d.unwatchViews(c)
		d.pushSessions()
	}()
	for {
		message, err := c.read()
		if err != nil {
			return
		}
		if err := d.handle(cl, message); err != nil {
			_ = c.write(frame{Type: "error", ID: message.ID, Error: err.Error(), Code: exitCodeFor(err), Reason: reasonFor(err)})
		}
	}
}

func (d *daemon) handle(cl *client, message frame) error {
	if cl.remote && !cl.asserted && remoteLocked[message.Type] {
		return d.typingRefusal(cl, "")
	}
	switch message.Type {
	case "spawn":
		var s *ptySession
		var err error
		switch message.Kind {
		case "":
			if message.Label != "" {
				return withExit(exitUsage, errors.New("a label belongs to a terminal spawn, and a seat is named by its role and identity"))
			}
			s, err = d.spawn(message)
		case kindTerminal:
			// A shell on this host is what COI-2488 gates for a remote device.
			if cl.remote {
				return withReason(reasonRemoteTerminal, errors.New("a remote device cannot open a terminal here yet"))
			}
			s, err = d.spawnTerminal(message)
		default:
			err = withExit(exitUsage, fmt.Errorf("no spawn kind named %q", message.Kind))
		}
		if err != nil {
			return err
		}
		cl.owned[s.name] = true
		cl.attached[s.name] = s
		// The reply goes first, since a client reads up to it and no further.
		// The replay then carries whatever the child printed in between.
		if err := cl.c.write(frame{Type: "spawned", ID: message.ID, Session: s.name, PID: s.pid, Kind: s.kind, Label: s.label}); err != nil {
			return err
		}
		s.attach(cl.c, true, 0)
		d.pushSessions()
		return nil
	case "attach":
		s := d.attachable(message.Session)
		if s == nil {
			if code, ok := d.endedCode(message.Session); ok {
				return cl.c.write(frame{Type: "exited", ID: message.ID, Session: message.Session, Code: code})
			}
			return withExit(exitOffRoster, fmt.Errorf("no live session named %q", message.Session))
		}
		cl.attached[s.name] = s
		if err := cl.c.write(frame{Type: "attached", ID: message.ID, Session: s.name, PID: s.pid}); err != nil {
			return err
		}
		s.attach(cl.c, message.Replay, message.Offset)
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
		if !cl.owned[s.name] {
			if err := d.typingRefusal(cl, "a process inside an aterm session cannot type into one, use `aterm send`"); err != nil {
				return err
			}
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
	case "status":
		s, err := d.oneTarget(message.Target)
		if err != nil {
			return err
		}
		status := s.status(message.Lines)
		return cl.c.write(frame{Type: "status", ID: message.ID, Session: s.name, Status: &status})
	case "clear":
		return d.clearSession(cl, message)
	case "inbox":
		// Only the token names whose inbox it is, never a target.
		s := d.byToken(message.Token)
		if s == nil {
			return withExit(exitUsage, errors.New("the token names no live session, and an inbox belongs to one"))
		}
		return cl.c.write(frame{Type: "inbox", ID: message.ID, Session: s.name, Inbox: s.inbox.take(message.All)})
	case "claim":
		name, err := d.claimName(message.Session, message.Peek)
		if err != nil {
			return err
		}
		return cl.c.write(frame{Type: "claimed", ID: message.ID, Session: name})
	case "list":
		return cl.c.write(frame{Type: "sessions", ID: message.ID, Sessions: d.views(), Terminals: d.terminalViews()})
	case "launch":
		if cl.web {
			if err := d.typingRefusal(cl, "a process inside an aterm session cannot launch a seat as Kai, use `aterm send --launch`"); err != nil {
				return err
			}
		}
		if !safeRoleSlug(message.Role) || (message.Seat != "" && !isNativeHarness(message.Seat)) {
			return withExit(exitUsage, fmt.Errorf("launch needs a role slug and, optionally, a native seat"))
		}
		if err := d.launch(message.Role, message.Seat); err != nil {
			return withExit(exitSpawn, err)
		}
		return cl.c.write(frame{Type: "launched", ID: message.ID, Role: message.Role, Seat: message.Seat})
	case "passkey_mint", "passkey_revoke", "passkey_enroll_begin", "passkey_enroll_finish", "passkey_assert_begin", "passkey_assert_finish":
		return d.passkey(cl, message)
	case "roster":
		roster, err := d.roster(context.Background())
		if err != nil {
			return err
		}
		return cl.c.write(frame{Type: "roster", ID: message.ID, Roster: &roster})
	case "hosts":
		return d.hosts(cl.c, message)
	case "push_key", "push_subscribe", "push_unsubscribe":
		return d.pushFrame(cl, message)
	case "whoami":
		s := d.byToken(message.Token)
		if s == nil {
			return withExit(exitUsage, errors.New("the token names no live session"))
		}
		return cl.c.write(frame{Type: "sessions", ID: message.ID, Sessions: []sessionView{s.view()}})
	case "subscribe":
		if message.Channel == "views" {
			return d.watchViews(cl, message)
		}
		if message.Channel != "sessions" {
			return fmt.Errorf("no channel named %q", message.Channel)
		}
		d.mu.Lock()
		d.subscribers[cl.c] = true
		d.mu.Unlock()
		if err := cl.c.write(frame{Type: "sessions", ID: message.ID, Channel: "sessions", Sessions: d.views(), Terminals: d.terminalViews()}); err != nil {
			return err
		}
		for _, ask := range d.pendingAsks() {
			if err := cl.c.write(frame{Type: "ask", Ask: &ask}); err != nil {
				return err
			}
		}
		return nil
	case "browser_watch":
		return d.browserWatch(cl, message)
	case "browser_unwatch":
		return d.browserUnwatch(cl, message)
	case "browser_control":
		return d.browserControl(cl, message)
	case "browser_input", "browser_navigate":
		return d.browserDrive(cl, message)
	case "gateway_add":
		return d.addGateway(cl, message)
	case "view_call":
		return d.viewCall(cl, message)
	case "view_close":
		return d.closeView(cl, message)
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
	delete(d.claims, name)
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
	d.recordSession(s, message)
	return s, nil
}

// claimHold is how long a granted pool name waits for its spawn. A launch that
// dies first frees the name when this lapses.
const claimHold = 2 * time.Minute

// claimName grants the first free name of base, base-2, base-3, free meaning no live
// session and no unexpired grant. See docs/aterm-daemon.md.
func (d *daemon) claimName(base string, peek bool) (string, error) {
	if base == "" || slugify(base) != base {
		return "", withExit(exitUsage, errors.New("claim needs a slugged session name"))
	}
	now := time.Now()
	d.mu.Lock()
	defer d.mu.Unlock()
	for name, until := range d.claims {
		if !now.Before(until) {
			delete(d.claims, name)
		}
	}
	name := poolName(base, func(candidate string) bool {
		_, live := d.sessions[candidate]
		_, shell := d.terminals[candidate]
		_, held := d.claims[candidate]
		return live || shell || held
	})
	if !peek {
		d.claims[name] = now.Add(claimHold)
	}
	return name, nil
}

// endedMemory is how long a finished session's exit code stays answerable.
const endedMemory = 10 * time.Minute

type endedSession struct {
	code int
	at   time.Time
}

func (d *daemon) endedCode(name string) (int, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	earlier, ok := d.ended[name]
	return earlier.code, ok && time.Since(earlier.at) <= endedMemory
}

// recordSession writes the session's ledger record, logging a failure. A child
// that already exited gets its end stamp here, since markEnded ran first.
func (d *daemon) recordSession(s *ptySession, message frame) {
	if d.ledgerDir == "" {
		return
	}
	d.ledgerMu.Lock()
	defer d.ledgerMu.Unlock()
	entry := ledgerEntry{
		Name: s.name, Role: s.role, Identity: s.identity, Seat: s.seat,
		Cwd: message.Cwd, Argv: message.Argv, Conversation: conversationOf(message.Argv), Home: homeOf(message.Env), Dirs: harnessDirsOf(message.Env), Started: s.started.UTC(),
	}
	select {
	case <-s.done:
		s.mu.Lock()
		now, code := time.Now().UTC(), s.exitCode
		s.mu.Unlock()
		entry.Ended, entry.Code = &now, &code
	default:
	}
	if err := writeLedger(d.ledgerDir, entry); err != nil {
		d.logf("could not record %s for resume: %v", s.name, err)
	}
}

// markEnded stamps the exit onto a session's record, which stays resumable.
func (d *daemon) markEnded(s *ptySession) {
	if d.ledgerDir == "" {
		return
	}
	d.ledgerMu.Lock()
	defer d.ledgerMu.Unlock()
	for _, entry := range readLedger(d.ledgerDir) {
		// A pool name recurs, so the record under it may already be a newer session's.
		if entry.Name != s.name || !entry.Started.Equal(s.started) {
			continue
		}
		s.mu.Lock()
		now, code := time.Now().UTC(), s.exitCode
		s.mu.Unlock()
		entry.Ended, entry.Code = &now, &code
		if err := writeLedger(d.ledgerDir, entry); err != nil {
			d.logf("could not mark %s ended: %v", s.name, err)
		}
	}
}

// forget drops an ended session, unless a newer one already took its name.
func (d *daemon) forget(s *ptySession) {
	if s.kind == kindTerminal {
		d.forgetTerminal(s)
		return
	}
	d.mu.Lock()
	if d.sessions[s.name] == s {
		delete(d.sessions, s.name)
	}
	delete(d.tokens, s.token)
	d.lastActive = time.Now()
	d.ended[s.name] = endedSession{code: s.exitCode, at: time.Now()}
	defer d.markEnded(s)
	for name, earlier := range d.ended {
		if time.Since(earlier.at) > endedMemory {
			delete(d.ended, name)
		}
	}
	d.mu.Unlock()
	d.settleWhere(func(ask choiceAsk) bool { return ask.Session == s.name }, s.name+" ended")
	d.browsers.end(s.name)
	d.dropApps(s.name)
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
		text:       envelope(sender.role, sender.identity, message.Body),
		done:       make(chan struct{}),
		d:          d,
		sender:     sender,
		notifyIdle: message.NotifyIdle,
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
		go d.reportSent(c, message.ID, pending, message.Wait)
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
	go d.reportSent(c, message.ID, pending, message.Wait)
	return nil
}

// reportSent waits 3s, or as long as the sender asked, for a final state. A launching
// message answers at once. One not final by then owes its sender a receipt.
func (d *daemon) reportSent(c *conn, id string, pending *pendingSend, wait int) {
	defer d.guard("send report")
	limit := 3 * time.Second
	if wait > 0 {
		limit = min(time.Duration(wait)*time.Second, maxSendWait)
	}
	if pending.snapshot().State != "launching" {
		select {
		case <-pending.done:
		case <-time.After(limit):
		}
	}
	snapshot := pending.snapshot()
	_ = c.write(frame{Type: "sent", ID: id, Message: &snapshot})
	if !finalState(snapshot.State) {
		pending.armReceipt()
	}
}

// forgetGrace is how long a close waits for the daemon to drop an ended session's name.
const forgetGrace = 5 * time.Second

// closeSession ends a live session and drops it, which closing its window does
// not. It never ends the caller or a session the caller runs inside.
func (d *daemon) closeSession(cl *client, message frame) error {
	if shell := d.terminal(message.Target); shell != nil {
		return d.closeTerminal(cl, message, shell)
	}
	s, err := d.oneTarget(message.Target)
	if err != nil {
		return err
	}
	caller := d.byToken(message.Token)
	if caller == s {
		return withExit(exitUsage, fmt.Errorf("%s is this session", s.name))
	}
	if d.clientDescendsFrom(cl, map[int]bool{s.pid: true}) {
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
	select {
	case <-s.forgotten:
	case <-time.After(forgetGrace):
		return fmt.Errorf("%s ended but the daemon still holds its name", s.name)
	}
	s.mu.Lock()
	code := s.exitCode
	s.mu.Unlock()
	return cl.c.write(frame{Type: "closed", ID: message.ID, Session: s.name, Code: code})
}

// clearCommands is the line each harness reads as "start over", typed unstamped.
// A seat not listed cannot be cleared, since the line would arrive as text.
var clearCommands = map[string]string{"claude": "/clear"}

// clearRoles may clear another seat. Kai's own client always may, and clearing
// discards a seat's context, so no other role does.
var clearRoles = map[string]bool{"prod-director": true}

// clearSession types a harness's clear command into an idle session, for Kai or
// the director. See docs/aterm-daemon.md.
func (d *daemon) clearSession(cl *client, message frame) error {
	if cl.remote {
		if err := d.typingRefusal(cl, ""); err != nil {
			return err
		}
	}
	s, err := d.oneTarget(message.Target)
	if err != nil {
		return err
	}
	caller := d.byToken(message.Token)
	switch {
	case caller != nil && !clearRoles[caller.role]:
		return withExit(exitUsage, fmt.Errorf("the %s role may not clear a session, only Kai and the director do", caller.role))
	case caller == nil && d.clientDescendsFrom(cl, d.sessionRoots()):
		return withExit(exitUsage, errors.New("a process inside a session must present its session token to clear another"))
	case caller == s:
		return withExit(exitUsage, fmt.Errorf("%s is this session, and clearing it would discard the context doing the clearing", s.name))
	}
	command, known := clearCommands[s.seat]
	if !known {
		return withExit(exitUsage, fmt.Errorf("%s runs the %s harness, which has no clear command aterm knows", s.name, s.seat))
	}
	view := s.view()
	switch {
	case view.State == statePrompt:
		return withExit(exitUsage, fmt.Errorf("%s sits on a prompt, and the command would answer it", s.name))
	case !message.Force && (view.Drafted || view.Pending > 0):
		return withExit(exitUsage, fmt.Errorf("%s holds Kai's draft or undelivered messages, so it stays unless forced", s.name))
	case !message.Force && view.State != stateIdle:
		return withExit(exitUsage, fmt.Errorf("%s is %s, so it stays unless forced", s.name, view.State))
	}
	by := "Kai's client"
	if caller != nil {
		by = caller.name
	}
	d.logf("clearing session %s (%s) for %s", s.name, command, by)
	if err := s.typeCommand(command); err != nil {
		return err
	}
	return cl.c.write(frame{Type: "cleared", ID: message.ID, Session: s.name, Text: command})
}

// oneTarget resolves a target to exactly one live session, or says why not.
func (d *daemon) oneTarget(target string) (*ptySession, error) {
	targets := d.resolve(target)
	switch {
	case len(targets) == 0:
		return nil, withExit(exitOffRoster, fmt.Errorf("no live session answers to %q. Live: %s", target, d.liveNames()))
	case len(targets) > 1:
		names := make([]string, 0, len(targets))
		for _, match := range targets {
			names = append(names, match.name)
		}
		return nil, withExit(exitUsage, fmt.Errorf("%q matches %s, name one", target, strings.Join(names, ", ")))
	}
	return targets[0], nil
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
	d.broadcast(frame{Type: "sessions", Channel: "sessions", Sessions: d.views(), Terminals: d.terminalViews()})
}

func (d *daemon) broadcast(message frame) {
	d.mu.Lock()
	subscribers := make([]*conn, 0, len(d.subscribers))
	for c := range d.subscribers {
		subscribers = append(subscribers, c)
	}
	d.mu.Unlock()
	for _, c := range subscribers {
		if !c.offer(message) {
			d.mu.Lock()
			delete(d.subscribers, c)
			d.mu.Unlock()
			_ = c.Close()
		}
	}
}

// insideSession reports whether pid runs under a session this daemon started,
// an unreadable pid counting as inside. See docs/aterm-daemon.md.
func (d *daemon) insideSession(pid int) bool {
	return d.descendsFrom(pid, d.sessionRoots())
}

func (d *daemon) sessionRoots() map[int]bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	roots := map[int]bool{}
	for _, s := range d.sessions {
		roots[s.pid] = true
	}
	return roots
}

// clientDescendsFrom is descendsFrom for a connection held by several pids.
// A remote device never descends, and a local peer left unnamed counts as under.
func (d *daemon) clientDescendsFrom(cl *client, roots map[int]bool) bool {
	if cl.remote {
		return false
	}
	if len(cl.pids) == 0 {
		return true
	}
	for _, pid := range cl.pids {
		if d.descendsFrom(pid, roots) {
			return true
		}
	}
	return false
}

// typingRefusal is the typing guard for what is Kai's to do, as a refusal a
// client can show: nil when the peer may, otherwise message with a reason.
func (d *daemon) typingRefusal(cl *client, message string) error {
	if cl.remote {
		if cl.asserted {
			return nil
		}
		return withReason(reasonPasskeyRequired, errors.New("assert this device's passkey before it types"))
	}
	if !d.clientDescendsFrom(cl, d.sessionRoots()) {
		return nil
	}
	if len(cl.pids) == 0 {
		return withReason(reasonPeerUnread, errors.New("the daemon could not tell which process this connection is, so it cannot type"))
	}
	return withReason(reasonSessionDescendant, errors.New(message))
}

// typingStanding is the answer a welcome carries, so a client can show its
// read-only state before a keystroke is refused. Each frame is still judged.
func (d *daemon) typingStanding(cl *client) *typingStanding {
	standing := &typingStanding{Allowed: true}
	if err := d.typingRefusal(cl, ""); err != nil {
		standing = &typingStanding{Reason: reasonFor(err)}
	}
	if cl.remote {
		standing.Passkey = "unenrolled"
		if d.passkeys.enrolled() {
			standing.Passkey = "enrolled"
		}
	}
	return standing
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
	// sender, armed and receiptSent drive the receipt. See receipt.go.
	sender      *ptySession
	armed       bool
	receiptSent bool
	// notifyIdle asks for one line in the sender when the target goes idle.
	notifyIdle bool
}

func (p *pendingSend) setState(state, reason string) {
	p.mu.Lock()
	changed := p.msg.State != state || p.msg.Reason != reason
	p.msg.State, p.msg.Reason = state, reason
	snapshot := p.msg
	p.mu.Unlock()
	if finalState(state) {
		p.once.Do(func() { close(p.done) })
	}
	if changed && p.d != nil {
		p.d.broadcast(frame{Type: "message", Message: &snapshot})
	}
	if finalState(state) {
		p.sendReceipt()
	}
	if changed && state == "delivered" && p.notifyIdle {
		p.watchIdle()
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
