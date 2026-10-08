package main

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// scriptedBrowser answers the daemon's page session, plays Playwright's side of a
// flattened attach, and records every command with the session it came on.
type scriptedBrowser struct {
	t      *testing.T
	out    io.Writer
	outMu  sync.Mutex
	mu     sync.Mutex
	calls  []cdpMessage
	exited chan struct{}
	once   sync.Once
}

func newScriptedBrowser(t *testing.T) (*browserLink, *scriptedBrowser) {
	t.Helper()
	toBrowserR, toBrowserW := io.Pipe()
	fromBrowserR, fromBrowserW := io.Pipe()
	fake := &scriptedBrowser{t: t, out: fromBrowserW, exited: make(chan struct{})}
	go func() {
		reader := bufio.NewReader(toBrowserR)
		for {
			line, err := reader.ReadBytes(0)
			if err != nil {
				return
			}
			var request cdpMessage
			if json.Unmarshal(line[:len(line)-1], &request) != nil {
				continue
			}
			fake.mu.Lock()
			fake.calls = append(fake.calls, request)
			fake.mu.Unlock()
			fake.answer(request)
		}
	}()
	t.Cleanup(fake.exit)
	return &browserLink{r: fromBrowserR, w: toBrowserW, exited: fake.exited, stop: fake.exit}, fake
}

func (f *scriptedBrowser) exit() { f.once.Do(func() { close(f.exited) }) }

func (f *scriptedBrowser) send(message map[string]any) {
	encoded, _ := json.Marshal(message)
	f.outMu.Lock()
	defer f.outMu.Unlock()
	_, _ = f.out.Write(append(encoded, 0))
}

func (f *scriptedBrowser) event(sessionID, method string, params any) {
	message := map[string]any{"method": method, "params": params}
	if sessionID != "" {
		message["sessionId"] = sessionID
	}
	f.send(message)
}

// answer replies the way Chromium does, echoing the session a command came on.
func (f *scriptedBrowser) answer(request cdpMessage) {
	result := map[string]any{}
	var after func()
	switch request.Method {
	case "Target.getTargets":
		result = map[string]any{"targetInfos": []targetInfo{{TargetID: "T1", Type: "page", URL: "about:blank", Title: "blank"}}}
	case "Target.attachToTarget":
		result = map[string]any{"sessionId": "PAGE"}
	case "Target.setAutoAttach":
		// Chromium attaches the open page to the connection that asked, as a
		// session of its own beside the one the daemon holds.
		after = func() {
			f.event("", "Target.attachedToTarget", map[string]any{
				"sessionId": "AGENT1", "targetInfo": targetInfo{TargetID: "T1", Type: "page"}, "waitingForDebugger": true,
			})
		}
	case "Target.createTarget":
		result = map[string]any{"targetId": "T2"}
	case "Target.createBrowserContext":
		result = map[string]any{"browserContextId": "CTX1"}
	case "Page.captureScreenshot":
		result = map[string]any{"data": fakeJPEG}
	case "Page.getLayoutMetrics":
		result = map[string]any{"cssLayoutViewport": map[string]any{"clientWidth": 1280, "clientHeight": 800}}
	case "Runtime.evaluate":
		result = map[string]any{"result": map[string]any{"type": "string", "value": "from " + request.SessionID}}
	case "Page.navigate":
		// A navigation on any session shows on every other session of the page.
		after = func() {
			f.event("AGENT1", "Page.frameNavigated", map[string]any{"frame": map[string]any{"url": "https://example.com/"}})
		}
	}
	f.send(map[string]any{"id": request.ID, "result": result, "sessionId": request.SessionID})
	if after != nil {
		after()
	}
}

func (f *scriptedBrowser) called(method string) []cdpMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	var matched []cdpMessage
	for _, call := range f.calls {
		if call.Method == method {
			matched = append(matched, call)
		}
	}
	return matched
}

func (f *scriptedBrowser) waitFor(method string, count int) []cdpMessage {
	f.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if calls := f.called(method); len(calls) >= count {
			return calls
		}
		time.Sleep(10 * time.Millisecond)
	}
	f.t.Fatalf("the browser never received %d %s, got %d", count, method, len(f.called(method)))
	return nil
}

