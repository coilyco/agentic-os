package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// browserEnv names a Chromium or Chrome binary, ahead of the places looked in.
const browserEnv = "ATERM_BROWSER"

// browserLink is a started browser as the daemon sees it: the CDP stream pair,
// when the process ended, and how to end it.
type browserLink struct {
	r io.Reader
	w io.Writer
	// exited closes when the browser process is gone.
	exited <-chan struct{}
	stop   func()
	// note is the reason a live browser_state carries, empty for the default.
	note string
}

// browserLauncher starts a browser for one session.
type browserLauncher func(session string) (*browserLink, error)

var chromiumCandidates = []string{
	"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
	"/Applications/Chromium.app/Contents/MacOS/Chromium",
	"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "chrome",
}

// chromiumPath is the first browser found, empty when this host has none.
func chromiumPath() string {
	if named := strings.TrimSpace(os.Getenv(browserEnv)); named != "" {
		if resolved, err := exec.LookPath(named); err == nil {
			return resolved
		}
		return ""
	}
	for _, candidate := range chromiumCandidates {
		if resolved, err := exec.LookPath(candidate); err == nil {
			return resolved
		}
	}
	return ""
}

// browserEnviron is what a page-rendering process needs and no more, since the
// daemon's own environment can hold credentials a renderer has no use for.
func browserEnviron() []string {
	keep := map[string]bool{"HOME": true, "PATH": true, "TMPDIR": true, "LANG": true, "LC_ALL": true, "USER": true, "DISPLAY": true, "XDG_RUNTIME_DIR": true}
	var kept []string
	for _, entry := range os.Environ() {
		if name, _, found := strings.Cut(entry, "="); found && keep[name] {
			kept = append(kept, entry)
		}
	}
	return kept
}

// launchChromium starts headless Chromium with CDP on a pipe pair, not a port.
// A role profile is released on exit or failure and never deleted.
func launchChromium(dir, session string, role browserProfile) (link *browserLink, err error) {
	release := role.release
	if release == nil {
		release = func() {}
	}
	defer func() {
		if err != nil {
			release()
		}
	}()
	binary := chromiumPath()
	if binary == "" {
		return nil, fmt.Errorf("no Chromium or Chrome on this host, set %s to one", browserEnv)
	}
	if err := ensureSocketDir(dir); err != nil {
		return nil, err
	}
	profile, throwaway := role.dir, role.dir == ""
	if throwaway {
		if profile, err = os.MkdirTemp(dir, slugify(session)+"-"); err != nil {
			return nil, err
		}
	} else if err := os.MkdirAll(profile, 0o700); err != nil {
		return nil, err
	}
	dropProfile := func() {
		if throwaway {
			_ = os.RemoveAll(profile)
		}
	}
	toBrowserR, toBrowserW, err := os.Pipe()
	if err != nil {
		dropProfile()
		return nil, err
	}
	fromBrowserR, fromBrowserW, err := os.Pipe()
	if err != nil {
		_ = toBrowserR.Close()
		_ = toBrowserW.Close()
		dropProfile()
		return nil, err
	}
	args := []string{
		"--headless=new", "--remote-debugging-pipe", "--user-data-dir=" + profile,
		"--no-first-run", "--no-default-browser-check", "--window-size=1280,800",
	}
	// Chromium refuses to sandbox as root, which is how a dev-base container runs.
	if os.Geteuid() == 0 {
		args = append(args, "--no-sandbox")
	}
	command := exec.Command(binary, append(args, "about:blank")...)
	command.Env = browserEnviron()
	command.ExtraFiles = []*os.File{toBrowserR, fromBrowserW}
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := command.Start(); err != nil {
		_ = toBrowserR.Close()
		_ = toBrowserW.Close()
		_ = fromBrowserR.Close()
		_ = fromBrowserW.Close()
		dropProfile()
		return nil, fmt.Errorf("start %s: %w", binary, err)
	}
	_ = toBrowserR.Close()
	_ = fromBrowserW.Close()
	pid := command.Process.Pid
	// Written before anything else can fail, so a daemon killed outright leaves a trail.
	if err := writeBrowserRecord(dir, browserRecord{PID: pid, Session: session, Profile: profile}); err != nil {
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		_ = command.Wait()
		_ = toBrowserW.Close()
		_ = fromBrowserR.Close()
		dropProfile()
		return nil, fmt.Errorf("record the browser for cleanup: %w", err)
	}
	exited := make(chan struct{})
	go func() {
		_ = command.Wait()
		_ = os.Remove(browserRecordPath(dir, pid))
		_ = toBrowserW.Close()
		dropProfile()
		release()
		close(exited)
	}()
	stop := func() {
		select {
		case <-exited:
			return
		default:
		}
		// Browser.close lets Chromium write its cookies and storage out, which
		// SIGTERM skips and which a role profile's logins depend on.
		_, _ = toBrowserW.Write([]byte(`{"id":2147483647,"method":"Browser.close"}` + "\x00"))
		select {
		case <-exited:
			return
		case <-time.After(2 * time.Second):
		}
		// The process group, since Chromium's renderers and helpers are children.
		_ = syscall.Kill(-command.Process.Pid, syscall.SIGTERM)
		select {
		case <-exited:
		case <-time.After(3 * time.Second):
			_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
			<-exited
		}
	}
	return &browserLink{r: fromBrowserR, w: toBrowserW, exited: exited, stop: stop, note: role.note}, nil
}

// browserProfileDir is where a session's throwaway profile lives, beside the
// holders and under the same 0700 directory.
func (d *daemon) browserProfileDir() string {
	if d.holdDir == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(d.holdDir), "browser")
}

// startBrowser launches through the test hook when set, else real Chromium.
func (d *daemon) startBrowser(session string) (*browserLink, error) {
	if d.browserLaunch != nil {
		return d.browserLaunch(session)
	}
	dir := d.browserProfileDir()
	if dir == "" {
		return nil, errors.New("this daemon has no directory for browser profiles")
	}
	return launchChromium(dir, session, d.chooseProfile(session))
}

// browserServed is whether welcome offers `browser`: a launcher is set or a
// browser binary exists on this host.
func (d *daemon) browserServed() bool {
	return d.browserLaunch != nil || chromiumPath() != ""
}
