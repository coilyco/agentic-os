package main

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// terminateGrace is how long an ended session gets between SIGTERM and SIGKILL.
const terminateGrace = 3 * time.Second

type processEntry struct {
	PID     int
	PPID    int
	Command string
}

// listProcesses reads this user's processes, which is all a session may end.
// processLister is what a new daemon lists processes with. A test swaps it.
var processLister = listProcesses

// processRetryPause spaces the one retry. A failed list refuses a person's answer.
var processRetryPause = 100 * time.Millisecond

func listProcesses() ([]processEntry, error) { return listWithRetry(psProcesses) }

func listWithRetry(list func() ([]processEntry, error)) ([]processEntry, error) {
	entries, err := list()
	if err != nil {
		time.Sleep(processRetryPause)
		return list()
	}
	return entries, nil
}

func psProcesses() ([]processEntry, error) {
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
