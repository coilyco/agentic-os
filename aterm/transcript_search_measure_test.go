package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// A measurement for COI-3797, not a check. It runs only with ATERM_MEASURE_SEARCH set.
// How it was run and what it found are on the COI-3797 record.

type searchSample struct {
	wall time.Duration
	cpu  time.Duration
}

func rusageCPU() (time.Duration, int64) {
	var usage syscall.Rusage
	_ = syscall.Getrusage(syscall.RUSAGE_SELF, &usage)
	cpu := time.Duration(usage.Utime.Nano() + usage.Stime.Nano())
	return cpu, usage.Maxrss
}

func measureRuns(t *testing.T, label string, runs int, run func() int) {
	t.Helper()
	var samples []searchSample
	matches := 0
	for index := 0; index < runs; index++ {
		cpuBefore, _ := rusageCPU()
		start := time.Now()
		matches = run()
		wall := time.Since(start)
		cpuAfter, _ := rusageCPU()
		samples = append(samples, searchSample{wall: wall, cpu: cpuAfter - cpuBefore})
	}
	_, maxrss := rusageCPU()
	var walls []string
	for _, sample := range samples {
		walls = append(walls, fmt.Sprintf("%dms/%.2fcores", sample.wall.Milliseconds(), float64(sample.cpu)/float64(sample.wall)))
	}
	sorted := append([]searchSample(nil), samples[1:]...)
	sort.Slice(sorted, func(a, b int) bool { return sorted[a].wall < sorted[b].wall })
	median := time.Duration(0)
	if len(sorted) > 0 {
		median = sorted[len(sorted)/2].wall
	}
	t.Logf("MEASURE %s: first %dms, warm median %dms, runs [%s], matches %d, peak RSS %d MiB",
		label, samples[0].wall.Milliseconds(), median.Milliseconds(), strings.Join(walls, " "), matches, maxrss>>20)
}

// synthClaude writes a claude transcript of about size bytes, a phrase in its first line.
func synthClaude(t *testing.T, path string, size int64) {
	t.Helper()
	if info, err := os.Stat(path); err == nil && info.Size() >= size {
		return
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	out := bufio.NewWriterSize(file, 1<<20)
	defer out.Flush()
	fmt.Fprintln(out, `{"type":"user","uuid":"early","parentUuid":"","timestamp":"2026-11-27T08:00:00.000Z","message":{"role":"user","content":"check the lighthouse migration before lunch"},"origin":{"kind":"human"}}`)
	output := strings.Repeat("ok line of tool output with some words in it\\n", 40)
	reply := strings.Repeat("A sentence of a reply that reads like a seat explaining its work. ", 12)
	var written int64
	for n := 0; written < size; n++ {
		lines := fmt.Sprintf(`{"type":"assistant","uuid":"a%d","parentUuid":"p","timestamp":"2026-11-27T09:00:00.000Z","message":{"id":"msg_%d","model":"m","stop_reason":"tool_use","content":[{"type":"text","text":"%s"},{"type":"tool_use","id":"toolu_%d","name":"Bash","input":{"command":"just test %d"}}]}}`+"\n"+
			`{"type":"user","uuid":"u%d","parentUuid":"a%d","timestamp":"2026-11-27T09:00:01.000Z","message":{"role":"user","content":[{"tool_use_id":"toolu_%d","type":"tool_result","content":"%s"}]}}`+"\n", n, n, reply, n, n, n, n, n, output)
		written += int64(len(lines))
		out.WriteString(lines)
	}
}

func TestMeasureTranscriptSearch(t *testing.T) {
	mode := os.Getenv("ATERM_MEASURE_SEARCH")
	if mode == "" {
		t.Skip("a measurement, run with ATERM_MEASURE_SEARCH set")
	}
	query := firstNonEmpty(os.Getenv("ATERM_MEASURE_QUERY"), "lighthouse")
	runs := 6
	one := func(path string) int {
		keyed, err := fileItems("claude", path, func(lines []jsonLine) parsedLines { return parseClaude(lines, nil) })
		if err != nil {
			t.Fatal(err)
		}
		return searchTranscript(keyed, query).Total
	}
	switch mode {
	case "one":
		path := os.Getenv("ATERM_MEASURE_FILE")
		measureRuns(t, fmt.Sprintf("one seat %q, %d MiB", query, fileSize(path)>>20), runs, func() int { return one(path) })
	case "synth":
		path := os.Getenv("ATERM_MEASURE_FILE")
		synthClaude(t, path, transcriptMaxWindow-1<<20)
		measureRuns(t, fmt.Sprintf("synthetic %q, %d MiB", query, fileSize(path)>>20), runs, func() int { return one(path) })
	case "list":
		// The files of every seat live now, searched one after another as searchEverySeat does.
		paths := readPaths(t, os.Getenv("ATERM_MEASURE_LIST"))
		measureRuns(t, fmt.Sprintf("every live seat (%d) %q", len(paths), query), runs, func() int {
			total := 0
			for _, path := range paths {
				total += one(path)
			}
			return total
		})
	case "ten", "ten-churn", "ten-concurrent":
		d, recorded := measureSeats(t, os.Getenv("ATERM_MEASURE_DIR"), 10, 12_800_000)
		defer func() {
			t.Logf("MEASURE cache holds %d MiB of %d MiB", d.searching.cachedBytes()>>20, transcriptCacheBudget>>20)
		}()
		if mode != "ten-concurrent" {
			measureRuns(t, fmt.Sprintf("%s over 10 x 12.8 MB %q", mode, query), runs, func() int {
				// Three seats at work write a line before each search.
				for index := 0; mode == "ten-churn" && index < 3; index++ {
					growSeat(t, os.Getenv("ATERM_MEASURE_DIR"), index)
				}
				return seatTotal(everySeat(t, d, recorded, query))
			})
			return
		}
		// Three searches at once, as three devices, or keystrokes past the debounce, send them.
		measureRuns(t, fmt.Sprintf("3 concurrent searchEverySeat over 10 x 12.8 MB %q", query), 3, func() int {
			var wait sync.WaitGroup
			for range 3 {
				wait.Add(1)
				go func() { defer wait.Done(); everySeat(t, d, recorded, query) }()
			}
			wait.Wait()
			return 0
		})
	default:
		t.Fatalf("unknown mode %q", mode)
	}
}

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

func readPaths(t *testing.T, list string) []string {
	raw, err := os.ReadFile(list)
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if fields := strings.Fields(line); len(fields) == 2 {
			paths = append(paths, fields[1])
		}
	}
	return paths
}

