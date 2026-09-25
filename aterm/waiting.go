// aterm resolves a seat, a roster, and an overlay by shelling out before it
// opens anything, and any of those can sit behind something slow: a wrapped
// `aos` that converges the host first, a cold catalogue read, a stalled fetch.
// The launcher captures their output, so a slow call used to present as a
// terminal that had simply stopped. Naming what is being waited on is the whole
// fix: it turns "stuck?" into a process tree anyone can go read.

package main

import (
	"fmt"
	"io"
	"strings"
	"time"
)

// slowCallNotice is long enough that an ordinary sub-second read stays silent.
const slowCallNotice = 2 * time.Second

// whileWaiting names the command on the notice stream once the threshold
// passes. It never times the call out. docs/aterm.md
func whileWaiting[T any](notice io.Writer, command []string, call func() T) T {
	if notice == nil || len(command) == 0 {
		return call()
	}
	finished := make(chan struct{})
	// Joined before returning, so a notice never lands after the caller has
	// moved on to the writer.
	quiet := make(chan struct{})
	go func() {
		defer close(quiet)
		select {
		case <-finished:
		case <-time.After(slowCallNotice):
			fmt.Fprintf(notice, "aterm: waiting on `%s`\n", strings.Join(command, " "))
		}
	}()
	result := call()
	close(finished)
	<-quiet
	return result
}

// whileWaiting2 is the same notice around a call that also returns an error.
func whileWaiting2[T any](notice io.Writer, command []string, call func() (T, error)) (T, error) {
	type outcome struct {
		value T
		err   error
	}
	result := whileWaiting(notice, command, func() outcome {
		value, err := call()
		return outcome{value: value, err: err}
	})
	return result.value, result.err
}
