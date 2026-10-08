package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

const (
	// browserProxyEnv=1 in a launch opts its Playwright MCP in to the daemon's browser.
	// Off by default: that browser has a temporary profile, with no role logins yet.
	browserProxyEnv = "ATERM_BROWSER_PROXY"
	// cdpEndpointEnv is where @playwright/mcp reads --cdp-endpoint from, so the
	// projected MCP config stays static.
	cdpEndpointEnv = "PLAYWRIGHT_MCP_CDP_ENDPOINT"
	// cdpPrefix is where a session's Playwright reaches its browser.
	cdpPrefix       = "/cdp/"
	cdpStartWait    = 25 * time.Second
	cdpQueue        = 4096
	maxAgentRequest = 16 << 20
)

// cdpProxies holds the one agent connection each session's browser serves. A
// newer connection replaces the older, whose process is most likely gone.
type cdpProxies struct {
	mu        sync.Mutex
	bySession map[string]*agentConn
}

func (p *cdpProxies) swap(session string, next *agentConn) *agentConn {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.bySession == nil {
		p.bySession = map[string]*agentConn{}
	}
	previous := p.bySession[session]
	p.bySession[session] = next
	return previous
}

func (p *cdpProxies) drop(session string, gone *agentConn) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.bySession[session] == gone {
		delete(p.bySession, session)
	}
}

// cdpEndpoint is where a session's Playwright connects, empty without a loopback
// listener or a browser. Playwright then launches its own, as without the opt-in.
func (d *daemon) cdpEndpoint(token string) string {
	d.mu.Lock()
	address := d.loopbackAddr
	d.mu.Unlock()
	if address == "" || !d.browserServed() {
		return ""
	}
	return "ws://" + address + cdpPrefix + token
}

// withCDPEndpoint sets cdpEndpointEnv in a launch's environment when the
// launch opted in and the daemon can serve it.
func (d *daemon) withCDPEndpoint(environ []string, token string) []string {
	if !containsEntry(environ, browserProxyEnv+"=1") {
		return environ
	}
	endpoint := d.cdpEndpoint(token)
	if endpoint == "" {
		d.logf("a launch asked for the shared browser, and this daemon cannot serve it")
		return environ
	}
	kept := make([]string, 0, len(environ)+1)
	for _, entry := range environ {
		if key, _, _ := strings.Cut(entry, "="); key != cdpEndpointEnv {
			kept = append(kept, entry)
		}
	}
	return append(kept, cdpEndpointEnv+"="+endpoint)
}

func containsEntry(environ []string, want string) bool {
	for _, entry := range environ {
		if entry == want {
			return true
		}
	}
	return false
}

// runsUnder reports whether one of pids runs under root. Unlike descendsFrom, what
// cannot be read answers false, since this vouches for a peer instead of refusing one.
func (d *daemon) runsUnder(pids []int, root int) bool {
	if root <= 1 || len(pids) == 0 {
		return false
	}
	entries, err := d.processes()
	if err != nil {
		return false
	}
	parents := map[int]int{}
	for _, entry := range entries {
		parents[entry.PID] = entry.PPID
	}
	for _, pid := range pids {
		seen := map[int]bool{}
		for current := pid; current > 1 && !seen[current]; current = parents[current] {
			if current == root {
				return true
			}
			seen[current] = true
		}
	}
	return false
}

