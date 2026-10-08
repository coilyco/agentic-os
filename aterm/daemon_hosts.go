package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// Discovery answers the `hosts` frame and grants nothing: each daemon still admits
// by `tailscale whois`. Why: .agents/skills/tooling-aterm-client/references/discovery.md.
var hostProbe = struct {
	// ttl reuses an answer, so a page load and its reconnects make one sweep.
	ttl     time.Duration
	timeout time.Duration
	// parallel bounds the dials in flight at once.
	parallel int
}{30 * time.Second, 4 * time.Second, 8}

// tailnetPeerNode is one peer from `tailscale status --json`, the part
// discovery reads.
type tailnetPeerNode struct {
	FQDN string
	OS   string
}

// hostView is one daemon that answered. It names the host and port rather than
// a URL, so the client builds the address the way it does for a typed name.
type hostView struct {
	Name    string `json:"name"`
	Host    string `json:"host"`
	Port    string `json:"port"`
	Version string `json:"version,omitempty"`
}

// parseTailnetPeers keeps online, named macOS and Linux peers. Phones and Windows
// towers never answer, so asking them only costs a timeout.
func parseTailnetPeers(raw []byte) ([]tailnetPeerNode, error) {
	var status struct {
		BackendState string `json:"BackendState"`
		Peer         map[string]struct {
			DNSName string `json:"DNSName"`
			OS      string `json:"OS"`
			Online  bool   `json:"Online"`
		} `json:"Peer"`
	}
	if err := json.Unmarshal(raw, &status); err != nil {
		return nil, fmt.Errorf("tailscale status: %w", err)
	}
	if status.BackendState != "Running" {
		return nil, fmt.Errorf("the tailscale backend is %q, not Running", status.BackendState)
	}
	var peers []tailnetPeerNode
	for _, peer := range status.Peer {
		fqdn := strings.TrimSuffix(peer.DNSName, ".")
		if !peer.Online || fqdn == "" || !slices.Contains([]string{"macos", "linux"}, strings.ToLower(peer.OS)) {
			continue
		}
		peers = append(peers, tailnetPeerNode{FQDN: fqdn, OS: peer.OS})
	}
	slices.SortFunc(peers, func(a, b tailnetPeerNode) int { return strings.Compare(a.FQDN, b.FQDN) })
	return peers, nil
}

// readTailnetPeers is the gated `tailscale status --json` read, like readTailnetNode.
func readTailnetPeers() (self string, peers []tailnetPeerNode, err error) {
	node, err := readTailnetNode()
	if err != nil {
		return "", nil, err
	}
	command, err := tailscaleCommand("status", "--json")
	if err != nil {
		return "", nil, err
	}
	raw, err := command.Output()
	if err != nil {
		return "", nil, fmt.Errorf("tailscale status: %w", err)
	}
	peers, err = parseTailnetPeers(raw)
	return node.FQDN, peers, err
}

// daemonProber dials the websocket this node's served page would open and reads
// the welcome. A nil client verifies the peer's tailscale certificate.
func daemonProber(client *http.Client) func(ctx context.Context, self, pagePort, host, port string) (string, error) {
	return func(ctx context.Context, self, pagePort, host, port string) (string, error) {
		return probeDaemonPeer(ctx, client, self, pagePort, host, port)
	}
}

// probeDaemonPeer dials host on port. The Origin names this node's page on
// pagePort, its own tailnet port, which a peer on another port still allows.
func probeDaemonPeer(ctx context.Context, client *http.Client, self, pagePort, host, port string) (string, error) {
	authority := net.JoinHostPort(host, port)
	ctx, cancel := context.WithTimeout(ctx, hostProbe.timeout)
	defer cancel()
	ws, response, err := websocket.Dial(ctx, "wss://"+authority+"/", &websocket.DialOptions{
		HTTPClient: client,
		// This node's page, not the peer's own, so a peer a browser here could not
		// attach to stays absent.
		HTTPHeader: http.Header{"Origin": []string{"https://" + net.JoinHostPort(self, pagePort)}},
	})
	if err != nil {
		if response != nil {
			return "", fmt.Errorf("%s answered %d", authority, response.StatusCode)
		}
		return "", fmt.Errorf("no daemon at %s: %w", authority, err)
	}
	defer ws.CloseNow()
	ws.SetReadLimit(maxFrame)
	hello, _ := json.Marshal(frame{Type: "hello", Format: daemonFormat})
	if err := ws.Write(ctx, websocket.MessageText, hello); err != nil {
		return "", err
	}
	_, data, err := ws.Read(ctx)
	if err != nil {
		return "", err
	}
	var welcome frame
	if err := json.Unmarshal(data, &welcome); err != nil {
		return "", fmt.Errorf("%s did not speak %s: %w", authority, daemonFormat, err)
	}
	if welcome.Type != "welcome" || welcome.Format != daemonFormat || welcome.Error != "" {
		return "", fmt.Errorf("%s did not speak %s", authority, daemonFormat)
	}
	return welcome.Version, nil
}