func seatTotal(results []transcriptSeatResult) int {
	total := 0
	for _, result := range results {
		total += result.Total
	}
	return total
}

// measureSeats makes count claude seats of size bytes under dir, recorded in a ledger
// there, and a daemon that holds them as live.
func measureSeats(t *testing.T, dir string, count int, size int64) (*daemon, []ledgerEntry) {
	t.Helper()
	state := filepath.Join(dir, "state")
	d := newDaemon(func(string, ...any) {})
	d.ledgerDir = state
	for n := range count {
		name := fmt.Sprintf("eng-platform-measure-%02d", n)
		home := filepath.Join(dir, name)
		id := fmt.Sprintf("00000000-0000-4000-8000-%012d", n)
		project := filepath.Join(home, ".claude", "projects", "measure")
		if err := os.MkdirAll(project, 0o700); err != nil {
			t.Fatal(err)
		}
		synthClaude(t, filepath.Join(project, id+".jsonl"), size)
		if err := writeLedger(state, ledgerEntry{Name: name, Role: "eng-platform", Seat: "claude", Conversation: id, Home: home, Started: time.Now().UTC()}); err != nil {
			t.Fatal(err)
		}
		d.sessions[name] = &ptySession{d: d, name: name, seat: "claude", done: make(chan struct{})}
	}
	return d, readLedger(state)
}

func growSeat(t *testing.T, dir string, n int) {
	path := filepath.Join(dir, fmt.Sprintf("eng-platform-measure-%02d", n), ".claude", "projects", "measure", fmt.Sprintf("00000000-0000-4000-8000-%012d.jsonl", n))
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	fmt.Fprintf(file, `{"type":"assistant","uuid":"g%d","parentUuid":"p","timestamp":"2026-11-28T09:00:00.000Z","message":{"id":"msg_g%d","model":"m","stop_reason":"end_turn","content":[{"type":"text","text":"another line"}]}}`+"\n", time.Now().UnixNano(), time.Now().UnixNano())
}