// serveCDP admits a session's Playwright to the browser shared with the client. The
// token names the session and the peer's ancestry must reach it, so a leaked token fails.
func (d *daemon) serveCDP(w http.ResponseWriter, r *http.Request) {
	remote, err := net.ResolveTCPAddr("tcp", r.RemoteAddr)
	if err != nil || !remote.IP.IsLoopback() || r.Header.Get("Origin") != "" {
		http.Error(w, "aterm browser: local harnesses only", http.StatusForbidden)
		return
	}
	s := d.byToken(strings.Trim(strings.TrimPrefix(r.URL.Path, cdpPrefix), "/"))
	if s == nil {
		http.Error(w, "aterm browser: no such session", http.StatusNotFound)
		return
	}
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		http.Error(w, "aterm browser: a CDP websocket only", http.StatusUpgradeRequired)
		return
	}
	local, _ := r.Context().Value(http.LocalAddrContextKey).(net.Addr)
	pids, err := d.peerLookup(remote, local)
	if err != nil || !d.runsUnder(pids, s.pid) {
		d.logf("refused a CDP peer for %s: not provably under the session (%v)", s.name, err)
		http.Error(w, "aterm browser: this process does not run under that session", http.StatusForbidden)
		return
	}
	if !d.browserServed() {
		http.Error(w, "aterm browser: this host has no browser", http.StatusServiceUnavailable)
		return
	}
	sb := d.browsers.sharedFor(d, s.name)
	sb.ensureStarted()
	cdp, err := sb.awaitLive(r.Context(), cdpStartWait)
	if err != nil {
		http.Error(w, "aterm browser: "+err.Error(), http.StatusBadGateway)
		return
	}
	ws, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	ws.SetReadLimit(maxAgentRequest)
	a := newAgentConn(d, sb, cdp, ws)
	if previous := d.agents.swap(s.name, a); previous != nil {
		previous.close("a newer connection took this browser")
	}
	defer d.agents.drop(s.name, a)
	a.run(r.Context())
}

