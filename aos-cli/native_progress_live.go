// A terminal wants proof the wait is moving, not a transcript of it. The live
// line rewrites itself in place, so seventeen fetches and seventeen worktrees
// leave one row on screen instead of thirty-four. Anything that is not a
// terminal keeps the line-per-step form, which is the one a log is read from
// later. See docs/native-session-start.md.

package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"
)

const (
	nativeProgressTick  = 90 * time.Millisecond
	nativeProgressWidth = 120
)

// Braille frames advance visibly at the tick above and cost one cell.
var nativeProgressFrames = []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")

// nativeProgressLive owns the single rewritten row. Every field behind the
// mutex is touched by the redraw goroutine as well as the launch goroutine.
type nativeProgressLive struct {
	out     io.Writer
	now     func() time.Time
	mutex   sync.Mutex
	frame   int
	label   string
	detail  string
	begun   time.Time
	painted int
	paused  bool
	stopped bool
	done    chan struct{}
}

// nativeProgressTerminal reports whether the stream can carry a rewritten row.
// A variable so a test reaches the live branch without owning a terminal.
var nativeProgressTerminal = func(stream io.Writer) bool {
	file, ok := stream.(*os.File)
	return ok && term.IsTerminal(int(file.Fd()))
}

func newNativeProgressLive(out io.Writer, now func() time.Time) *nativeProgressLive {
	live := &nativeProgressLive{out: out, now: now, begun: now(), done: make(chan struct{})}
	go live.animate()
	return live
}

func (live *nativeProgressLive) animate() {
	ticker := time.NewTicker(nativeProgressTick)
	defer ticker.Stop()
	for {
		select {
		case <-live.done:
			return
		case <-ticker.C:
			live.mutex.Lock()
			live.frame++
			live.paint()
			live.mutex.Unlock()
		}
	}
}

// Set opens a new phase on the row and restarts its elapsed clock.
func (live *nativeProgressLive) Set(label string) {
	live.mutex.Lock()
	defer live.mutex.Unlock()
	live.label = label
	live.detail = ""
	live.begun = live.now()
	live.paint()
}

// Detail names the item the phase is on, so a stalled fetch says which
// repository it is stalled on without spending a row on every other one.
func (live *nativeProgressLive) Detail(detail string) {
	live.mutex.Lock()
	defer live.mutex.Unlock()
	live.detail = detail
	live.paint()
}

// Passthrough lets a direct message cross the row: the row is erased, the
// message lands on a clean line, and the row comes back underneath it.
func (live *nativeProgressLive) Passthrough(write func() error) error {
	live.mutex.Lock()
	defer live.mutex.Unlock()
	live.erase()
	err := write()
	live.paint()
	return err
}

// Pause erases the row and holds it off the terminal until Resume, so a prompt
// waiting on the person is not repainted under their cursor.
func (live *nativeProgressLive) Pause() {
	live.mutex.Lock()
	defer live.mutex.Unlock()
	live.paused = true
	live.erase()
}

// Resume brings the row back below whatever the pause left on screen.
func (live *nativeProgressLive) Resume() {
	live.mutex.Lock()
	defer live.mutex.Unlock()
	live.paused = false
	live.paint()
}

// Stop ends the animation and leaves the cursor on an empty line, so whatever
// prints next owns the row.
func (live *nativeProgressLive) Stop() {
	live.mutex.Lock()
	defer live.mutex.Unlock()
	if live.stopped {
		return
	}
	live.stopped = true
	close(live.done)
	live.erase()
}

// paint rewrites the row in place. The caller holds the mutex.
func (live *nativeProgressLive) paint() {
	if live.stopped || live.paused || strings.TrimSpace(live.label) == "" {
		return
	}
	frame := nativeProgressFrames[live.frame%len(nativeProgressFrames)]
	row := fmt.Sprintf("aos %c %s", frame, live.label)
	if live.detail != "" {
		row += " // " + live.detail
	}
	row += " " + formatNativeDuration(live.now().Sub(live.begun))
	cells := []rune(row)
	if len(cells) > nativeProgressWidth {
		cells = cells[:nativeProgressWidth]
		row = string(cells)
	}
	// Every write starts at column zero, so the row only has to cover what the
	// previous one left behind.
	pad := live.painted - len(cells)
	if pad < 0 {
		pad = 0
	}
	fmt.Fprintf(live.out, "\r%s%s", row, strings.Repeat(" ", pad))
	live.painted = len(cells)
}

// erase clears whatever the row last painted. The caller holds the mutex.
func (live *nativeProgressLive) erase() {
	if live.painted == 0 {
		return
	}
	fmt.Fprintf(live.out, "\r%s\r", strings.Repeat(" ", live.painted))
	live.painted = 0
}

// nativeProgressPassthrough is the stderr seam that keeps a warning off the
// live row. docs/native-session-start.md
type nativeProgressPassthrough struct {
	live   *nativeProgressLive
	stream io.Writer
}

func (writer nativeProgressPassthrough) Write(payload []byte) (int, error) {
	written := 0
	err := writer.live.Passthrough(func() error {
		count, err := writer.stream.Write(payload)
		written = count
		return err
	})
	return written, err
}
