package main

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeIngest is the Sentry event endpoint, never the real one. It keeps each
// envelope the SDK posts.
type fakeIngest struct {
	server    *httptest.Server
	mu        sync.Mutex
	paths     []string
	envelopes []string
}

func newFakeIngest(t *testing.T) *fakeIngest {
	t.Helper()
	fake := &fakeIngest{}
	fake.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reader io.Reader = r.Body
		if r.Header.Get("Content-Encoding") == "gzip" {
			if zipped, err := gzip.NewReader(r.Body); err == nil {
				reader = zipped
			}
		}
		raw, _ := io.ReadAll(reader)
		fake.mu.Lock()
		fake.paths, fake.envelopes = append(fake.paths, r.Method+" "+r.URL.Path), append(fake.envelopes, string(raw))
		fake.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(fake.server.Close)
	return fake
}

func (f *fakeIngest) dsn() string {
	return "http://" + testDSNKey + "@" + strings.TrimPrefix(f.server.URL, "http://") + "/7"
}

func (f *fakeIngest) received() (paths, envelopes []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.paths...), append([]string(nil), f.envelopes...)
}

// crashOf runs fn as a goroutine and returns the value it ends in a panic with,
// the way the runtime would see it once the guard has raised it again.
func crashOf(fn func()) any {
	crashed := make(chan any, 1)
	go func() {
		defer func() { crashed <- recover() }()
		fn()
	}()
	return <-crashed
}

func TestPanicInAGuardedDaemonGoroutineReachesTheIngestAndStillCrashes(t *testing.T) {
	prev := sentryCheckIn
	t.Cleanup(func() { sentryCheckIn = prev })
	sentryCheckIn.first = time.Millisecond

	fake := newFakeIngest(t)
	capture, err := newSentryCapture(fake.dsn(), "test-host", "aos-test")
	if err != nil {
		t.Fatal(err)
	}
	d := newDaemon(func(string, ...any) {})
	d.capture = capture

	// A nil cron panics on the first check-in, inside the real check-in goroutine.
	crashed := crashOf(func() { d.sentryCheckIns(make(chan struct{}), nil) })
	if crashed == nil {
		t.Fatal("the guard swallowed the panic, so the daemon would live on")
	}

	paths, envelopes := fake.received()
	if len(envelopes) != 1 || paths[0] != "POST /api/7/envelope/" {
		t.Fatalf("ingest saw %v", paths)
	}
	for _, want := range []string{`"level":"fatal"`, `"goroutine":"check-in"`, `"server_name":"test-host"`, `"release":"aos-test"`, "nil pointer dereference"} {
		if !strings.Contains(envelopes[0], want) {
			t.Fatalf("the event lacks %s:\n%s", want, envelopes[0])
		}
	}
}

func TestGuardRaisesTheSamePanicValueAgain(t *testing.T) {
	fake := newFakeIngest(t)
	capture, err := newSentryCapture(fake.dsn(), "h", "r")
	if err != nil {
		t.Fatal(err)
	}
	d := newDaemon(func(string, ...any) {})
	d.capture = capture
	crashed := crashOf(func() {
		defer d.guard("worker")
		panic("boom")
	})
	if crashed != "boom" {
		t.Fatalf("the goroutine ended in %v, want the original panic", crashed)
	}
	if _, envelopes := fake.received(); len(envelopes) != 1 || !strings.Contains(envelopes[0], "boom") {
		t.Fatalf("ingest saw %v", envelopes)
	}
}

func TestGuardIsInertWithoutADSNAndWithoutNoise(t *testing.T) {
	d := newDaemon(func(string, ...any) {})
	if crashed := crashOf(func() {
		defer d.guard("worker")
		panic("boom")
	}); crashed != "boom" {
		t.Fatalf("a switched-off guard ended the goroutine in %v", crashed)
	}
	var none *daemon
	if crashed := crashOf(func() {
		defer none.guard("worker")
		panic("boom")
	}); crashed != "boom" {
		t.Fatalf("a nil daemon's guard ended the goroutine in %v", crashed)
	}

	fake := newFakeIngest(t)
	capture, err := newSentryCapture(fake.dsn(), "h", "r")
	if err != nil {
		t.Fatal(err)
	}
	d.capture = capture
	if crashed := crashOf(func() { defer d.guard("worker") }); crashed != nil {
		t.Fatalf("a goroutine that did not panic ended in %v", crashed)
	}
	if _, envelopes := fake.received(); len(envelopes) != 0 {
		t.Fatalf("a clean return sent %d events", len(envelopes))
	}
}

func TestSentryCaptureErrorNeverQuotesTheKey(t *testing.T) {
	for _, bad := range []string{"https://" + testDSNKey + "@o42.ingest.sentry.io/", "ftp://" + testDSNKey + "@o42.ingest.sentry.io/7", testDSNKey + "@@@"} {
		if _, err := newSentryCapture(bad, "h", "r"); err == nil {
			t.Fatalf("DSN %q was accepted", bad)
		} else if strings.Contains(err.Error(), testDSNKey) {
			t.Fatalf("the error for %q quotes the key: %v", bad, err)
		}
	}
}

// A DSN both parsers reject starts no check-in loop, whose pacing global the
// check-in tests rewrite.
func TestDaemonLogNeverQuotesARejectedDSN(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "aterm-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	log := &syncBuffer{}
	stop, stopped := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(stopped)
		dsn := "https://" + testDSNKey + "@o42.ingest.sentry.io/"
		_ = runDaemon(daemonOptions{Socket: filepath.Join(dir, "d.sock"), Idle: time.Hour, SentryDSN: dsn, Stop: stop}, log)
	}()
	time.Sleep(200 * time.Millisecond)
	close(stop)
	select {
	case <-stopped:
	case <-time.After(10 * time.Second):
		t.Fatal("the daemon did not stop")
	}
	if !strings.Contains(log.String(), "no Sentry error capture") {
		t.Fatalf("the rejection was not logged:\n%s", log.String())
	}
	if strings.Contains(log.String(), testDSNKey) {
		t.Fatalf("the daemon log quotes the DSN key:\n%s", log.String())
	}
}

// syncBuffer lets the daemon's goroutines and the test share one log.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}
