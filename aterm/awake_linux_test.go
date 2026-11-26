package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func inhibitList(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("systemd-inhibit", "--list", "--no-pager").Output()
	if err != nil {
		t.Fatalf("systemd-inhibit --list: %v", err)
	}
	return string(out)
}

func TestInhibitLockShowsInLogindAndReleases(t *testing.T) {
	if _, err := exec.LookPath("systemd-inhibit"); err != nil {
		t.Skip("no systemd-inhibit on this host")
	}
	if _, err := os.Stat("/run/systemd/seats"); err != nil {
		t.Skip("logind is not running on this host")
	}
	lock, err := acquireAwake(awakeWhy)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(inhibitList(t), "Treehouse") {
		t.Fatalf("logind should list Treehouse while held: %s", lock.describe())
	}
	t.Log(lock.describe())
	lock.release()
	if lock.alive() || strings.Contains(inhibitList(t), "Treehouse") {
		t.Fatal("a released lock should leave logind's list")
	}
}
