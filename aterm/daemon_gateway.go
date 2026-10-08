package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// mcpAppsFeature is how a client knows the daemon runs a gateway and serves the
// `views` channel. See docs/aterm-daemon.md.
const mcpAppsFeature = "mcp-apps"

// mcpAppsEnv=1 in a launch's environment opts that session in to the gateway.
// Off by default, so no seat's MCP path changes unless its launch asks.
const mcpAppsEnv = "ATERM_MCP_APPS"

const (
	// uiExtension is the client capability that makes a server register
	// UI-bearing tools, and uiMime what a view's resource must be.
	uiExtension = "io.modelcontextprotocol/ui"
	uiMime      = "text/html;profile=mcp-app"
	// gatewayPrefix is where a harness reaches a session's server.
	gatewayPrefix = "/mcp/"
	callTimeout   = 10 * time.Minute
)

// toolUI is a tool's `_meta.ui`: the resource that draws it, and who may call it.
type toolUI struct {
	ResourceURI string
	Visibility  []string
}

// has reports whether audience ("model" or "app") may call the tool. The spec
// defaults an absent visibility to both.
func (t toolUI) has(audience string) bool {
	return len(t.Visibility) == 0 || slices.Contains(t.Visibility, audience)
}

// metaUI reads a result's or tool's `_meta`, including the spec's deprecated
// flat `ui/resourceUri` key that older servers still send.
func metaUI(meta mcp.Meta) toolUI {
	var parsed struct {
		UI struct {
			ResourceURI string   `json:"resourceUri"`
			Visibility  []string `json:"visibility"`
		} `json:"ui"`
		Flat string `json:"ui/resourceUri"`
	}
	raw, _ := json.Marshal(meta)
	_ = json.Unmarshal(raw, &parsed)
	if parsed.UI.ResourceURI == "" {
		parsed.UI.ResourceURI = parsed.Flat
	}
	return toolUI{ResourceURI: parsed.UI.ResourceURI, Visibility: parsed.UI.Visibility}
}

// gatewayServer is one server a session's gateway fronts. The upstream starts
// on first use and is replaced, with a fresh harness-facing server, if it dies.
type gatewayServer struct {
	d       *daemon
	session string
	name    string
	spec    gatewaySpec

	mu          sync.Mutex
	up          *upstream
	handler     http.Handler
	tools       map[string]toolUI
	toolsLoaded bool
}

func (g *gatewayServer) ensure(_ context.Context) (*upstream, http.Handler, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.up != nil && g.up.alive() {
		return g.up, g.handler, nil
	}
	if g.up != nil {
		g.up.close()
		g.up = nil
	}
	up, err := openUpstream(g.spec, g.d.logf)
	if err != nil {
		return nil, nil, err
	}
	g.up, g.handler = up, g.proxyHandler(up)
	g.tools, g.toolsLoaded = map[string]toolUI{}, false
	return g.up, g.handler, nil
}

func (g *gatewayServer) close() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.up != nil {
		g.up.close()
		g.up = nil
	}
}

// proxyHandler is the harness-facing side: an SDK server that advertises the
// upstream's own capabilities and answers its methods from the upstream.
func (g *gatewayServer) proxyHandler(up *upstream) http.Handler {
	init := up.session.InitializeResult()
	info := &mcp.Implementation{Name: "aterm-gateway", Version: version}
	options := &mcp.ServerOptions{Capabilities: &mcp.ServerCapabilities{}}
	if init != nil {
		if init.ServerInfo != nil {
			info = init.ServerInfo
		}
		options.Instructions = init.Instructions
		if init.Capabilities != nil {
			options.Capabilities = init.Capabilities
		}
	}
	server := mcp.NewServer(info, options)
	server.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			return g.forward(ctx, up.session, next, method, req)
		}
	})
	return mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)
}

