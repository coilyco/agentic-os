//go:build !windows

package main

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

func hostIdentity() (int, int) {
	return os.Getuid(), os.Getgid()
}

func chownPath(path string, symlink bool, uid, gid int) error {
	if symlink {
		return os.Lchown(path, uid, gid)
	}
	return os.Chown(path, uid, gid)
}

func processStartIdentity(pid int) (string, error) {
	output, err := exec.Command("ps", "-o", "lstart=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return "", err
	}
	identity := strings.TrimSpace(string(output))
	if identity == "" {
		return "", fmt.Errorf("process %d has no start identity", pid)
	}
	return identity, nil
}

// processStartIdentities resolves many PIDs in one ps call. Startup asks about
// every lease at once, so a per-PID spawn there costs more than the work.
func processStartIdentities(pids []int) (map[int]string, error) {
	identities := map[int]string{}
	for _, batch := range batchProcessPIDs(pids) {
		selectors := make([]string, 0, len(batch))
		for _, pid := range batch {
			selectors = append(selectors, strconv.Itoa(pid))
		}
		command := exec.Command(
			"ps", "-o", "pid=,lstart=", "-p", strings.Join(selectors, ","),
		)
		var complaint bytes.Buffer
		command.Stderr = &complaint
		output, err := command.Output()
		if err != nil {
			// A silent non-zero exit means no listed PID runs, which is an answer.
			// A complaint means ps rejected the query, so the caller asks per PID.
			if complaint.Len() == 0 && len(output) == 0 {
				if _, ok := err.(*exec.ExitError); ok {
					continue
				}
			}
			return nil, fmt.Errorf("resolve process start identities: %w: %s",
				err, strings.TrimSpace(complaint.String()))
		}
		for _, line := range strings.Split(string(output), "\n") {
			pid, identity, ok := parseProcessStartLine(line)
			if ok {
				identities[pid] = identity
			}
		}
	}
	return identities, nil
}

func parseProcessStartLine(line string) (int, string, bool) {
	field, rest, found := strings.Cut(strings.TrimSpace(line), " ")
	if !found {
		return 0, "", false
	}
	pid, err := strconv.Atoi(field)
	if err != nil {
		return 0, "", false
	}
	identity := strings.TrimSpace(rest)
	if identity == "" {
		return 0, "", false
	}
	return pid, identity, true
}

func execNative(command []string) error {
	path, err := lookPath(command[0])
	if err != nil {
		return err
	}
	return syscall.Exec(path, command, harnessEnvironment(os.Environ()))
}

func execAs(uid, gid int, spec execSpec) error {
	path, err := lookPath(spec.Command[0])
	if err != nil {
		return err
	}
	if err := syscall.Setgroups([]int{}); err != nil {
		return err
	}
	if err := syscall.Setgid(gid); err != nil {
		return err
	}
	if err := syscall.Setuid(uid); err != nil {
		return err
	}
	return syscall.Exec(path, spec.Command, spec.Environment)
}

func startSOCKSForwarders(uid, gid int, spec execSpec) error {
	if len(spec.TailnetForwards) == 0 {
		return nil
	}
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve AOS forwarder binary: %w", err)
	}
	for _, forward := range spec.TailnetForwards {
		listener, err := net.Listen("tcp", forward.listenAddress())
		if err != nil {
			return fmt.Errorf("listen for tailnet MCP server %s: %w", forward.Server, err)
		}
		tcpListener, ok := listener.(*net.TCPListener)
		if !ok {
			_ = listener.Close()
			return fmt.Errorf("tailnet MCP server %s did not receive a TCP listener", forward.Server)
		}
		file, err := tcpListener.File()
		_ = listener.Close()
		if err != nil {
			return fmt.Errorf("inherit tailnet MCP listener for %s: %w", forward.Server, err)
		}
		command := exec.Command(
			executable,
			"_container-socks-forward",
			"--fd", "3",
			"--proxy", tailnetSOCKS5Proxy,
			"--target", forward.targetAddress(),
		)
		command.Env = spec.Environment
		command.ExtraFiles = []*os.File{file}
		command.Stdout = os.Stdout
		command.Stderr = os.Stderr
		command.SysProcAttr = &syscall.SysProcAttr{
			Credential: &syscall.Credential{
				Uid: uint32(uid),
				Gid: uint32(gid),
			},
		}
		if err := command.Start(); err != nil {
			_ = file.Close()
			return fmt.Errorf(
				"start tailnet MCP forwarder for %s on %s: %w",
				forward.Server,
				strconv.Itoa(forward.ListenPort),
				err,
			)
		}
		if err := file.Close(); err != nil {
			return fmt.Errorf("close inherited tailnet MCP listener for %s: %w", forward.Server, err)
		}
	}
	return nil
}
