package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// relayUpstream is a real SDK server behind the gateway, so it sends the
// notifications a live server does.
func relayUpstream(t *testing.T) (*mcp.Server, *httptest.Server) {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "relay-fake", Version: "1"}, &mcp.ServerOptions{
		Capabilities: &mcp.ServerCapabilities{
			Tools: &mcp.ToolCapabilities{ListChanged: true}, Logging: &mcp.LoggingCapabilities{},
			Resources: &mcp.ResourceCapabilities{Subscribe: true},
		},
		SubscribeHandler:   func(context.Context, *mcp.SubscribeRequest) error { return nil },
		UnsubscribeHandler: func(context.Context, *mcp.UnsubscribeRequest) error { return nil },
	})
	server.AddTool(&mcp.Tool{Name: "work", InputSchema: object}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		token := req.Params.GetProgressToken()
		for step := 1.0; step <= 2; step++ {
			err := req.Session.NotifyProgress(ctx, &mcp.ProgressNotificationParams{
				ProgressToken: token, Progress: step, Total: 2, Message: fmt.Sprintf("step %v", step),
			})
			if err != nil {
				return nil, err
			}
		}
		// A result may overtake a notification still queued in the client, and
		// progress after completion is dropped, so a real call has a gap here too.
		time.Sleep(100 * time.Millisecond)
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "worked"}}}, nil
	})
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)
	httpServer := httptest.NewServer(handler)
	t.Cleanup(httpServer.Close)
	return server, httpServer
}

// relayHarness is a harness that records what the gateway sends it unasked.
type relayHarness struct {
	session  *mcp.ClientSession
	progress chan *mcp.ProgressNotificationParams
	changed  chan struct{}
	logs     chan *mcp.LoggingMessageParams
	updated  chan string
}

func (h *appsHarness) relayHarness(path string, progressBuffer int) *relayHarness {
	h.t.Helper()
	rh := &relayHarness{progress: make(chan *mcp.ProgressNotificationParams, progressBuffer), changed: make(chan struct{}, 16),
		logs: make(chan *mcp.LoggingMessageParams, 16), updated: make(chan string, 16)}
	client := mcp.NewClient(&mcp.Implementation{Name: "relay-harness", Version: "1"}, &mcp.ClientOptions{
		ProgressNotificationHandler: func(_ context.Context, req *mcp.ProgressNotificationClientRequest) { rh.progress <- req.Params },
		ToolListChangedHandler:      func(context.Context, *mcp.ToolListChangedRequest) { rh.changed <- struct{}{} },
		LoggingMessageHandler:       func(_ context.Context, req *mcp.LoggingMessageRequest) { rh.logs <- req.Params },
		ResourceUpdatedHandler: func(_ context.Context, req *mcp.ResourceUpdatedNotificationRequest) {
			rh.updated <- req.Params.URI
		},
	})
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: h.http.URL + path}, nil)
	if err != nil {
		h.t.Fatalf("connect %s: %v", path, err)
	}
	h.t.Cleanup(func() { _ = session.Close() })
	rh.session = session
	return rh
}

func (rh *relayHarness) work(token any) (*mcp.CallToolResult, error) {
	params := &mcp.CallToolParams{Name: "work"}
	params.SetProgressToken(token)
	return rh.session.CallTool(context.Background(), params)
}

func TestGatewayRelaysProgressUnderTheHarnessToken(t *testing.T) {
	_, upstream := relayUpstream(t)
	h := newAppsHarness(t)
	path := h.register("fake", gatewaySpec{URL: upstream.URL}).Path
	first, second := h.relayHarness(path, 4), h.relayHarness(path, 4)

	// Both sessions use token 1, which the gateway must keep apart.
	for _, rh := range []*relayHarness{first, second} {
		if _, err := rh.work("1"); err != nil {
			t.Fatalf("call: %v", err)
		}
	}
	for name, rh := range map[string]*relayHarness{"first": first, "second": second} {
		for step := 1.0; step <= 2; step++ {
			select {
			case got := <-rh.progress:
				if got.ProgressToken != "1" || got.Progress != step || got.Total != 2 || got.Message != fmt.Sprintf("step %v", step) {
					t.Errorf("%s step %v: got %+v", name, step, got)
				}
			case <-time.After(5 * time.Second):
				t.Fatalf("%s: no progress for step %v", name, step)
			}
		}
		select {
		case extra := <-rh.progress:
			t.Errorf("%s got progress that was another session's: %+v", name, extra)
		default:
		}
	}
}

