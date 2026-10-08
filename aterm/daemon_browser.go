package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sync"
	"time"
)

// browserFeature is how a client knows the daemon streams a session's browser.
const browserFeature = "browser"

// Reasons a browser refusal names, for a client to branch on.
const (
	reasonBrowserHeld  = "browser_held"
	reasonNotDriver    = "not_driver"
	reasonNoBrowser    = "no_browser"
	reasonNotWatching  = "not_watching"
	maxBrowserParams   = 64 << 10
	browserCallTimeout = 5 * time.Second
	// A screencast frame is a JPEG at this quality, which reads text and stays small.
	screencastQuality = 70
	// seedQuiet is how long a silent screencast waits before a screenshot stands in.
	seedQuiet = 600 * time.Millisecond
)

const profileNote = "A temporary profile on the host. Logins from the role's Playwright profile are not here yet."

// browserHub keeps one sharedBrowser per session, started when the first
// client watches it. The zero value is ready.
type browserHub struct {
	mu     sync.Mutex
	byName map[string]*sharedBrowser
	names  map[*conn]string
}

// clientName is what the daemon calls a connection in browser frames, so a
// client can tell that the holder is itself.
func (h *browserHub) clientName(c *conn) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.names == nil {
		h.names = map[*conn]string{}
	}
	if name, ok := h.names[c]; ok {
		return name
	}
	name := "screen-" + randomID(3)
	h.names[c] = name
	return name
}

func (h *browserHub) sharedFor(d *daemon, session string) *sharedBrowser {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.byName == nil {
		h.byName = map[string]*sharedBrowser{}
	}
	sb := h.byName[session]
	if sb == nil {
		sb = &sharedBrowser{d: d, session: session, state: "none", driver: "agent", watchers: map[*conn]*watcher{}}
		h.byName[session] = sb
	}
	return sb
}

func (h *browserHub) existing(session string) *sharedBrowser {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.byName[session]
}

// leave drops a closed connection from every browser it watched, handing back
// any it held.
func (h *browserHub) leave(c *conn) {
	h.mu.Lock()
	all := make([]*sharedBrowser, 0, len(h.byName))
	for _, sb := range h.byName {
		all = append(all, sb)
	}
	delete(h.names, c)
	h.mu.Unlock()
	for _, sb := range all {
		sb.unwatch(c)
	}
}

// end closes the browser of a session that finished.
func (h *browserHub) end(session string) {
	h.mu.Lock()
	sb := h.byName[session]
	delete(h.byName, session)
	h.mu.Unlock()
	if sb != nil {
		sb.shutdown("The session ended.")
	}
}

func (h *browserHub) closeAll() {
	h.mu.Lock()
	all := make([]*sharedBrowser, 0, len(h.byName))
	for _, sb := range h.byName {
		all = append(all, sb)
	}
	h.byName = nil
	h.mu.Unlock()
	for _, sb := range all {
		sb.shutdown("The daemon stopped.")
	}
}

// sharedBrowser is one session's Chromium: its page, who watches, and who drives.
type sharedBrowser struct {
	d       *daemon
	session string

	mu       sync.Mutex
	starting bool
	state    string // none, live, closed
	reason   string
	driver   string // agent, person
	holder   string // the client name that took control
	url      string
	title    string
	seq      int64
	// last is the newest frame, replayed to a screen that starts watching late.
	last     *frame
	width    int
	height   int
	cdp      *cdpClient
	link     *browserLink
	targetID string
	// page is the CDP session id of the page, the one screencast and input go to.
	page     string
	watchers map[*conn]*watcher
}

// watcher is one client's view of a browser. It keeps the newest state and frame only,
// so a slow client misses frames and never stalls the stream.
type watcher struct {
	c    *conn
	name string
	mu   sync.Mutex
	// state and shot wait to be written, newest wins.
	state *frame
	shot  *frame
	kick  chan struct{}
	quit  chan struct{}
	once  sync.Once
}

func newWatcher(c *conn, name string) *watcher {
	return &watcher{c: c, name: name, kick: make(chan struct{}, 1), quit: make(chan struct{})}
}

func (w *watcher) post(state, shot *frame) {
	w.mu.Lock()
	if state != nil {
		// A request's id rides the first state written, even when a newer one replaces it.
		if state.ID == "" && w.state != nil {
			state.ID = w.state.ID
		}
		w.state = state
	}
	if shot != nil {
		w.shot = shot
	}
	w.mu.Unlock()
	select {
	case w.kick <- struct{}{}:
	default:
	}
}

