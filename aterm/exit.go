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
	exitDrift     = 7
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

// Reasons a typing refusal names, which a client branches on to show a
// read-only state. The words around them are for people.
const (
	reasonSessionDescendant = "session_descendant"
	reasonPeerUnread        = "peer_unread"
	reasonPasskeyRequired   = "passkey_required"
	// A remote device cannot open a shell until COI-2488's gating exists.
	reasonRemoteTerminal = "remote_terminal"
)

// typingStanding says whether a peer may type, and if not why.
type typingStanding struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason,omitempty"`
	// Passkey is "enrolled" or "unenrolled", for a remote device only.
	Passkey string `json:"passkey,omitempty"`
}

type reasonError struct {
	reason string
	err    error
}

func (e reasonError) Error() string { return e.err.Error() }

func (e reasonError) Unwrap() error { return e.err }

func withReason(reason string, err error) error {
	return reasonError{reason: reason, err: err}
}

func reasonFor(err error) string {
	var typed reasonError
	if errors.As(err, &typed) {
		return typed.reason
	}
	return ""
}
