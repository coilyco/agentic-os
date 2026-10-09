package main

import (
	"net"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestChooseSizeIsTheLargestBoxHeldToTheSmallestRawClient(t *testing.T) {
	a, b, c := &conn{}, &conn{}, &conn{}
	for name, test := range map[string]struct {
		reported   map[*conn]clientSize
		rows, cols int
		ok         bool
	}{
		"nobody":                   {nil, 0, 0, false},
		"one phone alone":          {map[*conn]clientSize{a: {20, 40, true}}, 20, 40, true},
		"phone beside a desktop":   {map[*conn]clientSize{a: {20, 40, true}, b: {50, 200, true}}, 50, 200, true},
		"widest and tallest apart": {map[*conn]clientSize{a: {60, 80, true}, b: {30, 200, true}}, 60, 200, true},
		"a raw window caps":        {map[*conn]clientSize{a: {50, 200, true}, b: {40, 100, false}}, 40, 100, true},
		"a raw window larger":      {map[*conn]clientSize{a: {20, 40, true}, b: {40, 100, false}}, 40, 100, true},
		"two raw windows":          {map[*conn]clientSize{a: {40, 100, false}, b: {50, 80, false}, c: {60, 300, true}}, 40, 80, true},
	} {
		rows, cols, ok := chooseSize(test.reported)
		if rows != test.rows || cols != test.cols || ok != test.ok {
			t.Fatalf("%s: got %dx%d ok=%v, want %dx%d ok=%v", name, rows, cols, ok, test.rows, test.cols, test.ok)
		}
	}
}

// frameSink collects what arrives at the far end of a pipe.
type frameSink struct {
	mu     sync.Mutex
	frames []frame
}

func pipeConn(t *testing.T) (*conn, *frameSink) {
	t.Helper()
	near, far := net.Pipe()
	t.Cleanup(func() { _ = near.Close(); _ = far.Close() })
	sink := &frameSink{}
	go func() {
		peer := newConn(far)
		for {
			message, err := peer.read()
			if err != nil {
				return
			}
			sink.mu.Lock()
			sink.frames = append(sink.frames, message)
			sink.mu.Unlock()
		}
	}()
	return newConn(near), sink
}

// last waits for the newest frame of a type to have this size.
func (f *frameSink) wait(t *testing.T, typ string, rows, cols int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		f.mu.Lock()
		for i := len(f.frames) - 1; i >= 0; i-- {
			if f.frames[i].Type == typ {
				got := f.frames[i]
				f.mu.Unlock()
				if got.Rows == rows && got.Cols == cols {
					return
				}
				goto retry
			}
		}
		f.mu.Unlock()
	retry:
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("no %s frame of %dx%d, saw %+v", typ, rows, cols, f.frames)
}

func (f *frameSink) count(typ string) (n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, message := range f.frames {
		if message.Type == typ {
			n++
		}
	}
	return n
}

func sizedSession(t *testing.T) (*ptySession, *frameSink) {
	t.Helper()
	holder, holderSink := pipeConn(t)
	s := &ptySession{
		d: newDaemon(func(string, ...any) {}), name: "claude-a", holder: holder, clients: map[*conn]bool{},
		done: make(chan struct{}), forgotten: make(chan struct{}),
	}
	return s, holderSink
}

func TestAPhoneAttachingNeverShrinksADesktopAndLeavingRestoresTheSmaller(t *testing.T) {
	s, holder := sizedSession(t)
	desktop, desktopSink := pipeConn(t)
	phone, phoneSink := pipeConn(t)
	yes := true

	s.attach(desktop, false, 0)
	s.report(desktop, 50, 200, &yes)
	holder.wait(t, "resize", 50, 200)
	desktopSink.wait(t, "size", 50, 200)

	s.attach(phone, false, 0)
	s.report(phone, 20, 40, &yes)
	if frame, ok := s.sizeFrame(); !ok || frame.Rows != 50 || frame.Cols != 200 {
		t.Fatalf("a phone attaching changed the size to %+v", frame)
	}
	s.report(phone, 22, 42, nil)
	if holder.count("resize") != 1 {
		t.Fatalf("a phone's reports resized the PTY %d times, want once", holder.count("resize"))
	}

	// The desktop leaves, so the survivor is told the smaller size.
	s.detach(desktop)
	holder.wait(t, "resize", 22, 42)
	phoneSink.wait(t, "size", 22, 42)

	// A desktop arriving later grows it and the phone leaves it alone.
	s.attach(desktop, false, 0)
	s.report(desktop, 50, 200, &yes)
	holder.wait(t, "resize", 50, 200)
	phoneSink.wait(t, "size", 50, 200)
}

func TestARawWindowCapsTheSizeAndAClientThatNeverReportedConstrainsNothing(t *testing.T) {
	s, holder := sizedSession(t)
	kitty, _ := pipeConn(t)
	web, _ := pipeConn(t)
	watcher, _ := pipeConn(t)
	yes := true

	s.attach(kitty, false, 0)
	s.report(kitty, 40, 100, nil)
	s.attach(web, false, 0)
	s.report(web, 50, 200, &yes)
	holder.wait(t, "resize", 40, 100)
	if frame, _ := s.sizeFrame(); frame.Rows != 40 || frame.Cols != 100 {
		t.Fatalf("a raw window beside a larger web client left the size at %+v", frame)
	}

	// A watcher sends no box, so it neither caps nor grows anything.
	s.attach(watcher, false, 0)
	s.report(watcher, 0, 0, nil)
	if frame, _ := s.sizeFrame(); frame.Rows != 40 || frame.Cols != 100 {
		t.Fatalf("a watcher moved the size to %+v", frame)
	}

	// With the raw window gone the web client gets its own size.
	s.detach(kitty)
	holder.wait(t, "resize", 50, 200)
}

func TestTheSoleClientIsToldTheSizeAndNobodyLeftKeepsIt(t *testing.T) {
	s, _ := sizedSession(t)
	only, _ := pipeConn(t)
	if _, ok := s.sizeFrame(); ok {
		t.Fatal("a session nobody sized reported a size")
	}
	s.attach(only, false, 0)
	s.report(only, 30, 90, nil)
	if frame, ok := s.sizeFrame(); !ok || frame.Rows != 30 || frame.Cols != 90 {
		t.Fatalf("the sole client's size read as %+v ok=%v", frame, ok)
	}
	s.detach(only)
	if frame, ok := s.sizeFrame(); !ok || frame.Rows != 30 || frame.Cols != 90 {
		t.Fatalf("a session with nobody attached lost its size: %+v ok=%v", frame, ok)
	}
}

// nextSize reads frames until the daemon pushes a size, or fails.
func (tc *testClient) nextSize() frame {
	tc.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		_ = tc.c.raw.SetReadDeadline(deadline)
		message, err := tc.c.read()
		if err != nil {
			tc.t.Fatalf("no size frame: %v", err)
		}
		if message.Type == "size" {
			return message
		}
	}
}

