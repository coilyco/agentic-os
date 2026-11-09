package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestLoopbackListenerTakesOnlyTheOriginsItIsNamed(t *testing.T) {
	const app = "https://tauri.localhost"
	d := newDaemon(func(string, ...any) {})
	dial := func(server *httptest.Server, origin string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		ws, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), &websocket.DialOptions{HTTPHeader: http.Header{"Origin": {origin}}})
		if err == nil {
			_ = ws.CloseNow()
		}
		return err
	}
	plain := httptest.NewServer(d.handler(loopbackPolicy()))
	named := httptest.NewServer(d.handler(loopbackPolicy(app)))
	t.Cleanup(plain.Close)
	t.Cleanup(named.Close)

	if err := dial(plain, app); err == nil {
		t.Fatal("with no origin named, the app's origin must stay refused on loopback")
	}
	if err := dial(named, app); err != nil {
		t.Fatalf("a named origin must open a socket: %v", err)
	}
	if err := dial(named, "https://TAURI.localhost"); err != nil {
		t.Fatalf("the host is case-insensitive: %v", err)
	}
	for _, other := range []string{"https://tauri.localhost.example", "http://tauri.localhost", "https://other.localhost", "https://coilyco.dev", ""} {
		if err := dial(named, other); err == nil {
			t.Fatalf("origin %q was not named and must stay refused", other)
		}
	}
	if err := dial(named, "http://127.0.0.1:5173"); err != nil {
		t.Fatalf("a page on loopback itself is still served: %v", err)
	}
}
