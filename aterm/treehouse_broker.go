package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	mathrand "math/rand/v2"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// The daemon dials the Treehouse broker and serves each attached client as a channel.
// Shapes are the contract's treehouse.reg.*.v1 and treehouse.chan.*.v1 schemas.

const (
	proofTokenType = "treehouse.proof.daemon.v1"
	brokerMaxChan  = 8
	chanQueueDepth = 64
	// brokerHandshake bounds the challenge, hello and ready exchange.
	brokerHandshake = 15 * time.Second
)

var brokerBackoff = struct{ first, max time.Duration }{time.Second, 30 * time.Second}

// brokerMsg is every message on the broker link, one JSON object per text message.
type brokerMsg struct {
	Type        string          `json:"type"`
	Nonce       string          `json:"nonce,omitempty"`
	Cert        string          `json:"cert,omitempty"`
	Proof       string          `json:"proof,omitempty"`
	Build       string          `json:"build,omitempty"`
	Protocol    string          `json:"protocol,omitempty"`
	Features    []string        `json:"features,omitempty"`
	MaxChannels int             `json:"max_channels,omitempty"`
	DaemonID    string          `json:"daemon_id,omitempty"`
	HeartbeatS  int             `json:"heartbeat_s,omitempty"`
	CertExpires int64           `json:"cert_expires,omitempty"`
	Kind        string          `json:"kind,omitempty"`
	SessionName string          `json:"session,omitempty"`
	Role        string          `json:"role,omitempty"`
	Identity    string          `json:"identity,omitempty"`
	Question    string          `json:"question,omitempty"`
	Channel     string          `json:"channel,omitempty"`
	Account     string          `json:"account,omitempty"`
	SID         string          `json:"sid,omitempty"`
	Frame       json.RawMessage `json:"frame,omitempty"`
	Reason      string          `json:"reason,omitempty"`
}

// brokerLink is the daemon's standing connection to the broker.
type brokerLink struct {
	url      string
	key      *ecdsa.PrivateKey
	identity daemonIdentity
	d        *daemon
	build    string
	dial     func(ctx context.Context, url string) (*websocket.Conn, error)
	// accounts, keys and certPath renew the certificate: the issuer address, its key set,
	// and the file the new certificate replaces.
	accounts string
	keys     *accountsKeys
	certPath string
	post     func(ctx context.Context, path string, body, out any) error
	views    func() []sessionView
	mu       sync.Mutex
	cert     string
	cur      *brokerSession
	// settle and sendEvent replace the wait tracker's delay and the send, for a test.
	settle    time.Duration
	sendEvent func(brokerMsg)
	// settings answers the per-user settings, nil reads every setting as off.
	settings *settingsReader
}

func dialBroker(ctx context.Context, url string) (*websocket.Conn, error) {
	ws, _, err := websocket.Dial(ctx, url, nil)
	return ws, err
}

// run holds the link until ctx ends, redialing with backoff.
func (l *brokerLink) run(ctx context.Context) {
	delay := brokerBackoff.first
	for ctx.Err() == nil {
		if time.Until(l.certExpiry()) < renewBefore {
			if err := l.renew(ctx); err != nil {
				l.d.logf("could not renew the Treehouse daemon certificate: %v", err)
			}
		}
		ready, err := l.session(ctx)
		if ctx.Err() != nil {
			return
		}
		if ready {
			delay = brokerBackoff.first
		}
		l.d.logf("the Treehouse broker link ended: %v", err)
		wait := delay/2 + mathrand.N(delay/2+1)
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		delay = min(delay*2, brokerBackoff.max)
	}
}