func TestAttachSizesTheRealPTYToTheLargestClient(t *testing.T) {
	testDaemon(t)
	name := "eng-platform-beetle-ox"
	dialTest(t).spawn(name, "eng-platform", "Beetle-Ox", `while :; do stty size; sleep 0.1; done`)

	desktop := dialTest(t)
	if !slices.Contains(desktop.c.features, ptySizeFeature) {
		t.Fatalf("welcome lacks %s, so a client keeps fitting and sending", ptySizeFeature)
	}
	if _, err := desktop.c.request(frame{Type: "attach", Session: name, Rows: 50, Cols: 200, Scales: true}); err != nil {
		t.Fatal(err)
	}
	if size := desktop.nextSize(); size.Session != name || size.Rows != 50 || size.Cols != 200 {
		t.Fatalf("the desktop was told %+v", size)
	}

	phone := dialTest(t)
	if _, err := phone.c.request(frame{Type: "attach", Session: name, Rows: 20, Cols: 40, Scales: true}); err != nil {
		t.Fatal(err)
	}
	if size := phone.nextSize(); size.Rows != 50 || size.Cols != 200 {
		t.Fatalf("a phone beside a desktop was told %+v, want the desktop's size", size)
	}
	phone.until("50 200")
	if strings.Contains(phone.output.String(), "20 40") {
		t.Fatalf("the PTY took the phone's size:\n%q", phone.output.String())
	}
}
