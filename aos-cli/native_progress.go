// Native startup runs several seconds of local Git and remote fetch work
// before the harness takes the terminal. nativeProgress narrates that work so
// the wait is never a silent hang: every step announces itself before it
// begins, reports its own elapsed time when it ends, and the run closes with a
// total plus the steps that dominated it. See docs/native-session-start.md.

package main

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
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
}

func newNativeProgress(out io.Writer, now func() time.Time) *nativeProgress {
	if now == nil {
		now = time.Now
	}
	return &nativeProgress{
		out:   out,
		level: parseNativeProgressLevel(os.Getenv(nativeProgressEnv)),
		now:   now,
		start: now(),
	}
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
	fmt.Fprintf(progress.out, "aos: %-8s %s\n", verb, message)
}

// Begin opens the narration with the launch that is about to happen.
func (progress *nativeProgress) Begin(harness string, command []string) {
	if !progress.enabled(nativeProgressSteps) {
		return
	}
	progress.line("launch", "native %s startup", harness)
	if progress.enabled(nativeProgressDebug) && len(command) > 0 {
		progress.line("command", "%s", strings.Join(command, " "))
	}
}

// Step announces a phase and returns the handle that closes it.
func (progress *nativeProgress) Step(format string, args ...any) *nativeStep {
	label := fmt.Sprintf(format, args...)
	if progress.enabled(nativeProgressSteps) {
		progress.line("start", "%s", label)
	}
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
	progress.line("skip", "%s (%s)", label, fmt.Sprintf(format, args...))
}

// Item narrates one unit inside a loop before that unit runs, so a stalled
// fetch or worktree names the repository it is stuck on.
func (progress *nativeProgress) Item(verb string, index, total int, format string, args ...any) {
	if !progress.enabled(nativeProgressSteps) {
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
	progress.line("wait", format, args...)
}

// Exec names the command the harness process becomes, so any remaining wait
// is attributed to that command rather than to AOS.
func (progress *nativeProgress) Exec(command []string) {
	if !progress.enabled(nativeProgressSteps) || len(command) == 0 {
		return
	}
	progress.line("exec", "%s", command[0])
}

// Ready closes the narration with the total and the slowest phases.
func (progress *nativeProgress) Ready() {
	if !progress.enabled(nativeProgressSummary) {
		return
	}
	total := progress.now().Sub(progress.start)
	detail := progress.slowestSpans()
	if detail == "" {
		progress.line("ready", "native startup %s", formatNativeDuration(total))
		return
	}
	progress.line("ready", "native startup %s (%s)", formatNativeDuration(total), detail)
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
	if !step.progress.enabled(nativeProgressSteps) {
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
