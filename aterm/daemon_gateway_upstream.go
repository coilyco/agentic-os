package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// gatewayProtocol is the MCP revision the gateway speaks to a server. The
// server answers with the one it keeps, which the harness then sees.
const gatewayProtocol = "2025-06-18"

// maxUpstreamLine bounds one JSON-RPC message from a server. A chart result is
// far larger than a terminal frame.
const maxUpstreamLine = 16 << 20

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

// upstreamError is a JSON-RPC error a server answered with, kept apart from a
// transport failure so the harness sees the server's own words.
type upstreamError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *upstreamError) Error() string { return e.Message }

type upstreamReply struct {
	result json.RawMessage
	err    *upstreamError
}

// mcpUpstream is one connection to a server. request returns the result, an
// *upstreamError for the server's refusal, or another error for a dead transport.
type mcpUpstream interface {
	request(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, error)
	notify(method string, params json.RawMessage) error
	alive() bool
	close()
}

func openUpstream(spec gatewaySpec, logf func(string, ...any)) (mcpUpstream, error) {
	if spec.URL != "" {
		return &httpUpstream{url: spec.URL, headers: spec.Headers, client: &http.Client{}}, nil
	}
	return startStdioUpstream(spec, logf)
}

type rpcEnvelope struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *upstreamError  `json:"error,omitempty"`
}

func encodeRPC(id int64, method string, params json.RawMessage) []byte {
	message := map[string]any{"jsonrpc": "2.0", "method": method}
	if id != 0 {
		message["id"] = id
	}
	if len(params) > 0 {
		message["params"] = params
	}
	encoded, _ := json.Marshal(message)
	return encoded
}

// stdioUpstream is a server the daemon started, one child per session server.
type stdioUpstream struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	writeMu sync.Mutex
	next    atomic.Int64
	mu      sync.Mutex
	pending map[int64]chan upstreamReply
	exited  chan struct{}
	exitErr error
}

func startStdioUpstream(spec gatewaySpec, logf func(string, ...any)) (*stdioUpstream, error) {
	command := exec.Command(spec.Command, spec.Args...)
	command.Env = append(os.Environ(), spec.Env...)
	command.Dir = spec.Cwd
	// Its own group, so closing it takes the server's children too.
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdin, err := command.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", spec.Command, err)
	}
	up := &stdioUpstream{cmd: command, stdin: stdin, pending: map[int64]chan upstreamReply{}, exited: make(chan struct{})}
	go func() {
		lines := bufio.NewScanner(stderr)
		for lines.Scan() {
			logf("gateway server %s: %s", spec.Command, lines.Text())
		}
	}()
	go up.readLoop(stdout)
	return up, nil
}

func (u *stdioUpstream) readLoop(stdout io.Reader) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64<<10), maxUpstreamLine)
	for scanner.Scan() {
		var message rpcEnvelope
		if json.Unmarshal(scanner.Bytes(), &message) != nil {
			continue
		}
		switch {
		case message.Method != "" && len(message.ID) > 0:
			// A server asking the client for something. The gateway offers no
			// sampling, roots or elicitation, so it answers instead of leaving it hanging.
			u.answerServer(message)
		case message.Method != "":
		case len(message.ID) > 0:
			id, err := strconv.ParseInt(string(message.ID), 10, 64)
			if err != nil {
				continue
			}
			u.mu.Lock()
			reply := u.pending[id]
			delete(u.pending, id)
			u.mu.Unlock()
			if reply != nil {
				reply <- upstreamReply{result: message.Result, err: message.Error}
			}
		}
	}
	waited := u.cmd.Wait()
	u.mu.Lock()
	u.exitErr = errors.New("the server exited")
	if waited != nil {
		u.exitErr = fmt.Errorf("the server exited: %w", waited)
	}
	close(u.exited)
	for id, reply := range u.pending {
		reply <- upstreamReply{err: &upstreamError{Code: -32603, Message: u.exitErr.Error()}}
		delete(u.pending, id)
	}
	u.mu.Unlock()
}

func (u *stdioUpstream) answerServer(message rpcEnvelope) {
	reply := map[string]any{"jsonrpc": "2.0", "id": message.ID}
	if message.Method == "ping" {
		reply["result"] = map[string]any{}
	} else {
		reply["error"] = map[string]any{"code": -32601, "message": "the aterm gateway does not answer " + message.Method}
	}
	encoded, _ := json.Marshal(reply)
	_ = u.write(encoded)
}

func (u *stdioUpstream) write(line []byte) error {
	u.writeMu.Lock()
	defer u.writeMu.Unlock()
	_, err := u.stdin.Write(append(line, '\n'))
	return err
}

