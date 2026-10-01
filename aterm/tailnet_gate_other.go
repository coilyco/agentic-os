//go:build !darwin

package main

// platformTailnetGate has nothing to hold back off macOS, where the CLI
// only talks to a tailscaled that is already running.
func platformTailnetGate() error { return nil }
