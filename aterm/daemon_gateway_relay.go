package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// relayTimeout bounds one notification written to a harness, so a harness that
// stopped reading cannot stall the upstream's read loop.
const relayTimeout = 10 * time.Second

// relaySentinel names the throwaway feature that makes the harness-facing
// server send its own list-changed notification. The SDK exposes no direct send.
const relaySentinel = "aterm-relay-sentinel"

// progressRoute is the harness session and token an upstream progress token
// goes back to. ctx is the call's, so progress rides that call's stream.
type progressRoute struct {
	ctx     context.Context
	session *mcp.ServerSession
	token   any
}

// relay carries one upstream's notifications to the harness-facing server. It
// registers no server-request handler, so sampling and elicitation stay refused.
type relay struct {
	logf func(string, ...any)
	// toolsChanged drops the gateway's cached tool visibility.
	toolsChanged func()

	// mu is never held across toolsChanged or an SDK call, so no handler waits
	// on a lock held during an upstream close.
	mu         sync.Mutex
	server     *mcp.Server
	upstream   *mcp.ClientSession
	routes     map[string]progressRoute
	issued     int
	subscribed map[string]bool
}

// bind connects the relay to the harness-facing server. Earlier notifications
// are dropped, since no harness has listed anything yet.
func (r *relay) bind(server *mcp.Server, upstream *mcp.ClientSession) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.server, r.upstream = server, upstream
}

func (r *relay) bound() (*mcp.Server, *mcp.ClientSession) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.server, r.upstream
}

// attach registers the handlers on the upstream client's options.
func (r *relay) attach(options *mcp.ClientOptions) {
	options.ToolListChangedHandler = func(context.Context, *mcp.ToolListChangedRequest) {
		server, _ := r.bound()
		if server == nil {
			return
		}
		r.toolsChanged()
		server.AddTool(&mcp.Tool{Name: relaySentinel, InputSchema: map[string]any{"type": "object"}}, nil)
		server.RemoveTools(relaySentinel)
	}
	options.PromptListChangedHandler = func(context.Context, *mcp.PromptListChangedRequest) {
		if server, _ := r.bound(); server != nil {
			server.AddPrompt(&mcp.Prompt{Name: relaySentinel}, nil)
			server.RemovePrompts(relaySentinel)
		}
	}
	options.ResourceListChangedHandler = func(context.Context, *mcp.ResourceListChangedRequest) {
		if server, _ := r.bound(); server != nil {
			server.AddResource(&mcp.Resource{Name: relaySentinel, URI: "aterm://" + relaySentinel}, nil)
			server.RemoveResources("aterm://" + relaySentinel)
		}
	}
	options.ResourceUpdatedHandler = func(_ context.Context, req *mcp.ResourceUpdatedNotificationRequest) {
		server, _ := r.bound()
		if server == nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), relayTimeout)
		defer cancel()
		if err := server.ResourceUpdated(ctx, req.Params); err != nil {
			r.logf("gateway could not relay a resource update: %v", err)
		}
	}
	options.ProgressNotificationHandler = r.progress
	options.LoggingMessageHandler = r.logging
}

// routeProgress swaps a harness call's token for an upstream one, since two
// sessions can both send 1. The release func ends the route with the call.
func (r *relay) routeProgress(ctx context.Context, session *mcp.ServerSession, token any) (string, func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.routes == nil {
		r.routes = map[string]progressRoute{}
	}
	r.issued++
	key := fmt.Sprintf("aterm-%d", r.issued)
	r.routes[key] = progressRoute{ctx: ctx, session: session, token: token}
	return key, func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		delete(r.routes, key)
	}
}

func (r *relay) progress(_ context.Context, req *mcp.ProgressNotificationClientRequest) {
	key, _ := req.Params.ProgressToken.(string)
	r.mu.Lock()
	route, ok := r.routes[key]
	r.mu.Unlock()
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(route.ctx, relayTimeout)
	defer cancel()
	err := route.session.NotifyProgress(ctx, &mcp.ProgressNotificationParams{
		ProgressToken: route.token, Progress: req.Params.Progress, Total: req.Params.Total, Message: req.Params.Message,
	})
	if err != nil {
		r.logf("gateway could not relay progress: %v", err)
	}
}

// logging sends a server's log message to every harness session. A session that
// never set a level gets none, which the SDK's Log applies.
func (r *relay) logging(_ context.Context, req *mcp.LoggingMessageRequest) {
	server, _ := r.bound()
	if server == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), relayTimeout)
	defer cancel()
	for session := range server.Sessions() {
		_ = session.Log(ctx, req.Params)
	}
}

// subscribe asks the upstream for a resource once, on the first harness
// subscribe, and keeps it until the upstream closes.
func (r *relay) subscribe(ctx context.Context, req *mcp.SubscribeRequest) error {
	_, upstream := r.bound()
	if upstream == nil {
		return fmt.Errorf("the upstream is not connected")
	}
	r.mu.Lock()
	if r.subscribed[req.Params.URI] {
		r.mu.Unlock()
		return nil
	}
	r.mu.Unlock()
	if err := upstream.Subscribe(ctx, &mcp.SubscribeParams{URI: req.Params.URI}); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.subscribed == nil {
		r.subscribed = map[string]bool{}
	}
	r.subscribed[req.Params.URI] = true
	return nil
}
