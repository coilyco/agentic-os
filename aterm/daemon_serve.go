package main

import (
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"time"
)

// A Serve front reaches the loopback listener through the proxy's own socket.
// See .agents/skills/tooling-aterm-client/references/tailnet-access.md.

// forwarded reports a request through a reverse proxy, by any marker header. A browser
// here sends none, and a local process faking one is held to more, never less.
func forwarded(r *http.Request) bool {
	for name := range r.Header {
		lower := strings.ToLower(name)
		if strings.HasPrefix(lower, "x-forwarded-") || strings.HasPrefix(lower, "tailscale-") || lower == "forwarded" || lower == "x-real-ip" {
			return true
		}
	}
	return false
}

// nodeReader reads this node from tailscaled when a forwarded request needs it,
// since the loopback listener starts before the tailnet is up.
type nodeReader struct {
	read    func() (tailnetNode, error)
	mu      sync.Mutex
	node    tailnetNode
	expires time.Time
}

func (n *nodeReader) get() (tailnetNode, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if time.Now().Before(n.expires) {
		return n.node, nil
	}
	node, err := n.read()
	if err != nil {
		return tailnetNode{}, err
	}
	n.node, n.expires = node, time.Now().Add(whoisTTL)
	return node, nil
}

// forwardedPolicy admits a proxied request as a direct tailnet peer is admitted, by
// whois on the peer the proxy names. Every doubt refuses, which the doc page lists.
func forwardedPolicy(node *nodeReader, allowTags, allowOwners, allowOrigins []string, whois func(string) (tailnetPeer, error)) accessPolicy {
	admission := &whoisAdmission{whois: whois, allowTags: allowTags}
	return accessPolicy{
		admit: func(r *http.Request) error {
			if r.Header.Get("Tailscale-Funnel-Request") != "" {
				return &refusal{"ownership", "a public Funnel request carries no tailnet identity"}
			}
			self, err := node.get()
			if err != nil {
				return &refusal{"name", "this node's tailnet name is unknown, so a proxied request cannot be judged: " + err.Error()}
			}
			host := r.Host
			if split, _, err := net.SplitHostPort(r.Host); err == nil {
				host = split
			}
			if !strings.EqualFold(host, self.FQDN) {
				return &refusal{"name", "open this daemon by its tailnet name"}
			}
			if forwardedHost := r.Header.Get("X-Forwarded-Host"); forwardedHost != "" && !strings.EqualFold(forwardedHost, r.Host) {
				return &refusal{"name", "the proxy's host does not match the request's"}
			}
			peer, err := netip.ParseAddr(r.Header.Get("X-Forwarded-For"))
			if err != nil || peer.IsLoopback() || peer.IsUnspecified() {
				return &refusal{"ownership", "a proxied request must name one tailnet peer"}
			}
			login, err := admission.judge(peer.String(), admittedOwners(self, allowOwners))
			if err != nil {
				return err
			}
			// Serve strips a client's copy and sets this one from its own whois, so a
			// mismatch means the proxy is not Serve.
			if !strings.EqualFold(r.Header.Get("Tailscale-User-Login"), login) {
				return &refusal{"ownership", "the proxy's identity header disagrees with whois"}
			}
			return nil
		},
		origin: tailnetOrigin(allowOrigins),
	}
}

// loopbackOrForwarded judges a plain local request as loopback and a proxied one
// by tailnet identity. With no forwarded policy set, a proxied request is refused.
func loopbackOrForwarded(forwardedAs accessPolicy) accessPolicy {
	local := loopbackPolicy()
	return accessPolicy{
		admit: func(r *http.Request) error {
			if !forwarded(r) {
				return local.admit(r)
			}
			if forwardedAs.admit == nil {
				return errors.New("this listener has no proxy policy")
			}
			return forwardedAs.admit(r)
		},
		origin: func(r *http.Request, origin *url.URL) bool {
			if !forwarded(r) {
				return local.origin(r, origin)
			}
			return forwardedAs.origin != nil && forwardedAs.origin(r, origin)
		},
	}
}