// proxyHarness is a daemon with one session, a scripted browser behind it, and
// the address an agent under that session dials.
type proxyHarness struct {
	t       *testing.T
	d       *daemon
	fake    *scriptedBrowser
	started chan struct{}
	url     string
	token   string
	// peer is the pid the next connection's socket is attributed to.
	peer atomic.Int64
}

func newProxyHarness(t *testing.T) *proxyHarness {
	t.Helper()
	d, address := wsDaemon(t)
	wsSession(t, d, true)
	s := d.session("scientist-evie")
	d.mu.Lock()
	d.tokens[s.token] = s
	d.mu.Unlock()
	h := &proxyHarness{t: t, d: d, started: make(chan struct{}, 4), token: s.token}
	h.peer.Store(int64(os.Getpid()))
	d.peerLookup = func(net.Addr, net.Addr) ([]int, error) { return []int{int(h.peer.Load())}, nil }
	d.browserLaunch = func(string) (*browserLink, error) {
		link, fake := newScriptedBrowser(t)
		h.fake = fake
		h.started <- struct{}{}
		return link, nil
	}
	h.url = address + cdpPrefix + s.token
	return h
}

type cdpAgent struct {
	t  *testing.T
	ws *websocket.Conn
}

func (h *proxyHarness) dial() (*cdpAgent, *http.Response, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ws, response, err := websocket.Dial(ctx, h.url, nil)
	if err != nil {
		return nil, response, err
	}
	h.t.Cleanup(func() { _ = ws.CloseNow() })
	return &cdpAgent{h.t, ws}, response, nil
}

func (h *proxyHarness) connect() *cdpAgent {
	h.t.Helper()
	agent, response, err := h.dial()
	if err != nil {
		h.t.Fatalf("dial %s: %v (%+v)", h.url, err, response)
	}
	return agent
}

func (a *cdpAgent) send(id int, sessionID, method string, params any) {
	a.t.Helper()
	message := map[string]any{"id": id, "method": method, "params": params}
	if sessionID != "" {
		message["sessionId"] = sessionID
	}
	encoded, _ := json.Marshal(message)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.ws.Write(ctx, websocket.MessageText, encoded); err != nil {
		a.t.Fatalf("write: %v", err)
	}
}

type cdpIn struct {
	ID        *int            `json:"id"`
	Method    string          `json:"method"`
	Params    json.RawMessage `json:"params"`
	Result    json.RawMessage `json:"result"`
	SessionID string          `json:"sessionId"`
	Error     *cdpErrorBody   `json:"error"`
}

func (a *cdpAgent) next() (cdpIn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, data, err := a.ws.Read(ctx)
	if err != nil {
		return cdpIn{}, err
	}
	var in cdpIn
	_ = json.Unmarshal(data, &in)
	return in, nil
}

func (a *cdpAgent) until(label string, accept func(cdpIn) bool) cdpIn {
	a.t.Helper()
	for {
		in, err := a.next()
		if err != nil {
			a.t.Fatalf("waiting for %s: %v", label, err)
		}
		if accept(in) {
			return in
		}
	}
}

func (a *cdpAgent) answer(id int) cdpIn {
	a.t.Helper()
	return a.until("the answer to command", func(in cdpIn) bool { return in.ID != nil && *in.ID == id })
}

// attached has the agent set auto-attach and returns once its page session arrives.
func (a *cdpAgent) attached() {
	a.t.Helper()
	a.send(1, "", "Target.setAutoAttach", map[string]any{"autoAttach": true, "waitForDebuggerOnStart": true, "flatten": true})
	// Chromium may send the answer or the attach first, so take both in any order.
	var answered, attached bool
	a.until("the answer and the attach", func(in cdpIn) bool {
		answered = answered || in.ID != nil && *in.ID == 1
		attached = attached || in.Method == "Target.attachedToTarget"
		return answered && attached
	})
}

