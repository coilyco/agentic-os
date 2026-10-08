package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"sync"
)

const (
	// maxViewBytes bounds a view's HTML and its result, so one frame stays
	// well inside what a client reads (maxFrame).
	maxViewBytes = 2 << 20
	// A session keeps its newest views, the rest closed as if dismissed.
	maxViewsPerSession   = 16
	maxServersPerSession = 16
)

var gatewayServerName = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// viewCSP is a view's `_meta.ui.csp`, in the snake_case the wire uses.
type viewCSP struct {
	ConnectDomains  []string `json:"connect_domains,omitempty"`
	ResourceDomains []string `json:"resource_domains,omitempty"`
	FrameDomains    []string `json:"frame_domains,omitempty"`
	BaseURIDomains  []string `json:"base_uri_domains,omitempty"`
}

func (c *viewCSP) empty() bool {
	return c == nil || len(c.ConnectDomains)+len(c.ResourceDomains)+len(c.FrameDomains)+len(c.BaseURIDomains) == 0
}

// viewFrame is the static part of a view: what `view` carries, and what a
// replay sends before the latest update. See docs/aterm-daemon.md.
type viewFrame struct {
	ID            string          `json:"id"`
	Server        string          `json:"server"`
	Tool          string          `json:"tool"`
	ResourceURI   string          `json:"resource_uri"`
	HTML          string          `json:"html"`
	CSP           *viewCSP        `json:"csp,omitempty"`
	PrefersBorder bool            `json:"prefers_border,omitempty"`
	ToolInput     json.RawMessage `json:"tool_input"`
}

// viewUpdate is the part that arrives later: the tool's result, or why there
// will not be one.
type viewUpdate struct {
	ToolResult json.RawMessage
	Cancelled  string
}

type liveView struct {
	session string
	server  *gatewayServer
	frame   viewFrame
	update  *viewUpdate
}

// appsState is the gateway servers and captured views. One lock covers it and
// every emit, so a replay and the live frames never reorder.
type appsState struct {
	mu       sync.Mutex
	servers  map[string]map[string]*gatewayServer // session, then server name
	views    []*liveView                          // oldest first
	watchers map[*conn]bool
}

func (a *appsState) emit(message frame) {
	for c := range a.watchers {
		if err := c.write(message); err != nil {
			delete(a.watchers, c)
		}
	}
}

// registerGateway fronts a server for a session, replacing one of that name.
func (d *daemon) registerGateway(session, name string, spec gatewaySpec) (gatewaySpec, error) {
	if !gatewayServerName.MatchString(name) {
		return spec, withExit(exitUsage, fmt.Errorf("server name %q must be lowercase letters, digits, - or _", name))
	}
	if err := spec.validate(); err != nil {
		return spec, withExit(exitUsage, err)
	}
	s := d.session(session)
	if s == nil {
		return spec, withExit(exitOffRoster, fmt.Errorf("no live session named %q", session))
	}
	spec.Path = gatewayPrefix + s.token + "/" + name
	g := &gatewayServer{d: d, session: session, name: name, spec: spec}
	d.apps.mu.Lock()
	if d.apps.servers == nil {
		d.apps.servers = map[string]map[string]*gatewayServer{}
	}
	if d.apps.servers[session] == nil {
		d.apps.servers[session] = map[string]*gatewayServer{}
	}
	replaced := d.apps.servers[session][name]
	if replaced == nil && len(d.apps.servers[session]) >= maxServersPerSession {
		d.apps.mu.Unlock()
		return spec, withExit(exitUsage, fmt.Errorf("a session's gateway fronts at most %d servers", maxServersPerSession))
	}
	d.apps.servers[session][name] = g
	d.apps.mu.Unlock()
	if replaced != nil {
		replaced.close()
	}
	return spec, nil
}

func (d *daemon) gatewayServer(session, name string) *gatewayServer {
	d.apps.mu.Lock()
	defer d.apps.mu.Unlock()
	return d.apps.servers[session][name]
}

func (d *daemon) addView(g *gatewayServer, view viewFrame) *liveView {
	view.ID = randomID(6)
	live := &liveView{session: g.session, server: g, frame: view}
	d.apps.mu.Lock()
	defer d.apps.mu.Unlock()
	d.apps.views = append(d.apps.views, live)
	d.apps.emit(frame{Type: "view", Session: g.session, View: &live.frame})
	// A session keeps its newest views.
	owned := 0
	for _, v := range d.apps.views {
		if v.session == g.session {
			owned++
		}
	}
	for _, v := range slices.Clone(d.apps.views) {
		if owned <= maxViewsPerSession {
			break
		}
		if v.session == g.session {
			d.dropViewLocked(v)
			owned--
		}
	}
	return live
}

// dropViewLocked removes a view and tells every watcher.
func (d *daemon) dropViewLocked(view *liveView) {
	d.apps.views = slices.DeleteFunc(d.apps.views, func(v *liveView) bool { return v == view })
	d.apps.emit(frame{Type: "view_closed", Session: view.session, ViewID: view.frame.ID})
}

func (d *daemon) updateView(view *liveView, result json.RawMessage) {
	if len(result) > maxViewBytes {
		d.cancelView(view, fmt.Sprintf("the result is over %d bytes, too large to show", maxViewBytes))
		return
	}
	d.setUpdate(view, viewUpdate{ToolResult: result})
}

func (d *daemon) cancelView(view *liveView, reason string) {
	d.setUpdate(view, viewUpdate{Cancelled: reason})
}