func (w *watcher) stop() { w.once.Do(func() { close(w.quit) }) }

// run writes what waits, state first. A write that fails ends the watcher,
// since the connection is gone or too slow to be waited on.
func (w *watcher) run(onFail func()) {
	for {
		select {
		case <-w.quit:
			return
		case <-w.kick:
		}
		w.mu.Lock()
		state, shot := w.state, w.shot
		w.state, w.shot = nil, nil
		w.mu.Unlock()
		for _, next := range []*frame{state, shot} {
			if next == nil {
				continue
			}
			if err := w.c.write(*next); err != nil {
				onFail()
				return
			}
		}
	}
}

// stateFrame is the snapshot a watcher gets. client is its own name.
func (sb *sharedBrowser) stateFrame(client, id string) *frame {
	return &frame{
		Type: "browser_state", ID: id, Session: sb.session, State: sb.state, Driver: sb.driver,
		Holder: sb.holder, URL: sb.url, Title: sb.title, Reason: sb.reason, Client: client,
	}
}

// broadcast posts the state to every watcher. Call with sb.mu held.
func (sb *sharedBrowser) broadcast() {
	for _, w := range sb.watchers {
		w.post(sb.stateFrame(w.name, ""), nil)
	}
}

func (d *daemon) browserWatch(cl *client, message frame) error {
	if d.session(message.Session) == nil {
		return withExit(exitOffRoster, fmt.Errorf("no live session named %q", message.Session))
	}
	sb := d.browsers.sharedFor(d, message.Session)
	name := d.browsers.clientName(cl.c)
	sb.watch(cl.c, name, message.ID)
	sb.size(message.Width, message.Height)
	sb.ensureStarted()
	return nil
}

func (d *daemon) browserUnwatch(cl *client, message frame) error {
	if sb := d.browsers.existing(message.Session); sb != nil {
		sb.unwatch(cl.c)
	}
	return nil
}

// browserRequest is the watched browser a driving frame names, and the
// connection's name for it.
func (d *daemon) browserRequest(cl *client, session string) (*sharedBrowser, string, error) {
	sb := d.browsers.existing(session)
	if sb == nil || !sb.watchedBy(cl.c) {
		return nil, "", withReason(reasonNotWatching, fmt.Errorf("watch %q's browser before driving it", session))
	}
	return sb, d.browsers.clientName(cl.c), nil
}

func (d *daemon) browserControl(cl *client, message frame) error {
	sb, name, err := d.browserRequest(cl, message.Session)
	if err != nil {
		return err
	}
	if !message.Take {
		return sb.release(name)
	}
	// Taking control is Kai's input, so it takes the typing guard (passkey included).
	if err := d.typingRefusal(cl, "a process inside an aterm session cannot take a browser as Kai"); err != nil {
		return err
	}
	return sb.take(name, message.Force)
}

func (d *daemon) browserDrive(cl *client, message frame) error {
	sb, name, err := d.browserRequest(cl, message.Session)
	if err != nil {
		return err
	}
	if message.Type == "browser_navigate" {
		return sb.navigate(name, message.URL)
	}
	return sb.input(name, message.Kind, message.Params)
}

func (sb *sharedBrowser) watch(c *conn, name, id string) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	if sb.watchers[c] != nil {
		sb.watchers[c].post(sb.stateFrame(name, id), nil)
		return
	}
	w := newWatcher(c, name)
	sb.watchers[c] = w
	go w.run(func() { sb.unwatch(c) })
	w.post(sb.stateFrame(name, id), sb.last)
}

func (sb *sharedBrowser) watchedBy(c *conn) bool {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.watchers[c] != nil
}

func (sb *sharedBrowser) unwatch(c *conn) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	w := sb.watchers[c]
	if w == nil {
		return
	}
	w.stop()
	delete(sb.watchers, c)
	if sb.holder != "" && sb.holder == w.name {
		sb.driver, sb.holder = "agent", ""
		sb.broadcast()
	}
}

// size records the pane's size for the screencast. The page's own viewport is
// left alone, since resizing it would change what the agent sees.
func (sb *sharedBrowser) size(width, height int) {
	if width <= 0 || height <= 0 {
		return
	}
	sb.mu.Lock()
	changed := sb.width != width || sb.height != height
	sb.width, sb.height = width, height
	live := sb.state == "live" && sb.cdp != nil
	cdp, page := sb.cdp, sb.page
	sb.mu.Unlock()
	if changed && live {
		go sb.startScreencast(cdp, page)
	}
}