func TestCDPProxyStartsTheBrowserOnTheFirstConnectionAndRelaysByID(t *testing.T) {
	h := newProxyHarness(t)
	select {
	case <-h.started:
		t.Fatal("the browser must not start before an agent asks for it")
	default:
	}
	agent := h.connect()
	select {
	case <-h.started:
	case <-time.After(5 * time.Second):
		t.Fatal("the first CDP connection must start the session's browser")
	}
	agent.attached()

	agent.send(7, "AGENT1", "Runtime.evaluate", map[string]any{"expression": "1"})
	reply := agent.answer(7)
	if reply.SessionID != "AGENT1" || reply.Error != nil || !strings.Contains(string(reply.Result), "from AGENT1") {
		t.Fatalf("the answer must carry the agent's id and session: %+v", reply)
	}
	sent := h.fake.waitFor("Runtime.evaluate", 1)[0]
	if sent.SessionID != "AGENT1" {
		t.Fatalf("the browser should see the command on the agent's session: %+v", sent)
	}
}

func TestCDPProxyNeverLetsTheAgentAddressTheDaemonsPageSession(t *testing.T) {
	h := newProxyHarness(t)
	agent := h.connect()
	agent.attached()
	before := len(h.fake.called("Page.navigate"))

	agent.send(2, "PAGE", "Page.navigate", map[string]any{"url": "https://example.com/"})
	refused := agent.answer(2)
	if refused.Error == nil || !strings.Contains(refused.Error.Message, "Session with given id not found") || refused.SessionID != "PAGE" {
		t.Fatalf("the daemon's session is not the agent's: %+v", refused)
	}
	agent.send(3, "NOPE", "Runtime.evaluate", map[string]any{})
	if unknown := agent.answer(3); unknown.Error == nil {
		t.Fatalf("an invented session must fail: %+v", unknown)
	}
	if after := len(h.fake.called("Page.navigate")); after != before {
		t.Fatalf("a refused command must not reach the browser, it saw %d more Page.navigate", after-before)
	}
}

func TestCDPProxyKeepsTheDaemonsScreencastFramesFromTheAgent(t *testing.T) {
	h := newProxyHarness(t)
	agent := h.connect()
	agent.attached()
	// A frame on the daemon's session, then an event on the agent's.
	h.fake.event("PAGE", "Page.screencastFrame", map[string]any{"data": fakeJPEG, "metadata": map[string]any{}, "sessionId": 1})
	h.fake.event("AGENT1", "Page.loadEventFired", map[string]any{"timestamp": 1})
	got := agent.until("an event", func(in cdpIn) bool { return in.Method != "" })
	if got.Method != "Page.loadEventFired" || got.SessionID != "AGENT1" {
		t.Fatalf("the first event the agent sees must be its own page's, got %+v", got)
	}
}

func TestCDPProxyAgentSeesWhatAPersonDoesOnThePage(t *testing.T) {
	h := newProxyHarness(t)
	agent := h.connect()
	agent.attached()
	// The person's screen is a process outside the session, unlike the agent's Playwright.
	h.peer.Store(1 << 30)
	person, _ := dialLoopbackClient(t, strings.Replace(h.url, cdpPrefix+h.token, "", 1))
	person.send(frame{Type: "browser_watch", Session: "scientist-evie"})
	person.untilState("live", nil)
	person.send(frame{Type: "browser_control", Session: "scientist-evie", Take: true})
	person.untilState("live", func(m frame) bool { return m.Driver == "person" })
	person.send(frame{Type: "browser_navigate", Session: "scientist-evie", URL: "https://example.com/"})
	nav := agent.until("the person's navigation", func(in cdpIn) bool { return in.Method == "Page.frameNavigated" })
	if nav.SessionID != "AGENT1" || !strings.Contains(string(nav.Params), "https://example.com/") {
		t.Fatalf("the agent's page model must follow the person: %+v", nav)
	}
}

