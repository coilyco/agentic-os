package main

import (
	"strings"
	"testing"
)

// local is the daemon's user management as a test reaches into it. A daemon
// built by newDaemon always carries localUsers.
func (d *daemon) local() *localUsers { return d.users.(*localUsers) }

// The push goroutines defer host.guard, a func value over the method. It must
// still capture the panic and raise it again, or a crash there goes unreported.
func TestHostGuardCapturesAndRaisesAgainThroughTheSeam(t *testing.T) {
	fake := newFakeIngest(t)
	capture, err := newSentryCapture(fake.dsn(), "h", "r")
	if err != nil {
		t.Fatal(err)
	}
	d := newDaemon(func(string, ...any) {})
	d.capture = capture
	crashed := crashOf(func() {
		defer d.userHost().guard("push send")
		panic("boom")
	})
	if crashed != "boom" {
		t.Fatalf("the guard raised %v, not the original panic", crashed)
	}
	if _, envelopes := fake.received(); len(envelopes) != 1 || !strings.Contains(envelopes[0], `"goroutine":"push send"`) {
		t.Fatalf("the ingest saw %v", envelopes)
	}
}
