package main

import (
	"net/url"
	"strings"
)

// A Cloudflare check never passes under CDP, a person's click through the pane included,
// so the pane offers the device's own browser instead (COI-3803, browser.md).

// challengeTitles open the title of Cloudflare's interstitial, compared lowercased.
var challengeTitles = []string{"just a moment", "attention required! | cloudflare", "performing security verification"}

// challengeFrameHost serves the Turnstile widget, in its own cross-site iframe.
const challengeFrameHost = "challenges.cloudflare.com"

// isChallengePage reads Cloudflare's interstitial off a page's address and title.
func isChallengePage(address, title string) bool {
	lowered := strings.ToLower(strings.TrimSpace(title))
	for _, each := range challengeTitles {
		if strings.HasPrefix(lowered, each) {
			return true
		}
	}
	parsed, err := url.Parse(address)
	if err != nil {
		return false
	}
	return strings.Contains(parsed.RawQuery, "__cf_chl_") || strings.HasPrefix(parsed.Path, "/cdn-cgi/challenge-platform/")
}

func isChallengeFrame(info targetInfo) bool {
	parsed, err := url.Parse(info.URL)
	return info.Type == "iframe" && err == nil && parsed.Hostname() == challengeFrameHost
}

// iframeChanged keeps the Turnstile iframes the browser has open, by target, with the
// frame each sits in. Call with sb.mu held.
func (sb *sharedBrowser) iframeChanged(info targetInfo) {
	before := sb.challenged()
	if isChallengeFrame(info) {
		if sb.challengeFrames == nil {
			sb.challengeFrames = map[string]string{}
		}
		sb.challengeFrames[info.TargetID] = info.ParentFrameID
	} else {
		delete(sb.challengeFrames, info.TargetID)
	}
	if sb.challenged() != before {
		sb.broadcast()
	}
}

// iframeGone drops a closed iframe. Call with sb.mu held.
func (sb *sharedBrowser) iframeGone(targetID string) {
	if _, ok := sb.challengeFrames[targetID]; !ok {
		return
	}
	before := sb.challenged()
	delete(sb.challengeFrames, targetID)
	if sb.challenged() != before {
		sb.broadcast()
	}
}

// challenged is whether the followed page shows a check. An iframe counts unless its
// parent is another open page, since a nested one names an inner frame. Hold sb.mu.
func (sb *sharedBrowser) challenged() bool {
	if sb.state != "live" {
		return false
	}
	if isChallengePage(sb.url, sb.title) {
		return true
	}
	for _, parent := range sb.challengeFrames {
		if parent == "" || parent == sb.targetID || sb.tabIndex(parent) < 0 {
			return true
		}
	}
	return false
}
