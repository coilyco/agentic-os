package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	daemonTailnetPortEnv = "ATERM_DAEMON_TAILNET_PORT"
	defaultTailnetPort   = "7419"
	daemonPeerPortsEnv   = "ATERM_DAEMON_PEER_PORTS"
	daemonAllowTagsEnv   = "ATERM_DAEMON_ALLOW_TAGS"
	daemonAllowOrigins   = "ATERM_DAEMON_ALLOW_ORIGINS"
	clientDirEnv         = "ATERM_CLIENT_DIR"
	// A whois answer is reused this long, so a page load's requests make one call.
	whoisTTL = time.Minute
)

// whoisRefusedTTL keeps a refusal just long enough for one page load to cost one
// whois, so a device tagged a moment ago is admitted on its next load.
var whoisRefusedTTL = 2 * time.Second

// defaultAllowTags mirrors the tailnet's SSH grant between physical devices,
// so admitting them grants nothing SSH does not. See docs/aterm-daemon.md.
var defaultAllowTags = []string{"tag:physical"}

// defaultAllowOrigins are hosted client pages that may open a session socket
// on the tailnet, besides the page the daemon serves itself.
var defaultAllowOrigins = []string{"https://coilyco.dev"}

// appOrigin is the Android app's webview, always allowed beside the configured
// pages, since ATERM_DAEMON_ALLOW_ORIGINS replaces the default.
const appOrigin = "https://tauri.localhost"

var tailscaleBin = "tailscale"

// tailnetGate says whether the tailscale CLI may run without starting the
// tunnel. Platform files set it, and tests replace it.
var tailnetGate = platformTailnetGate

// tailnetBackoff paces listenTailnet retries, capped well above a login's
// settling time so a standing refusal costs one CLI call every few minutes.
var tailnetBackoff = struct{ first, max time.Duration }{2 * time.Second, 5 * time.Minute}

// tailnetProbe paces the self-probe, and `fails` in a row get the listener
// bound again. Why: docs/aterm-daemon.md.
var tailnetProbe = struct {
	every, timeout time.Duration
	fails          int
}{30 * time.Second, 5 * time.Second, 2}

// vpnGate reads the first line of `scutil --nc status <service>`. No service
// means no macsys VPN, so the CLI cannot start one (docs/aterm-daemon.md).
func vpnGate(statusLine string) error {
	switch state := strings.TrimSpace(statusLine); state {
	case "Connected", "No service":
		return nil
	default:
		return fmt.Errorf("the Tailscale VPN is %q, and the tailscale CLI would start it", state)
	}
}

// tailscaleCommand is the only way the daemon builds a tailscale CLI call.
func tailscaleCommand(args ...string) (*exec.Cmd, error) {
	if err := tailnetGate(); err != nil {
		return nil, err
	}
	return exec.Command(tailscaleBin, args...), nil
}

// tailnetNode is this host as the tailnet names it.
type tailnetNode struct {
	FQDN  string
	IPv4  string
	Owner string
}

// tailnetPeer is the part of `tailscale whois` admission reads.
type tailnetPeer struct {
	Node struct {
		Tags []string `json:"Tags"`
	} `json:"Node"`
	UserProfile struct {
		LoginName string `json:"LoginName"`
	} `json:"UserProfile"`
}

func readTailnetNode() (tailnetNode, error) {
	command, err := tailscaleCommand("status", "--json")
	if err != nil {
		return tailnetNode{}, err
	}
	raw, err := command.Output()
	if err != nil {
		return tailnetNode{}, fmt.Errorf("tailscale status: %w", err)
	}
	var status struct {
		BackendState string `json:"BackendState"`
		Self         struct {
			DNSName      string   `json:"DNSName"`
			TailscaleIPs []string `json:"TailscaleIPs"`
			UserID       int64    `json:"UserID"`
			Tags         []string `json:"Tags"`
		} `json:"Self"`
		User map[string]struct {
			LoginName string `json:"LoginName"`
		} `json:"User"`
	}
	if err := json.Unmarshal(raw, &status); err != nil {
		return tailnetNode{}, fmt.Errorf("tailscale status: %w", err)
	}
	node := tailnetNode{FQDN: strings.TrimSuffix(status.Self.DNSName, ".")}
	// A tagged node has no owner. Tailscale lends it a login named after the
	// node, which no person's device carries.
	if len(status.Self.Tags) == 0 {
		node.Owner = status.User[strconv.FormatInt(status.Self.UserID, 10)].LoginName
	}
	for _, ip := range status.Self.TailscaleIPs {
		if parsed := net.ParseIP(ip); parsed != nil && parsed.To4() != nil {
			node.IPv4 = ip
			break
		}
	}
	if status.BackendState != "Running" {
		return tailnetNode{}, fmt.Errorf("the tailscale backend is %q, not Running", status.BackendState)
	}
	if node.FQDN == "" || node.IPv4 == "" || (node.Owner == "" && len(status.Self.Tags) == 0) {
		return tailnetNode{}, errors.New("tailscale reports no name, address, or owner for this node")
	}
	return node, nil
}

