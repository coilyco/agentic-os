package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// fakeMCPCommand is the test binary run as a stdio MCP server with one chart
// tool that returns a view, one app-only tool, and one text-only tool.
const fakeMCPCommand = "fake-mcp-server"

const fakeViewHTML = "<html><body>chart</body></html>"

// fakeMCPReply is the fake server's whole behavior, shared by both transports.
var object = map[string]any{"type": "object"}

// fakeRPC is the one request shape the fake servers read.
type fakeRPC struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

type upstreamError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func fakeMCPReply(method string, params json.RawMessage) (any, *upstreamError) {
	switch method {
	case "initialize":
		var init struct {
			ProtocolVersion string `json:"protocolVersion"`
			Capabilities    struct {
				Extensions map[string]json.RawMessage `json:"extensions"`
			} `json:"capabilities"`
		}
		_ = json.Unmarshal(params, &init)
		_, ui := init.Capabilities.Extensions[uiExtension]
		return map[string]any{
			"protocolVersion": init.ProtocolVersion, "capabilities": map[string]any{"tools": map[string]any{}},
			"serverInfo": map[string]any{"name": "fake", "version": "1"}, "instructions": fmt.Sprintf("ui=%v", ui),
		}, nil
	case "tools/list":
		return map[string]any{"tools": []map[string]any{
			{"name": "chart", "inputSchema": object, "_meta": map[string]any{"ui": map[string]any{"resourceUri": "ui://fake/chart"}}},
			{"name": "refresh", "inputSchema": object, "_meta": map[string]any{"ui": map[string]any{"visibility": []string{"app"}}}},
			{"name": "plain", "inputSchema": object},
			{"name": "carried", "inputSchema": object},
			{"name": "agent-only", "inputSchema": object, "_meta": map[string]any{"ui": map[string]any{"visibility": []string{"model"}}}},
		}}, nil
	case "tools/call":
		var call struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		_ = json.Unmarshal(params, &call)
		result := map[string]any{
			"content":           []map[string]any{{"type": "text", "text": "ran " + call.Name}},
			"structuredContent": map[string]any{"tool": call.Name, "arguments": call.Arguments},
		}
		if call.Name == "carried" {
			// The view is named on the result, by the spec's deprecated flat key.
			result["_meta"] = map[string]any{"ui/resourceUri": "ui://fake/chart"}
		}
		return result, nil
	case "resources/read":
		return map[string]any{"contents": []map[string]any{{
			"uri": "ui://fake/chart", "mimeType": uiMime, "text": fakeViewHTML,
			"_meta": map[string]any{"ui": map[string]any{
				"csp": map[string]any{"connectDomains": []string{"https://data.test"}}, "prefersBorder": true}},
		}}}, nil
	}
	return nil, &upstreamError{Code: -32601, Message: "no method " + method}
}

func runFakeMCP() {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var request fakeRPC
		if json.Unmarshal(scanner.Bytes(), &request) != nil || len(request.ID) == 0 {
			continue
		}
		result, failure := fakeMCPReply(request.Method, request.Params)
		reply := map[string]any{"jsonrpc": "2.0", "id": request.ID}
		if failure != nil {
			reply["error"] = failure
		} else {
			reply["result"] = result
		}
		encoded, _ := json.Marshal(reply)
		fmt.Fprintf(os.Stdout, "%s\n", encoded)
	}
}

// fakeHTTPMCP answers tools/call as an event stream and the rest as JSON, so
// both ways a Streamable HTTP server may answer are read.
func fakeHTTPMCP(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var request fakeRPC
		_ = json.NewDecoder(r.Body).Decode(&request)
		if len(request.ID) == 0 {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		result, failure := fakeMCPReply(request.Method, request.Params)
		reply := map[string]any{"jsonrpc": "2.0", "id": request.ID}
		if failure != nil {
			reply["error"] = failure
		} else {
			reply["result"] = result
		}
		encoded, _ := json.Marshal(reply)
		if request.Method == "tools/call" {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprintf(w, "event: message\ndata: %s\n\n", encoded)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(encoded)
	}))
	t.Cleanup(server.Close)
	return server
}

// wire is a client connection read by a goroutine, since the daemon writes to a
// watcher while holding its lock and a pipe nobody reads would stall it.
type wire struct {
	t   *testing.T
	c   *conn
	in  chan frame
	pid int
}

