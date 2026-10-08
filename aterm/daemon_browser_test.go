package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

var fakeJPEG = []byte{0xff, 0xd8, 0xff, 0xe0, 'f', 'a', 'k', 'e'}

// fakeBrowser answers the CDP commands a page session needs and records what
// reached it, so the daemon's frames are judged without a real Chromium.
type fakeBrowser struct {
	t      *testing.T
	out    io.Writer
	outMu  sync.Mutex
	mu     sync.Mutex
	calls  []cdpMessage
	exited chan struct{}
	once   sync.Once
	// silent leaves a started screencast without frames, as a page that never repaints does.
	silent bool
}

func newFakeBrowser(t *testing.T) (*browserLink, *fakeBrowser) {
	t.Helper()
	toBrowserR, toBrowserW := io.Pipe()
	fromBrowserR, fromBrowserW := io.Pipe()
	fake := &fakeBrowser{t: t, out: fromBrowserW, exited: make(chan struct{})}
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
	link := &browserLink{r: fromBrowserR, w: toBrowserW, exited: fake.exited, stop: fake.exit}
	t.Cleanup(fake.exit)
	return link, fake
}

func (f *fakeBrowser) exit() {
	f.once.Do(func() { close(f.exited) })
}

func (f *fakeBrowser) send(message any) {
	encoded, _ := json.Marshal(message)
	f.outMu.Lock()
	defer f.outMu.Unlock()
	_, _ = f.out.Write(append(encoded, 0))
}

func (f *fakeBrowser) event(sessionID, method string, params any) {
	f.send(map[string]any{"method": method, "params": params, "sessionId": sessionID})
}

func (f *fakeBrowser) answer(request cdpMessage) {
	result := map[string]any{}
	switch request.Method {
	case "Target.getTargets":
		result = map[string]any{"targetInfos": []targetInfo{{TargetID: "T1", Type: "page", URL: "about:blank", Title: "blank"}}}
	case "Target.attachToTarget":
		result = map[string]any{"sessionId": "PAGE"}
	case "Page.captureScreenshot":
		result = map[string]any{"data": fakeJPEG}
	case "Page.getLayoutMetrics":
		result = map[string]any{"cssLayoutViewport": map[string]any{"clientWidth": 1280, "clientHeight": 800}}
	case "Page.navigate":
		var params struct{ URL string }
		_ = json.Unmarshal(request.Params, &params)
		defer f.event("", "Target.targetInfoChanged", map[string]any{"targetInfo": targetInfo{TargetID: "T1", Type: "page", URL: params.URL, Title: "Loaded"}})
	}
	f.send(map[string]any{"id": request.ID, "result": result})
	if request.Method == "Page.startScreencast" && !f.silent {
		f.frame(1)
	}
}

func (f *fakeBrowser) frame(ack int64) {
	f.event("PAGE", "Page.screencastFrame", map[string]any{
		"data":      fakeJPEG,
		"metadata":  map[string]any{"offsetTop": 0, "pageScaleFactor": 1, "deviceWidth": 1280, "deviceHeight": 800, "scrollOffsetX": 0, "scrollOffsetY": 0, "timestamp": 1.5},
		"sessionId": ack,
	})
}

