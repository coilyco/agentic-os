// Native startup runs several seconds of local Git and remote fetch work
// before the harness takes the terminal. nativeProgress narrates that work so
// the wait is never a silent hang. On a terminal the narration is one row that
// rewrites itself (native_progress_live.go) and closes as a single summary
// line; anywhere else it stays line-per-step for the log. See
// docs/native-session-start.md.

package main

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// nativeProgressEnv selects how much startup narration reaches stderr.
const nativeProgressEnv = "AOS_NATIVE_PROGRESS"

type nativeProgressLevel int

const (
	nativeProgressOff nativeProgressLevel = iota
	nativeProgressSummary
	nativeProgressSteps
	nativeProgressDebug
)

const (
	// nativeProgressSlowest caps how many step timings the closing line names,
	// and nativeProgressNotable keeps instant steps out of that list.
	nativeProgressSlowest = 3
	nativeProgressNotable = 100 * time.Millisecond
)

type nativeProgress struct {
	out   io.Writer
	level nativeProgressLevel
	now   func() time.Time
	start time.Time
	spans []nativeSpan
	// live is nil unless the stream is a terminal at exactly the step level.
	// Debug asks for the transcript, so it never collapses to one row.
	live      *nativeProgressLive
	workspace string
	worktrees int
	// lineMutex keeps a parallel phase's narration lines whole.
	lineMutex sync.Mutex
}

type nativeSpan struct {
	label   string
	elapsed time.Duration
}

// nativeStep is one announced startup phase. Track records the sub-items it
// loops over so Done can attribute the phase's time to its slowest item.
type nativeStep struct {
	progress *nativeProgress
	label    string
	begun    time.Time
	slowest  nativeSpan
	items    int
	// mutex guards items and slowest, which a parallel phase updates from
	// several goroutines.
	mutex sync.Mutex
}

func newNativeProgress(out io.Writer, now func() time.Time) *nativeProgress {
	if now == nil {
		now = time.Now
	}
	progress := &nativeProgress{
		out:   out,
		level: parseNativeProgressLevel(os.Getenv(nativeProgressEnv)),
		now:   now,
		start: now(),
	}
	if progress.level == nativeProgressSteps && out != nil && nativeProgressTerminal(out) {
		progress.live = newNativeProgressLive(out, now)
	}
	return progress
}

// Writer wraps a stream so a warning written straight to it never lands on top
// of the live row. Off a terminal it hands the stream back untouched.
func (progress *nativeProgress) Writer(stream io.Writer) io.Writer {
	if progress == nil || progress.live == nil {
		return stream
	}
	return nativeProgressPassthrough{live: progress.live, stream: stream}
}

// parseNativeProgressLevel keeps step narration the default, because the
// silent multi-second wait is the problem this exists to solve.
func parseNativeProgressLevel(raw string) nativeProgressLevel {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "off", "none", "0", "false", "quiet":
		return nativeProgressOff
	case "summary", "total":
		return nativeProgressSummary
	case "debug", "verbose", "all":
		return nativeProgressDebug
	default:
		return nativeProgressSteps
	}
}

func (progress *nativeProgress) enabled(level nativeProgressLevel) bool {
	return progress != nil && progress.out != nil && progress.level >= level
}

func (progress *nativeProgress) line(verb, format string, args ...any) {
	if progress == nil || progress.out == nil {
		return
	}
	message := strings.TrimSpace(fmt.Sprintf(format, args...))
	progress.lineMutex.Lock()
	defer progress.lineMutex.Unlock()
	if progress.live != nil {
		_ = progress.live.Passthrough(func() error {
			_, err := fmt.Fprintf(progress.out, "aos: %-8s %s\n", verb, message)
			return err
		})
		return
	}
	fmt.Fprintf(progress.out, "aos: %-8s %s\n", verb, message)
}