// awaitLive waits for the browser to be live, or says why it will not be.
func (sb *sharedBrowser) awaitLive(ctx context.Context, wait time.Duration) (*cdpClient, error) {
	ctx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()
	for {
		sb.mu.Lock()
		cdp, starting, reason := sb.cdp, sb.starting, sb.reason
		live := sb.state == "live" && cdp != nil
		sb.mu.Unlock()
		switch {
		case live:
			return cdp, nil
		case !starting:
			return nil, errors.New(reason)
		}
		select {
		case <-ctx.Done():
			return nil, errors.New("the browser did not come up in time")
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// pageSession is the CDP session the daemon's own screencast and input use. An
// agent is never given it.
func (sb *sharedBrowser) pageSession() string {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.page
}

// agentConn is one Playwright connection to a session's browser. The daemon holds the
// only pipe, so commands go out under its ids and only the agent's own sessions answer.
type agentConn struct {
	d   *daemon
	sb  *sharedBrowser
	cdp *cdpClient
	ws  *websocket.Conn

	out  chan []byte
	dead chan struct{}
	once sync.Once

	mu       sync.Mutex
	owned    map[string]bool
	discover bool
	attach   bool
	// contexts and targets are what the agent created and has not closed, which
	// leave with it. The browser outlives the agent.
	contexts map[string]bool
	targets  map[string]bool
	stopTap  func()
}

func newAgentConn(d *daemon, sb *sharedBrowser, cdp *cdpClient, ws *websocket.Conn) *agentConn {
	return &agentConn{
		d: d, sb: sb, cdp: cdp, ws: ws,
		out: make(chan []byte, cdpQueue), dead: make(chan struct{}),
		owned: map[string]bool{}, contexts: map[string]bool{}, targets: map[string]bool{},
	}
}

func (a *agentConn) run(ctx context.Context) {
	go a.write(ctx)
	a.stopTap = a.cdp.tap(a.onEvent)
	go func() {
		select {
		case <-a.cdp.done:
			a.close("the browser ended")
		case <-a.dead:
		}
	}()
	for {
		kind, data, err := a.ws.Read(ctx)
		if err != nil {
			break
		}
		if kind != websocket.MessageText {
			a.close("CDP messages are text")
			break
		}
		a.handle(data)
	}
	a.close("")
}

func (a *agentConn) write(ctx context.Context) {
	for {
		select {
		case <-a.dead:
			return
		case line := <-a.out:
			writeCtx, cancel := context.WithTimeout(ctx, clientWriteTimeout)
			err := a.ws.Write(writeCtx, websocket.MessageText, line)
			cancel()
			if err != nil {
				a.close("")
				return
			}
		}
	}
}

// enqueue keeps the browser's order. A reader that falls behind is dropped, since a CDP
// client that missed an event has a page model it cannot trust.
func (a *agentConn) enqueue(line []byte) {
	select {
	case <-a.dead:
	case a.out <- line:
	default:
		a.d.logf("browser for %s: the agent stopped reading CDP, so its connection ends", a.sb.session)
		go a.close("")
	}
}

// close ends the connection once, and tidies what the agent left in the
// browser, which stays up for the person watching it.
func (a *agentConn) close(reason string) {
	a.once.Do(func() {
		close(a.dead)
		if a.stopTap != nil {
			a.stopTap()
		}
		a.mu.Lock()
		owned, contexts, targets, attach := a.owned, a.contexts, a.targets, a.attach
		a.mu.Unlock()
		ignore := func(cdpMessage) {}
		for id := range targets {
			a.cdp.relay("", "Target.closeTarget", mustJSON(map[string]any{"targetId": id}), ignore)
		}
		for id := range contexts {
			a.cdp.relay("", "Target.disposeBrowserContext", mustJSON(map[string]any{"browserContextId": id}), ignore)
		}
		for id := range owned {
			a.cdp.relay("", "Target.detachFromTarget", mustJSON(map[string]any{"sessionId": id}), ignore)
		}
		if attach {
			a.cdp.relay("", "Target.setAutoAttach", mustJSON(map[string]any{"autoAttach": false, "waitForDebuggerOnStart": false, "flatten": true}), ignore)
		}
		status, text := websocket.StatusNormalClosure, reason
		if len(text) > 100 {
			text = text[:100]
		}
		// The closing handshake waits on a peer that may be gone, so it runs apart.
		go func() { _ = a.ws.Close(status, text) }()
	})
}

func mustJSON(value any) json.RawMessage {
	encoded, _ := json.Marshal(value)
	return encoded
}

type agentRequest struct {
	ID        json.RawMessage `json:"id"`
	Method    string          `json:"method"`
	Params    json.RawMessage `json:"params"`
	SessionID string          `json:"sessionId"`
}

// reply writes the answer to one agent command, with the sessionId a CDP
// client routes it by.
func (a *agentConn) reply(request agentRequest, result json.RawMessage, failure *cdpErrorBody) {
	message := map[string]any{"id": request.ID}
	if failure != nil {
		message["error"] = failure
	} else {
		if len(result) == 0 {
			result = json.RawMessage("{}")
		}
		message["result"] = result
	}
	if request.SessionID != "" {
		message["sessionId"] = request.SessionID
	}
	encoded, _ := json.Marshal(message)
	a.enqueue(encoded)
}

func (a *agentConn) handle(data []byte) {
	var request agentRequest
	if json.Unmarshal(data, &request) != nil || len(request.ID) == 0 || request.Method == "" {
		a.close("a CDP command needs an id and a method")
		return
	}
	if request.SessionID != "" {
		a.mu.Lock()
		known := a.owned[request.SessionID]
		a.mu.Unlock()
		if !known {
			a.reply(request, nil, &cdpErrorBody{Code: -32001, Message: "Session with given id not found."})
			return
		}
	} else if a.answeredHere(request) {
		return
	}
	if refusal := a.gate(request); refusal != nil {
		a.refuse(request, refusal)
		return
	}
	a.cdp.relay(request.SessionID, request.Method, request.Params, func(answer cdpMessage) {
		if answer.Error == nil {
			a.learn(request, answer.Result)
		}
		a.reply(request, answer.Result, answer.Error)
	})
}

// answeredHere handles the browser-level commands that would reach past this
// agent: the daemon owns target discovery and the browser's life.
func (a *agentConn) answeredHere(request agentRequest) bool {
	switch request.Method {
	case "Browser.close":
		// Closing a browser the agent only borrowed would end the person's view.
		a.reply(request, nil, nil)
	case "Target.setDiscoverTargets":
		var params struct {
			Discover bool `json:"discover"`
		}
		_ = json.Unmarshal(request.Params, &params)
		a.mu.Lock()
		a.discover = params.Discover
		a.mu.Unlock()
		a.reply(request, nil, nil)
		if params.Discover {
			a.announceTargets()
		}
	default:
		return false
	}
	return true
}

// announceTargets sends the targets already open, as turning discovery on
// would have, since the daemon turned it on for itself long before.
func (a *agentConn) announceTargets() {
	a.cdp.relay("", "Target.getTargets", nil, func(answer cdpMessage) {
		var listed struct {
			TargetInfos []json.RawMessage `json:"targetInfos"`
		}
		if answer.Error != nil || json.Unmarshal(answer.Result, &listed) != nil {
			return
		}
		for _, info := range listed.TargetInfos {
			a.sendEvent("Target.targetCreated", "", mustJSON(map[string]json.RawMessage{"targetInfo": info}))
		}
	})
}

// learn records what a command made, for the agent's own tidying and routing.
func (a *agentConn) learn(request agentRequest, result json.RawMessage) {
	a.mu.Lock()
	defer a.mu.Unlock()
	var made struct {
		SessionID        string `json:"sessionId"`
		TargetID         string `json:"targetId"`
		BrowserContextID string `json:"browserContextId"`
	}
	_ = json.Unmarshal(result, &made)
	var asked struct {
		TargetID         string `json:"targetId"`
		BrowserContextID string `json:"browserContextId"`
		AutoAttach       bool   `json:"autoAttach"`
	}
	_ = json.Unmarshal(request.Params, &asked)
	switch request.Method {
	case "Target.attachToTarget":
		if made.SessionID != "" {
			a.owned[made.SessionID] = true
		}
	case "Target.createTarget":
		a.targets[made.TargetID] = true
	case "Target.closeTarget":
		delete(a.targets, asked.TargetID)
	case "Target.createBrowserContext":
		a.contexts[made.BrowserContextID] = true
	case "Target.disposeBrowserContext":
		delete(a.contexts, asked.BrowserContextID)
	case "Target.setAutoAttach":
		a.attach = asked.AutoAttach
	}
}

func (a *agentConn) sendEvent(method, sessionID string, params json.RawMessage) {
	message := map[string]any{"method": method, "params": params}
	if sessionID != "" {
		message["sessionId"] = sessionID
	}
	encoded, _ := json.Marshal(message)
	a.enqueue(encoded)
}

// onEvent runs on the read loop and passes on only this agent's own sessions' events,
// never those of the session the daemon streams and types into.
func (a *agentConn) onEvent(event cdpMessage) {
	if !a.admits(event) {
		return
	}
	a.sendEvent(event.Method, event.SessionID, event.Params)
}

func (a *agentConn) admits(event cdpMessage) bool {
	var target struct {
		SessionID string `json:"sessionId"`
	}
	if event.Method == "Target.attachedToTarget" || event.Method == "Target.detachedFromTarget" {
		_ = json.Unmarshal(event.Params, &target)
	}
	own := a.sb.pageSession()
	a.mu.Lock()
	defer a.mu.Unlock()
	switch event.Method {
	case "Target.attachedToTarget":
		if target.SessionID == "" || target.SessionID == own || (event.SessionID != "" && !a.owned[event.SessionID]) {
			return false
		}
		a.owned[target.SessionID] = true
		return true
	case "Target.detachedFromTarget":
		if !a.owned[target.SessionID] {
			return false
		}
		delete(a.owned, target.SessionID)
		return true
	}
	if event.SessionID != "" {
		return a.owned[event.SessionID]
	}
	switch event.Method {
	case "Target.targetCreated", "Target.targetInfoChanged", "Target.targetDestroyed", "Target.targetCrashed":
		return a.discover
	}
	return true
}