func (sb *sharedBrowser) startScreencast(cdp *cdpClient, page string) {
	ctx, cancel := context.WithTimeout(context.Background(), browserCallTimeout)
	defer cancel()
	_, _ = cdp.call(ctx, page, "Page.stopScreencast", nil)
	sb.mu.Lock()
	params := map[string]any{"format": "jpeg", "quality": screencastQuality, "everyNthFrame": 1}
	if sb.width > 0 && sb.height > 0 {
		params["maxWidth"], params["maxHeight"] = sb.width, sb.height
	}
	before := sb.seq
	sb.mu.Unlock()
	if _, err := cdp.call(ctx, page, "Page.startScreencast", params); err != nil {
		sb.d.logf("browser for %s: screencast did not start: %v", sb.session, err)
		return
	}
	sb.seedAfter(cdp, page, seedQuiet, before)
}

// ensureStarted starts the browser unless one is live or on its way. A closed
// or failed one starts again, so watching again is how a person retries.
func (sb *sharedBrowser) ensureStarted() {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	if sb.starting || sb.state == "live" {
		return
	}
	sb.starting = true
	sb.state, sb.reason = "none", "Starting the browser on the host."
	sb.broadcast()
	go sb.start()
}

func (sb *sharedBrowser) fail(reason string) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	sb.starting = false
	sb.state, sb.reason = "none", reason
	sb.broadcast()
}

func (sb *sharedBrowser) start() {
	defer sb.d.guard("browser")
	link, err := sb.d.startBrowser(sb.session)
	if err != nil {
		sb.fail("The browser did not start: " + err.Error())
		return
	}
	cdp := newCDPClient(link.r, link.w, sb.onEvent)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	target, page, err := attachPage(ctx, cdp)
	if err != nil {
		link.stop()
		sb.fail("The browser started but its page did not attach: " + err.Error())
		return
	}
	sb.mu.Lock()
	sb.cdp, sb.link = cdp, link
	sb.targetID, sb.page = target.TargetID, page
	sb.url, sb.title = target.URL, target.Title
	sb.starting = false
	sb.state, sb.reason = "live", profileNote
	sb.driver, sb.holder = "agent", ""
	sb.broadcast()
	sb.mu.Unlock()
	sb.startScreencast(cdp, page)
	go func() {
		select {
		case <-link.exited:
		case <-cdp.done:
		}
		sb.closed(cdp, "The browser process exited.")
	}()
}

type targetInfo struct {
	TargetID string `json:"targetId"`
	Type     string `json:"type"`
	Title    string `json:"title"`
	URL      string `json:"url"`
}

// attachPage finds the browser's first page and attaches to it flattened, so
// its commands travel on the one pipe under a session id.
func attachPage(ctx context.Context, cdp *cdpClient) (targetInfo, string, error) {
	if _, err := cdp.call(ctx, "", "Target.setDiscoverTargets", map[string]any{"discover": true}); err != nil {
		return targetInfo{}, "", err
	}
	raw, err := cdp.call(ctx, "", "Target.getTargets", nil)
	if err != nil {
		return targetInfo{}, "", err
	}
	var listed struct {
		TargetInfos []targetInfo `json:"targetInfos"`
	}
	if err := json.Unmarshal(raw, &listed); err != nil {
		return targetInfo{}, "", err
	}
	for _, target := range listed.TargetInfos {
		if target.Type != "page" {
			continue
		}
		raw, err := cdp.call(ctx, "", "Target.attachToTarget", map[string]any{"targetId": target.TargetID, "flatten": true})
		if err != nil {
			return targetInfo{}, "", err
		}
		var attached struct {
			SessionID string `json:"sessionId"`
		}
		if err := json.Unmarshal(raw, &attached); err != nil || attached.SessionID == "" {
			return targetInfo{}, "", errors.New("the page gave no session")
		}
		if _, err := cdp.call(ctx, attached.SessionID, "Page.enable", nil); err != nil {
			return targetInfo{}, "", err
		}
		return target, attached.SessionID, nil
	}
	return targetInfo{}, "", errors.New("the browser has no page")
}

