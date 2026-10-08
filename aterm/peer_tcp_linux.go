package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// platformTCPOwners finds the socket's inode in the kernel's TCP tables, then
// the processes holding a descriptor on it. Only this user's are readable.
func platformTCPOwners(clientPort, serverPort int) ([]int, error) {
	inodes := map[string]bool{}
	for _, table := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		file, err := os.Open(table)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(file)
		scanner.Scan() // header
		for scanner.Scan() {
			// sl local rem st tx:rx tr:when retrnsmt uid timeout inode
			fields := strings.Fields(scanner.Text())
			if len(fields) < 10 || fields[3] != "01" {
				continue
			}
			if hexPort(fields[1]) == clientPort && hexPort(fields[2]) == serverPort {
				inodes["socket:["+fields[9]+"]"] = true
			}
		}
		_ = file.Close()
	}
	if len(inodes) == 0 {
		return nil, nil
	}
	links, err := filepath.Glob("/proc/[0-9]*/fd/*")
	if err != nil {
		return nil, err
	}
	var owners []int
	seen := map[int]bool{}
	for _, link := range links {
		target, err := os.Readlink(link)
		if err != nil || !inodes[target] {
			continue
		}
		var pid int
		if _, err := fmt.Sscanf(link, "/proc/%d/", &pid); err == nil && !seen[pid] {
			seen[pid] = true
			owners = append(owners, pid)
		}
	}
	return owners, nil
}

func hexPort(address string) int {
	_, port, found := strings.Cut(address, ":")
	if !found {
		return 0
	}
	var number int
	_, _ = fmt.Sscanf(port, "%X", &number)
	return number
}