func TestCDPProxyKeepsTargetDiscoveryAndTheBrowserForThePerson(t *testing.T) {
	h := newProxyHarness(t)
	agent := h.connect()
	agent.attached()
	discoveries := len(h.fake.called("Target.setDiscoverTargets"))

	agent.send(2, "", "Target.setDiscoverTargets", map[string]any{"discover": false})
	agent.answer(2)
	agent.send(3, "", "Browser.close", map[string]any{})
	if reply := agent.answer(3); reply.Error != nil {
		t.Fatalf("Browser.close is acknowledged: %+v", reply)
	}
	agent.send(4, "", "Target.setDiscoverTargets", map[string]any{"discover": true})
	created := agent.until("targetCreated", func(in cdpIn) bool { return in.Method == "Target.targetCreated" })
	if !strings.Contains(string(created.Params), `"T1"`) {
		t.Fatalf("turning discovery on lists the open targets: %s", created.Params)
	}
	if len(h.fake.called("Browser.close")) != 0 || len(h.fake.called("Target.setDiscoverTargets")) != discoveries {
		t.Fatalf("the browser must not see Browser.close or the agent's discovery switch")
	}
}

func TestCDPProxyTidiesWhatTheAgentMadeAndLeavesTheBrowserUp(t *testing.T) {
	h := newProxyHarness(t)
	watcher, _ := dialLoopbackClient(t, strings.Replace(h.url, cdpPrefix+h.token, "", 1))
	agent := h.connect()
	agent.attached()
	agent.send(2, "", "Target.createBrowserContext", map[string]any{})
	agent.answer(2)
	agent.send(3, "", "Target.createTarget", map[string]any{"url": "about:blank", "browserContextId": "CTX1"})
	agent.answer(3)
	_ = agent.ws.Close(websocket.StatusNormalClosure, "")

	closed := h.fake.waitFor("Target.closeTarget", 1)[0]
	if !strings.Contains(string(closed.Params), `"T2"`) {
		t.Fatalf("the tab the agent opened should close: %s", closed.Params)
	}
	h.fake.waitFor("Target.disposeBrowserContext", 1)
	h.fake.waitFor("Target.detachFromTarget", 1)
	off := h.fake.waitFor("Target.setAutoAttach", 2)[1]
	if !strings.Contains(string(off.Params), `"autoAttach":false`) {
		t.Fatalf("auto-attach should be switched off: %s", off.Params)
	}
	watcher.send(frame{Type: "browser_watch", Session: "scientist-evie"})
	if live := watcher.untilState("live", nil); live.Driver != "agent" {
		t.Fatalf("the browser outlives its agent: %+v", live)
	}
}

func TestCDPProxyANewerConnectionReplacesTheOlder(t *testing.T) {
	h := newProxyHarness(t)
	first := h.connect()
	first.attached()
	second := h.connect()
	second.attached()
	if _, err := first.next(); err == nil {
		t.Fatal("the replaced connection should have ended")
	}
	second.send(5, "AGENT1", "Runtime.evaluate", map[string]any{})
	if reply := second.answer(5); reply.Error != nil {
		t.Fatalf("the newer connection should work: %+v", reply)
	}
}

func TestCDPProxyEndsWithTheBrowser(t *testing.T) {
	h := newProxyHarness(t)
	agent := h.connect()
	agent.attached()
	h.fake.exit()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := agent.next(); err != nil {
			return
		}
	}
	t.Fatal("the agent's connection should end when the browser does")
}