// session runs one connection, reporting whether it reached ready.
func (l *brokerLink) session(ctx context.Context) (ready bool, err error) {
	hctx, cancel := context.WithTimeout(ctx, brokerHandshake)
	ws, err := l.dial(hctx, l.url)
	cancel()
	if err != nil {
		return false, err
	}
	ws.SetReadLimit(maxFrame + 4096)
	defer ws.CloseNow()
	session := &brokerSession{link: l, ws: ws, channels: map[string]*brokerChannel{}}
	defer session.closeAll()

	hctx, cancel = context.WithTimeout(ctx, brokerHandshake)
	defer cancel()
	challenge, err := session.read(hctx)
	if err != nil || challenge.Type != "treehouse.reg.challenge.v1" || challenge.Nonce == "" {
		return false, fmt.Errorf("no challenge from the broker: %v", err)
	}
	proof, err := signDaemonProof(l.key, "treehouse-broker", challenge.Nonce, time.Now())
	if err != nil {
		return false, err
	}
	hello := brokerMsg{Type: "treehouse.reg.hello.v1", Cert: l.currentCert(), Proof: proof, Build: l.build, Protocol: daemonFormat, Features: []string{}, MaxChannels: brokerMaxChan}
	if err := session.send(hctx, hello); err != nil {
		return false, err
	}
	ok, err := session.read(hctx)
	if err != nil || ok.Type != "treehouse.reg.ready.v1" || ok.DaemonID != l.identity.ID {
		return false, fmt.Errorf("the broker did not accept this daemon: %v", err)
	}
	cancel()
	l.d.logf("registered with the Treehouse broker as %s", l.identity.ID)
	sctx, end := context.WithCancel(ctx)
	defer end()
	l.mu.Lock()
	l.cur = session
	l.mu.Unlock()
	defer func() {
		l.mu.Lock()
		l.cur = nil
		l.mu.Unlock()
	}()
	beat := time.Duration(max(ok.HeartbeatS, 5)) * time.Second
	go session.heartbeat(sctx, beat)
	go session.renewLoop(sctx)
	return true, session.serve(sctx)
}

// brokerSession is one dialed connection and the channels riding it.
type brokerSession struct {
	link     *brokerLink
	ws       *websocket.Conn
	writeMu  sync.Mutex
	mu       sync.Mutex
	channels map[string]*brokerChannel
}

func (s *brokerSession) read(ctx context.Context) (brokerMsg, error) {
	kind, data, err := s.ws.Read(ctx)
	if err != nil {
		return brokerMsg{}, err
	}
	if kind != websocket.MessageText {
		return brokerMsg{}, errors.New("messages are text")
	}
	var m brokerMsg
	if err := json.Unmarshal(data, &m); err != nil {
		return brokerMsg{}, errors.New("bad message")
	}
	return m, nil
}

func (s *brokerSession) send(ctx context.Context, m brokerMsg) error {
	raw, err := json.Marshal(m)
	if err != nil {
		return err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	wctx, cancel := context.WithTimeout(ctx, clientWriteTimeout)
	defer cancel()
	return s.ws.Write(wctx, websocket.MessageText, raw)
}

// heartbeat pings until the link ends, and ends it when a ping goes unanswered, so a
// half-open link is noticed instead of waited on.
func (s *brokerSession) heartbeat(ctx context.Context, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pctx, cancel := context.WithTimeout(ctx, 2*every)
			err := s.ws.Ping(pctx)
			cancel()
			if err != nil {
				_ = s.ws.CloseNow()
				return
			}
		}
	}
}

// serve reads until the link ends.
func (s *brokerSession) serve(ctx context.Context) error {
	for {
		m, err := s.read(ctx)
		if err != nil {
			return err
		}
		switch m.Type {
		case "treehouse.chan.open.v1":
			s.open(ctx, m)
		case "treehouse.chan.frame.v1":
			s.deliver(m)
		case "treehouse.chan.close.v1":
			s.closeChannel(m.Channel, false, "")
		}
	}
}

// brokerChannel is one client the broker attached.
type brokerChannel struct {
	id   string
	in   chan []byte
	done chan struct{}
	once sync.Once
}

