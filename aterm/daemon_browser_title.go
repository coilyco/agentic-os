package main

import (
	"context"
	"encoding/json"
	"time"
)

// titlePollEvery is how often a watched browser is asked for its page's title.
// Chromium sends no targetInfoChanged for a title-only change (COI-2556).
const titlePollEvery = 500 * time.Millisecond

// followTitle polls the followed page's target info while anyone watches, until
// cdp ends. It runs off the CDP read loop, since a call waits on that loop.
func (sb *sharedBrowser) followTitle(cdp *cdpClient) {
	defer sb.d.guard("browser title")
	tick := time.NewTicker(titlePollEvery)
	defer tick.Stop()
	for {
		select {
		case <-cdp.done:
			return
		case <-tick.C:
		}
		sb.mu.Lock()
		target, watched := sb.targetID, len(sb.watchers) > 0
		sb.mu.Unlock()
		if !watched || target == "" {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), browserCallTimeout)
		raw, err := cdp.call(ctx, "", "Target.getTargetInfo", map[string]any{"targetId": target})
		cancel()
		var got struct {
			TargetInfo targetInfo `json:"targetInfo"`
		}
		if err != nil || json.Unmarshal(raw, &got) != nil {
			continue
		}
		sb.mu.Lock()
		if sb.cdp == cdp && sb.targetID == target && got.TargetInfo.TargetID == target && (sb.url != got.TargetInfo.URL || sb.title != got.TargetInfo.Title) {
			sb.url, sb.title = got.TargetInfo.URL, got.TargetInfo.Title
			sb.broadcast()
		}
		sb.mu.Unlock()
	}
}