func (d *daemon) dial(t *testing.T, peer peerStanding) *wire {
	t.Helper()
	client, server := net.Pipe()
	go d.serveConn(newConn(server), peer)
	w := &wire{t: t, c: newConn(client), in: make(chan frame, 64)}
	t.Cleanup(func() { _ = w.c.Close() })
	go func() {
		for {
			message, err := w.c.read()
			if err != nil {
				close(w.in)
				return
			}
			w.in <- message
		}
	}()
	w.send(frame{Type: "hello", Format: daemonFormat})
	w.next("welcome")
	return w
}

func (w *wire) send(message frame) {
	w.t.Helper()
	if err := w.c.write(message); err != nil {
		w.t.Fatalf("write %s: %v", message.Type, err)
	}
}

// next reads up to the wanted type, failing on a deadline.
func (w *wire) next(want string) frame {
	w.t.Helper()
	deadline := time.After(10 * time.Second)
	for {
		select {
		case message, open := <-w.in:
			if !open {
				w.t.Fatalf("connection closed waiting for %s", want)
			}
			if message.Type == want {
				return message
			}
		case <-deadline:
			w.t.Fatalf("no %s frame within 10s", want)
		}
	}
}

// following is the very next frame, whatever it is.
func (w *wire) following() frame {
	w.t.Helper()
	select {
	case message := <-w.in:
		return message
	case <-time.After(10 * time.Second):
		w.t.Fatal("no frame within 10s")
		return frame{}
	}
}

func (w *wire) silent() {
	w.t.Helper()
	select {
	case message := <-w.in:
		w.t.Fatalf("expected no frame, got %s %+v", message.Type, message)
	case <-time.After(300 * time.Millisecond):
	}
}

func (w *wire) reply(message frame) frame {
	w.t.Helper()
	message.ID = randomID(4)
	w.send(message)
	for {
		got := w.following()
		if got.ID == message.ID {
			return got
		}
	}
}

// appsHarness is a daemon with one session, its gateway reachable over HTTP.
type appsHarness struct {
	t      *testing.T
	d      *daemon
	http   *httptest.Server
	token  string
	seat   string
	client *wire
	// sessions are the harness connections by gateway path.
	sessions map[string]*mcp.ClientSession
}

func newAppsHarness(t *testing.T) *appsHarness {
	t.Helper()
	d := newDaemon(func(string, ...any) {})
	d.holdDir = testHoldDir(t)
	t.Cleanup(d.endAll)
	t.Cleanup(d.closeApps)
	// Nobody in this process tree runs under a session, so the typing guard passes.
	d.processes = func() ([]processEntry, error) { return nil, nil }
	server := httptest.NewServer(d.websocketHandler())
	t.Cleanup(server.Close)
	h := &appsHarness{t: t, d: d, http: server, seat: "eng-platform-beetle-ox", sessions: map[string]*mcp.ClientSession{}}
	h.client = d.dial(t, peerStanding{pids: []int{os.Getpid()}})
	h.client.send(frame{
		Type: "spawn", ID: "spawn", Session: h.seat, Role: "eng-platform", Identity: "Beetle-Ox",
		Argv: []string{"/bin/sh", "-c", "sleep 60"}, Env: append(os.Environ(), mcpAppsEnv+"=1"), Cwd: "/",
	})
	h.client.next("spawned")
	h.token = d.session(h.seat).token
	return h
}

func (h *appsHarness) register(name string, spec gatewaySpec) gatewaySpec {
	h.t.Helper()
	reply := h.client.reply(frame{Type: "gateway_add", Token: h.token, Server: name, Gateway: &spec})
	if reply.Type != "gateway_added" || reply.Gateway == nil {
		h.t.Fatalf("gateway_add %s: %+v", name, reply)
	}
	return *reply.Gateway
}

// watcher subscribes a fresh client to the views channel.
func (h *appsHarness) watcher() *wire {
	h.t.Helper()
	w := h.d.dial(h.t, peerStanding{pids: []int{os.Getpid()}})
	w.send(frame{Type: "subscribe", ID: "v", Channel: "views"})
	w.next("subscribed")
	return w
}

// connect is a harness: an SDK MCP client on the session's gateway path.
func (h *appsHarness) connect(path string) *mcp.ClientSession {
	h.t.Helper()
	if cs := h.sessions[path]; cs != nil {
		return cs
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-harness", Version: "1"}, nil)
	cs, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: h.http.URL + path}, nil)
	if err != nil {
		h.t.Fatalf("connect %s: %v", path, err)
	}
	h.t.Cleanup(func() { _ = cs.Close() })
	h.sessions[path] = cs
	return cs
}