func (u *stdioUpstream) request(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, error) {
	id := u.next.Add(1)
	reply := make(chan upstreamReply, 1)
	u.mu.Lock()
	if u.exitErr != nil {
		u.mu.Unlock()
		return nil, u.exitErr
	}
	u.pending[id] = reply
	u.mu.Unlock()
	if err := u.write(encodeRPC(id, method, params)); err != nil {
		u.forgetPending(id)
		return nil, err
	}
	select {
	case answer := <-reply:
		if answer.err != nil {
			return nil, answer.err
		}
		return answer.result, nil
	case <-ctx.Done():
		u.forgetPending(id)
		return nil, ctx.Err()
	}
}

func (u *stdioUpstream) forgetPending(id int64) {
	u.mu.Lock()
	delete(u.pending, id)
	u.mu.Unlock()
}

func (u *stdioUpstream) notify(method string, params json.RawMessage) error {
	return u.write(encodeRPC(0, method, params))
}

func (u *stdioUpstream) alive() bool {
	select {
	case <-u.exited:
		return false
	default:
		return true
	}
}

func (u *stdioUpstream) close() {
	_ = u.stdin.Close()
	if u.cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-u.cmd.Process.Pid, syscall.SIGTERM)
	go func() {
		select {
		case <-u.exited:
		case <-time.After(2 * time.Second):
			_ = syscall.Kill(-u.cmd.Process.Pid, syscall.SIGKILL)
		}
	}()
}

// httpUpstream is a server reached over Streamable HTTP. Each request is one
// POST, answered as JSON or as an event stream that ends with its result.
type httpUpstream struct {
	url     string
	headers map[string]string
	client  *http.Client
	next    atomic.Int64
	mu      sync.Mutex
	session string
	version string
}

func (u *httpUpstream) post(ctx context.Context, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	for name, value := range u.headers {
		req.Header.Set(name, value)
	}
	u.mu.Lock()
	if u.session != "" {
		req.Header.Set("Mcp-Session-Id", u.session)
	}
	if u.version != "" {
		req.Header.Set("MCP-Protocol-Version", u.version)
	}
	u.mu.Unlock()
	return u.client.Do(req)
}

func (u *httpUpstream) request(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, error) {
	id := u.next.Add(1)
	resp, err := u.post(ctx, encodeRPC(id, method, params))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("the server answered %s: %s", resp.Status, strings.TrimSpace(string(detail)))
	}
	if method == "initialize" {
		u.mu.Lock()
		u.session = resp.Header.Get("Mcp-Session-Id")
		u.mu.Unlock()
	}
	var reply upstreamReply
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		reply, err = readEventStream(resp.Body, id)
	} else {
		reply, err = readJSONReply(resp.Body, id)
	}
	if err != nil {
		return nil, err
	}
	if reply.err != nil {
		return nil, reply.err
	}
	if method == "initialize" {
		var negotiated struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(reply.result, &negotiated)
		u.mu.Lock()
		u.version = negotiated.ProtocolVersion
		u.mu.Unlock()
	}
	return reply.result, nil
}

func replyFor(raw []byte, id int64) (upstreamReply, bool) {
	var message rpcEnvelope
	if json.Unmarshal(raw, &message) != nil || message.Method != "" {
		return upstreamReply{}, false
	}
	if got, err := strconv.ParseInt(string(message.ID), 10, 64); err != nil || got != id {
		return upstreamReply{}, false
	}
	return upstreamReply{result: message.Result, err: message.Error}, true
}

func readJSONReply(body io.Reader, id int64) (upstreamReply, error) {
	raw, err := io.ReadAll(io.LimitReader(body, maxUpstreamLine))
	if err != nil {
		return upstreamReply{}, err
	}
	if reply, ok := replyFor(raw, id); ok {
		return reply, nil
	}
	return upstreamReply{}, errors.New("the server's answer held no result for the request")
}

func readEventStream(body io.Reader, id int64) (upstreamReply, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64<<10), maxUpstreamLine)
	var data []string
	flush := func() (upstreamReply, bool) {
		defer func() { data = data[:0] }()
		return replyFor([]byte(strings.Join(data, "\n")), id)
	}
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case line == "":
			if reply, ok := flush(); ok {
				return reply, nil
			}
		case strings.HasPrefix(line, "data:"):
			data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	if reply, ok := flush(); ok {
		return reply, nil
	}
	if err := scanner.Err(); err != nil {
		return upstreamReply{}, err
	}
	return upstreamReply{}, errors.New("the server's event stream ended without a result")
}

func (u *httpUpstream) notify(method string, params json.RawMessage) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	resp, err := u.post(ctx, encodeRPC(0, method, params))
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	return nil
}

func (u *httpUpstream) alive() bool { return true }

func (u *httpUpstream) close() {
	u.mu.Lock()
	session := u.session
	u.mu.Unlock()
	if session == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, u.url, nil)
	if err != nil {
		return
	}
	req.Header.Set("Mcp-Session-Id", session)
	for name, value := range u.headers {
		req.Header.Set(name, value)
	}
	if resp, err := u.client.Do(req); err == nil {
		_ = resp.Body.Close()
	}
}