func (s *brokerSession) open(ctx context.Context, m brokerMsg) {
	refuse := func(reason string) {
		_ = s.send(ctx, brokerMsg{Type: "treehouse.chan.close.v1", Channel: m.Channel, Reason: reason})
	}
	s.mu.Lock()
	_, dup := s.channels[m.Channel]
	full := len(s.channels) >= brokerMaxChan
	s.mu.Unlock()
	switch {
	case m.Channel == "" || dup:
		refuse("unauthorized")
		return
	case m.Account != s.link.identity.Owner:
		// The unlock token is judged again against the owner, so this only saves the work.
		refuse("unauthorized")
		return
	case full:
		refuse("daemon_closed")
		return
	}
	ch := &brokerChannel{id: m.Channel, in: make(chan []byte, chanQueueDepth), done: make(chan struct{})}
	s.mu.Lock()
	s.channels[ch.id] = ch
	s.mu.Unlock()
	c := &conn{
		readLine: func() ([]byte, error) {
			select {
			case line := <-ch.in:
				return line, nil
			case <-ch.done:
				return nil, io.EOF
			}
		},
		writeLine: func(line []byte) error {
			select {
			case <-ch.done:
				return io.ErrClosedPipe
			default:
			}
			return s.send(ctx, brokerMsg{Type: "treehouse.chan.frame.v1", Channel: ch.id, Frame: json.RawMessage(append([]byte(nil), line...))})
		},
		closer: func() error {
			s.closeChannel(ch.id, true, "daemon_closed")
			return nil
		},
	}
	go s.link.d.serveConn(c, peerStanding{web: true, remote: true, brokered: true})
}

// deliver queues a client's frame for its channel, dropping a client that falls behind.
func (s *brokerSession) deliver(m brokerMsg) {
	s.mu.Lock()
	ch := s.channels[m.Channel]
	s.mu.Unlock()
	if ch == nil || len(m.Frame) == 0 {
		return
	}
	select {
	case ch.in <- []byte(m.Frame):
	default:
		s.closeChannel(m.Channel, true, "daemon_closed")
	}
}

// closeChannel ends a channel, telling the broker when the daemon was the one that did.
func (s *brokerSession) closeChannel(id string, notify bool, reason string) {
	s.mu.Lock()
	ch := s.channels[id]
	delete(s.channels, id)
	s.mu.Unlock()
	if ch == nil {
		return
	}
	ch.once.Do(func() { close(ch.done) })
	if notify {
		_ = s.send(context.Background(), brokerMsg{Type: "treehouse.chan.close.v1", Channel: id, Reason: reason})
	}
}

func (s *brokerSession) closeAll() {
	s.mu.Lock()
	ids := make([]string, 0, len(s.channels))
	for id := range s.channels {
		ids = append(ids, id)
	}
	s.mu.Unlock()
	for _, id := range ids {
		s.closeChannel(id, false, "")
	}
}

// signDaemonProof is the proof in reg.hello and in a renewal: a JWS over aud, a nonce
// when the broker gave one, iat and a fresh jti, with the public key in its header.
func signDaemonProof(key *ecdsa.PrivateKey, aud, nonce string, now time.Time) (string, error) {
	jti := make([]byte, 12)
	if _, err := rand.Read(jti); err != nil {
		return "", err
	}
	header := map[string]any{"alg": "ES256", "typ": proofTokenType, "jwk": map[string]string{
		"kty": "EC", "crv": "P-256",
		"x": rawURL.EncodeToString(key.X.FillBytes(make([]byte, 32))),
		"y": rawURL.EncodeToString(key.Y.FillBytes(make([]byte, 32))),
	}}
	claims := map[string]any{"aud": aud, "iat": now.Unix(), "jti": rawURL.EncodeToString(jti)}
	if nonce != "" {
		claims["nonce"] = nonce
	}
	h, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	c, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	signing := rawURL.EncodeToString(h) + "." + rawURL.EncodeToString(c)
	digest := sha256.Sum256([]byte(signing))
	r, sv, err := ecdsa.Sign(rand.Reader, key, digest[:])
	if err != nil {
		return "", err
	}
	sig := append(r.FillBytes(make([]byte, 32)), sv.FillBytes(make([]byte, 32))...)
	return signing + "." + rawURL.EncodeToString(sig), nil
}

