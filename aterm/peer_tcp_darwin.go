package main

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"time"
)

// platformTCPOwners asks lsof, which reads only this user's processes, and a
// browser the user launched is one of them.
func platformTCPOwners(clientPort, serverPort int) ([]int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "lsof", "-nP", "-iTCP:"+strconv.Itoa(clientPort), "-sTCP:ESTABLISHED", "-Fpn").Output()
	if err != nil && len(output) == 0 {
		return nil, fmt.Errorf("lsof: %w", err)
	}
	return parseLsofOwners(string(output), clientPort, serverPort), nil
}