// phase moves the narration on to a new label: one rewritten row on a
// terminal, one more line anywhere else.
func (progress *nativeProgress) phase(verb, label string) {
	if !progress.enabled(nativeProgressSteps) {
		return
	}
	if progress.live != nil {
		progress.live.Set(label)
		return
	}
	progress.line(verb, "%s", label)
}

// Begin opens the narration with the launch that is about to happen.
func (progress *nativeProgress) Begin(harness string, command []string) {
	if !progress.enabled(nativeProgressSteps) {
		return
	}
	progress.phase("launch", fmt.Sprintf("native %s startup", harness))
	if progress.enabled(nativeProgressDebug) && len(command) > 0 {
		progress.line("command", "%s", strings.Join(command, " "))
	}
}

// Step announces a phase and returns the handle that closes it.
func (progress *nativeProgress) Step(format string, args ...any) *nativeStep {
	label := fmt.Sprintf(format, args...)
	progress.phase("start", label)
	begun := time.Now()
	if progress != nil {
		begun = progress.now()
	}
	return &nativeStep{progress: progress, label: label, begun: begun}
}

// Skip reports a phase that did not need to run, so a fast startup still
// explains why it was fast.
func (progress *nativeProgress) Skip(label, format string, args ...any) {
	if !progress.enabled(nativeProgressSteps) {
		return
	}
	if progress.live != nil {
		progress.live.Set(label + " skipped")
		return
	}
	progress.line("skip", "%s (%s)", label, fmt.Sprintf(format, args...))
}

// Item narrates one unit inside a loop before that unit runs, so a stalled
// fetch or worktree names the repository it is stuck on.
func (progress *nativeProgress) Item(verb string, index, total int, format string, args ...any) {
	if !progress.enabled(nativeProgressSteps) {
		return
	}
	if progress.live != nil {
		progress.live.Detail(fmt.Sprintf("%d/%d %s", index, total, fmt.Sprintf(format, args...)))
		return
	}
	progress.line(verb, "%d/%d %s", index, total, fmt.Sprintf(format, args...))
}

// Note carries detail that only matters when someone is debugging startup.
func (progress *nativeProgress) Note(format string, args ...any) {
	if !progress.enabled(nativeProgressDebug) {
		return
	}
	progress.line("note", format, args...)
}

// Wait reports blocking on another launch's startup lock.
func (progress *nativeProgress) Wait(format string, args ...any) {
	if !progress.enabled(nativeProgressSteps) {
		return
	}
	progress.phase("wait", fmt.Sprintf(format, args...))
}

// Workspace records the session the launch just linked. The closing line
// carries it on a terminal, so the whole startup stays one row.
func (progress *nativeProgress) Workspace(path string, worktrees int) {
	if progress == nil {
		return
	}
	progress.workspace = path
	progress.worktrees = worktrees
	if progress.live == nil {
		progress.line("session", "workspace %s", path)
	}
}

// Exec names the command the harness process becomes, so any remaining wait
// is attributed to that command rather than to AOS.
func (progress *nativeProgress) Exec(command []string) {
	if !progress.enabled(nativeProgressSteps) || len(command) == 0 || progress.live != nil {
		return
	}
	progress.line("exec", "%s", command[0])
}

// Pause takes the live row off the terminal for a question the person must
// read, and Resume puts it back. Both are no-ops off a terminal.
func (progress *nativeProgress) Pause() {
	if progress != nil && progress.live != nil {
		progress.live.Pause()
	}
}

func (progress *nativeProgress) Resume() {
	if progress != nil && progress.live != nil {
		progress.live.Resume()
	}
}

// Stop ends the live row without closing the narration, so a failing launch
// does not leave a spinner turning behind its own error.
func (progress *nativeProgress) Stop() {
	if progress != nil && progress.live != nil {
		progress.live.Stop()
	}
}

