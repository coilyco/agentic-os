package main

import (
	"errors"
	"time"

	"github.com/getsentry/sentry-go"
)

// sentryFlushWait bounds how long a dying daemon waits for its event to leave.
var sentryFlushWait = 2 * time.Second

// sentryCapture sends the daemon's panics to Sentry through its own hub, so
// nothing else in the process reports. See docs/aterm-bundles.md.
type sentryCapture struct {
	hub *sentry.Hub
}

// newSentryCapture reads the same DSN as the cron check-in. The SDK's own
// parse error is dropped, since the DSN is a credential.
func newSentryCapture(dsn, host, release string) (*sentryCapture, error) {
	// Synchronous, so the event has left or timed out when record returns. The
	// queued transport lost events to a flush that raced the queue.
	transport := sentry.NewHTTPSyncTransport()
	transport.Timeout = sentryFlushWait
	client, err := sentry.NewClient(sentry.ClientOptions{Dsn: dsn, ServerName: host, Release: release, Transport: transport})
	if err != nil {
		return nil, errors.New("the Sentry DSN was rejected")
	}
	return &sentryCapture{hub: sentry.NewHub(client, sentry.NewScope())}, nil
}

// record sends one panic as a fatal event, waiting for it to leave because the
// caller is about to crash the process.
func (c *sentryCapture) record(where string, panicked any) {
	c.hub.WithScope(func(scope *sentry.Scope) {
		scope.SetLevel(sentry.LevelFatal)
		scope.SetTag("goroutine", where)
		c.hub.Recover(panicked)
	})
	c.flush()
}

func (c *sentryCapture) flush() {
	c.hub.Flush(sentryFlushWait)
}

// guard is deferred at the top of a goroutine the daemon cannot lose quietly.
// It captures a panic and raises it again, and switched off recovers nothing.
func (d *daemon) guard(where string) {
	if d == nil || d.capture == nil {
		return
	}
	panicked := recover()
	if panicked == nil {
		return
	}
	d.capture.record(where, panicked)
	panic(panicked)
}
