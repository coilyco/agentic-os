package main

import "errors"

// Exit codes a caller can branch on, so distinguishing a stale role from a
// missing binary does not mean grepping prose. See docs/aterm.md.
const (
	exitFailure   = 1
	exitUsage     = 2
	exitOffRoster = 3
	exitMissing   = 4
	exitSpawn     = 5
	exitNested    = 6
)

type exitError struct {
	code int
	err  error
}

func (e exitError) Error() string { return e.err.Error() }

func (e exitError) Unwrap() error { return e.err }

func withExit(code int, err error) error {
	if err == nil {
		return nil
	}
	return exitError{code: code, err: err}
}

// exitCodeFor reads the code back through any wrapping, so a caller-facing
// `fmt.Errorf("...: %w", err)` keeps the classification.
func exitCodeFor(err error) int {
	var typed exitError
	if errors.As(err, &typed) {
		return typed.code
	}
	return exitFailure
}