func TestGatewayRelaysAToolListChangeAndForgetsTheCachedVisibility(t *testing.T) {
	server, upstream := relayUpstream(t)
	h := newAppsHarness(t)
	path := h.register("fake", gatewaySpec{URL: upstream.URL}).Path
	rh := h.relayHarness(path, 4)

	// A call loads the tool list once, so the gateway now caches every tool's visibility.
	if _, err := rh.work("p"); err != nil {
		t.Fatalf("call: %v", err)
	}
	server.AddTool(&mcp.Tool{
		Name: "late", InputSchema: object, Meta: mcp.Meta{"ui": map[string]any{"visibility": []string{"app"}}},
	}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "late ran"}}}, nil
	})

	// The harness's standalone stream may open after the first change, so keep
	// changing the list until one arrives.
	deadline := time.After(10 * time.Second)
	for received := false; !received; {
		server.AddTool(&mcp.Tool{Name: "noise", InputSchema: object}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{}, nil
		})
		select {
		case <-rh.changed:
			received = true
		case <-time.After(100 * time.Millisecond):
		case <-deadline:
			t.Fatal("the harness never received tools/list_changed")
		}
	}

	// Before any re-list, so only the dropped cache can know the tool is app-only.
	_, err := rh.session.CallTool(context.Background(), &mcp.CallToolParams{Name: "late"})
	if err == nil || !strings.Contains(err.Error(), "only callable by a view") {
		t.Errorf("a model call to a tool added later as app-only should be refused, got %v", err)
	}

	listed, err := rh.session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, tool := range listed.Tools {
		if tool.Name == "late" {
			t.Error("a tool only a view may call was listed to the agent")
		}
	}
}

// untilReceived repeats send until receive returns true, since a harness's
// standalone stream may open after the first notification.
func untilReceived(t *testing.T, what string, send func(), receive func(timeout time.Duration) bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		send()
		if receive(100 * time.Millisecond) {
			return
		}
	}
	t.Fatalf("the harness never received %s", what)
}

func TestGatewayRelaysLogMessagesAtTheLevelTheHarnessSet(t *testing.T) {
	server, upstream := relayUpstream(t)
	h := newAppsHarness(t)
	path := h.register("fake", gatewaySpec{URL: upstream.URL}).Path
	quiet, loud := h.relayHarness(path, 1), h.relayHarness(path, 1)
	if err := loud.session.SetLoggingLevel(context.Background(), &mcp.SetLoggingLevelParams{Level: "info"}); err != nil {
		t.Fatalf("set level: %v", err)
	}
	untilReceived(t, "a log message", func() {
		for session := range server.Sessions() {
			_ = session.Log(context.Background(), &mcp.LoggingMessageParams{Level: "warning", Logger: "fake", Data: "disk low"})
		}
	}, func(timeout time.Duration) bool {
		select {
		case got := <-loud.logs:
			if got.Data != "disk low" || got.Logger != "fake" || got.Level != "warning" {
				t.Errorf("got %+v", got)
			}
			return true
		case <-time.After(timeout):
			return false
		}
	})
	select {
	case got := <-quiet.logs:
		t.Errorf("a session that set no level got %+v", got)
	default:
	}
}

func TestGatewayRelaysAResourceUpdateToItsSubscribers(t *testing.T) {
	server, upstream := relayUpstream(t)
	h := newAppsHarness(t)
	path := h.register("fake", gatewaySpec{URL: upstream.URL}).Path
	subscriber, bystander := h.relayHarness(path, 1), h.relayHarness(path, 1)
	if err := subscriber.session.Subscribe(context.Background(), &mcp.SubscribeParams{URI: "file:///a"}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	untilReceived(t, "a resource update", func() {
		_ = server.ResourceUpdated(context.Background(), &mcp.ResourceUpdatedNotificationParams{URI: "file:///a"})
	}, func(timeout time.Duration) bool {
		select {
		case uri := <-subscriber.updated:
			if uri != "file:///a" {
				t.Errorf("got %q", uri)
			}
			return true
		case <-time.After(timeout):
			return false
		}
	})
	select {
	case uri := <-bystander.updated:
		t.Errorf("a session that never subscribed got %q", uri)
	default:
	}
}
