package main

import (
	"net"
	"testing"
	"time"
)

func queuedMessage(d *daemon, text string) *pendingSend {
	return &pendingSend{
		msg:    peerMessage{ID: randomID(6), Target: "claude-a", State: "queued"},
		text:   text,
		target: "claude-a",
		done:   make(chan struct{}),
		d:      d,
	}
}

// A session ending mid-submit has taken the bytes, so that message is delivered and the
// one queued behind it fails (COI-2490). submitHeld pins the order (COI-2526).
func TestFinishMidSubmitDeliversTheMessageBeingWrittenAndFailsTheQueuedOne(t *testing.T) {
	daemonEnd, holderEnd := net.Pipe()
	t.Cleanup(func() { _ = holderEnd.Close() })
	go func() {
		peer := newConn(holderEnd)
		for {
			if _, err := peer.read(); err != nil {
				return
			}
		}
	}()
	d := newDaemon(func(string, ...any) {})
	started := time.Now().Add(-time.Minute)
	s := &ptySession{
		d: d, name: "claude-a", seat: "claude", started: started, promptSeen: started,
		holder: newConn(daemonEnd), clients: map[*conn]bool{},
		done: make(chan struct{}), forgotten: make(chan struct{}),
	}
	d.sessions[s.name] = s
	writing, queued := queuedMessage(d, "being written"), queuedMessage(d, "still queued")
	s.pending = []*pendingSend{writing, queued}
	typed, release := make(chan struct{}), make(chan struct{})
	s.submitHeld = func() {
		close(typed)
		<-release
	}

	returned := make(chan struct{})
	go func() {
		defer close(returned)
		s.deliverNext(time.Now())
	}()
	select {
	case <-typed:
	case <-time.After(5 * time.Second):
		t.Fatal("submit never reached the hold")
	}
	s.mu.Lock()
	sending := s.sending
	s.mu.Unlock()
	if sending != writing {
		t.Fatalf("sending = %v, want the message being written", sending)
	}

	s.finish(0)

	if got := queued.snapshot(); got.State != "failed" || got.Reason != "claude-a ended before it was delivered" {
		t.Fatalf("queued message = %+v, want failed with %q", got, "claude-a ended before it was delivered")
	}
	if got := writing.snapshot(); finalState(got.State) {
		t.Fatalf("the message being written was settled by finish: %+v", got)
	}

	close(release)
	select {
	case <-returned:
	case <-time.After(5 * time.Second):
		t.Fatal("submit never returned after the session ended")
	}
	if got := writing.snapshot(); got.State != "delivered" {
		t.Fatalf("message being written = %+v, want delivered", got)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sending != nil || len(s.pending) != 0 {
		t.Fatalf("sending = %v, pending = %d, want both cleared", s.sending, len(s.pending))
	}
}
