package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const testDSNKey = "0123456789abcdef"

func TestNewSentryCronBuildsTheCheckInURLFromTheDSN(t *testing.T) {
	cron, err := newSentryCron("https://"+testDSNKey+"@o42.ingest.sentry.io/456", "aterm-daemon-mac")
	if err != nil {
		t.Fatal(err)
	}
	if want := "https://o42.ingest.sentry.io/api/456/cron/aterm-daemon-mac/" + testDSNKey + "/"; cron.endpoint != want {
		t.Fatalf("endpoint = %s, want %s", cron.endpoint, want)
	}
	for _, bad := range []string{"", "not a dsn", "https://o42.ingest.sentry.io/456", "https://" + testDSNKey + "@o42.ingest.sentry.io/", "https://" + testDSNKey + "@o42.ingest.sentry.io/a/b"} {
		if _, err := newSentryCron(bad, "x"); err == nil {
			t.Fatalf("DSN %q was accepted", bad)
		} else if strings.Contains(err.Error(), testDSNKey) {
			t.Fatalf("the error for %q quotes the key: %v", bad, err)
		}
	}
}

func TestSentryMonitorSlugIsPerHostAndSafe(t *testing.T) {
	for host, want := range map[string]string{
		"kais-macbook-pro": "aterm-daemon-kais-macbook-pro",
		"Kai's Mac_Book":   "aterm-daemon-kai-s-mac-book",
		"":                 "aterm-daemon-host",
	} {
		if got := sentryMonitorSlug(host); got != want {
			t.Fatalf("slug(%q) = %q, want %q", host, got, want)
		}
	}
	if got := sentryMonitorSlug(strings.Repeat("a", 90)); len(got) > 50 {
		t.Fatalf("slug %q is %d long, over Sentry's 50", got, len(got))
	}
}

// fakeSentry records the check-ins it receives and answers with status.
type fakeSentry struct {
	server *httptest.Server
	mu     sync.Mutex
	bodies []map[string]any
	paths  []string
	status atomic.Int32
}

func newFakeSentry(t *testing.T) *fakeSentry {
	t.Helper()
	fake := &fakeSentry{}
	fake.status.Store(http.StatusAccepted)
	fake.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		fake.mu.Lock()
		fake.bodies, fake.paths = append(fake.bodies, body), append(fake.paths, r.Method+" "+r.URL.Path)
		fake.mu.Unlock()
		w.WriteHeader(int(fake.status.Load()))
	}))
	t.Cleanup(fake.server.Close)
	return fake
}

func (f *fakeSentry) dsn() string {
	return "http://" + testDSNKey + "@" + strings.TrimPrefix(f.server.URL, "http://") + "/7"
}

func (f *fakeSentry) statuses() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, body := range f.bodies {
		out = append(out, body["status"].(string))
	}
	return out
}

func TestSentryCheckInUpsertsTheMonitorAndReportsStatus(t *testing.T) {
	fake := newFakeSentry(t)
	cron, err := newSentryCron(fake.dsn(), "aterm-daemon-mac")
	if err != nil {
		t.Fatal(err)
	}
	if err := cron.checkIn(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if err := cron.checkIn(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if got := fake.statuses(); len(got) != 2 || got[0] != "ok" || got[1] != "error" {
		t.Fatalf("statuses = %v", got)
	}
	if want := "POST /api/7/cron/aterm-daemon-mac/" + testDSNKey + "/"; fake.paths[0] != want {
		t.Fatalf("request = %s, want %s", fake.paths[0], want)
	}
	config := fake.bodies[0]["monitor_config"].(map[string]any)
	schedule := config["schedule"].(map[string]any)
	if schedule["type"] != "interval" || schedule["unit"] != "minute" || schedule["value"] != float64(5) {
		t.Fatalf("schedule = %v", schedule)
	}
	if config["failure_issue_threshold"] != float64(2) || config["checkin_margin"] != float64(5) {
		t.Fatalf("monitor_config = %v", config)
	}

	fake.status.Store(http.StatusTooManyRequests)
	if err := cron.checkIn(context.Background(), true); err == nil || !strings.Contains(err.Error(), "HTTP 429") {
		t.Fatalf("a refused check-in read as %v", err)
	}
}

func TestSentryCheckInErrorNeverQuotesTheKey(t *testing.T) {
	fake := newFakeSentry(t)
	dsn := fake.dsn()
	fake.server.Close()
	cron, err := newSentryCron(dsn, "aterm-daemon-mac")
	if err != nil {
		t.Fatal(err)
	}
	err = cron.checkIn(context.Background(), true)
	if err == nil {
		t.Fatal("a check-in to a closed server succeeded")
	}
	if strings.Contains(err.Error(), testDSNKey) {
		t.Fatalf("the error quotes the DSN key: %v", err)
	}
}

func TestTailnetHealthReadsTheBoundListener(t *testing.T) {
	d := newDaemon(func(string, ...any) {})
	ctx := context.Background()
	if err := d.tailnetHealth(ctx); err != nil {
		t.Fatalf("a switched-off listener read as unhealthy: %v", err)
	}
	d.tailnet.want()
	d.tailnet.set(nil, "the tailscale backend is \"Stopped\"")
	if err := d.tailnetHealth(ctx); err == nil || !strings.Contains(err.Error(), "Stopped") {
		t.Fatalf("a wanted listener that never bound read as %v", err)
	}
	var broken atomic.Bool
	d.tailnet.set(&tailnetServing{probe: func(context.Context) error {
		if broken.Load() {
			return errors.New("TCP connected but the TLS handshake failed")
		}
		return nil
	}}, "")
	if err := d.tailnetHealth(ctx); err != nil {
		t.Fatalf("a serving listener read as %v", err)
	}
	broken.Store(true)
	if err := d.tailnetHealth(ctx); err == nil || !strings.Contains(err.Error(), "handshake failed") {
		t.Fatalf("a stale listener read as %v", err)
	}
}

func TestSentryCheckInsFollowTheListenerAndStopOnDone(t *testing.T) {
	prev := sentryCheckIn
	t.Cleanup(func() { sentryCheckIn = prev })
	sentryCheckIn.first, sentryCheckIn.every = time.Millisecond, 10*time.Millisecond

	fake := newFakeSentry(t)
	cron, err := newSentryCron(fake.dsn(), "aterm-daemon-mac")
	if err != nil {
		t.Fatal(err)
	}
	var broken atomic.Bool
	d := newDaemon(func(string, ...any) {})
	d.tailnet.want()
	d.tailnet.set(&tailnetServing{probe: func(context.Context) error {
		if broken.Load() {
			return errors.New("handshake failed")
		}
		return nil
	}}, "")
	done, finished := make(chan struct{}), make(chan struct{})
	go func() {
		d.sentryCheckIns(done, cron)
		close(finished)
	}()
	waitFor := func(want string) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if got := fake.statuses(); len(got) > 0 && got[len(got)-1] == want {
				return
			}
			time.Sleep(time.Millisecond)
		}
		t.Fatalf("no %q check-in, saw %v", want, fake.statuses())
	}
	waitFor("ok")
	broken.Store(true)
	waitFor("error")
	broken.Store(false)
	waitFor("ok")
	close(done)
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("the check-in loop outlived done")
	}
	sent := len(fake.statuses())
	time.Sleep(40 * time.Millisecond)
	if after := len(fake.statuses()); after != sent {
		t.Fatalf("check-ins continued after done: %d then %d", sent, after)
	}
}