// closed marks the browser gone, unless a newer one has replaced cdp.
func (sb *sharedBrowser) closed(cdp *cdpClient, reason string) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	if sb.cdp != cdp {
		return
	}
	sb.cdp, sb.link = nil, nil
	sb.state, sb.reason = "closed", reason
	sb.driver, sb.holder = "agent", ""
	sb.broadcast()
}

// shutdown ends the browser for good, telling watchers why.
func (sb *sharedBrowser) shutdown(reason string) {
	sb.mu.Lock()
	link := sb.link
	sb.cdp, sb.link = nil, nil
	sb.state, sb.reason = "closed", reason
	sb.driver, sb.holder = "agent", ""
	sb.broadcast()
	watchers := sb.watchers
	sb.watchers = map[*conn]*watcher{}
	sb.mu.Unlock()
	// Each watcher drains its last state before it stops.
	time.AfterFunc(time.Second, func() {
		for _, w := range watchers {
			w.stop()
		}
	})
	if link != nil {
		link.stop()
	}
}

// onEvent runs on the CDP read loop, so it only acks, copies and posts.
func (sb *sharedBrowser) onEvent(sessionID, method string, params json.RawMessage) {
	switch method {
	case "Page.screencastFrame":
		var shot struct {
			Data     []byte `json:"data"`
			Metadata struct {
				OffsetTop       float64 `json:"offsetTop"`
				PageScaleFactor float64 `json:"pageScaleFactor"`
				DeviceWidth     float64 `json:"deviceWidth"`
				DeviceHeight    float64 `json:"deviceHeight"`
				ScrollOffsetX   float64 `json:"scrollOffsetX"`
				ScrollOffsetY   float64 `json:"scrollOffsetY"`
				Timestamp       float64 `json:"timestamp"`
			} `json:"metadata"`
			SessionID int64 `json:"sessionId"`
		}
		if err := json.Unmarshal(params, &shot); err != nil {
			return
		}
		sb.mu.Lock()
		cdp, page := sb.cdp, sb.page
		sb.mu.Unlock()
		if cdp == nil || sessionID != page {
			return
		}
		// Acked here, whatever the clients do, so a slow one never throttles the page.
		cdp.notify(page, "Page.screencastFrameAck", map[string]any{"sessionId": shot.SessionID})
		meta, _ := json.Marshal(map[string]float64{
			"offset_top": shot.Metadata.OffsetTop, "page_scale_factor": shot.Metadata.PageScaleFactor,
			"device_width": shot.Metadata.DeviceWidth, "device_height": shot.Metadata.DeviceHeight,
			"scroll_offset_x": shot.Metadata.ScrollOffsetX, "scroll_offset_y": shot.Metadata.ScrollOffsetY,
			"timestamp": shot.Metadata.Timestamp,
		})
		sb.mu.Lock()
		sb.publish(shot.Data, meta)
		sb.mu.Unlock()
	case "Target.targetInfoChanged":
		var changed struct {
			TargetInfo targetInfo `json:"targetInfo"`
		}
		if err := json.Unmarshal(params, &changed); err != nil {
			return
		}
		sb.mu.Lock()
		defer sb.mu.Unlock()
		if changed.TargetInfo.TargetID != sb.targetID {
			return
		}
		if sb.url != changed.TargetInfo.URL || sb.title != changed.TargetInfo.Title {
			sb.url, sb.title = changed.TargetInfo.URL, changed.TargetInfo.Title
			sb.broadcast()
		}
	case "Target.targetDestroyed":
		var gone struct {
			TargetID string `json:"targetId"`
		}
		if err := json.Unmarshal(params, &gone); err != nil {
			return
		}
		sb.mu.Lock()
		link, cdp, mine := sb.link, sb.cdp, gone.TargetID == sb.targetID
		sb.mu.Unlock()
		if mine && link != nil {
			// A browser with no page is of no use, so end it and let a watch start a new one.
			go func() {
				sb.closed(cdp, "The page closed.")
				link.stop()
			}()
		}
	}
}

func (sb *sharedBrowser) take(name string, force bool) error {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	if sb.state != "live" {
		return withReason(reasonNoBrowser, errors.New("there is no live browser to take"))
	}
	if sb.holder != "" && sb.holder != name && !force {
		return withReason(reasonBrowserHeld, errors.New("another screen has control, take it over with force"))
	}
	sb.driver, sb.holder = "person", name
	sb.broadcast()
	return nil
}

