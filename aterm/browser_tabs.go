package main

import (
	"context"
	"encoding/json"
	"errors"
)

// pageTab is a page target the browser has open, kept in the order they appeared.
type pageTab struct {
	targetID   string
	session    string
	url, title string
}

// attachTarget attaches to a page flattened, so its commands travel on the one
// pipe under the session id it returns.
func attachTarget(ctx context.Context, cdp *cdpClient, targetID string) (string, error) {
	raw, err := cdp.call(ctx, "", "Target.attachToTarget", map[string]any{"targetId": targetID, "flatten": true})
	if err != nil {
		return "", err
	}
	var attached struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(raw, &attached); err != nil || attached.SessionID == "" {
		return "", errors.New("the page gave no session")
	}
	if _, err := cdp.call(ctx, attached.SessionID, "Page.enable", nil); err != nil {
		return "", err
	}
	return attached.SessionID, nil
}

// tabNumbers is the followed page's place among the open ones, 1-based, and how
// many are open. Call with sb.mu held.
func (sb *sharedBrowser) tabNumbers() (tab, tabs int) {
	if sb.state != "live" {
		return 0, 0
	}
	for i, page := range sb.pages {
		if page.targetID == sb.targetID {
			tab = i + 1
		}
	}
	return tab, len(sb.pages)
}

func (sb *sharedBrowser) tabIndex(targetID string) int {
	for i, page := range sb.pages {
		if page.targetID == targetID {
			return i
		}
	}
	return -1
}

// tabCreated adds a page target and reports whether it was new. Call with sb.mu held.
func (sb *sharedBrowser) tabCreated(info targetInfo) bool {
	if info.Type != "page" || sb.state != "live" || sb.tabIndex(info.TargetID) >= 0 {
		return false
	}
	sb.pages = append(sb.pages, pageTab{targetID: info.TargetID, url: info.URL, title: info.Title})
	sb.broadcast()
	return true
}

// tabChanged keeps a page's url and title, the followed one's going to the pane.
// Call with sb.mu held.
func (sb *sharedBrowser) tabChanged(info targetInfo) {
	i := sb.tabIndex(info.TargetID)
	if i < 0 {
		return
	}
	sb.pages[i].url, sb.pages[i].title = info.URL, info.Title
	if info.TargetID == sb.targetID && (sb.url != info.URL || sb.title != info.Title) {
		sb.url, sb.title = info.URL, info.Title
		sb.broadcast()
	}
}

// tabGone drops a page and reports whether it was the followed one and how many
// remain. Call with sb.mu held.
func (sb *sharedBrowser) tabGone(targetID string) (followed bool, remaining int) {
	i := sb.tabIndex(targetID)
	if i < 0 {
		return false, len(sb.pages)
	}
	sb.pages = append(sb.pages[:i], sb.pages[i+1:]...)
	if targetID != sb.targetID {
		sb.broadcast()
	}
	return targetID == sb.targetID, len(sb.pages)
}

// followNewest points the stream at the newest open page, repeating until it is
// the one followed, since another can open while it attaches.
func (sb *sharedBrowser) followNewest() {
	sb.following.Lock()
	defer sb.following.Unlock()
	for {
		sb.mu.Lock()
		cdp := sb.cdp
		if cdp == nil || sb.state != "live" || len(sb.pages) == 0 {
			sb.mu.Unlock()
			return
		}
		want := sb.pages[len(sb.pages)-1]
		if want.targetID == sb.targetID {
			sb.mu.Unlock()
			return
		}
		sb.mu.Unlock()

		session := want.session
		if session == "" {
			ctx, cancel := context.WithTimeout(context.Background(), browserCallTimeout)
			var err error
			session, err = attachTarget(ctx, cdp, want.targetID)
			cancel()
			if err != nil {
				sb.d.logf("browser for %s: could not follow a new page: %v", sb.session, err)
				return
			}
		}

		sb.mu.Lock()
		i := sb.tabIndex(want.targetID)
		if sb.cdp != cdp || i < 0 {
			sb.mu.Unlock()
			continue
		}
		sb.pages[i].session = session
		previous := sb.page
		sb.targetID, sb.page = want.targetID, session
		sb.url, sb.title = sb.pages[i].url, sb.pages[i].title
		// The old page's last frame would greet a screen that starts watching late.
		sb.last = nil
		sb.broadcast()
		sb.mu.Unlock()

		ctx, cancel := context.WithTimeout(context.Background(), browserCallTimeout)
		_, _ = cdp.call(ctx, previous, "Page.stopScreencast", nil)
		cancel()
		sb.startScreencast(cdp, session)
	}
}

// daemonSession is whether id is a CDP session the daemon attached to a page, the
// streamed one or an earlier one. An agent is never given any of them.
func (sb *sharedBrowser) daemonSession(id string) bool {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	if id == "" {
		return false
	}
	if id == sb.page {
		return true
	}
	for _, page := range sb.pages {
		if page.session == id {
			return true
		}
	}
	return false
}
