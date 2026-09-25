//go:build !darwin && !linux

package main

import (
	"errors"
	"net"
)

// peerPID has no reading here, so every peer counts as inside a session.
func peerPID(net.Conn) (int, error) {
	return 0, errors.New("peer credentials are not read on this platform")
}