// Ready closes the narration with the total and the slowest phases.
func (progress *nativeProgress) Ready() {
	progress.Stop()
	if !progress.enabled(nativeProgressSummary) {
		return
	}
	total := progress.now().Sub(progress.start)
	parts := []string{"native startup " + formatNativeDuration(total)}
	if progress.workspace != "" {
		worktrees := fmt.Sprintf("%d worktrees", progress.worktrees)
		if progress.worktrees == 1 {
			worktrees = "1 worktree"
		}
		parts = append(parts, worktrees, progress.workspace)
	}
	if detail := progress.slowestSpans(); detail != "" {
		parts = append(parts, "slowest "+detail)
	}
	progress.line("ready", "%s", strings.Join(parts, " // "))
}

func (progress *nativeProgress) slowestSpans() string {
	if progress == nil || len(progress.spans) == 0 {
		return ""
	}
	ranked := make([]nativeSpan, 0, len(progress.spans))
	for _, span := range progress.spans {
		if span.elapsed >= nativeProgressNotable {
			ranked = append(ranked, span)
		}
	}
	sort.SliceStable(ranked, func(left, right int) bool {
		return ranked[left].elapsed > ranked[right].elapsed
	})
	if len(ranked) > nativeProgressSlowest {
		ranked = ranked[:nativeProgressSlowest]
	}
	parts := make([]string, 0, len(ranked))
	for _, span := range ranked {
		parts = append(parts, span.label+" "+formatNativeDuration(span.elapsed))
	}
	return strings.Join(parts, ", ")
}

// Track records one loop item's cost so the closing step line can name the
// item that dominated the phase.
func (step *nativeStep) Track(label string, elapsed time.Duration) {
	if step == nil {
		return
	}
	step.mutex.Lock()
	defer step.mutex.Unlock()
	step.items++
	if elapsed > step.slowest.elapsed {
		step.slowest = nativeSpan{label: label, elapsed: elapsed}
	}
}

// Done closes a step. An empty detail leaves the line as label plus timing.
func (step *nativeStep) Done(format string, args ...any) {
	if step == nil {
		return
	}
	elapsed := step.close()
	// A terminal already showed this phase live and the next Set replaces it,
	// so only the log form spends a line on the close.
	if !step.progress.enabled(nativeProgressSteps) || step.progress.live != nil {
		return
	}
	var detail []string
	if summary := strings.TrimSpace(fmt.Sprintf(format, args...)); summary != "" {
		detail = append(detail, summary)
	}
	if step.items > 1 && step.slowest.label != "" {
		detail = append(detail, fmt.Sprintf("slowest %s %s",
			step.slowest.label, formatNativeDuration(step.slowest.elapsed)))
	}
	line := step.label + " " + formatNativeDuration(elapsed)
	if len(detail) > 0 {
		line += " (" + strings.Join(detail, ", ") + ")"
	}
	step.progress.line("done", "%s", line)
}

// Fail closes a step that errored. The caller still returns the error; this
// only keeps the narration honest about where the time went.
func (step *nativeStep) Fail(err error) {
	if step == nil {
		return
	}
	elapsed := step.close()
	step.progress.Stop()
	if !step.progress.enabled(nativeProgressSteps) {
		return
	}
	step.progress.line("failed", "%s %s: %v", step.label, formatNativeDuration(elapsed), err)
}

// close reads the clock once, so a step's recorded span and its printed line
// always agree.
func (step *nativeStep) close() time.Duration {
	if step.progress == nil {
		return time.Since(step.begun)
	}
	elapsed := step.progress.now().Sub(step.begun)
	step.progress.spans = append(step.progress.spans, nativeSpan{
		label:   step.label,
		elapsed: elapsed,
	})
	return elapsed
}

func formatNativeDuration(elapsed time.Duration) string {
	seconds := elapsed.Seconds()
	if seconds < 0 {
		seconds = 0
	}
	if seconds < 10 {
		return fmt.Sprintf("%.2fs", seconds)
	}
	return fmt.Sprintf("%.1fs", seconds)
}