func (d *daemon) setUpdate(view *liveView, update viewUpdate) {
	d.apps.mu.Lock()
	defer d.apps.mu.Unlock()
	if !slices.Contains(d.apps.views, view) {
		return
	}
	view.update = &update
	d.apps.emit(frame{Type: "view_update", Session: view.session, ViewID: view.frame.ID,
		ToolResult: update.ToolResult, Cancelled: update.Cancelled})
}

func (d *daemon) viewByID(id string) *liveView {
	d.apps.mu.Lock()
	defer d.apps.mu.Unlock()
	for _, v := range d.apps.views {
		if v.frame.ID == id {
			return v
		}
	}
	return nil
}

// dropApps ends a session's gateway servers and closes its views, once the
// session has ended and its token with it.
func (d *daemon) dropApps(session string) {
	d.apps.mu.Lock()
	servers := d.apps.servers[session]
	delete(d.apps.servers, session)
	for _, v := range slices.Clone(d.apps.views) {
		if v.session == session {
			d.dropViewLocked(v)
		}
	}
	d.apps.mu.Unlock()
	for _, g := range servers {
		g.close()
	}
}

// closeApps ends every gateway server, for a daemon that is stopping.
func (d *daemon) closeApps() {
	d.apps.mu.Lock()
	var all []*gatewayServer
	for _, servers := range d.apps.servers {
		for _, g := range servers {
			all = append(all, g)
		}
	}
	d.apps.servers = nil
	d.apps.mu.Unlock()
	for _, g := range all {
		g.close()
	}
}

func (d *daemon) unwatchViews(c *conn) {
	d.apps.mu.Lock()
	delete(d.apps.watchers, c)
	d.apps.mu.Unlock()
}

// watchViews is `subscribe` to the views channel: every live view replays as
// `view` and then its latest `view_update`, and later ones follow.
func (d *daemon) watchViews(cl *client, message frame) error {
	d.apps.mu.Lock()
	defer d.apps.mu.Unlock()
	if d.apps.watchers == nil {
		d.apps.watchers = map[*conn]bool{}
	}
	if err := cl.c.write(frame{Type: "subscribed", ID: message.ID, Channel: "views"}); err != nil {
		return err
	}
	for _, v := range d.apps.views {
		if err := cl.c.write(frame{Type: "view", Session: v.session, View: &v.frame}); err != nil {
			return err
		}
		if v.update != nil {
			if err := cl.c.write(frame{Type: "view_update", Session: v.session, ViewID: v.frame.ID,
				ToolResult: v.update.ToolResult, Cancelled: v.update.Cancelled}); err != nil {
				return err
			}
		}
	}
	d.apps.watchers[cl.c] = true
	return nil
}

// addGateway is `gateway_add`. A stdio spec is a command the daemon runs, so
// only a local socket may ask, never a browser.
func (d *daemon) addGateway(cl *client, message frame) error {
	if cl.web {
		return withExit(exitUsage, errors.New("a browser cannot register a gateway server"))
	}
	s := d.byToken(message.Token)
	if s == nil {
		return withExit(exitUsage, errors.New("gateway_add speaks for a session aterm launched, and this token names no live one"))
	}
	if message.Gateway == nil {
		return withExit(exitUsage, errors.New("gateway_add carries a gateway"))
	}
	if !s.mcpApps {
		return withExit(exitUsage, errors.New("this session did not opt in, so its MCP path stays as launched. Launch it with "+mcpAppsEnv+"=1"))
	}
	spec, err := d.registerGateway(s.name, message.Server, *message.Gateway)
	if err != nil {
		return err
	}
	return cl.c.write(frame{Type: "gateway_added", ID: message.ID, Session: s.name, Server: message.Server, Gateway: &spec})
}

// viewCall proxies a view's call to its server with no consent step (Kai). It
// takes the typing guard, and runs off the read loop since a tool takes minutes.
func (d *daemon) viewCall(cl *client, message frame) error {
	if err := d.insideRefusal(cl, "a process inside an aterm session cannot use a view as Kai"); err != nil {
		return err
	}
	view := d.viewByID(message.ViewID)
	if view == nil {
		return withExit(exitOffRoster, fmt.Errorf("no view %q", message.ViewID))
	}
	if message.Method != "tools/call" && message.Method != "resources/read" {
		return withExit(exitUsage, fmt.Errorf("a view may call tools/call or resources/read, not %q", message.Method))
	}
	go func() {
		defer d.guard("view_call")
		ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
		defer cancel()
		result, err := view.server.fromView(ctx, message.Method, message.Params)
		if err != nil {
			_ = cl.c.write(frame{Type: "error", ID: message.ID, ViewID: message.ViewID, Error: err.Error(), Code: exitCodeFor(err)})
			return
		}
		_ = cl.c.write(frame{Type: "view_result", ID: message.ID, ViewID: message.ViewID, Result: result})
	}()
	return nil
}

// closeView is `view_close`: the person dismissed it, so every client drops it
// and a later replay leaves it out.
func (d *daemon) closeView(cl *client, message frame) error {
	view := d.viewByID(message.ViewID)
	if view == nil {
		return withExit(exitOffRoster, fmt.Errorf("no view %q", message.ViewID))
	}
	d.apps.mu.Lock()
	d.dropViewLocked(view)
	d.apps.mu.Unlock()
	return cl.c.write(frame{Type: "view_closed", ID: message.ID, ViewID: message.ViewID})
}
