package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
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
	maxGatewayRPC = 16 << 20
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

func parseToolUI(meta json.RawMessage) toolUI {
	var parsed struct {
		UI struct {
			ResourceURI string   `json:"resourceUri"`
			Visibility  []string `json:"visibility"`
		} `json:"ui"`
		// The spec's deprecated flat key, still what older servers send.
		Flat string `json:"ui/resourceUri"`
	}
	_ = json.Unmarshal(meta, &parsed)
	if parsed.UI.ResourceURI == "" {
		parsed.UI.ResourceURI = parsed.Flat
	}
	return toolUI{ResourceURI: parsed.UI.ResourceURI, Visibility: parsed.UI.Visibility}
}

// gatewayServer is one server a session's gateway fronts. The upstream starts
// on first use and is replaced if it dies.
type gatewayServer struct {
	d       *daemon
	session string
	name    string
	spec    gatewaySpec

	mu          sync.Mutex
	up          mcpUpstream
	initResult  json.RawMessage
	tools       map[string]toolUI
	toolsLoaded bool
}

func (g *gatewayServer) ensure(ctx context.Context) (mcpUpstream, json.RawMessage, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.up != nil && g.up.alive() {
		return g.up, g.initResult, nil
	}
	if g.up != nil {
		g.up.close()
		g.up = nil
	}
	up, err := openUpstream(g.spec, g.d.logf)
	if err != nil {
		return nil, nil, err
	}
	params, _ := json.Marshal(map[string]any{
		"protocolVersion": gatewayProtocol,
		"capabilities": map[string]any{
			"extensions": map[string]any{uiExtension: map[string]any{"mimeTypes": []string{uiMime}}},
		},
		"clientInfo": map[string]any{"name": "aterm", "version": version},
	})
	initCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	result, err := up.request(initCtx, "initialize", params)
	if err != nil {
		up.close()
		return nil, nil, err
	}
	if err := up.notify("notifications/initialized", nil); err != nil {
		up.close()
		return nil, nil, err
	}
	g.up, g.initResult = up, result
	g.tools, g.toolsLoaded = map[string]toolUI{}, false
	return up, result, nil
}

func (g *gatewayServer) close() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.up != nil {
		g.up.close()
		g.up = nil
	}
}

func (g *gatewayServer) toolMeta(name string) (toolUI, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	meta, ok := g.tools[name]
	return meta, ok
}

// observeTools records each tool's `_meta.ui` and returns the list without the
// tools only a view may call, which the spec keeps from the agent.
func (g *gatewayServer) observeTools(result json.RawMessage) json.RawMessage {
	var document map[string]json.RawMessage
	if json.Unmarshal(result, &document) != nil {
		return result
	}
	var listed []json.RawMessage
	if json.Unmarshal(document["tools"], &listed) != nil {
		return result
	}
	kept := make([]json.RawMessage, 0, len(listed))
	g.mu.Lock()
	if g.tools == nil {
		g.tools = map[string]toolUI{}
	}
	for _, raw := range listed {
		var tool struct {
			Name string          `json:"name"`
			Meta json.RawMessage `json:"_meta"`
		}
		if json.Unmarshal(raw, &tool) != nil || tool.Name == "" {
			kept = append(kept, raw)
			continue
		}
		meta := parseToolUI(tool.Meta)
		g.tools[tool.Name] = meta
		if meta.has("model") {
			kept = append(kept, raw)
		}
	}
	g.mu.Unlock()
	document["tools"], _ = json.Marshal(kept)
	filtered, err := json.Marshal(document)
	if err != nil {
		return result
	}
	return filtered
}