func tailscaleWhois(address string) (tailnetPeer, error) {
	command, err := tailscaleCommand("whois", "--json", address)
	if err != nil {
		return tailnetPeer{}, err
	}
	raw, err := command.Output()
	if err != nil {
		return tailnetPeer{}, fmt.Errorf("tailscale whois: %w", err)
	}
	var peer tailnetPeer
	if err := json.Unmarshal(raw, &peer); err != nil {
		return tailnetPeer{}, fmt.Errorf("tailscale whois: %w", err)
	}
	return peer, nil
}

// admitPeer lets in a device of this node's own owner, and a tagged device,
// which has no owner, only when it carries an allowed tag.
func admitPeer(peer tailnetPeer, owner string, allowTags []string) error {
	if len(peer.Node.Tags) == 0 {
		if login := peer.UserProfile.LoginName; login != "" && login == owner {
			return nil
		}
		return errors.New("that device belongs to another tailnet user")
	}
	for _, tag := range peer.Node.Tags {
		if slices.Contains(allowTags, tag) {
			return nil
		}
	}
	return fmt.Errorf("that device carries none of %s", strings.Join(allowTags, ", "))
}

// tailnetPolicy asks tailscaled who each peer is rather than trusting anything
// the request says, and serves a websocket only to the page it served itself.
func tailnetPolicy(node tailnetNode, allowTags, allowOrigins []string, whois func(string) (tailnetPeer, error)) accessPolicy {
	type verdict struct {
		err     error
		expires time.Time
	}
	var mu sync.Mutex
	cache := map[string]verdict{}
	return accessPolicy{
		admit: func(r *http.Request) error {
			if host, _, err := net.SplitHostPort(r.Host); err != nil || !strings.EqualFold(host, node.FQDN) {
				return &refusal{"name", "open this daemon by its tailnet name"}
			}
			ip, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				return err
			}
			mu.Lock()
			cached, ok := cache[ip]
			mu.Unlock()
			if ok && time.Now().Before(cached.expires) {
				return cached.err
			}
			peer, err := whois(r.RemoteAddr)
			if err == nil {
				err = admitPeer(peer, node.Owner, allowTags)
			}
			mu.Lock()
			ttl := whoisTTL
			if err != nil {
				ttl = whoisRefusedTTL
			}
			cache[ip] = verdict{err: err, expires: time.Now().Add(ttl)}
			mu.Unlock()
			return err
		},
		origin: func(r *http.Request, origin *url.URL) bool {
			if origin.Scheme != "https" {
				return false
			}
			site := "https://" + strings.ToLower(origin.Host)
			return strings.EqualFold(origin.Host, r.Host) || slices.Contains(allowOrigins, site) || site == appOrigin
		},
	}
}

// tailnetState is what the Sentry check-in reads: whether a tailnet listener is
// wanted, the one now bound, or why there is none.
type tailnetState struct {
	mu      sync.Mutex
	wanted  bool
	serving *tailnetServing
	reason  string
}

func (s *tailnetState) want() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.wanted = true
}

func (s *tailnetState) set(serving *tailnetServing, reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.serving, s.reason = serving, reason
}

// tailnetHealth is nil when the listener completes a handshake, or none is wanted.
func (d *daemon) tailnetHealth(ctx context.Context) error {
	d.tailnet.mu.Lock()
	wanted, serving, reason := d.tailnet.wanted, d.tailnet.serving, d.tailnet.reason
	d.tailnet.mu.Unlock()
	switch {
	case !wanted:
		return nil
	case serving == nil:
		return fmt.Errorf("no tailnet listener: %s", reason)
	case serving.probe == nil:
		return nil
	}
	return serving.probe(ctx)
}

// tailnetServing is one bound tailnet listener and the two ways to tell it has
// gone bad: Serve returning, and a handshake against it failing.
type tailnetServing struct {
	server *http.Server
	// ended receives what ServeTLS returned.
	ended <-chan error
	// probe completes one TLS handshake against the listener.
	probe func(context.Context) error
}

// probeTailnetTLS dials address and completes a TLS handshake. Its errors name
// the half that failed: no connection, or a connection that cannot be served.
func probeTailnetTLS(ctx context.Context, address string, config *tls.Config) error {
	raw, err := (&net.Dialer{}).DialContext(ctx, "tcp", address)
	if err != nil {
		return fmt.Errorf("no TCP connection to %s: %w", address, err)
	}
	defer raw.Close()
	if err := tls.Client(raw, config).HandshakeContext(ctx); err != nil {
		return fmt.Errorf("TCP connected to %s but the TLS handshake failed: %w", address, err)
	}
	return nil
}

