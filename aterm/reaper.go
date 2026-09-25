package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	terminateGrace = 3 * time.Second
	terminatePoll  = 500 * time.Millisecond
)

// sessionReaper stops earlier claude sessions of a name, the ones aterm's own
// daemon did not start. See docs/aterm-daemon.md.
type sessionReaper struct {
	alive     func(pid int) bool
	signal    func(pid int, sig syscall.Signal) error
	processes func() ([]processEntry, error)
	self      int
	wait      func(time.Duration)
	notice    io.Writer
}

type processEntry struct {
	PID     int
	PPID    int
	Command string
}

func systemReaper(notice io.Writer) sessionReaper {
	return sessionReaper{
		alive: func(pid int) bool {
			process, err := os.FindProcess(pid)
			return err == nil && process.Signal(syscall.Signal(0)) == nil && !isZombie(pid)
		},
		signal: func(pid int, sig syscall.Signal) error {
			process, err := os.FindProcess(pid)
			if err != nil {
				return err
			}
			return process.Signal(sig)
		},
		processes: listProcesses,
		self:      os.Getpid(),
		wait:      time.Sleep,
		notice:    notice,
	}
}

// isZombie is a process that has ended and waits to be reaped. Signal 0 still
// reaches it, so without this a stopped process reads as one that would not end.
func isZombie(pid int) bool {
	raw, err := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
	return err == nil && strings.HasPrefix(strings.TrimSpace(string(raw)), "Z")
}

// stop ends a process with SIGTERM, then SIGKILL after the grace, and reports
// whether it is gone.
func (reaper sessionReaper) stop(pid int) bool {
	_ = reaper.signal(pid, syscall.SIGTERM)
	for waited := time.Duration(0); waited < terminateGrace && reaper.alive(pid); waited += terminatePoll {
		reaper.wait(terminatePoll)
	}
	if reaper.alive(pid) {
		_ = reaper.signal(pid, syscall.SIGKILL)
		reaper.wait(100 * time.Millisecond)
	}
	return !reaper.alive(pid)
}

// clearClaude stops running claude sessions of this name and leaves their
// transcripts, and never this process or a host of it. See docs/aterm-bundles.md.
func (reaper sessionReaper) clearClaude(name string) int {
	entries, err := reaper.processes()
	// claudeSessionName is "" for an unnamed claude, so "" would match all of them.
	if name == "" || err != nil {
		return 0
	}
	parents := map[int]int{}
	for _, entry := range entries {
		parents[entry.PID] = entry.PPID
	}
	hosts := map[int]bool{reaper.self: true}
	for pid := parents[reaper.self]; pid > 1 && !hosts[pid]; pid = parents[pid] {
		hosts[pid] = true
	}
	stopped := 0
	for _, entry := range entries {
		if hosts[entry.PID] || claudeSessionName(entry.Command) != name {
			continue
		}
		if !reaper.stop(entry.PID) {
			fmt.Fprintf(reaper.notice, "aterm: Claude session %d named %s would not end\n", entry.PID, name)
			continue
		}
		stopped++
	}
	return stopped
}

// listProcesses reads this user's processes, which is all a session may end.
func listProcesses() ([]processEntry, error) {
	command := exec.Command("ps", "-xo", "pid=,ppid=,command=")
	command.Env = append(os.Environ(), "LC_ALL=C")
	raw, err := command.Output()
	if err != nil {
		return nil, err
	}
	var entries []processEntry
	for _, line := range strings.Split(string(raw), "\n") {
		pidText, rest, _ := strings.Cut(strings.TrimSpace(line), " ")
		ppidText, commandText, _ := strings.Cut(strings.TrimSpace(rest), " ")
		pid, pidErr := strconv.Atoi(pidText)
		ppid, ppidErr := strconv.Atoi(ppidText)
		if pidErr != nil || ppidErr != nil {
			continue
		}
		entries = append(entries, processEntry{PID: pid, PPID: ppid, Command: strings.TrimSpace(commandText)})
	}
	return entries, nil
}