// loadTools reads the server's whole tool list once, for a call that arrives
// before the harness listed.
func (g *gatewayServer) loadTools(ctx context.Context, up mcpUpstream) {
	g.mu.Lock()
	loaded := g.toolsLoaded
	g.mu.Unlock()
	if loaded {
		return
	}
	cursor := ""
	for page := 0; page < 50; page++ {
		params := json.RawMessage(`{}`)
		if cursor != "" {
			params, _ = json.Marshal(map[string]string{"cursor": cursor})
		}
		result, err := up.request(ctx, "tools/list", params)
		if err != nil {
			g.d.logf("gateway %s/%s could not list tools: %v", g.session, g.name, err)
			return
		}
		g.observeTools(result)
		var next struct {
			NextCursor string `json:"nextCursor"`
		}
		_ = json.Unmarshal(result, &next)
		if cursor = next.NextCursor; cursor == "" {
			break
		}
	}
	g.mu.Lock()
	g.toolsLoaded = true
	g.mu.Unlock()
}

// handle forwards a harness request unchanged, except initialize (done at
// start), the tools/list filter, and tools/call, where a view is captured.
func (g *gatewayServer) handle(ctx context.Context, request rpcRequest) (json.RawMessage, *rpcError) {
	switch request.Method {
	case "ping":
		return json.RawMessage(`{}`), nil
	case "initialize":
		_, result, err := g.ensure(ctx)
		return result, upstreamToRPC(err)
	case "tools/call":
		return g.callTool(ctx, request.Params)
	}
	up, _, err := g.ensure(ctx)
	if err != nil {
		return nil, upstreamToRPC(err)
	}
	result, err := up.request(ctx, request.Method, request.Params)
	if err != nil {
		return nil, upstreamToRPC(err)
	}
	if request.Method == "tools/list" {
		result = g.observeTools(result)
	}
	return result, nil
}

func upstreamToRPC(err error) *rpcError {
	if err == nil {
		return nil
	}
	var refused *upstreamError
	if errors.As(err, &refused) {
		return &rpcError{Code: refused.Code, Message: refused.Message}
	}
	return &rpcError{Code: -32603, Message: "the aterm gateway could not reach the server: " + err.Error()}
}

func (g *gatewayServer) callTool(ctx context.Context, params json.RawMessage) (json.RawMessage, *rpcError) {
	var call struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(params, &call); err != nil || call.Name == "" {
		return nil, &rpcError{Code: -32602, Message: "tools/call needs a tool name"}
	}
	up, _, err := g.ensure(ctx)
	if err != nil {
		return nil, upstreamToRPC(err)
	}
	g.loadTools(ctx, up)
	meta, _ := g.toolMeta(call.Name)
	if !meta.has("model") {
		return nil, &rpcError{Code: -32602, Message: "tool " + call.Name + " is only callable by a view"}
	}
	var view *liveView
	if meta.ResourceURI != "" {
		view = g.openView(ctx, up, call.Name, meta.ResourceURI, call.Arguments)
	}
	result, err := up.request(ctx, "tools/call", params)
	if err != nil {
		if view != nil {
			g.d.cancelView(view, err.Error())
		}
		return nil, upstreamToRPC(err)
	}
	if view == nil {
		// A server may name the view on the result instead of on the tool.
		var carried struct {
			Meta json.RawMessage `json:"_meta"`
		}
		_ = json.Unmarshal(result, &carried)
		if uri := parseToolUI(carried.Meta).ResourceURI; uri != "" {
			view = g.openView(ctx, up, call.Name, uri, call.Arguments)
		}
	}
	if view != nil {
		g.d.updateView(view, result)
	}
	return result, nil
}