// serveTailnetWhenUp binds with backoff, holds the listener until done closes,
// and binds a fresh one when it goes bad. It never changes Tailscale's state.
func (d *daemon) serveTailnetWhenUp(done <-chan struct{}, listen func() (*tailnetServing, error)) {
	defer d.guard("tailnet")
	delay, last := tailnetBackoff.first, ""
	for {
		serving, err := listen()
		if err == nil {
			d.tailnet.set(serving, "")
			finished := d.holdTailnet(done, serving)
			d.tailnet.set(nil, "rebinding")
			if finished {
				return
			}
			delay, last = tailnetBackoff.first, ""
			continue
		}
		d.tailnet.set(nil, err.Error())
		if reason := err.Error(); reason != last {
			d.logf("no tailnet listener yet, so other devices cannot attach, retrying: %v", err)
			last = reason
		}
		select {
		case <-done:
			return
		case <-time.After(delay):
		}
		delay = min(delay*2, tailnetBackoff.max)
	}
}

// executableGone reports that the file this process runs from no longer exists,
// as after an upgrade removes the old Cellar version. macOS then drops its flows.
var executableGone = func() bool {
	self, err := os.Executable()
	if err != nil {
		return false
	}
	_, err = os.Stat(self)
	return errors.Is(err, fs.ErrNotExist)
}

// errExecutableReplaced is what runDaemon returns after stopping for a new build.
// It is not nil so the exit is non-zero and launchd restarts the daemon.
var errExecutableReplaced = errors.New("the binary this daemon started from was removed, so it stopped for launchd to start the installed one")

// holdTailnet closes the listener when it returns. It reports true when done
// closed, and false when the listener went bad and wants rebinding.
func (d *daemon) holdTailnet(done <-chan struct{}, serving *tailnetServing) bool {
	defer serving.server.Close()
	var tick <-chan time.Time
	if serving.probe != nil {
		ticker := time.NewTicker(tailnetProbe.every)
		defer ticker.Stop()
		tick = ticker.C
	}
	failed := 0
	for {
		select {
		case <-done:
			return true
		case err := <-serving.ended:
			d.logf("the tailnet listener stopped serving, binding a new one: %v", err)
			return false
		case <-tick:
			ctx, cancel := context.WithTimeout(context.Background(), tailnetProbe.timeout)
			err := serving.probe(ctx)
			cancel()
			if err == nil {
				failed = 0
				continue
			}
			if failed++; failed >= tailnetProbe.fails {
				if executableGone() {
					d.logf("the tailnet listener failed %d handshakes and this binary is gone from disk, so macOS drops its flows. Stopping for launchd to start the installed build: %v", failed, err)
					d.stopForReplacement()
					return true
				}
				d.logf("the tailnet listener failed %d handshakes in a row, binding a new one: %v", failed, err)
				return false
			}
		}
	}
}

// stopForReplacement ends the daemon the way a signal does, so the sessions go
// back to their holders for the next daemon to adopt, and marks the exit as a failure.
func (d *daemon) stopForReplacement() {
	d.replaced.Store(true)
	if d.stopServing != nil {
		d.stopServing()
	}
}

// listenTailnet serves HTTPS on this node's tailnet address, certified by
// tailscaled. A failure costs remote clients, never the daemon.
func (d *daemon) listenTailnet(port string, allowTags, allowOrigins []string, certDir string) (*tailnetServing, error) {
	node, err := readTailnetNode()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(certDir, 0o700); err != nil {
		return nil, err
	}
	certFile, keyFile := filepath.Join(certDir, "tls.crt"), filepath.Join(certDir, "tls.key")
	command, err := tailscaleCommand("cert", "--cert-file", certFile, "--key-file", keyFile, node.FQDN)
	if err != nil {
		return nil, err
	}
	if output, err := command.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("tailscale cert: %v: %s", err, strings.TrimSpace(string(output)))
	}
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp", net.JoinHostPort(node.IPv4, port))
	if err != nil {
		return nil, err
	}
	server := &http.Server{
		Handler:   d.handler(tailnetPolicy(node, allowTags, allowOrigins, tailscaleWhois)),
		TLSConfig: &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12},
	}
	ended := make(chan error, 1)
	go func() { ended <- server.ServeTLS(listener, "", "") }()
	who := "its own owner"
	if node.Owner == "" {
		who = "no untagged device, since this node is tagged and has no owner"
	}
	d.logf("serving https on this node's tailnet name, port %s, for %s and %s", port, strings.Join(allowTags, ", "), who)
	address, config := net.JoinHostPort(node.IPv4, port), &tls.Config{ServerName: node.FQDN, MinVersion: tls.VersionTLS12}
	return &tailnetServing{
		server: server,
		ended:  ended,
		probe:  func(ctx context.Context) error { return probeTailnetTLS(ctx, address, config) },
	}, nil
}

// defaultClientDir is ATERM_CLIENT_DIR, empty for the embedded client. No
// installed default: one outlived its binary and served a stale page.
func defaultClientDir() string {
	return strings.TrimSpace(os.Getenv(clientDirEnv))
}
