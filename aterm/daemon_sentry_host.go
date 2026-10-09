package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// hostSleeps says whether this host runs on a battery. A failed probe reads as
// always-on, which keeps alerting. See docs/aterm-bundles.md.
var hostSleeps = batteryHost

func batteryHost() bool {
	switch runtime.GOOS {
	case "darwin":
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, "pmset", "-g", "batt").Output()
		return err == nil && strings.Contains(string(out), "InternalBattery")
	case "linux":
		return sysfsHasBattery("/sys/class/power_supply")
	}
	return false
}

// sysfsHasBattery finds a system battery under a power_supply directory. A
// wireless mouse also lists a Battery, with scope Device, and a UPS lists as UPS.
func sysfsHasBattery(dir string) bool {
	entries, _ := os.ReadDir(dir)
	for _, entry := range entries {
		kind, _ := os.ReadFile(filepath.Join(dir, entry.Name(), "type"))
		scope, _ := os.ReadFile(filepath.Join(dir, entry.Name(), "scope"))
		if strings.TrimSpace(string(kind)) == "Battery" && strings.TrimSpace(string(scope)) != "Device" {
			return true
		}
	}
	return false
}

// startSentryCheckIns starts the check-ins unless the host sleeps. The channel
// closes when the loop ends.
func (d *daemon) startSentryCheckIns(dsn string, done <-chan struct{}) <-chan struct{} {
	finished := make(chan struct{})
	if hostSleeps() {
		d.logf("this host runs on a battery and sleeps, so no Sentry cron check-ins")
		close(finished)
		return finished
	}
	cron, err := newSentryCron(dsn, sentryMonitorSlug(hostName()))
	if err != nil {
		d.logf("no Sentry check-ins: %v", err)
		close(finished)
		return finished
	}
	go func() {
		defer close(finished)
		d.sentryCheckIns(done, cron)
	}()
	return finished
}