func TestCDPProxyRefusesAPeerItCannotVouchFor(t *testing.T) {
	cases := map[string]func(h *proxyHarness) (status int){
		"a token no session has": func(h *proxyHarness) int {
			h.url = strings.Replace(h.url, h.token, "not-a-token", 1)
			_, response, _ := h.dial()
			return response.StatusCode
		},
		"a peer outside the session": func(h *proxyHarness) int {
			h.d.processes = func() ([]processEntry, error) {
				return []processEntry{{PID: h.d.session("scientist-evie").pid, PPID: 1}, {PID: os.Getpid(), PPID: 1}}, nil
			}
			_, response, _ := h.dial()
			return response.StatusCode
		},
		"a process table it cannot read": func(h *proxyHarness) int {
			h.d.processes = func() ([]processEntry, error) { return nil, io.ErrClosedPipe }
			_, response, _ := h.dial()
			return response.StatusCode
		},
		"a socket table it cannot read": func(h *proxyHarness) int {
			h.d.peerLookup = func(net.Addr, net.Addr) ([]int, error) { return nil, io.ErrClosedPipe }
			_, response, _ := h.dial()
			return response.StatusCode
		},
		"a peer with no named process": func(h *proxyHarness) int {
			h.d.peerLookup = func(net.Addr, net.Addr) ([]int, error) { return nil, nil }
			_, response, _ := h.dial()
			return response.StatusCode
		},
	}
	for name, try := range cases {
		t.Run(name, func(t *testing.T) {
			h := newProxyHarness(t)
			status := try(h)
			if status != http.StatusForbidden && status != http.StatusNotFound {
				t.Fatalf("status = %d, want a refusal", status)
			}
			select {
			case <-h.started:
				t.Fatal("a refused peer must not start the browser")
			default:
			}
		})
	}
}