// loadDaemonKey reads the daemon's P-256 private key, which others must not read.
func loadDaemonKey(path string) (*ecdsa.PrivateKey, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("%s is readable by others, run chmod 600", path)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, errors.New("the daemon key is not PEM")
	}
	var key *ecdsa.PrivateKey
	if parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		var ok bool
		if key, ok = parsed.(*ecdsa.PrivateKey); !ok {
			return nil, errors.New("the daemon key is not an EC key")
		}
	} else if key, err = x509.ParseECPrivateKey(block.Bytes); err != nil {
		return nil, errors.New("the daemon key does not parse")
	}
	if key.Curve != elliptic.P256() {
		return nil, errors.New("the daemon key is not P-256")
	}
	return key, nil
}

// fetchJWKS reads the accounts key set over https, or http to loopback for a test.
func fetchJWKS(ctx context.Context, address string) ([]byte, error) {
	u, err := url.Parse(address)
	if err != nil || (u.Scheme != "https" && !(u.Scheme == "http" && isLoopbackHost(u.Host))) {
		return nil, errors.New("the accounts key set must be https")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("accounts answered %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 64<<10))
}

// treehouseOptions turn the brokered path on. All three empty leaves it off.
type treehouseOptions struct {
	Broker, Accounts, Dir string
	// ProvisionSecretFile, Name and Substrate let a daemon with no certificate claim one
	// with a secret its secret store holds (treehouse_provision.go).
	ProvisionSecretFile, Name, Substrate string
	// ConfigDir holds the configuration snapshot, empty for the default place.
	ConfigDir string
}

func (o treehouseOptions) on() bool { return o.Broker != "" || o.Accounts != "" || o.Dir != "" }

// startTreehouse brings up the brokered path when configured. A fault costs the brokered
// path only, never the daemon or its local listeners.
func (d *daemon) startTreehouse(ctx context.Context, o treehouseOptions) {
	if !o.on() {
		return
	}
	users, ok := d.users.(*localUsers)
	if !ok {
		return
	}
	if o.Broker == "" || o.Accounts == "" || o.Dir == "" {
		d.logf("no Treehouse broker path: --treehouse-broker, --treehouse-accounts and --treehouse-dir go together")
		return
	}
	if !strings.HasPrefix(o.Broker, "wss://") && !strings.HasPrefix(o.Broker, "ws://127.0.0.1") {
		d.logf("no Treehouse broker path: the broker address must be wss")
		return
	}
	if _, err := os.Stat(o.Dir + "/daemon.cert"); errors.Is(err, os.ErrNotExist) && o.ProvisionSecretFile != "" {
		d.logf("no Treehouse certificate yet, claiming one with the provisioned secret")
		go func() {
			if _, err := d.provisionDaemon(ctx, provisionOptions{Accounts: o.Accounts, Dir: o.Dir, SecretFile: o.ProvisionSecretFile, Name: provisionName(o.Name), Substrate: provisionSubstrate(o.Substrate)}); err != nil {
				return
			}
			d.startTreehouse(ctx, o)
		}()
		return
	}
	key, err := loadDaemonKey(o.Dir + "/daemon.key")
	if err != nil {
		d.logf("no Treehouse broker path: %v", err)
		return
	}
	cert, err := os.ReadFile(o.Dir + "/daemon.cert")
	if err != nil {
		d.logf("no Treehouse broker path: %v", err)
		return
	}
	keys := newAccountsKeys(strings.TrimRight(o.Accounts, "/")+"/.well-known/jwks.json", fetchJWKS)
	identity, err := loadDaemonIdentity(ctx, keys, o.Accounts, string(cert), time.Now(), true)
	if err != nil {
		d.logf("no Treehouse broker path: %v", err)
		return
	}
	users.brokered = &brokeredGate{verifier: newUnlockVerifier(keys, o.Accounts, identity), host: users.host}
	link := &brokerLink{url: o.Broker, cert: strings.TrimSpace(string(cert)), key: key, identity: identity, d: d, build: version, dial: dialBroker,
		accounts: o.Accounts, keys: keys, certPath: o.Dir + "/daemon.cert", post: accountsPoster(o.Accounts), views: d.views}
	if dir := configDir(o.ConfigDir); dir != "" {
		link.settings = newSettingsReader(dir)
	}
	users.brokerEvent = link.emit
	d.logf("dialing the Treehouse broker as %s", identity.ID)
	go link.run(ctx)
	go link.watchSeats(ctx, pushWatchEvery)
}
