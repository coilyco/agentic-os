package main

import "testing"

func TestIsChallengePageReadsCloudflaresInterstitial(t *testing.T) {
	cases := []struct {
		address, title string
		want           bool
	}{
		{"https://claude.ai/oauth/authorize?client_id=x", "Just a moment...", true},
		{"https://example.com/", "Performing security verification", true},
		{"https://example.com/", "Attention Required! | Cloudflare", true},
		{"https://example.com/login?__cf_chl_rt_tk=abc", "Example", true},
		{"https://example.com/cdn-cgi/challenge-platform/h/b/orchestrate", "", true},
		{"https://example.com/", "Example Domain", false},
		{"https://example.com/blog/just-a-moment", "A post", false},
	}
	for _, each := range cases {
		if got := isChallengePage(each.address, each.title); got != each.want {
			t.Errorf("isChallengePage(%q, %q) = %v, want %v", each.address, each.title, got, each.want)
		}
	}
}

func TestBrowserStateSaysWhenTheFollowedPageShowsACloudflareCheck(t *testing.T) {
	_, address, fake := browserDaemon(t)
	client, _ := dialLoopbackClient(t, address)
	client.send(frame{Type: "browser_watch", Session: "scientist-evie"})
	if first := client.untilState("live", nil); first.CloudflareCheck {
		t.Fatalf("a blank page is no challenge: %+v", first)
	}

	// A Turnstile widget is a cross-site iframe target inside the page.
	turnstile := targetInfo{TargetID: "F1", Type: "iframe", URL: "https://challenges.cloudflare.com/cdn-cgi/challenge-platform/h/b/turnstile/if/ov2/av0/rcv/x", ParentFrameID: "T1"}
	fake.event("", "Target.targetCreated", map[string]any{"targetInfo": turnstile})
	client.untilState("live", func(m frame) bool { return m.CloudflareCheck })
	fake.event("", "Target.targetDestroyed", map[string]any{"targetId": "F1"})
	client.untilState("live", func(m frame) bool { return !m.CloudflareCheck })

	// Any other iframe is not one.
	fake.event("", "Target.targetCreated", map[string]any{"targetInfo": targetInfo{TargetID: "F2", Type: "iframe", URL: "https://ads.example/"}})

	// The interstitial is read off the page's own title, which the daemon polls.
	fake.mu.Lock()
	fake.pageTitle = "Just a moment..."
	fake.mu.Unlock()
	if shown := client.untilState("live", func(m frame) bool { return m.Title == "Just a moment..." }); !shown.CloudflareCheck {
		t.Fatalf("the interstitial's title is a challenge: %+v", shown)
	}
}
