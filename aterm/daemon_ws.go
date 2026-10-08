package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/coder/websocket"
)

const (
	daemonWSEnv = "ATERM_DAEMON_WS"
	// Loopback only until the tailnet milestone brings client auth.
	// See docs/aterm-daemon.md.
	defaultDaemonWS = "127.0.0.1:7419"
)

// requireLoopback refuses a listen address that is not a loopback IP, since
// the listener has no auth beyond Origin.
func requireLoopback(address string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("websocket address %q: %w", address, err)
	}
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("websocket address %q is not a loopback IP, and tailnet auth is not built", address)
	}
	return nil
}

func isLoopbackHost(hostport string) bool {
	host := hostport
	if split, _, err := net.SplitHostPort(hostport); err == nil {
		host = split
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// accessPolicy is who one listener serves. admit judges the connecting peer,
// and origin the page that asks to open a websocket.
type accessPolicy struct {
	admit  func(*http.Request) error
	origin func(*http.Request, *url.URL) bool
}

// loopbackPolicy refuses a Host that is not loopback, which is how a rebound
// DNS name arrives, and any page that is not on loopback itself.
func loopbackPolicy() accessPolicy {
	return accessPolicy{
		admit: func(r *http.Request) error {
			if !isLoopbackHost(r.Host) {
				return errors.New("loopback hosts only")
			}
			return nil
		},
		origin: func(_ *http.Request, origin *url.URL) bool { return isLoopbackHost(origin.Host) },
	}
}

func (d *daemon) websocketHandler() http.Handler { return d.handler(loopbackPolicy()) }

// handler serves the client's files, and upgrades a websocket to the same
// frames as the unix socket. A browser on this host is judged by its socket's owner.
func (d *daemon) handler(policy accessPolicy) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := policy.admit(r); err != nil {
			http.Error(w, "aterm daemon: "+err.Error(), http.StatusForbidden)
			return
		}
		if strings.HasPrefix(r.URL.Path, gatewayPrefix) {
			d.serveGateway(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, cdpPrefix) {
			d.serveCDP(w, r)
			return
		}
		if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			d.serveClient(w, r)
			return
		}
		origin := r.Header.Get("Origin")
		parsed, err := url.Parse(origin)
		if origin == "" || err != nil || !policy.origin(r, parsed) {
			http.Error(w, "aterm daemon: this page may not open a session socket", http.StatusForbidden)
			return
		}
		// The origin was judged above, per listener, so the library's own
		// same-host check would only repeat it.
		ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		ws.SetReadLimit(maxFrame)
		d.serveConn(newWebsocketConn(r.Context(), ws), d.webPeer(r))
	})
}

// webPeer stands for an upgraded request's other end. A browser on this host is
// named by the socket table and walked like a unix peer. An unnamed one counts as under.
func (d *daemon) webPeer(r *http.Request) peerStanding {
	remote, err := net.ResolveTCPAddr("tcp", r.RemoteAddr)
	if err != nil {
		return peerStanding{web: true}
	}
	if !onThisHost(remote) {
		return peerStanding{web: true, remote: true}
	}
	local, _ := r.Context().Value(http.LocalAddrContextKey).(net.Addr)
	pids, err := d.peerLookup(remote, local)
	if err != nil {
		d.logf("could not name the process behind %s, so it cannot type: %v", r.RemoteAddr, err)
	}
	return peerStanding{web: true, pids: pids}
}

// serveClient is the built aterm client, so a browser opens the daemon's own
// address and gets a page whose websocket is same-origin.
func (d *daemon) serveClient(w http.ResponseWriter, r *http.Request) {
	if info, err := os.Stat(d.clientDir); d.clientDir == "" || err != nil || !info.IsDir() {
		http.Error(w, "aterm daemon: no client is installed at "+d.clientDir, http.StatusNotFound)
		return
	}
	http.FileServer(http.Dir(d.clientDir)).ServeHTTP(w, r)
}

func newWebsocketConn(ctx context.Context, ws *websocket.Conn) *conn {
	return &conn{
		readLine: func() ([]byte, error) {
			kind, data, err := ws.Read(ctx)
			if err == nil && kind != websocket.MessageText {
				return nil, errors.New("frames are text messages")
			}
			return data, err
		},
		writeLine: func(line []byte) error {
			writeCtx, cancel := context.WithTimeout(ctx, clientWriteTimeout)
			defer cancel()
			return ws.Write(writeCtx, websocket.MessageText, line)
		},
		closer: func() error { return ws.Close(websocket.StatusNormalClosure, "") },
	}
}

// listenWebsocket serves until the returned server is closed. A port already
// taken costs the browser clients, never the daemon.
func (d *daemon) listenWebsocket(address string) (*http.Server, error) {
	if err := requireLoopback(address); err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}
	server := &http.Server{Handler: d.handler(loopbackPolicy())}
	d.mu.Lock()
	d.loopbackAddr = listener.Addr().String()
	d.mu.Unlock()
	go func() { _ = server.Serve(listener) }()
	d.logf("serving websocket on ws://%s", listener.Addr())
	return server, nil
}
