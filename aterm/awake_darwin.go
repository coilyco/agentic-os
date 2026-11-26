package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"
)

// caffeinateLock is `caffeinate -i -w <daemon pid>`, released when it or the daemon ends.
// No cgo for IOKit. tooling-aterm-client/references/keep-awake.md
type caffeinateLock struct {
	cmd  *exec.Cmd
	done chan struct{}
}

func acquireAwake(string) (awakeLock, error) {
	cmd := exec.Command("/usr/bin/caffeinate", "-i", "-w", strconv.Itoa(os.Getpid()))
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("caffeinate: %w", err)
	}
	lock := &caffeinateLock{cmd: cmd, done: make(chan struct{})}
	go func() {
		_ = cmd.Wait()
		close(lock.done)
	}()
	return lock, nil
}

func (l *caffeinateLock) alive() bool {
	select {
	case <-l.done:
		return false
	default:
		return true
	}
}

func (l *caffeinateLock) describe() string {
	return fmt.Sprintf("caffeinate pid %d holds off idle sleep for daemon pid %d", l.cmd.Process.Pid, os.Getpid())
}

func (l *caffeinateLock) release() {
	_ = l.cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-l.done:
	case <-time.After(3 * time.Second):
		_ = l.cmd.Process.Kill()
		<-l.done
	}
}