// hostDiscovery sweeps the tailnet on demand. Both reads are fields so a test
// supplies a fake status and a fake dial.
type hostDiscovery struct {
	mu    sync.Mutex
	peers func() (self string, peers []tailnetPeerNode, err error)
	probe func(ctx context.Context, self, pagePort, host, port string) (version string, err error)
	// port is this daemon's tailnet port, which a peer uses unless peerPorts names it.
	// Empty is off.
	port func() string
	// peerPorts maps a lowercase peer name, short or full, to the port its daemon
	// listens on. A peer absent from it is dialed on port.
	peerPorts map[string]string
	cached    []hostView
	expires   time.Time
}

func newHostDiscovery() *hostDiscovery {
	return &hostDiscovery{peers: readTailnetPeers, probe: daemonProber(nil), port: func() string { return "" }}
}

// find returns the daemons that answered, never an error for one that did not.
// A tailnet that cannot be read gives the reason, and the client shows no hosts.
func (h *hostDiscovery) find(ctx context.Context) ([]hostView, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if time.Now().Before(h.expires) {
		return h.cached, nil
	}
	port := h.port()
	if port == "" {
		return nil, nil
	}
	self, peers, err := h.peers()
	if err != nil {
		return nil, err
	}
	found := make([]hostView, 0, len(peers))
	var foundMu sync.Mutex
	var group sync.WaitGroup
	slots := make(chan struct{}, hostProbe.parallel)
	for _, peer := range peers {
		if strings.EqualFold(peer.FQDN, self) {
			continue
		}
		group.Add(1)
		slots <- struct{}{}
		go func() {
			defer group.Done()
			defer func() { <-slots }()
			peerPort := h.portFor(peer.FQDN, port)
			version, err := h.probe(ctx, self, port, peer.FQDN, peerPort)
			if err != nil {
				return
			}
			foundMu.Lock()
			defer foundMu.Unlock()
			found = append(found, hostView{Name: shortName(peer.FQDN), Host: peer.FQDN, Port: peerPort, Version: version})
		}()
	}
	group.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	slices.SortFunc(found, func(a, b hostView) int { return strings.Compare(a.Host, b.Host) })
	h.cached, h.expires = found, time.Now().Add(hostProbe.ttl)
	return found, nil
}

func (h *hostDiscovery) portFor(fqdn, fallback string) string {
	for _, key := range []string{strings.ToLower(fqdn), strings.ToLower(shortName(fqdn))} {
		if port, ok := h.peerPorts[key]; ok {
			return port
		}
	}
	return fallback
}

// parsePeerPorts reads `name=port` entries, the name short or full.
func parsePeerPorts(entries []string) (map[string]string, error) {
	ports := map[string]string{}
	for _, entry := range entries {
		name, port, ok := strings.Cut(entry, "=")
		name, port = strings.TrimSpace(name), strings.TrimSpace(port)
		number, err := strconv.Atoi(port)
		if !ok || name == "" || err != nil || number < 1 || number > 65535 {
			return nil, fmt.Errorf("peer port %q should read name=port, with a port from 1 to 65535", entry)
		}
		ports[strings.ToLower(name)] = port
	}
	return ports, nil
}

func shortName(fqdn string) string {
	name, _, _ := strings.Cut(fqdn, ".")
	return name
}

var errNoDiscovery = errors.New("host discovery is off, the tailnet listener has no port")

// hosts answers the frame of the same name.
func (d *daemon) hosts(c *conn, message frame) error {
	if d.discovery == nil {
		return errNoDiscovery
	}
	found, err := d.discovery.find(context.Background())
	if err != nil {
		return fmt.Errorf("could not discover hosts: %w", err)
	}
	return c.write(frame{Type: "hosts", ID: message.ID, Hosts: found})
}
