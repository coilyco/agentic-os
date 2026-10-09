package main

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestSysfsHasBatteryOnlyForASystemBattery(t *testing.T) {
	supply := func(t *testing.T, devices map[string][2]string) string {
		t.Helper()
		dir := t.TempDir()
		for name, files := range devices {
			if err := os.MkdirAll(filepath.Join(dir, name), 0o755); err != nil {
				t.Fatal(err)
			}
			for file, value := range map[string]string{"type": files[0], "scope": files[1]} {
				if value == "" {
					continue
				}
				if err := os.WriteFile(filepath.Join(dir, name, file), []byte(value+"\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
		}
		return dir
	}
	for name, test := range map[string]struct {
		devices map[string][2]string
		want    bool
	}{
		"laptop":      {map[string][2]string{"AC": {"Mains", ""}, "BAT0": {"Battery", "System"}}, true},
		"no scope":    {map[string][2]string{"BAT0": {"Battery", ""}}, true},
		"server":      {map[string][2]string{"AC": {"Mains", ""}}, false},
		"UPS":         {map[string][2]string{"ups": {"UPS", ""}}, false},
		"mouse":       {map[string][2]string{"hid-0": {"Battery", "Device"}}, false},
		"no supplies": {nil, false},
	} {
		if got := sysfsHasBattery(supply(t, test.devices)); got != test.want {
			t.Fatalf("%s: sysfsHasBattery = %v, want %v", name, got, test.want)
		}
	}
	if sysfsHasBattery(filepath.Join(t.TempDir(), "absent")) {
		t.Fatal("a missing power_supply directory read as a battery")
	}
}

func TestSentryCheckInsSkipABatteryHostAndRunOnAnAlwaysOnOne(t *testing.T) {
	prevCheckIn, prevSleeps := sentryCheckIn, hostSleeps
	t.Cleanup(func() { sentryCheckIn, hostSleeps = prevCheckIn, prevSleeps })
	sentryCheckIn.first, sentryCheckIn.every = time.Millisecond, 10*time.Millisecond

	run := func(t *testing.T, sleeps bool) (int, string) {
		t.Helper()
		hostSleeps = func() bool { return sleeps }
		fake := newFakeSentry(t)
		var mu sync.Mutex
		var seen string
		d := newDaemon(func(format string, args ...any) {
			mu.Lock()
			defer mu.Unlock()
			seen += format
		})
		done := make(chan struct{})
		finished := d.startSentryCheckIns(fake.dsn(), done)
		time.Sleep(100 * time.Millisecond)
		close(done)
		<-finished
		mu.Lock()
		defer mu.Unlock()
		return len(fake.statuses()), seen
	}

	if got, logged := run(t, true); got != 0 || logged == "" {
		t.Fatalf("a battery host sent %d check-ins and logged %q, want none and a reason", got, logged)
	}
	// The control: the same wiring on an always-on host does check in.
	if got, _ := run(t, false); got == 0 {
		t.Fatal("an always-on host sent no check-ins")
	}
}