func TestCDPProxyRefusesABrowserPageAndAPlainRequest(t *testing.T) {
	h := newProxyHarness(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, response, err := websocket.Dial(ctx, h.url, &websocket.DialOptions{HTTPHeader: http.Header{"Origin": []string{"http://localhost:5173"}}})
	if err == nil || response == nil || response.StatusCode != http.StatusForbidden {
		t.Fatalf("a page with an Origin must be refused: %v %+v", err, response)
	}
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http"+strings.TrimPrefix(h.url, "ws"), nil)
	plain, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer plain.Body.Close()
	if plain.StatusCode != http.StatusUpgradeRequired {
		t.Fatalf("a plain GET should be told to upgrade, got %d", plain.StatusCode)
	}
}

func TestCDPProxyReportsABrowserThatDoesNotStart(t *testing.T) {
	h := newProxyHarness(t)
	h.d.browserLaunch = func(string) (*browserLink, error) { return nil, io.ErrClosedPipe }
	_, response, err := h.dial()
	if err == nil || response == nil || response.StatusCode != http.StatusBadGateway {
		t.Fatalf("a browser that will not start is a bad gateway: %v %+v", err, response)
	}
}

func TestWithCDPEndpointNeedsTheOptInAndAListener(t *testing.T) {
	d := newDaemon(func(string, ...any) {})
	d.browserLaunch = func(string) (*browserLink, error) { return nil, nil }
	environ := []string{"A=1", cdpEndpointEnv + "=ws://stale", browserProxyEnv + "=1"}

	if got := d.withCDPEndpoint(environ, "tok"); strings.Join(got, "\n") != strings.Join(environ, "\n") {
		t.Fatalf("no listener leaves the environment as launched: %v", got)
	}
	d.loopbackAddr = "127.0.0.1:7419"
	got := d.withCDPEndpoint(environ, "tok")
	if want := cdpEndpointEnv + "=ws://127.0.0.1:7419/cdp/tok"; !containsEntry(got, want) || strings.Count(strings.Join(got, "\n"), cdpEndpointEnv) != 1 {
		t.Fatalf("the opted-in launch should carry exactly %q: %v", want, got)
	}
	plain := []string{"A=1"}
	if got := d.withCDPEndpoint(plain, "tok"); len(got) != 1 {
		t.Fatalf("a launch that did not opt in is left alone: %v", got)
	}
}

// personTakes has a screen outside the session watch the browser and take control.
func (h *proxyHarness) personTakes() wsClient {
	h.t.Helper()
	h.peer.Store(1 << 30)
	person, _ := dialLoopbackClient(h.t, strings.Replace(h.url, cdpPrefix+h.token, "", 1))
	person.send(frame{Type: "browser_watch", Session: "scientist-evie"})
	person.untilState("live", nil)
	person.send(frame{Type: "browser_control", Session: "scientist-evie", Take: true})
	person.untilState("live", func(m frame) bool { return m.Driver == "person" })
	return person
}

func TestCDPProxyRefusesTheAgentsCommandsWhileAPersonHoldsControl(t *testing.T) {
	h := newProxyHarness(t)
	agent := h.connect()
	agent.attached()
	person := h.personTakes()

	for id, method := range map[int]string{10: "Page.navigate", 11: "Input.dispatchMouseEvent", 12: "DOM.getDocument", 13: "Emulation.setUserAgentOverride", 14: "Page.captureScreenshot"} {
		agent.send(id, "AGENT1", method, map[string]any{})
		if refused := agent.answer(id); refused.Error == nil || refused.Error.Message != personHasControl || refused.SessionID != "AGENT1" {
			t.Fatalf("%s must be refused with the handback message: %+v", method, refused)
		}
	}
	agent.send(15, "", "Target.createTarget", map[string]any{"url": "about:blank"})
	if refused := agent.answer(15); refused.Error == nil || refused.Error.Message != personHasControl {
		t.Fatalf("opening a tab is a command too: %+v", refused)
	}
	// An evaluation is refused as a thrown exception, which Playwright reports as sent.
	for id, method := range map[int]string{16: "Runtime.evaluate", 17: "Runtime.callFunctionOn"} {
		agent.send(id, "AGENT1", method, map[string]any{"expression": "document.title"})
		thrown := agent.answer(id)
		if thrown.Error != nil || !strings.Contains(string(thrown.Result), personHasControl) || !strings.Contains(string(thrown.Result), "exceptionDetails") {
			t.Fatalf("%s should be refused as a thrown error: %+v", method, thrown)
		}
	}
	for _, method := range []string{"Page.navigate", "Input.dispatchMouseEvent", "DOM.getDocument", "Emulation.setUserAgentOverride", "Page.captureScreenshot", "Target.createTarget", "Runtime.callFunctionOn"} {
		if len(h.fake.called(method)) != 0 {
			t.Fatalf("the browser must never see a refused %s", method)
		}
	}

	// Events keep flowing, so the agent's page model follows the person.
	h.fake.event("AGENT1", "Page.frameNavigated", map[string]any{"frame": map[string]any{"url": "https://person.example/"}})
	if nav := agent.until("the person's navigation", func(in cdpIn) bool { return in.Method == "Page.frameNavigated" }); nav.SessionID != "AGENT1" {
		t.Fatalf("events must reach the agent during person control: %+v", nav)
	}

	person.send(frame{Type: "browser_control", Session: "scientist-evie", Take: false})
	person.untilState("live", func(m frame) bool { return m.Driver == "agent" })
	agent.send(20, "AGENT1", "Page.navigate", map[string]any{"url": "https://agent.example/"})
	if reply := agent.answer(20); reply.Error != nil {
		t.Fatalf("handing back restores the agent's commands: %+v", reply)
	}
	if len(h.fake.called("Page.navigate")) != 1 {
		t.Fatalf("only the command sent after the hand back reaches the browser, with no replay: %d", len(h.fake.called("Page.navigate")))
	}
}

func TestCDPProxyKeepsPlaywrightsBookkeepingWhileAPersonHoldsControl(t *testing.T) {
	h := newProxyHarness(t)
	agent := h.connect()
	agent.attached()
	h.personTakes()
	allowed := []string{
		"Page.enable", "Runtime.enable", "Network.enable", "Log.enable", "Target.setAutoAttach", "Target.getTargetInfo",
		"Target.detachFromTarget", "Runtime.runIfWaitingForDebugger", "Page.getFrameTree", "Page.setLifecycleEventsEnabled",
		"Page.addScriptToEvaluateOnNewDocument", "Page.createIsolatedWorld", "Page.setFontFamilies", "Page.setInterceptFileChooserDialog",
	}
	for i, method := range allowed {
		agent.send(100+i, "AGENT1", method, map[string]any{})
		if reply := agent.answer(100 + i); reply.Error != nil {
			t.Fatalf("%s is bookkeeping and must pass during person control: %+v", method, reply.Error)
		}
	}
	agent.send(200, "", "Browser.getVersion", map[string]any{})
	if reply := agent.answer(200); reply.Error != nil {
		t.Fatalf("Browser.getVersion must pass: %+v", reply.Error)
	}
}