// call calls a tool the way a harness does, through the gateway.
func (h *appsHarness) call(path, name string, arguments map[string]any) (*mcp.CallToolResult, error) {
	h.t.Helper()
	return h.connect(path).CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: arguments})
}

func (h *appsHarness) callOK(path, name string, arguments map[string]any) *mcp.CallToolResult {
	h.t.Helper()
	result, err := h.call(path, name, arguments)
	if err != nil {
		h.t.Fatalf("call %s: %v", name, err)
	}
	return result
}

func resultText(result *mcp.CallToolResult) string {
	var text strings.Builder
	for _, content := range result.Content {
		if block, ok := content.(*mcp.TextContent); ok {
			text.WriteString(block.Text)
		}
	}
	return text.String()
}

func stdioSpec() gatewaySpec {
	self, _ := os.Executable()
	return gatewaySpec{Command: self, Args: []string{fakeMCPCommand}}
}

// eachTransport runs the test against a stdio server and an http one.
func eachTransport(t *testing.T, run func(t *testing.T, spec gatewaySpec)) {
	t.Run("stdio", func(t *testing.T) { run(t, stdioSpec()) })
	t.Run("http", func(t *testing.T) { run(t, gatewaySpec{URL: fakeHTTPMCP(t).URL}) })
}

func TestGatewayForwardsACallAndPushesTheViewThenItsResult(t *testing.T) {
	eachTransport(t, func(t *testing.T, spec gatewaySpec) {
		h := newAppsHarness(t)
		path := h.register("fake", spec).Path
		if want := gatewayPrefix + h.token + "/fake"; path != want {
			t.Fatalf("path = %q, want %q", path, want)
		}
		watcher := h.watcher()

		harness := h.connect(path)
		if got := harness.InitializeResult().Instructions; got != "ui=true" {
			t.Fatalf("the gateway should advertise the ui extension to the server, and answer with its instructions: %q", got)
		}
		listed, err := harness.ListTools(context.Background(), nil)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		var names []string
		for _, tool := range listed.Tools {
			names = append(names, tool.Name)
		}
		if !slices.Equal(names, []string{"chart", "plain", "carried", "agent-only"}) {
			t.Fatalf("the agent's list should drop the app-only tool: %v", names)
		}

		called := h.callOK(path, "chart", map[string]any{"n": 3})
		if got := resultText(called); got != "ran chart" {
			t.Fatalf("the harness should get the server's own result: %q", got)
		}

		view := watcher.next("view")
		if view.Session != h.seat || view.View == nil {
			t.Fatalf("view = %+v", view)
		}
		if v := view.View; v.Server != "fake" || v.Tool != "chart" || v.ResourceURI != "ui://fake/chart" || v.HTML != fakeViewHTML ||
			!v.PrefersBorder || v.CSP == nil || len(v.CSP.ConnectDomains) != 1 || string(v.ToolInput) != `{"n":3}` {
			t.Fatalf("view frame = %+v csp=%+v input=%s", v, v.CSP, v.ToolInput)
		}
		update := watcher.following()
		if update.Type != "view_update" || update.ViewID != view.View.ID || !strings.Contains(string(update.ToolResult), "ran chart") {
			t.Fatalf("the view should be followed by its result: %+v", update)
		}

		// The view calls back: an app-only tool, an ordinary one, and a resource.
		for _, call := range []frame{
			{Method: "tools/call", Params: json.RawMessage(`{"name":"refresh"}`)},
			{Method: "tools/call", Params: json.RawMessage(`{"name":"chart","arguments":{"n":4}}`)},
			{Method: "resources/read", Params: json.RawMessage(`{"uri":"ui://fake/chart"}`)},
		} {
			call.Type, call.ViewID = "view_call", view.View.ID
			reply := h.client.reply(call)
			if reply.Type != "view_result" || len(reply.Result) == 0 {
				t.Fatalf("%s %s = %+v", call.Method, call.Params, reply)
			}
		}
	})
}

func TestAViewNamedOnTheResultOpensToo(t *testing.T) {
	h := newAppsHarness(t)
	path := h.register("fake", stdioSpec()).Path
	watcher := h.watcher()
	h.callOK(path, "carried", nil)
	view := watcher.next("view").View
	if view.Tool != "carried" || view.ResourceURI != "ui://fake/chart" || view.HTML != fakeViewHTML {
		t.Fatalf("view = %+v", view)
	}
	if update := watcher.following(); update.Type != "view_update" || update.ViewID != view.ID {
		t.Fatalf("update = %+v", update)
	}
}

