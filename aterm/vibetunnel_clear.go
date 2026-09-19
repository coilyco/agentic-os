package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	terminateGrace = 3 * time.Second
	terminatePoll  = 500 * time.Millisecond
	// A recorded pid that started further from the session than this was
	// reused by an unrelated process.
	startTolerance = 10 * time.Second
)

// The pattern is the server's own, so a directory it would never name is left alone.
var sessionIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

type vibeTunnelRecord struct {
	Name      string `json:"name"`
	Status    string `json:"status"`
	PID       int    `json:"pid"`
	StartedAt string `json:"startedAt"`
}

// sessionReaper mirrors the server's own kill and cleanup for a forwarded
// session, so no server or token is needed. See docs/aterm-bundles.md.
type sessionReaper struct {
	control   string
	alive     func(pid int) bool
	signal    func(pid int, sig syscall.Signal) error
	startedAt func(pid int) (time.Time, error)
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
	home, _ := os.UserHomeDir()
	return sessionReaper{
		control: filepath.Join(home, ".vibetunnel", "control"),
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
		startedAt: processStart,
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

func processStart(pid int) (time.Time, error) {
	command := exec.Command("ps", "-o", "lstart=", "-p", strconv.Itoa(pid))
	command.Env = append(os.Environ(), "LC_ALL=C")
	raw, err := command.Output()
	if err != nil {
		return time.Time{}, err
	}
	return time.ParseInLocation("Mon Jan 2 15:04:05 2006",
		strings.Join(strings.Fields(string(raw)), " "), time.Local)
}

// clear ends every running session under name and removes every record of one,
// and returns how many records it removed. A record it cannot settle stays.
func (reaper sessionReaper) clear(name string) int {
	entries, err := os.ReadDir(reaper.control)
	if err != nil {
		return 0
	}
	cleared := 0
	for _, entry := range entries {
		id := entry.Name()
		if !entry.IsDir() || !sessionIDPattern.MatchString(id) {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(reaper.control, id, "session.json"))
		if err != nil {
			continue
		}
		var record vibeTunnelRecord
		if json.Unmarshal(raw, &record) != nil || record.Name != name {
			continue
		}
		if record.Status == "running" && !reaper.end(id, record) {
			continue
		}
		if err := os.RemoveAll(filepath.Join(reaper.control, id)); err != nil {
			fmt.Fprintf(reaper.notice, "aterm: could not remove VibeTunnel session %s: %v\n", id, err)
			continue
		}
		cleared++
	}
	return cleared
}

// end reports whether the session is gone. A pid that is dead, or that started
// nowhere near the session, is stale rather than the session, and is not signalled.
func (reaper sessionReaper) end(id string, record vibeTunnelRecord) bool {
	if record.PID <= 0 || !reaper.alive(record.PID) {
		return true
	}
	began, err := reaper.startedAt(record.PID)
	started, parseErr := time.Parse(time.RFC3339, record.StartedAt)
	if err != nil || parseErr != nil {
		fmt.Fprintf(reaper.notice, "aterm: left VibeTunnel session %s running, its process could not be verified\n", id)
		return false
	}
	if began.Sub(started).Abs() > startTolerance {
		return true
	}
	if !reaper.stop(record.PID) {
		fmt.Fprintf(reaper.notice, "aterm: VibeTunnel session %s would not end\n", id)
		return false
	}
	return true
}

// stop ends a process the way the server ends a forwarded session, and reports
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
	if err != nil {
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