// forward answers a harness request from the upstream. The server registers no
// features, so initialize, ping and notifications are all that fall through.
func (g *gatewayServer) forward(ctx context.Context, cs *mcp.ClientSession, next mcp.MethodHandler, method string, req mcp.Request) (mcp.Result, error) {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	switch params := req.GetParams().(type) {
	case *mcp.ListToolsParams:
		result, err := cs.ListTools(ctx, params)
		if err != nil {
			return nil, err
		}
		g.observeTools(result)
		return result, nil
	case *mcp.CallToolParamsRaw:
		return g.callTool(ctx, cs, params)
	case *mcp.ListResourcesParams:
		return cs.ListResources(ctx, params)
	case *mcp.ListResourceTemplatesParams:
		return cs.ListResourceTemplates(ctx, params)
	case *mcp.ReadResourceParams:
		return cs.ReadResource(ctx, params)
	case *mcp.ListPromptsParams:
		return cs.ListPrompts(ctx, params)
	case *mcp.GetPromptParams:
		return cs.GetPrompt(ctx, params)
	case *mcp.CompleteParams:
		return cs.Complete(ctx, params)
	}
	return next(ctx, method, req)
}

func (g *gatewayServer) toolMeta(name string) (toolUI, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	meta, ok := g.tools[name]
	return meta, ok
}

// observeTools records each tool's `_meta.ui` and drops from the list the
// tools only a view may call, which the spec keeps from the agent.
func (g *gatewayServer) observeTools(result *mcp.ListToolsResult) {
	kept := result.Tools[:0]
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.tools == nil {
		g.tools = map[string]toolUI{}
	}
	for _, tool := range result.Tools {
		meta := metaUI(tool.Meta)
		g.tools[tool.Name] = meta
		if meta.has("model") {
			kept = append(kept, tool)
		}
	}
	result.Tools = kept
}

// loadTools reads the server's whole tool list once, for a call that arrives
// before the harness listed.
func (g *gatewayServer) loadTools(ctx context.Context, cs *mcp.ClientSession) {
	g.mu.Lock()
	loaded := g.toolsLoaded
	g.mu.Unlock()
	if loaded {
		return
	}
	params := &mcp.ListToolsParams{}
	for page := 0; page < 50; page++ {
		result, err := cs.ListTools(ctx, params)
		if err != nil {
			g.d.logf("gateway %s/%s could not list tools: %v", g.session, g.name, err)
			return
		}
		g.observeTools(result)
		if result.NextCursor == "" {
			break
		}
		params = &mcp.ListToolsParams{Cursor: result.NextCursor}
	}
	g.mu.Lock()
	g.toolsLoaded = true
	g.mu.Unlock()
}

func (g *gatewayServer) callTool(ctx context.Context, cs *mcp.ClientSession, call *mcp.CallToolParamsRaw) (*mcp.CallToolResult, error) {
	g.loadTools(ctx, cs)
	meta, _ := g.toolMeta(call.Name)
	if !meta.has("model") {
		return nil, &jsonrpc.Error{Code: jsonrpc.CodeInvalidParams, Message: "tool " + call.Name + " is only callable by a view"}
	}
	var view *liveView
	if meta.ResourceURI != "" {
		view = g.openView(ctx, cs, call.Name, meta.ResourceURI, call.Arguments)
	}
	result, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Meta: call.Meta, Name: call.Name, Arguments: rawArguments(call.Arguments),
		InputResponses: call.InputResponses, RequestState: call.RequestState,
	})
	if err != nil {
		if view != nil {
			g.d.cancelView(view, err.Error())
		}
		return nil, err
	}
	if view == nil {
		// A server may name the view on the result instead of on the tool.
		if uri := metaUI(result.Meta).ResourceURI; uri != "" {
			view = g.openView(ctx, cs, call.Name, uri, call.Arguments)
		}
	}
	if view != nil {
		raw, _ := json.Marshal(result)
		g.d.updateView(view, raw)
	}
	return result, nil
}

// rawArguments passes a call's arguments on exactly as the harness sent them.
func rawArguments(arguments json.RawMessage) any {
	if len(arguments) == 0 {
		return nil
	}
	return arguments
}