func (sb *sharedBrowser) release(name string) error {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	if sb.holder == "" {
		return nil
	}
	if sb.holder != name {
		return withReason(reasonBrowserHeld, errors.New("another screen has control, only it can hand the browser back"))
	}
	sb.driver, sb.holder = "agent", ""
	sb.broadcast()
	return nil
}

// driving is the CDP client and page when name holds a live browser.
func (sb *sharedBrowser) driving(name string) (*cdpClient, string, error) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	if sb.state != "live" || sb.cdp == nil {
		return nil, "", withReason(reasonNoBrowser, errors.New("there is no live browser"))
	}
	if sb.driver != "person" || sb.holder != name {
		return nil, "", withReason(reasonNotDriver, errors.New("take control of the browser first"))
	}
	return sb.cdp, sb.page, nil
}

var inputMethods = map[string]string{
	"mouse": "Input.dispatchMouseEvent",
	"wheel": "Input.dispatchMouseEvent",
	"key":   "Input.dispatchKeyEvent",
	"text":  "Input.insertText",
}

func (sb *sharedBrowser) input(name, kind string, params json.RawMessage) error {
	method, known := inputMethods[kind]
	if !known {
		return fmt.Errorf("browser_input kind %q is not mouse, wheel, key or text", kind)
	}
	var object map[string]json.RawMessage
	if len(params) > maxBrowserParams || json.Unmarshal(params, &object) != nil || object == nil {
		return errors.New("browser_input params must be a JSON object under 64 KiB")
	}
	cdp, page, err := sb.driving(name)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), browserCallTimeout)
	defer cancel()
	_, err = cdp.call(ctx, page, method, params)
	return err
}

// navigate is limited to pages a person would open, since file: and the like
// would read the host's disk into a frame any watcher gets.
func (sb *sharedBrowser) navigate(name, address string) error {
	parsed, err := url.Parse(address)
	if err != nil || !(parsed.Scheme == "http" || parsed.Scheme == "https" || address == "about:blank") {
		return errors.New("browser_navigate takes an http or https address, or about:blank")
	}
	cdp, page, err := sb.driving(name)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), browserCallTimeout)
	defer cancel()
	raw, err := cdp.call(ctx, page, "Page.navigate", map[string]any{"url": address})
	if err != nil {
		return err
	}
	var result struct {
		ErrorText string `json:"errorText"`
	}
	if json.Unmarshal(raw, &result) == nil && result.ErrorText != "" {
		return fmt.Errorf("the page did not load: %s", result.ErrorText)
	}
	return nil
}

// publish numbers a frame and hands it to every watcher. Call with sb.mu held.
func (sb *sharedBrowser) publish(jpeg []byte, metadata json.RawMessage) {
	sb.seq++
	sb.last = &frame{Type: "browser_frame", Session: sb.session, Data: jpeg, Metadata: metadata, Seq: sb.seq}
	for _, w := range sb.watchers {
		w.post(nil, sb.last)
	}
}

// seedAfter sends one screenshot as a frame when the screencast stays silent,
// since Chromium casts only what repaints and a still page may never.
func (sb *sharedBrowser) seedAfter(cdp *cdpClient, page string, quiet time.Duration, before int64) {
	time.AfterFunc(quiet, func() {
		sb.mu.Lock()
		stale := sb.seq != before || sb.cdp != cdp
		sb.mu.Unlock()
		if stale {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), browserCallTimeout)
		defer cancel()
		raw, err := cdp.call(ctx, page, "Page.captureScreenshot", map[string]any{"format": "jpeg", "quality": screencastQuality})
		if err != nil {
			return
		}
		var shot struct {
			Data []byte `json:"data"`
		}
		layout, err := cdp.call(ctx, page, "Page.getLayoutMetrics", nil)
		if err != nil || json.Unmarshal(raw, &shot) != nil {
			return
		}
		var metrics struct {
			Viewport struct {
				Width  float64 `json:"clientWidth"`
				Height float64 `json:"clientHeight"`
			} `json:"cssLayoutViewport"`
		}
		_ = json.Unmarshal(layout, &metrics)
		meta, _ := json.Marshal(map[string]float64{
			"offset_top": 0, "page_scale_factor": 1, "scroll_offset_x": 0, "scroll_offset_y": 0,
			"device_width": metrics.Viewport.Width, "device_height": metrics.Viewport.Height,
		})
		sb.mu.Lock()
		defer sb.mu.Unlock()
		if sb.seq == before && sb.cdp == cdp {
			sb.publish(shot.Data, meta)
		}
	})
}