func TestAViewCannotCallWhatItShouldNot(t *testing.T) {
	h := newAppsHarness(t)
	path := h.register("fake", stdioSpec()).Path
	watcher := h.watcher()
	h.callOK(path, "chart", nil)
	view := watcher.next("view").View
	for name, call := range map[string]frame{
		"a tool the server lacks":  {Method: "tools/call", Params: json.RawMessage(`{"name":"nope"}`)},
		"a method outside the two": {Method: "prompts/get", Params: json.RawMessage(`{}`)},
		"an unknown view":          {Method: "tools/call", Params: json.RawMessage(`{"name":"chart"}`), ViewID: "gone"},
	} {
		call.Type = "view_call"
		if call.ViewID == "" {
			call.ViewID = view.ID
		}
		if reply := h.client.reply(call); reply.Type != "error" || reply.Code == 0 {
			t.Fatalf("%s should answer an error with a code: %+v", name, reply)
		}
	}
}

func TestAViewCallRefusesAToolThatLeavesOutApp(t *testing.T) {
	h := newAppsHarness(t)
	path := h.register("fake", stdioSpec()).Path
	watcher := h.watcher()
	h.callOK(path, "chart", nil)
	view := watcher.next("view").View
	reply := h.client.reply(frame{Type: "view_call", ViewID: view.ID, Method: "tools/call", Params: json.RawMessage(`{"name":"agent-only"}`)})
	if reply.Type != "error" || reply.Code != exitUsage || !strings.Contains(reply.Error, "visibility") {
		t.Fatalf("a model-only tool should be refused to a view: %+v", reply)
	}
}

func TestATextOnlyResultMakesNoView(t *testing.T) {
	h := newAppsHarness(t)
	path := h.register("fake", stdioSpec()).Path
	watcher := h.watcher()
	h.callOK(path, "plain", nil)
	watcher.silent()
}

func TestAViewReplaysToALateWatcherAndClosesEverywhere(t *testing.T) {
	h := newAppsHarness(t)
	path := h.register("fake", stdioSpec()).Path
	early := h.watcher()
	h.callOK(path, "chart", nil)
	view := early.next("view").View
	early.next("view_update")

	late := h.d.dial(t, peerStanding{pids: []int{os.Getpid()}})
	late.send(frame{Type: "subscribe", Channel: "views"})
	if got := late.following(); got.Type != "subscribed" {
		t.Fatalf("first frame = %+v", got)
	}
	if got := late.following(); got.Type != "view" || got.View.ID != view.ID {
		t.Fatalf("a late watcher should get the view: %+v", got)
	}
	if got := late.following(); got.Type != "view_update" || got.ViewID != view.ID {
		t.Fatalf("then its latest update: %+v", got)
	}

	h.client.send(frame{Type: "view_close", ID: "c", ViewID: view.ID})
	for _, w := range []*wire{early, late} {
		if closed := w.next("view_closed"); closed.ViewID != view.ID {
			t.Fatalf("view_closed = %+v", closed)
		}
	}
	after := h.watcher()
	after.silent()
}