// openView fetches the ui:// resource and pushes the view. A resource that
// cannot be read costs the view, never the tool call.
func (g *gatewayServer) openView(ctx context.Context, cs *mcp.ClientSession, tool, uri string, args json.RawMessage) *liveView {
	if !strings.HasPrefix(uri, "ui://") {
		g.d.logf("gateway %s/%s: %s names %q, which is not a ui:// resource", g.session, g.name, tool, uri)
		return nil
	}
	readCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	read, err := cs.ReadResource(readCtx, &mcp.ReadResourceParams{URI: uri})
	if err != nil {
		g.d.logf("gateway %s/%s could not read %s: %v", g.session, g.name, uri, err)
		return nil
	}
	for _, content := range read.Contents {
		if strings.ReplaceAll(strings.ToLower(content.MIMEType), " ", "") != uiMime {
			continue
		}
		html := content.Text
		if html == "" {
			html = string(content.Blob)
		}
		if html == "" || len(html) > maxViewBytes {
			g.d.logf("gateway %s/%s: %s is empty or over %d bytes", g.session, g.name, uri, maxViewBytes)
			return nil
		}
		view := viewFrame{Server: g.name, Tool: tool, ResourceURI: uri, HTML: html, ToolInput: args}
		view.CSP, view.PrefersBorder = resourceUI(content.Meta)
		if len(view.ToolInput) == 0 {
			view.ToolInput = json.RawMessage(`{}`)
		}
		return g.d.addView(g, view)
	}
	g.d.logf("gateway %s/%s: %s is not %s", g.session, g.name, uri, uiMime)
	return nil
}

// resourceUI reads a view resource's `_meta.ui`, in the camelCase the spec uses.
func resourceUI(meta mcp.Meta) (*viewCSP, bool) {
	var parsed struct {
		UI struct {
			CSP struct {
				ConnectDomains  []string `json:"connectDomains"`
				ResourceDomains []string `json:"resourceDomains"`
				FrameDomains    []string `json:"frameDomains"`
				BaseURIDomains  []string `json:"baseUriDomains"`
			} `json:"csp"`
			PrefersBorder bool `json:"prefersBorder"`
		} `json:"ui"`
	}
	raw, _ := json.Marshal(meta)
	_ = json.Unmarshal(raw, &parsed)
	csp := &viewCSP{
		ConnectDomains: parsed.UI.CSP.ConnectDomains, ResourceDomains: parsed.UI.CSP.ResourceDomains,
		FrameDomains: parsed.UI.CSP.FrameDomains, BaseURIDomains: parsed.UI.CSP.BaseURIDomains,
	}
	if csp.empty() {
		csp = nil
	}
	return csp, parsed.UI.PrefersBorder
}

// fromView is a view's call back to its server: tools/call for a tool whose
// visibility lists "app", or resources/read. It returns a view_result's JSON.
func (g *gatewayServer) fromView(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, error) {
	up, _, err := g.ensure(ctx)
	if err != nil {
		return nil, err
	}
	var call struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
		URI       string          `json:"uri"`
	}
	if err := json.Unmarshal(params, &call); err != nil {
		return nil, withExit(exitUsage, err)
	}
	var result any
	switch method {
	case "tools/call":
		if call.Name == "" {
			return nil, withExit(exitUsage, errors.New("tools/call needs a tool name"))
		}
		g.loadTools(ctx, up.session)
		meta, known := g.toolMeta(call.Name)
		switch {
		case !known:
			return nil, withExit(exitOffRoster, errors.New("the server has no tool named "+call.Name))
		case !meta.has("app"):
			return nil, withExit(exitUsage, errors.New("tool "+call.Name+" is not callable by a view (its _meta.ui.visibility leaves out app)"))
		}
		result, err = up.session.CallTool(ctx, &mcp.CallToolParams{Name: call.Name, Arguments: rawArguments(call.Arguments)})
	default:
		result, err = up.session.ReadResource(ctx, &mcp.ReadResourceParams{URI: call.URI})
	}
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

// serveGateway is the harness side. The token in the path is the auth, so only
// a local non-browser peer is answered, by the SDK's Streamable HTTP handler.
func (d *daemon) serveGateway(w http.ResponseWriter, r *http.Request) {
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() || r.Header.Get("Origin") != "" {
		http.Error(w, "aterm gateway: local harnesses only", http.StatusForbidden)
		return
	}
	token, name, ok := strings.Cut(strings.TrimPrefix(r.URL.Path, gatewayPrefix), "/")
	session := d.byToken(token)
	var g *gatewayServer
	if ok && session != nil {
		g = d.gatewayServer(session.name, name)
	}
	if g == nil {
		http.Error(w, "aterm gateway: no such server for this session", http.StatusNotFound)
		return
	}
	_, handler, err := g.ensure(r.Context())
	if err != nil {
		http.Error(w, "aterm gateway: could not reach the server: "+err.Error(), http.StatusBadGateway)
		return
	}
	handler.ServeHTTP(w, r)
}
