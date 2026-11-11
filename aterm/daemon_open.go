package main

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// openURLFeature is how a client knows the daemon answers `open_url` and the
// `open_url` subscribe channel. See references/open-url.md in the aterm skill.
const openURLFeature = "open-url"

const (
	reasonBadURL = "bad_url"
	// maxOpenURL is generous for an OAuth URL and small enough to never be a payload.
	maxOpenURL = 8 << 10
	// A seat may ask this many times in the window, so a loop cannot flood a client's tabs.
	maxOpensPerWindow = 20
	openWindow        = time.Minute
	shimDirName       = "shims"
)

// shimNames are the commands a seat runs to open a page. Each execs `aterm open`.
var shimNames = []string{"open", "xdg-open"}

// checkOpenURL admits what a person would open, as browser_navigate does. The
// address is never logged, since an OAuth URL carries its state.
func checkOpenURL(address string) error {
	if len(address) > maxOpenURL {
		return withReason(reasonTooLarge, fmt.Errorf("the address is over %d bytes", maxOpenURL))
	}
	parsed, err := url.Parse(address)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return withReason(reasonBadURL, errors.New("open_url takes an http or https address"))
	}
	return nil
}

// allowOpen counts one ask against the seat's window and says whether it may.
func (s *ptySession) allowOpen(now time.Time) bool {
	s.openMu.Lock()
	defer s.openMu.Unlock()
	s.opened = slices.DeleteFunc(s.opened, func(at time.Time) bool { return now.Sub(at) > openWindow })
	if len(s.opened) >= maxOpensPerWindow {
		return false
	}
	s.opened = append(s.opened, now)
	return true
}

// openURL is a seat's request, judged by its token like `send`. It answers how many
// subscribers took the address, so `aterm open` falls back when none did.
func (d *daemon) openURL(cl *client, message frame) error {
	s := d.byToken(message.Token)
	if s == nil {
		return withExit(exitUsage, errors.New(
			"aterm open speaks for a session aterm launched, and this token names no live one"))
	}
	if err := checkOpenURL(message.URL); err != nil {
		return err
	}
	if !s.allowOpen(time.Now()) {
		return withReason(reasonTooMany, fmt.Errorf("%s asked to open more than %d addresses in a minute", s.name, maxOpensPerWindow))
	}
	taken := d.pushOpen(frame{Type: "open_url", Session: s.name, URL: message.URL})
	d.logf("%s asked to open an address, taken by %d clients", s.name, taken)
	return cl.c.write(frame{Type: "opened", ID: message.ID, Session: s.name, Delivered: taken})
}

// watchOpens subscribes a client that can show an address. A client that cannot
// never subscribes, so it is never counted as having taken one.
func (d *daemon) watchOpens(cl *client, message frame) error {
	if err := cl.c.write(frame{Type: "subscribed", ID: message.ID, Channel: "open_url"}); err != nil {
		return err
	}
	d.mu.Lock()
	if d.openers == nil {
		d.openers = map[*conn]bool{}
	}
	d.openers[cl.c] = true
	d.mu.Unlock()
	return nil
}

// pushOpen offers the frame to each opener and counts those it reached. A full
// outbox drops the client, as broadcast does.
func (d *daemon) pushOpen(message frame) int {
	d.mu.Lock()
	openers := make([]*conn, 0, len(d.openers))
	for c := range d.openers {
		openers = append(openers, c)
	}
	d.mu.Unlock()
	taken := 0
	for _, c := range openers {
		if c.offer(message) {
			taken++
			continue
		}
		d.mu.Lock()
		delete(d.openers, c)
		d.mu.Unlock()
		_ = c.Close()
	}
	return taken
}

// writeShims puts an `open` and `xdg-open` in dir that run this binary's `aterm open`.
// Each daemon start rewrites them atomically, so an upgrade replaces the old path.
func writeShims(dir, executable string) error {
	if err := ensureSocketDir(dir); err != nil {
		return err
	}
	script := "#!/bin/sh\nexec '" + strings.ReplaceAll(executable, "'", `'\''`) + "' open \"$@\"\n"
	for _, name := range shimNames {
		temporary := filepath.Join(dir, "."+name+".tmp")
		if err := os.WriteFile(temporary, []byte(script), 0o700); err != nil {
			return err
		}
		if err := os.Rename(temporary, filepath.Join(dir, name)); err != nil {
			return err
		}
	}
	return nil
}

// installShims writes the shims for this binary and turns them on for new seats. A
// failure leaves seats on their own browser, which is the behaviour before shims.
func (d *daemon) installShims(dir string) {
	executable, err := os.Executable()
	if err == nil {
		err = writeShims(dir, executable)
	}
	if err != nil {
		d.logf("no open shims, so a seat's browser requests open the native browser: %v", err)
		return
	}
	d.shimDir = dir
}

// withDaemonSocket tells a seat which socket this daemon listens on, since `aterm` in the
// seat dials the default one otherwise, which may be another daemon's or none (COI-3559).
func (d *daemon) withDaemonSocket(environ []string) []string {
	if d.socket == "" {
		return environ
	}
	kept := make([]string, 0, len(environ)+1)
	for _, entry := range environ {
		if key, _, _ := strings.Cut(entry, "="); key != daemonSocketEnv {
			kept = append(kept, entry)
		}
	}
	return append(kept, daemonSocketEnv+"="+d.socket)
}

// withOpenShims sets BROWSER and leads PATH with the shims, so a seat's page-opening
// runs `aterm open`. A daemon with no shims leaves the environment alone.
func (d *daemon) withOpenShims(environ []string) []string {
	if d.shimDir == "" {
		return environ
	}
	kept := make([]string, 0, len(environ)+2)
	for _, entry := range environ {
		key, value, _ := strings.Cut(entry, "=")
		switch key {
		case "BROWSER":
			continue
		case "PATH":
			// No PATH stays none, since a lone shim directory would hide every command.
			entry = "PATH=" + d.shimDir + string(os.PathListSeparator) + value
		}
		kept = append(kept, entry)
	}
	return append(kept, "BROWSER="+filepath.Join(d.shimDir, "open"))
}
