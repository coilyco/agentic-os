package main

import (
	"encoding/xml"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestLaunchdPlistIsWellFormedAndCarriesTheRestartPolicy(t *testing.T) {
	plist, err := renderLaunchdPlist(launchdOptions{Bin: "/opt/homebrew/bin/aterm", Home: "/Users/kai"})
	if err != nil {
		t.Fatal(err)
	}
	decoder := xml.NewDecoder(strings.NewReader(plist))
	decoder.Strict = false
	for {
		if _, err := decoder.Token(); err == io.EOF {
			break
		} else if err != nil {
			t.Fatalf("the plist is not well-formed XML: %v\n%s", err, plist)
		}
	}
	for _, want := range []string{
		"<string>" + launchdLabel + "</string>",
		"<key>RunAtLoad</key>\n\t<true/>",
		"<key>KeepAlive</key>\n\t<dict>\n\t\t<key>SuccessfulExit</key>\n\t\t<false/>",
		"<key>AbandonProcessGroup</key>\n\t<true/>",
		"<key>ThrottleInterval</key>\n\t<integer>10</integer>",
		"exec '/opt/homebrew/bin/aterm' daemon --idle 0",
		"/Users/kai/Library/Logs/aterm-daemon.log",
		`aterm/daemon.env`,
	} {
		if !strings.Contains(plist, want) {
			t.Fatalf("the plist should hold %q:\n%s", want, plist)
		}
	}
	if path, err := exec.LookPath("plutil"); err == nil {
		file := filepath.Join(t.TempDir(), "a.plist")
		if err := os.WriteFile(file, []byte(plist), 0o600); err != nil {
			t.Fatal(err)
		}
		if out, err := exec.Command(path, "-lint", file).CombinedOutput(); err != nil {
			t.Fatalf("plutil -lint: %v\n%s", err, out)
		}
	}
}

// The agent's shell reads daemon.env as data. A value that would run, expand, or
// move PATH under `.` must arrive as literal text, or not at all.
func TestDaemonEnvLoaderReadsDataAndNeverSources(t *testing.T) {
	home := t.TempDir()
	config := filepath.Join(home, ".config", "aterm")
	if err := os.MkdirAll(config, 0o700); err != nil {
		t.Fatal(err)
	}
	pwned := filepath.Join(home, "pwned")
	file := strings.Join([]string{
		"# a comment",
		"ATERM_DAEMON_ALLOW_ORIGINS=https://coilyco.dev",
		"ATERM_SPACED=a b  c",
		"ATERM_RUN=$(touch " + pwned + ")",
		"ATERM_TICK=`touch " + pwned + "`",
		"ATERM_HOME=$HOME",
		"PATH=/evil",
		"OTHER=1",
		"ATERM_BAD-KEY=1",
		"ATERM_LAST=no trailing newline",
	}, "\n")
	if err := os.WriteFile(filepath.Join(config, "daemon.env"), []byte(file), 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("/bin/sh", "-c", daemonEnvLoader+"; env")
	command.Env = []string{"HOME=" + home, "PATH=/usr/bin:/bin"}
	out, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("loader: %v\n%s", err, out)
	}
	got := string(out)
	for _, want := range []string{
		"ATERM_DAEMON_ALLOW_ORIGINS=https://coilyco.dev",
		"ATERM_SPACED=a b  c",
		"ATERM_RUN=$(touch " + pwned + ")",
		"ATERM_HOME=$HOME",
		"ATERM_LAST=no trailing newline",
		"PATH=/usr/bin:/bin",
	} {
		if !strings.Contains(got, want+"\n") {
			t.Fatalf("the environment should hold %q:\n%s", want, got)
		}
	}
	for _, refused := range []string{"OTHER=1", "PATH=/evil", "ATERM_BAD-KEY"} {
		if strings.Contains(got, refused) {
			t.Fatalf("%q must not reach the environment:\n%s", refused, got)
		}
	}
	if _, err := os.Stat(pwned); err == nil {
		t.Fatal("a daemon.env value was executed")
	}
}

func TestLaunchdPlistQuotesAPathThatWouldBreakTheShell(t *testing.T) {
	plist, err := renderLaunchdPlist(launchdOptions{Bin: "/Users/o'brien & co/aterm", Home: "/Users/kai"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(plist, "o'brien &") || strings.Contains(plist, "& co") {
		t.Fatalf("a quote or ampersand reached the plist unescaped:\n%s", plist)
	}
	if _, err := renderLaunchdPlist(launchdOptions{Home: "/Users/kai"}); err == nil {
		t.Fatal("a plist with no binary must refuse")
	}
}

func TestIdleZeroNeverExits(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "aterm-idle-")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	socket := filepath.Join(dir, "d.sock")
	stop := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		_ = runDaemon(daemonOptions{Socket: socket, Idle: 0, Stop: stop}, io.Discard)
	}()
	t.Cleanup(func() {
		close(stop)
		<-stopped
	})
	time.Sleep(2500 * time.Millisecond)
	raw, err := net.Dial("unix", socket)
	if err != nil {
		t.Fatalf("a daemon run with --idle 0 should still be serving after the idle ticks: %v", err)
	}
	_ = raw.Close()
}

func TestStartDaemonAsksLaunchdOnlyForTheSocketItServes(t *testing.T) {
	var calls [][]string
	earlier := launchctl
	launchctl = func(args ...string) error {
		calls = append(calls, args)
		return nil
	}
	t.Cleanup(func() { launchctl = earlier })
	if startViaLaunchd(filepath.Join(t.TempDir(), "other.sock")) || len(calls) != 0 {
		t.Fatalf("a daemon on another socket is not the agent's, so launchd stays out: %v", calls)
	}
	if !startViaLaunchd(defaultDaemonSocket()) {
		t.Fatal("the agent's own socket should start through launchd when it is loaded")
	}
	want := []string{"kickstart", launchdTarget()}
	if len(calls) != 2 || !slices.Equal(calls[1], want) {
		t.Fatalf("launchctl calls = %v, want a print then %v", calls, want)
	}
	launchctl = func(args ...string) error {
		if args[0] == "print" {
			return syscall.ENOENT
		}
		t.Fatalf("an agent that is not loaded must not be kickstarted: %v", args)
		return nil
	}
	if startViaLaunchd(defaultDaemonSocket()) {
		t.Fatal("an unloaded agent leaves the caller to start the daemon")
	}
}

func TestDoctorWarnsWhenStoppingTheDaemonWouldEndItsSessions(t *testing.T) {
	earlier := launchctl
	launchctl = func(...string) error { return syscall.ENOENT }
	t.Cleanup(func() { launchctl = earlier })
	old := &conn{features: []string{sendNewFeature, closeFeature}, peerVersion: "aos-v0.404.0"}
	if daemonStatus(old, 3) != doctorWarn || !strings.Contains(daemonDetail(old, "/s", 3), "predates session holders") {
		t.Fatalf("a daemon without holders and with sessions should warn: %q", daemonDetail(old, "/s", 3))
	}
	if daemonStatus(old, 0) != doctorOK {
		t.Fatal("a daemon with no sessions loses nothing when it stops")
	}
	current := &conn{features: []string{holdFeature}, peerVersion: version}
	if daemonStatus(current, 3) != doctorOK {
		t.Fatal("a daemon with holders is safe to stop")
	}
}

// Opt-in, ATERM_LIVE_LAUNCHD=1: loads a throwaway agent on its own socket and
// proves a kickstart and a SIGKILL each revive the daemon with the same session.
func TestLiveLaunchdKeepsASessionAcrossRestartAndCrash(t *testing.T) {
	if os.Getenv("ATERM_LIVE_LAUNCHD") != "1" {
		t.Skip("set ATERM_LIVE_LAUNCHD=1 to load a throwaway launchd agent")
	}
	dir, err := os.MkdirTemp("/tmp", "aterm-live-")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(dir, "d.sock")
	t.Setenv(daemonSocketEnv, socket)
	label := "dev.coilyco.aterm-daemon-test." + strconv.Itoa(os.Getpid())
	home, _ := os.UserHomeDir()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	// The test binary stands in for aterm: TestMain runs main for the daemon verb.
	plist, err := renderLaunchdPlist(launchdOptions{
		Label: label, Bin: self, Home: home,
		Extra: []string{"--socket", socket, "--websocket", "", "--tailnet-port", ""},
	})
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, label+".plist")
	if err := os.WriteFile(file, []byte(plist), 0o600); err != nil {
		t.Fatal(err)
	}
	domain := "gui/" + strconv.Itoa(os.Getuid())
	t.Cleanup(func() {
		_ = exec.Command("launchctl", "bootout", domain+"/"+label).Run()
		sockets, _ := filepath.Glob(filepath.Join(dir, "hold", "*.sock"))
		rig := &holdRig{t: t}
		for _, holder := range sockets {
			rig.endHolder(holder)
		}
		_ = os.RemoveAll(dir)
	})
	if out, err := exec.Command("launchctl", "bootstrap", domain, file).CombinedOutput(); err != nil {
		t.Fatalf("launchctl bootstrap: %v\n%s", err, out)
	}
	waitFor(t, "the launchd daemon to answer", 15*time.Second, func() bool {
		raw, err := net.Dial("unix", socket)
		if err == nil {
			_ = raw.Close()
		}
		return err == nil
	})
	first := dialTest(t)
	first.spawn("eng-platform-live", "eng-platform", "Beetle-Ox", "echo READY; exec cat")
	first.until("READY")
	list := func() []sessionView {
		c, err := dialDaemon(false)
		if err != nil {
			return nil
		}
		defer c.Close()
		reply, err := c.request(frame{Type: "list"})
		if err != nil {
			return nil
		}
		return reply.Sessions
	}
	pid := list()[0].PID
	daemonPID := func() int {
		out, err := exec.Command("launchctl", "print", domain+"/"+label).CombinedOutput()
		if err != nil {
			return 0
		}
		for _, line := range strings.Split(string(out), "\n") {
			if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "pid = "); ok {
				n, _ := strconv.Atoi(rest)
				return n
			}
		}
		return 0
	}
	adopted := func(step string) {
		t.Helper()
		waitFor(t, step+": the same session adopted by the new daemon", 20*time.Second, func() bool {
			views := list()
			return len(views) == 1 && views[0].PID == pid
		})
		if !alive(pid) {
			t.Fatalf("%s: the session pid %d died", step, pid)
		}
	}
	before := daemonPID()
	if before == 0 {
		t.Fatal("launchd reports no daemon pid")
	}
	if out, err := exec.Command("launchctl", "kickstart", "-k", domain+"/"+label).CombinedOutput(); err != nil {
		t.Fatalf("kickstart -k: %v\n%s", err, out)
	}
	waitFor(t, "a new daemon pid after kickstart -k", 20*time.Second, func() bool {
		now := daemonPID()
		return now != 0 && now != before
	})
	adopted("kickstart -k")
	crashed := daemonPID()
	if err := syscall.Kill(crashed, syscall.SIGKILL); err != nil {
		t.Fatalf("kill the daemon: %v", err)
	}
	waitFor(t, "launchd to revive the killed daemon", 30*time.Second, func() bool {
		now := daemonPID()
		return now != 0 && now != crashed
	})
	adopted("SIGKILL")
}