// openView fetches the ui:// resource and pushes the view. A resource that
// cannot be read costs the view, never the tool call.
func (g *gatewayServer) openView(ctx context.Context, up mcpUpstream, tool, uri string, args json.RawMessage) *liveView {
	if !strings.HasPrefix(uri, "ui://") {
		g.d.logf("gateway %s/%s: %s names %q, which is not a ui:// resource", g.session, g.name, tool, uri)
		return nil
	}
	readCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	params, _ := json.Marshal(map[string]string{"uri": uri})
	result, err := up.request(readCtx, "resources/read", params)
	if err != nil {
		g.d.logf("gateway %s/%s could not read %s: %v", g.session, g.name, uri, err)
		return nil
	}
	var read struct {
		Contents []struct {
			MimeType string `json:"mimeType"`
			Text     string `json:"text"`
			Blob     string `json:"blob"`
			Meta     struct {
				UI struct {
					CSP struct {
						ConnectDomains  []string `json:"connectDomains"`
						ResourceDomains []string `json:"resourceDomains"`
						FrameDomains    []string `json:"frameDomains"`
						BaseURIDomains  []string `json:"baseUriDomains"`
					} `json:"csp"`
					PrefersBorder bool `json:"prefersBorder"`
				} `json:"ui"`
			} `json:"_meta"`
		} `json:"contents"`
	}
	if json.Unmarshal(result, &read) != nil {
		return nil
	}
	for _, content := range read.Contents {
		if strings.ReplaceAll(strings.ToLower(content.MimeType), " ", "") != uiMime {
			continue
		}
		html := content.Text
		if html == "" && content.Blob != "" {
			decoded, err := base64.StdEncoding.DecodeString(content.Blob)
			if err != nil {
				return nil
			}
			html = string(decoded)
		}
		if html == "" || len(html) > maxViewBytes {
			g.d.logf("gateway %s/%s: %s is empty or over %d bytes", g.session, g.name, uri, maxViewBytes)
			return nil
		}
		csp := content.Meta.UI.CSP
		frame := viewFrame{
			Server: g.name, Tool: tool, ResourceURI: uri, HTML: html, PrefersBorder: content.Meta.UI.PrefersBorder,
			CSP: &viewCSP{ConnectDomains: csp.ConnectDomains, ResourceDomains: csp.ResourceDomains,
				FrameDomains: csp.FrameDomains, BaseURIDomains: csp.BaseURIDomains},
			ToolInput: args,
		}
		if frame.CSP.empty() {
			frame.CSP = nil
		}
		if len(frame.ToolInput) == 0 {
			frame.ToolInput = json.RawMessage(`{}`)
		}
		return g.d.addView(g, frame)
	}
	g.d.logf("gateway %s/%s: %s is not %s", g.session, g.name, uri, uiMime)
	return nil
}

// fromView is a view's own call back to its server: tools/call for a tool that
// lists "app" in its visibility, or resources/read.
func (g *gatewayServer) fromView(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, error) {
	up, _, err := g.ensure(ctx)
	if err != nil {
		return nil, err
	}
	if method == "tools/call" {
		var call struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(params, &call); err != nil || call.Name == "" {
			return nil, withExit(exitUsage, errors.New("tools/call needs a tool name"))
		}
		g.loadTools(ctx, up)
		meta, known := g.toolMeta(call.Name)
		switch {
		case !known:
			return nil, withExit(exitOffRoster, errors.New("the server has no tool named "+call.Name))
		case !meta.has("app"):
			return nil, withExit(exitUsage, errors.New("tool "+call.Name+" is not callable by a view (its _meta.ui.visibility leaves out app)"))
		}
	}
	return up.request(ctx, method, params)
}

// serveGateway is the harness side: Streamable HTTP, one JSON answer per POST.
// The token in the path is the auth, so only a local non-browser peer is answered.
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
	switch r.Method {
	case http.MethodDelete:
		w.WriteHeader(http.StatusOK)
		return
	case http.MethodPost:
	default:
		w.Header().Set("Allow", "POST, DELETE")
		http.Error(w, "aterm gateway: this server streams nothing to a GET", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxGatewayRPC))
	if err != nil {
		http.Error(w, "aterm gateway: "+err.Error(), http.StatusRequestEntityTooLarge)
		return
	}
	var request rpcRequest
	if json.Unmarshal(body, &request) != nil || request.Method == "" && len(request.ID) == 0 {
		http.Error(w, "aterm gateway: one JSON-RPC message per POST", http.StatusBadRequest)
		return
	}
	if len(request.ID) == 0 {
		// A notification wants no answer. The gateway keeps its own session with the server.
		w.WriteHeader(http.StatusAccepted)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), callTimeout)
	defer cancel()
	result, failure := g.handle(ctx, request)
	response := rpcResponse{JSONRPC: "2.0", ID: request.ID, Error: failure}
	if failure == nil {
		response.Result = result
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}
