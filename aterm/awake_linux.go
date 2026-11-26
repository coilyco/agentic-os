package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
)

// inhibitLock is `systemd-inhibit` running `cat` on a pipe only the daemon writes, so the
// daemon's death ends cat and the lock. tooling-aterm-client/references/keep-awake.md
type inhibitLock struct {
	cmd   *exec.Cmd
	stdin io.Closer
	what  string
	done  chan struct{}
}

// inhibitReady bounds the wait for logind to grant the lock.
const inhibitReady = 10 * time.Second

// acquireAwake asks for idle and sleep. Outside a login session polkit wants an admin
// for sleep, so idle alone is the fallback.
func acquireAwake(why string) (awakeLock, error) {
	lock, err := inhibit("idle:sleep", why)
	if err == nil {
		return lock, nil
	}
	if fallback, idleErr := inhibit("idle", why); idleErr == nil {
		return fallback, nil
	}
	return nil, err
}

func inhibit(what, why string) (*inhibitLock, error) {
	cmd := exec.Command("systemd-inhibit", "--what="+what, "--who=Treehouse", "--why="+why,
		"--mode=block", "sh", "-c", "echo held && exec cat")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("systemd-inhibit: %w", err)
	}
	lock := &inhibitLock{cmd: cmd, stdin: stdin, what: what, done: make(chan struct{})}
	ready := make(chan bool, 1)
	go func() {
		// The child runs, and prints, only once logind has granted the lock.
		reader := bufio.NewReader(stdout)
		line, _ := reader.ReadString('\n')
		ready <- strings.TrimSpace(line) == "held"
		_, _ = io.Copy(io.Discard, reader)
		_ = cmd.Wait()
		close(lock.done)
	}()
	select {
	case ok := <-ready:
		if ok {
			return lock, nil
		}
	case <-time.After(inhibitReady):
		_ = cmd.Process.Kill()
	}
	_ = stdin.Close()
	<-lock.done
	reason := strings.TrimSpace(stderr.String())
	if reason == "" {
		reason = "no lock granted"
	}
	return nil, errors.New("systemd-inhibit --what=" + what + ": " + reason)
}

func (l *inhibitLock) alive() bool {
	select {
	case <-l.done:
		return false
	default:
		return true
	}
}

func (l *inhibitLock) describe() string {
	return fmt.Sprintf("systemd-inhibit pid %d holds %s for Treehouse", l.cmd.Process.Pid, l.what)
}

func (l *inhibitLock) release() {
	_ = l.stdin.Close()
	select {
	case <-l.done:
	case <-time.After(3 * time.Second):
		_ = l.cmd.Process.Kill()
		<-l.done
	}
}