func TestAnEndedSessionTakesItsViewsAndServersWithIt(t *testing.T) {
	h := newAppsHarness(t)
	path := h.register("fake", stdioSpec()).Path
	watcher := h.watcher()
	h.callOK(path, "chart", nil)
	view := watcher.next("view").View
	g := h.d.gatewayServer(h.seat, "fake")
	h.d.forget(h.d.session(h.seat))
	if closed := watcher.next("view_closed"); closed.ViewID != view.ID {
		t.Fatalf("view_closed = %+v", closed)
	}
	if h.d.gatewayServer(h.seat, "fake") != nil {
		t.Fatal("the session's gateway server should be gone")
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.up != nil {
		t.Fatal("the server's upstream should be closed")
	}
}

func TestGatewayAnswersOnlyALocalHarnessWithAKnownPath(t *testing.T) {
	h := newAppsHarness(t)
	path := h.register("fake", gatewaySpec{URL: fakeHTTPMCP(t).URL}).Path
	initialize := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`
	do := func(target string, header map[string]string) int {
		request, _ := http.NewRequest(http.MethodPost, h.http.URL+target, strings.NewReader(initialize))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Accept", "application/json, text/event-stream")
		for name, value := range header {
			request.Header.Set(name, value)
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatalf("POST %s: %v", target, err)
		}
		_ = response.Body.Close()
		return response.StatusCode
	}
	for name, check := range map[string]struct{ got, want int }{
		"known path":     {do(path, nil), 200},
		"another token":  {do(gatewayPrefix+"nope/fake", nil), 404},
		"another server": {do(gatewayPrefix+h.token+"/other", nil), 404},
		"a browser page": {do(path, map[string]string{"Origin": "http://localhost:5173"}), 403},
	} {
		if check.got != check.want {
			t.Errorf("%s answered %d, want %d", name, check.got, check.want)
		}
	}
}

func TestAModelCannotCallAToolOnlyAViewMay(t *testing.T) {
	h := newAppsHarness(t)
	path := h.register("fake", stdioSpec()).Path
	if _, err := h.call(path, "refresh", nil); err == nil || !strings.Contains(err.Error(), "only callable by a view") {
		t.Fatalf("direct call to an app-only tool = %v", err)
	}
}

func TestGatewayAddRefusesABrowserAndABadSpec(t *testing.T) {
	h := newAppsHarness(t)
	browser := h.d.dial(t, peerStanding{web: true, pids: []int{os.Getpid()}})
	spec := stdioSpec()
	if reply := browser.reply(frame{Type: "gateway_add", Token: h.token, Server: "fake", Gateway: &spec}); reply.Type != "error" {
		t.Fatalf("a browser must not register a stdio command: %+v", reply)
	}
	for name, add := range map[string]frame{
		"a bad name":       {Server: "Fake Server", Gateway: &spec},
		"both":             {Server: "fake", Gateway: &gatewaySpec{Command: "x", URL: "http://x.test"}},
		"neither":          {Server: "fake", Gateway: &gatewaySpec{}},
		"a file url":       {Server: "fake", Gateway: &gatewaySpec{URL: "file:///etc/passwd"}},
		"an unknown token": {Server: "fake", Token: "nope", Gateway: &spec},
	} {
		add.Type = "gateway_add"
		if add.Token == "" {
			add.Token = h.token
		}
		if reply := h.client.reply(add); reply.Type != "error" || reply.Code != exitUsage {
			t.Errorf("%s should be refused with exit 2: %+v", name, reply)
		}
	}
}

func TestGatewayIsOffUnlessTheLaunchOptedIn(t *testing.T) {
	h := newAppsHarness(t)
	h.client.send(frame{
		Type: "spawn", ID: "plain", Session: "scientist-evie", Role: "scientist", Identity: "Evie",
		Argv: []string{"/bin/sh", "-c", "sleep 60"}, Env: os.Environ(), Cwd: "/",
	})
	h.client.next("spawned")
	spec := stdioSpec()
	reply := h.client.reply(frame{Type: "gateway_add", Token: h.d.session("scientist-evie").token, Server: "fake", Gateway: &spec})
	if reply.Type != "error" || reply.Code != exitUsage || !strings.Contains(reply.Error, mcpAppsEnv) {
		t.Fatalf("a session launched without the opt-in must not get a gateway: %+v", reply)
	}
	if h.d.gatewayServer("scientist-evie", "fake") != nil {
		t.Fatal("nothing should be registered for a session that did not opt in")
	}
}

func TestAViewCallFromInsideASessionTakesTheTypingGuard(t *testing.T) {
	h := newAppsHarness(t)
	path := h.register("fake", stdioSpec()).Path
	watcher := h.watcher()
	h.callOK(path, "chart", nil)
	view := watcher.next("view").View
	session := h.d.session(h.seat)
	h.d.processes = func() ([]processEntry, error) {
		return []processEntry{{PID: session.pid, PPID: 1}, {PID: os.Getpid(), PPID: session.pid}}, nil
	}
	reply := h.client.reply(frame{Type: "view_call", ViewID: view.ID, Method: "resources/read", Params: json.RawMessage(`{"uri":"ui://fake/chart"}`)})
	if reply.Type != "error" || reply.Reason != reasonSessionDescendant {
		t.Fatalf("a client under a session must not use a view as Kai: %+v", reply)
	}
}

func TestWelcomeListsMCPApps(t *testing.T) {
	d := newDaemon(func(string, ...any) {})
	client, server := net.Pipe()
	go d.serveConn(newConn(server), peerStanding{})
	c := newConn(client)
	t.Cleanup(func() { _ = c.Close() })
	_ = c.write(frame{Type: "hello", Format: daemonFormat})
	welcome := nextFrame(t, c, "welcome")
	for _, feature := range welcome.Features {
		if feature == mcpAppsFeature {
			return
		}
	}
	t.Fatalf("welcome.features = %v, want %s", welcome.Features, mcpAppsFeature)
}