func (f *fakeBrowser) called(method string) []cdpMessage {
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

func (f *fakeBrowser) waitFor(method string, count int) []cdpMessage {
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

// untilFrame reads until a frame the test accepts, failing on any refusal.
func (c wsClient) untilFrame(label string, accept func(frame) bool) frame {
	c.t.Helper()
	for {
		message := c.nextAny()
		if accept(message) {
			return message
		}
		if message.Type == "error" {
			c.t.Fatalf("waiting for %s, got a refusal: %+v", label, message)
		}
	}
}

func (c wsClient) untilState(state string, accept func(frame) bool) frame {
	c.t.Helper()
	return c.untilFrame("browser_state "+state, func(m frame) bool {
		return m.Type == "browser_state" && m.State == state && (accept == nil || accept(m))
	})
}

func (c wsClient) refusal(id string) frame {
	c.t.Helper()
	for {
		message := c.nextAny()
		if message.Type == "error" && message.ID == id {
			return message
		}
	}
}

// browserDaemon is a websocket daemon with one session and a fake browser behind it.
func browserDaemon(t *testing.T) (*daemon, string, *fakeBrowser) {
	t.Helper()
	d, address := wsDaemon(t)
	wsSession(t, d, false)
	d.peerLookup = func(net.Addr, net.Addr) ([]int, error) { return []int{os.Getpid()}, nil }
	link, fake := newFakeBrowser(t)
	d.browserLaunch = func(string) (*browserLink, error) { return link, nil }
	return d, address, fake
}

func TestBrowserWatchStartsTheBrowserAndStreamsFrames(t *testing.T) {
	_, address, fake := browserDaemon(t)
	client, welcome := dialLoopbackClient(t, address)
	if !slices.Contains(welcome.Features, browserFeature) {
		t.Fatalf("welcome should carry %s: %v", browserFeature, welcome.Features)
	}
	client.send(frame{Type: "browser_watch", ID: "w1", Session: "scientist-evie", Width: 390, Height: 600})
	first := client.untilState("none", nil)
	if first.ID != "w1" || first.Client == "" {
		t.Fatalf("the first state should answer the request and name the client: %+v", first)
	}
	live := client.untilState("live", nil)
	if live.Driver != "agent" || live.URL != "about:blank" || live.Client != first.Client {
		t.Fatalf("a fresh browser is the agent's: %+v", live)
	}
	shot := client.untilFrame("browser_frame", func(m frame) bool { return m.Type == "browser_frame" })
	if !bytes.Equal(shot.Data, fakeJPEG) || shot.Seq != 1 || shot.Session != "scientist-evie" {
		t.Fatalf("frame = %+v", shot)
	}
	if !strings.Contains(string(shot.Metadata), `"device_width":1280`) || !strings.Contains(string(shot.Metadata), `"offset_top":0`) {
		t.Fatalf("metadata should be snake_case: %s", shot.Metadata)
	}
	fake.waitFor("Page.screencastFrameAck", 1)
	cast := fake.waitFor("Page.startScreencast", 1)[0]
	if !strings.Contains(string(cast.Params), `"maxWidth":390`) || !strings.Contains(string(cast.Params), `"maxHeight":600`) {
		t.Fatalf("the pane size should bound the screencast: %s", cast.Params)
	}
}

func TestBrowserTakeThenNavigateChangesThePageAndWatchersSeeIt(t *testing.T) {
	_, address, fake := browserDaemon(t)
	person, _ := dialLoopbackClient(t, address)
	watcher, _ := dialLoopbackClient(t, address)
	person.send(frame{Type: "browser_watch", Session: "scientist-evie"})
	watcher.send(frame{Type: "browser_watch", Session: "scientist-evie"})
	live := person.untilState("live", nil)
	watcher.untilState("live", nil)

	person.send(frame{Type: "browser_navigate", ID: "n0", Session: "scientist-evie", URL: "https://example.com/"})
	if refused := person.refusal("n0"); refused.Reason != reasonNotDriver {
		t.Fatalf("navigating before taking control must be refused as %s: %+v", reasonNotDriver, refused)
	}
	person.send(frame{Type: "browser_input", ID: "i0", Session: "scientist-evie", Kind: "key", Params: json.RawMessage(`{"type":"keyDown","key":"a"}`)})
	if refused := person.refusal("i0"); refused.Reason != reasonNotDriver {
		t.Fatalf("input before taking control must be refused: %+v", refused)
	}

	person.send(frame{Type: "browser_control", Session: "scientist-evie", Take: true})
	taken := watcher.untilState("live", func(m frame) bool { return m.Driver == "person" })
	if taken.Holder == "" || taken.Holder == taken.Client || taken.Holder != live.Client {
		t.Fatalf("the holder is the person's client, not the watcher's: %+v", taken)
	}
	person.send(frame{Type: "browser_navigate", Session: "scientist-evie", URL: "https://example.com/"})
	moved := watcher.untilState("live", func(m frame) bool { return m.URL == "https://example.com/" })
	if moved.Title != "Loaded" {
		t.Fatalf("the watcher should see the new page: %+v", moved)
	}
	if calls := fake.waitFor("Page.navigate", 1); !strings.Contains(string(calls[0].Params), "https://example.com/") || calls[0].SessionID != "PAGE" {
		t.Fatalf("navigate reached the page wrongly: %+v", calls[0])
	}

	person.send(frame{Type: "browser_input", Session: "scientist-evie", Kind: "mouse", Params: json.RawMessage(`{"type":"mousePressed","x":10,"y":20,"button":"left","clickCount":1}`)})
	person.send(frame{Type: "browser_input", Session: "scientist-evie", Kind: "text", Params: json.RawMessage(`{"text":"hello"}`)})
	if mouse := fake.waitFor("Input.dispatchMouseEvent", 1)[0]; !strings.Contains(string(mouse.Params), `"x":10`) {
		t.Fatalf("mouse params should pass through: %s", mouse.Params)
	}
	fake.waitFor("Input.insertText", 1)

	person.send(frame{Type: "browser_navigate", ID: "n1", Session: "scientist-evie", URL: "file:///etc/passwd"})
	if refused := person.refusal("n1"); !strings.Contains(refused.Error, "http or https") {
		t.Fatalf("a file: address must be refused: %+v", refused)
	}
	person.send(frame{Type: "browser_input", ID: "i1", Session: "scientist-evie", Kind: "keys", Params: json.RawMessage(`{}`)})
	if refused := person.refusal("i1"); !strings.Contains(refused.Error, "kind") {
		t.Fatalf("an unknown input kind must be refused: %+v", refused)
	}
}

func TestBrowserControlRefusesASecondScreenUnlessForced(t *testing.T) {
	_, address, _ := browserDaemon(t)
	first, _ := dialLoopbackClient(t, address)
	second, _ := dialLoopbackClient(t, address)
	first.send(frame{Type: "browser_watch", Session: "scientist-evie"})
	second.send(frame{Type: "browser_watch", Session: "scientist-evie"})
	first.untilState("live", nil)
	second.untilState("live", nil)
	first.send(frame{Type: "browser_control", Session: "scientist-evie", Take: true})
	second.untilState("live", func(m frame) bool { return m.Driver == "person" })

	second.send(frame{Type: "browser_control", ID: "c2", Session: "scientist-evie", Take: true})
	if refused := second.refusal("c2"); refused.Reason != reasonBrowserHeld {
		t.Fatalf("a take while another screen holds must be refused as %s: %+v", reasonBrowserHeld, refused)
	}
	second.send(frame{Type: "browser_control", Session: "scientist-evie", Take: true, Force: true})
	forced := second.untilState("live", func(m frame) bool { return m.Holder == m.Client })
	if forced.Driver != "person" {
		t.Fatalf("force should move control: %+v", forced)
	}
	first.send(frame{Type: "browser_input", ID: "i", Session: "scientist-evie", Kind: "text", Params: json.RawMessage(`{"text":"x"}`)})
	if refused := first.refusal("i"); refused.Reason != reasonNotDriver {
		t.Fatalf("the screen that lost control must not type: %+v", refused)
	}
	first.send(frame{Type: "browser_control", ID: "h", Session: "scientist-evie", Take: false})
	if refused := first.refusal("h"); refused.Reason != reasonBrowserHeld {
		t.Fatalf("only the holder hands control back: %+v", refused)
	}
}

func TestBrowserHoldingScreenLeavingHandsControlBack(t *testing.T) {
	_, address, _ := browserDaemon(t)
	leaver, _ := dialLoopbackClient(t, address)
	stayer, _ := dialLoopbackClient(t, address)
	leaver.send(frame{Type: "browser_watch", Session: "scientist-evie"})
	stayer.send(frame{Type: "browser_watch", Session: "scientist-evie"})
	leaver.untilState("live", nil)
	stayer.untilState("live", nil)
	leaver.send(frame{Type: "browser_control", Session: "scientist-evie", Take: true})
	stayer.untilState("live", func(m frame) bool { return m.Driver == "person" })
	_ = leaver.ws.CloseNow()
	back := stayer.untilState("live", func(m frame) bool { return m.Driver == "agent" })
	if back.Holder != "" {
		t.Fatalf("nobody holds a browser whose holder left: %+v", back)
	}
}

func TestBrowserControlIsRefusedToAProcessInsideASession(t *testing.T) {
	d, address, _ := browserDaemon(t)
	wsSession2 := d.session("scientist-evie")
	d.processes = func() ([]processEntry, error) {
		return []processEntry{{PID: wsSession2.pid, PPID: 1}, {PID: os.Getpid(), PPID: wsSession2.pid}}, nil
	}
	client, _ := dialLoopbackClient(t, address)
	client.send(frame{Type: "browser_watch", Session: "scientist-evie"})
	client.untilState("live", nil)
	client.send(frame{Type: "browser_control", ID: "c", Session: "scientist-evie", Take: true})
	if refused := client.refusal("c"); refused.Reason != reasonSessionDescendant {
		t.Fatalf("an agent's browser may watch but not take: %+v", refused)
	}
}

func TestBrowserDrivingNeedsAWatchFirst(t *testing.T) {
	_, address, _ := browserDaemon(t)
	client, _ := dialLoopbackClient(t, address)
	client.send(frame{Type: "browser_control", ID: "c", Session: "scientist-evie", Take: true})
	if refused := client.refusal("c"); refused.Reason != reasonNotWatching {
		t.Fatalf("control without a watch must be refused: %+v", refused)
	}
	client.send(frame{Type: "browser_watch", ID: "w", Session: "no-such-session"})
	if refused := client.refusal("w"); !strings.Contains(refused.Error, "no live session") {
		t.Fatalf("watching a session that does not exist: %+v", refused)
	}
}

func TestBrowserStartFailureIsStateNoneWithAReason(t *testing.T) {
	d, address := wsDaemon(t)
	wsSession(t, d, false)
	d.browserLaunch = func(string) (*browserLink, error) { return nil, io.ErrClosedPipe }
	client, _ := dialLoopbackClient(t, address)
	client.send(frame{Type: "browser_watch", Session: "scientist-evie"})
	failed := client.untilState("none", func(m frame) bool { return strings.Contains(m.Reason, "did not start") })
	if failed.URL != "" {
		t.Fatalf("no browser, no page: %+v", failed)
	}
}

func TestBrowserEndsWithItsSessionAndWithTheProcess(t *testing.T) {
	d, address, fake := browserDaemon(t)
	client, _ := dialLoopbackClient(t, address)
	client.send(frame{Type: "browser_watch", Session: "scientist-evie"})
	client.untilState("live", nil)
	fake.exit()
	closed := client.untilState("closed", nil)
	if closed.Driver != "agent" || !strings.Contains(closed.Reason, "exited") {
		t.Fatalf("a dead process closes the browser: %+v", closed)
	}
	d.browsers.end("scientist-evie")
	if d.browsers.existing("scientist-evie") != nil {
		t.Fatal("an ended session keeps no browser")
	}
}

func TestWatcherKeepsOnlyTheNewestFrame(t *testing.T) {
	w := newWatcher(&conn{}, "screen")
	for seq := int64(1); seq <= 500; seq++ {
		w.post(nil, &frame{Type: "browser_frame", Seq: seq})
	}
	w.post(&frame{Type: "browser_state", State: "live"}, nil)
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.shot.Seq != 500 || w.state.State != "live" {
		t.Fatalf("a slow client should miss frames, not queue them: shot %+v state %+v", w.shot, w.state)
	}
}

func TestCDPClientMatchesRepliesAndFailsPendingCallsWhenTheStreamEnds(t *testing.T) {
	toR, toW := io.Pipe()
	fromR, fromW := io.Pipe()
	events := make(chan string, 4)
	client := newCDPClient(fromR, toW, func(session, method string, _ json.RawMessage) { events <- session + ":" + method })
	go func() {
		reader := bufio.NewReader(toR)
		line, _ := reader.ReadBytes(0)
		var request cdpMessage
		_ = json.Unmarshal(line[:len(line)-1], &request)
		_, _ = fromW.Write(append([]byte(`{"method":"Page.loadEventFired","sessionId":"S"}`), 0))
		_, _ = fromW.Write(append([]byte(`{"id":`+string(rune('0'+request.ID))+`,"result":{"ok":true}}`), 0))
		// The browser reads on, so a later call's write returns and it waits for a reply.
		_, _ = io.Copy(io.Discard, toR)
	}()
	result, err := client.call(context.Background(), "S", "Page.enable", nil)
	if err != nil || !strings.Contains(string(result), "ok") {
		t.Fatalf("call = %s, %v", result, err)
	}
	if got := <-events; got != "S:Page.loadEventFired" {
		t.Fatalf("event = %q", got)
	}
	pending := make(chan error, 1)
	go func() {
		_, err := client.call(context.Background(), "", "Never.answered", nil)
		pending <- err
	}()
	time.Sleep(50 * time.Millisecond)
	_ = fromW.Close()
	select {
	case err := <-pending:
		if err == nil {
			t.Fatal("a call pending when the stream ends must fail")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a call pending when the stream ends never returned")
	}
}

// A real Chromium over the pipe, skipped where this host has none.
func TestRealChromiumStreamsAScreencastFrame(t *testing.T) {
	if chromiumPath() == "" {
		t.Skip("no Chromium or Chrome on this host")
	}
	dir := testHoldDir(t)
	link, err := launchChromium(dir, "probe")
	if err != nil {
		t.Fatalf("launch: %v", err)
	}
	t.Cleanup(link.stop)
	frames := make(chan []byte, 1)
	var page string
	var pageMu sync.Mutex
	var cdp *cdpClient
	cdp = newCDPClient(link.r, link.w, func(session, method string, params json.RawMessage) {
		if method != "Page.screencastFrame" {
			return
		}
		var shot struct {
			Data      []byte `json:"data"`
			SessionID int64  `json:"sessionId"`
		}
		_ = json.Unmarshal(params, &shot)
		pageMu.Lock()
		current := page
		pageMu.Unlock()
		cdp.notify(current, "Page.screencastFrameAck", map[string]any{"sessionId": shot.SessionID})
		select {
		case frames <- shot.Data:
		default:
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, attached, err := attachPage(ctx, cdp)
	if err != nil {
		t.Fatalf("attach: %v", err)
	}
	pageMu.Lock()
	page = attached
	pageMu.Unlock()
	if _, err := cdp.call(ctx, attached, "Page.startScreencast", map[string]any{"format": "jpeg", "quality": 60}); err != nil {
		t.Fatalf("screencast: %v", err)
	}
	// Navigating while casting swaps the renderer, which the stream has to survive.
	if _, err := cdp.call(ctx, attached, "Page.navigate", map[string]any{"url": "data:text/html,<h1>aterm</h1>"}); err != nil {
		t.Fatalf("navigate: %v", err)
	}
	select {
	case data := <-frames:
		if len(data) < 4 || data[0] != 0xff || data[1] != 0xd8 {
			t.Fatalf("not a JPEG: % x", data[:min(8, len(data))])
		}
	case <-ctx.Done():
		t.Fatal("no screencast frame from a real Chromium")
	}
}

func TestBrowserReplaysTheNewestFrameToAScreenThatStartsWatchingLate(t *testing.T) {
	_, address, _ := browserDaemon(t)
	early, _ := dialLoopbackClient(t, address)
	early.send(frame{Type: "browser_watch", Session: "scientist-evie"})
	early.untilFrame("browser_frame", func(m frame) bool { return m.Type == "browser_frame" })
	late, _ := dialLoopbackClient(t, address)
	late.send(frame{Type: "browser_watch", Session: "scientist-evie"})
	shot := late.untilFrame("replayed browser_frame", func(m frame) bool { return m.Type == "browser_frame" })
	if !bytes.Equal(shot.Data, fakeJPEG) || shot.Seq != 1 {
		t.Fatalf("a page that is not repainting still shows a late screen its last frame: %+v", shot)
	}
}

func TestBrowserSeedsOneScreenshotWhenTheScreencastStaysSilent(t *testing.T) {
	d, address, fake := browserDaemon(t)
	fake.silent = true
	client, _ := dialLoopbackClient(t, address)
	client.send(frame{Type: "browser_watch", Session: "scientist-evie"})
	shot := client.untilFrame("seeded browser_frame", func(m frame) bool { return m.Type == "browser_frame" })
	if shot.Seq != 1 || !bytes.Equal(shot.Data, fakeJPEG) || !strings.Contains(string(shot.Metadata), `"device_width":1280`) {
		t.Fatalf("a silent cast should be seeded from a screenshot: %+v %s", shot, shot.Metadata)
	}
	if d.browsers.existing("scientist-evie").seq != 1 {
		t.Fatal("one seed, not a stream of them")
	}
}

func TestBrowserControlFromATailnetDeviceNeedsItsPasskey(t *testing.T) {
	d := newDaemon(func(string, ...any) {})
	c := &conn{}
	cl := &client{c: c, peerStanding: peerStanding{web: true, remote: true}}
	sb := d.browsers.sharedFor(d, "scientist-evie")
	sb.state = "live"
	sb.watchers[c] = newWatcher(c, d.browsers.clientName(c))
	err := d.browserControl(cl, frame{Type: "browser_control", Session: "scientist-evie", Take: true})
	if reasonFor(err) != reasonPasskeyRequired || sb.driver != "agent" {
		t.Fatalf("a device with no assertion must not take the browser: %v, driver %s", err, sb.driver)
	}
	cl.asserted = true
	if err := d.browserControl(cl, frame{Type: "browser_control", Session: "scientist-evie", Take: true}); err != nil || sb.driver != "person" {
		t.Fatalf("an asserted device may take it: %v, driver %s", err, sb.driver)
	}
}
