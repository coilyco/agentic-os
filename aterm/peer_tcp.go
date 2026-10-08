package main

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
)

// tcpPeerPIDs names the processes holding the client end of an accepted TCP
// connection on this host, read from the kernel's table, not from the peer.
func tcpPeerPIDs(remote, local net.Addr) ([]int, error) {
	clientPort, serverPort := addrPort(remote), addrPort(local)
	if clientPort == 0 || serverPort == 0 {
		return nil, fmt.Errorf("no ports in %v -> %v", remote, local)
	}
	pids, err := platformTCPOwners(clientPort, serverPort)
	if err != nil {
		return nil, err
	}
	if len(pids) == 0 {
		return nil, fmt.Errorf("no process on this host owns port %d", clientPort)
	}
	return pids, nil
}

func addrPort(addr net.Addr) int {
	tcp, ok := addr.(*net.TCPAddr)
	if !ok {
		return 0
	}
	return tcp.Port
}

// onThisHost reports whether a peer address is loopback or one of this host's
// own interface addresses, which is when a socket table can name the peer.
func onThisHost(addr net.Addr) bool {
	tcp, ok := addr.(*net.TCPAddr)
	if !ok {
		return false
	}
	if tcp.IP.IsLoopback() {
		return true
	}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return false
	}
	for _, own := range addrs {
		if ipNet, ok := own.(*net.IPNet); ok && ipNet.IP.Equal(tcp.IP) {
			return true
		}
	}
	return false
}

// parseLsofOwners reads `lsof -Fpn` output: a `p<pid>` line, then `n<local>-><remote>`
// per socket. The owners are the pids with a socket from clientPort to serverPort.
func parseLsofOwners(output string, clientPort, serverPort int) []int {
	var owners []int
	pid := 0
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "p"):
			pid, _ = strconv.Atoi(line[1:])
		case strings.HasPrefix(line, "n") && pid > 0:
			local, remote, found := strings.Cut(line[1:], "->")
			if found && endpointPort(local) == clientPort && endpointPort(remote) == serverPort {
				owners = append(owners, pid)
			}
		}
	}
	return owners
}

func endpointPort(endpoint string) int {
	_, port, err := net.SplitHostPort(strings.TrimSpace(endpoint))
	if err != nil {
		return 0
	}
	number, _ := strconv.Atoi(port)
	return number
}

var errNoSocketTable = errors.New("this platform has no socket table reading")
