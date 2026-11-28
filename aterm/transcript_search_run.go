package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"
)

// How a search runs (COI-3797): off the read loop, at most transcriptSearchSlots
// at once, a newer one from the same connection cancelling the last, over cached items.

const (
	transcriptSearchSlots = 2
	// transcriptItemOverhead is what an item costs beyond its text, for the budget.
	transcriptItemOverhead = 160
	reasonSearchCancelled  = "search_cancelled"
)

var errSearchCancelled = errors.New("a newer search replaced this one")

// transcriptCacheBudget bounds the items cached across every seat. A test narrows it.
var transcriptCacheBudget int64 = 256 << 20

// searchRunner is the daemon's search state. The zero value is ready.
type searchRunner struct {
	once  sync.Once
	slots chan struct{}
	mu    sync.Mutex
	cache map[string]*cachedTranscript
	bytes int64
	// misses counts the reads the cache could not answer, for its test.
	misses int
}

// cachedTranscript is one seat's parsed items, valid while its files are unchanged.
type cachedTranscript struct {
	stamp string
	keyed keyedItems
	bytes int64
	used  time.Time
}

func (r *searchRunner) init() {
	r.once.Do(func() {
		r.slots = make(chan struct{}, transcriptSearchSlots)
		r.cache = map[string]*cachedTranscript{}
	})
}

// acquire waits for a slot, or for ctx to end.
func (r *searchRunner) acquire(ctx context.Context) (func(), error) {
	r.init()
	select {
	case r.slots <- struct{}{}:
		return func() { <-r.slots }, nil
	case <-ctx.Done():
		return nil, searchCancelled()
	}
}

func searchCancelled() error {
	return withReason(reasonSearchCancelled, errSearchCancelled)
}

// cachedBytes is what the cache holds now, for the budget and its test.
func (r *searchRunner) cachedBytes() int64 {
	r.init()
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.bytes
}

// drop forgets a seat's items, as its session ends.
func (r *searchRunner) drop(name string) {
	r.init()
	r.mu.Lock()
	defer r.mu.Unlock()
	if entry := r.cache[name]; entry != nil {
		r.bytes -= entry.bytes
		delete(r.cache, name)
	}
}

// items is a seat's parsed items, from the cache while the files that hold them are the
// same size and age as when they were read.
func (r *searchRunner) items(name string, entry ledgerEntry, all []ledgerEntry) (keyedItems, error) {
	r.init()
	stamp := transcriptStamp(entry, all)
	if stamp != "" {
		r.mu.Lock()
		if hit := r.cache[name]; hit != nil && hit.stamp == stamp {
			hit.used = time.Now()
			r.mu.Unlock()
			return hit.keyed, nil
		}
		r.mu.Unlock()
	}
	r.mu.Lock()
	r.misses++
	r.mu.Unlock()
	keyed, err := allTranscriptItems(entry, all)
	if err == nil && stamp != "" {
		r.store(name, stamp, keyed)
	}
	return keyed, err
}

// store keeps a seat's items, dropping the least recently searched to stay in budget.
// A seat larger than the whole budget is searched and not kept.
func (r *searchRunner) store(name, stamp string, keyed keyedItems) {
	size := keyedBytes(keyed)
	if size > transcriptCacheBudget {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if old := r.cache[name]; old != nil {
		r.bytes -= old.bytes
		delete(r.cache, name)
	}
	if r.bytes+size > transcriptCacheBudget {
		names := make([]string, 0, len(r.cache))
		for each := range r.cache {
			names = append(names, each)
		}
		sort.Slice(names, func(a, b int) bool { return r.cache[names[a]].used.Before(r.cache[names[b]].used) })
		for _, oldest := range names {
			if r.bytes+size <= transcriptCacheBudget {
				break
			}
			r.bytes -= r.cache[oldest].bytes
			delete(r.cache, oldest)
		}
	}
	r.cache[name] = &cachedTranscript{stamp: stamp, keyed: keyed, bytes: size, used: time.Now()}
	r.bytes += size
}

func keyedBytes(keyed keyedItems) int64 {
	var size int64
	for index, item := range keyed.items {
		size += int64(len(item.ID)+len(item.Text)+len(item.At)+len(item.Model)+len(keyed.keys[index])) + transcriptItemOverhead
		if item.Tool != nil {
			size += int64(len(item.Tool.Name) + len(item.Tool.Summary) + len(item.Tool.FullSummary) + len(item.Tool.Output))
		}
	}
	return size
}

// transcriptStamp names the files a seat's items come from with their size and age, or
// is empty when they cannot be found, which reads again and caches nothing.
func transcriptStamp(entry ledgerEntry, all []ledgerEntry) string {
	var path string
	var err error
	switch entry.Seat {
	case "claude":
		path, err = transcriptPath(entry)
	case "codex":
		path, err = codexRollout(entry, all)
	case "opencode":
		// A SQLite write may land in the write-ahead log and leave the database untouched.
		path = opencodeDB(entry)
	default:
		return ""
	}
	if err != nil || path == "" {
		return ""
	}
	stamp := ""
	for _, each := range []string{path, path + "-wal"} {
		if info, err := os.Stat(each); err == nil {
			stamp += fmt.Sprintf("%s:%d:%d;", each, info.Size(), info.ModTime().UnixNano())
		}
	}
	return stamp
}

// startSearch answers a transcript_search on its own goroutine, so the connection keeps
// reading polls and typing, and cancels this connection's search still running.
func (d *daemon) startSearch(cl *client, message frame) error {
	ctx, cancel := context.WithCancel(context.Background())
	cl.searchMu.Lock()
	if cl.searchCancel != nil {
		cl.searchCancel()
	}
	cl.searchCancel = cancel
	cl.searchMu.Unlock()
	go func() {
		defer d.guard("transcript search")
		defer cancel()
		if err := d.transcriptSearch(ctx, cl, message); err != nil {
			_ = cl.c.write(frame{Type: "error", ID: message.ID, Error: err.Error(), Code: exitCodeFor(err), Reason: reasonFor(err)})
		}
	}()
	return nil
}

// stopSearch cancels a connection's search as the connection closes.
func (cl *client) stopSearch() {
	cl.searchMu.Lock()
	defer cl.searchMu.Unlock()
	if cl.searchCancel != nil {
		cl.searchCancel()
		cl.searchCancel = nil
	}
}
