package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// gatewayProtocol pins the revision the gateway speaks to a server. The SDK's
// newer stateless discovery revision is left out until a server here asks for it.
const gatewayProtocol = "2025-11-25"

// upstreamInitTimeout bounds starting a server and its initialize handshake.
const upstreamInitTimeout = 30 * time.Second

// gatewaySpec is one server a session's gateway fronts: a stdio command or an
// HTTP URL, never both. Path is the daemon's answer, where the harness points.
type gatewaySpec struct {
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     []string          `json:"env,omitempty"`
	Cwd     string            `json:"cwd,omitempty"`
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Path    string            `json:"path,omitempty"`
}

func (s gatewaySpec) validate() error {
	switch {
	case (s.Command == "") == (s.URL == ""):
		return errors.New("a gateway server is a command or a url, one of them")
	case s.URL != "":
		parsed, err := url.Parse(s.URL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return fmt.Errorf("gateway url %q is not an http or https URL", s.URL)
		}
	}
	return nil
}

// upstream is the gateway's client session to one server.
type upstream struct {
	session *mcp.ClientSession
	// cmd is the stdio child, nil for an HTTP server. Its own process group,
	// so closing it takes the server's children too.
	cmd  *exec.Cmd
	done chan struct{}
}

// headerTransport adds a spec's headers to every request to an HTTP server.
type headerTransport struct{ headers map[string]string }

func (t headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	for name, value := range t.headers {
		req.Header.Set(name, value)
	}
	return http.DefaultTransport.RoundTrip(req)
}

// logLines sends a server's stderr to the daemon log, a line at a time. The
// pipe closes when the command is waited on.
func logLines(label string, stderr io.Reader, logf func(string, ...any)) {
	lines := bufio.NewScanner(stderr)
	for lines.Scan() {
		logf("gateway server %s: %s", label, lines.Text())
	}
}

func openUpstream(spec gatewaySpec, logf func(string, ...any), r *relay) (*upstream, error) {
	capabilities := &mcp.ClientCapabilities{}
	capabilities.AddExtension(uiExtension, map[string]any{"mimeTypes": []string{uiMime}})
	options := &mcp.ClientOptions{Capabilities: capabilities}
	r.attach(options)
	client := mcp.NewClient(&mcp.Implementation{Name: "aterm", Version: version}, options)
	up := &upstream{done: make(chan struct{})}
	var transport mcp.Transport
	if spec.URL != "" {
		transport = &mcp.StreamableClientTransport{
			Endpoint: spec.URL, HTTPClient: &http.Client{Transport: headerTransport{headers: spec.Headers}},
		}
	} else {
		command := exec.Command(spec.Command, spec.Args...)
		command.Env = append(os.Environ(), spec.Env...)
		command.Dir = spec.Cwd
		command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		stderr, err := command.StderrPipe()
		if err != nil {
			return nil, err
		}
		go logLines(spec.Command, stderr, logf)
		up.cmd = command
		transport = &mcp.CommandTransport{Command: command, TerminateDuration: 2 * time.Second}
	}
	// The connection outlives this call, so a watchdog bounds only the handshake.
	ctx, cancel := context.WithCancel(context.Background())
	watchdog := time.AfterFunc(upstreamInitTimeout, cancel)
	session, err := client.Connect(ctx, transport, &mcp.ClientSessionOptions{ProtocolVersion: gatewayProtocol})
	watchdog.Stop()
	if err != nil {
		cancel()
		return nil, err
	}
	up.session = session
	// A server logs nothing until a level is set. The gateway asks for all of it
	// and each harness session filters by the level it set itself.
	if caps := session.InitializeResult().Capabilities; caps != nil && caps.Logging != nil {
		levelCtx, levelCancel := context.WithTimeout(ctx, 5*time.Second)
		_ = session.SetLoggingLevel(levelCtx, &mcp.SetLoggingLevelParams{Level: "debug"})
		levelCancel()
	}
	go func() {
		_ = session.Wait()
		cancel()
		close(up.done)
	}()
	return up, nil
}

func (u *upstream) alive() bool {
	select {
	case <-u.done:
		return false
	default:
		return true
	}
}

func (u *upstream) close() {
	_ = u.session.Close()
	if u.cmd != nil && u.cmd.Process != nil {
		_ = syscall.Kill(-u.cmd.Process.Pid, syscall.SIGTERM)
	}
}
