package main

import (
	"errors"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"
)

const (
	// reachPath answers an admitted device 200, so a hosted page can tell a
	// refusing daemon from a silent one. See the tooling-aterm-client skill, reach.md.
	reachPath     = "/_aterm/reach"
	refusalHeader = "X-Aterm-Refusal"
)

// refusalEvery is how long one peer's repeated refusal is logged once, since a
// phone's client retries on a backoff.
var refusalEvery = 5 * time.Minute

// refusal is why a listener turned a request away, and which layer did.
type refusal struct{ layer, reason string }

func (r *refusal) Error() string { return r.reason }

// layerOf names the layer behind an admit error. A failed whois lands on ownership.
func layerOf(err error) string {
	var named *refusal
	if errors.As(err, &named) {
		return named.layer
	}
	return "ownership"
}

// refusalLog remembers when each refusal was last logged.
type refusalLog struct {
	mu   sync.Mutex
	seen map[string]time.Time
}

func (l *refusalLog) due(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if last, ok := l.seen[key]; ok && now.Sub(last) < refusalEvery {
		return false
	}
	if l.seen == nil {
		l.seen = map[string]time.Time{}
	}
	l.seen[key] = now
	return true
}

// refuse logs a 403 once per peer and layer, then sends it with the layer in a
// header an allowed page can read, since a refused socket tells a browser nothing.
func (d *daemon) refuse(w http.ResponseWriter, r *http.Request, policy accessPolicy, layer, reason string) {
	peer, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		peer = r.RemoteAddr
	}
	if d.refused.due(peer+" "+layer+" "+reason, time.Now()) {
		d.logf("refused %s from %s (origin %q): %s", layer, peer, r.Header.Get("Origin"), reason)
	}
	allowCrossOrigin(w, r, policy)
	w.Header().Set(refusalHeader, layer)
	http.Error(w, "aterm daemon: "+reason, http.StatusForbidden)
}

// allowCrossOrigin lets a page this listener would give a socket read the answer.
func allowCrossOrigin(w http.ResponseWriter, r *http.Request, policy accessPolicy) {
	origin := r.Header.Get("Origin")
	if parsed, err := url.Parse(origin); origin == "" || err != nil || !policy.origin(r, parsed) {
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Access-Control-Expose-Headers", refusalHeader)
	w.Header().Add("Vary", "Origin")
}

func serveReach(w http.ResponseWriter, r *http.Request, policy accessPolicy) {
	allowCrossOrigin(w, r, policy)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"layer":"ok"}`))
}
