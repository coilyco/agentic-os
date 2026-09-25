package main

import (
	"errors"
	"net"

	"golang.org/x/sys/unix"
)

// peerPID is the process on the other end of a local socket, which the kernel
// reports and the peer cannot choose.
func peerPID(raw net.Conn) (int, error) {
	unixConn, ok := raw.(*net.UnixConn)
	if !ok {
		return 0, errors.New("not a unix socket")
	}
	control, err := unixConn.SyscallConn()
	if err != nil {
		return 0, err
	}
	pid, readErr := 0, error(nil)
	if err := control.Control(func(fd uintptr) {
		credentials, err := unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
		if err != nil {
			readErr = err
			return
		}
		pid = int(credentials.Pid)
	}); err != nil {
		return 0, err
	}
	return pid, readErr
}
