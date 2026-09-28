//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"

	"golang.org/x/sys/windows"
)

func hostIdentity() (int, int) {
	return 1000, 1000
}

func chownPath(_ string, _ bool, _, _ int) error {
	return nil
}

func processStartIdentity(pid int) (string, error) {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(handle)
	var creation, exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(handle, &creation, &exit, &kernel, &user); err != nil {
		return "", err
	}
	return strconv.FormatInt(creation.Nanoseconds(), 10), nil
}

// processStartIdentities keeps the batched shape the callers expect. Windows
// queries each process directly, so there is no spawn to amortize.
func processStartIdentities(pids []int) (map[int]string, error) {
	identities := map[int]string{}
	for _, batch := range batchProcessPIDs(pids) {
		for _, pid := range batch {
			if identity, err := processStartIdentity(pid); err == nil {
				identities[pid] = identity
			}
		}
	}
	return identities, nil
}

func execNative(command []string) error {
	child := exec.Command(command[0], command[1:]...)
	child.Env = harnessEnvironment(os.Environ())
	child.Stdin = os.Stdin
	child.Stdout = os.Stdout
	child.Stderr = os.Stderr
	return child.Run()
}

func execAs(_, _ int, _ execSpec) error {
	return fmt.Errorf("container process replacement is supported only by the Linux image")
}

func startSOCKSForwarders(_, _ int, spec execSpec) error {
	if len(spec.TailnetForwards) == 0 {
		return nil
	}
	return fmt.Errorf("tailnet MCP forwarding is supported only by the Linux image")
}
