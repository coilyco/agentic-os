package main

import (
	"fmt"
	"os/exec"
	"strings"
)

// macVPNService is the name the Tailscale GUI gives its VPN configuration.
const macVPNService = "Tailscale"

// platformTailnetGate reads the macOS VPN state without touching Tailscale.
// A failed read counts as down, since the cost of waiting is a retry.
func platformTailnetGate() error {
	output, err := exec.Command("scutil", "--nc", "status", macVPNService).Output()
	if err != nil {
		return fmt.Errorf("scutil --nc status: %w", err)
	}
	first, _, _ := strings.Cut(string(output), "\n")
	return vpnGate(first)
}
