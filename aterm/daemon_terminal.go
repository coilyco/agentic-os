package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// terminalName names a plain terminal. A seat is <role>-<identity>, so the
// prefix keeps the two namespaces from meeting in the one name a client holds.
const terminalName = "terminal-"

// terminalEnvKeep is what a shell inherits from the daemon, which started from
// launchd or a shadow and carries credentials a shell has no use for.
var terminalEnvKeep = map[string]bool{
	"HOME": true, "USER": true, "LOGNAME": true, "SHELL": true, "PATH": true,
	"TMPDIR": true, "LANG": true, "SSH_AUTH_SOCK": true,
}

// terminalEnviron is the canonical environment, home restored if the daemon
// started in a session shadow, cut to terminalEnvKeep and LC_*.
func terminalEnviron(environ []string) []string {
	if canonical := canonicalEnviron(environ, readCanonicalLaunch()); canonical != nil {
		environ = canonical
	}
	kept := make([]string, 0, len(terminalEnvKeep)+2)
	for _, entry := range environ {
		name, _, _ := strings.Cut(entry, "=")
		if terminalEnvKeep[name] || strings.HasPrefix(name, "LC_") {
			kept = append(kept, entry)
		}
	}
	return append(kept, "TERM=xterm-256color", "COLORTERM=truecolor")
}

func envValue(environ []string, name string) string {
	value := ""
	for _, entry := range environ {
		if v, ok := strings.CutPrefix(entry, name+"="); ok {
			value = v
		}
	}
	return value
}

// loginShell is the user's $SHELL when it is an executable path, else the
// first stock shell present.
func loginShell(environ []string) string {
	candidates := []string{envValue(environ, "SHELL"), "/bin/zsh", "/bin/bash", "/bin/sh"}
	for _, candidate := range candidates {
		if !filepath.IsAbs(candidate) {
			continue
		}
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return candidate
		}
	}
	return "/bin/sh"
}

// checkTerminalLabel refuses a label it could not echo back unchanged: not text,
// too long, or carrying control characters a client would have to guard against.
func checkTerminalLabel(label string) error {
	if len(label) > maxTerminalLabel {
		return fmt.Errorf("a terminal label is at most %d bytes, and this one is %d", maxTerminalLabel, len(label))
	}
	if !utf8.ValidString(label) || strings.IndexFunc(label, unicode.IsControl) >= 0 {
		return errors.New("a terminal label is text without control characters")
	}
	return nil
}

// spawnTerminal starts the user's login shell under a holder, named by the
// daemon. A terminal has no role, so none of a seat's fields may ride along.
func (d *daemon) spawnTerminal(message frame) (*ptySession, error) {
	if message.Session != "" || message.Role != "" || message.Identity != "" || message.Seat != "" ||
		len(message.Argv) > 0 || len(message.Env) > 0 {
		return nil, withExit(exitUsage, errors.New(
			"a terminal takes no session, role, identity, seat, argv, or env: the daemon names it and runs the login shell"))
	}
	if err := checkTerminalLabel(message.Label); err != nil {
		return nil, withExit(exitUsage, err)
	}
	environ := terminalEnviron(os.Environ())
	cwd := message.Cwd
	if cwd == "" {
		cwd = envValue(environ, "HOME")
	}
	if info, err := os.Stat(cwd); !filepath.IsAbs(cwd) || err != nil || !info.IsDir() {
		return nil, withExit(exitUsage, fmt.Errorf("a terminal starts in a directory that exists, and %q is not one", cwd))
	}
	message.Kind, message.Cwd, message.Env = kindTerminal, cwd, environ
	message.Argv = []string{loginShell(environ), "-l"}
	var name string
	for {
		name = terminalName + randomID(3)
		d.mu.Lock()
		_, seat := d.sessions[name]
		_, shell := d.terminals[name]
		d.mu.Unlock()
		if !seat && !shell {
			break
		}
	}
	s, err := startPTYSession(d, name, message)
	if err != nil {
		return nil, err
	}
	d.mu.Lock()
	d.terminals[name] = s
	d.lastActive = time.Now()
	d.mu.Unlock()
	d.logf("opened terminal %s as pid %d in %s", name, s.pid, cwd)
	return s, nil
}

func (d *daemon) terminal(name string) *ptySession {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.terminals[strings.TrimSpace(name)]
}

// attachable is a seat or a terminal by exact name, for the frames that take a
// client to a screen and not to a recipient.
func (d *daemon) attachable(name string) *ptySession {
	if s := d.session(name); s != nil {
		return s
	}
	return d.terminal(name)
}

func (d *daemon) terminalViews() []terminalView {
	d.mu.Lock()
	terminals := make([]*ptySession, 0, len(d.terminals))
	for _, s := range d.terminals {
		terminals = append(terminals, s)
	}
	d.mu.Unlock()
	views := make([]terminalView, 0, len(terminals))
	for _, s := range terminals {
		views = append(views, s.terminalView())
	}
	sort.Slice(views, func(i, j int) bool { return views[i].Name < views[j].Name })
	return views
}

// closeTerminal ends a shell and drops it. It never ends a caller running inside it.
func (d *daemon) closeTerminal(cl *client, message frame, s *ptySession) error {
	if d.clientDescendsFrom(cl, map[int]bool{s.pid: true}) {
		return withExit(exitUsage, fmt.Errorf("this caller runs inside %s, so closing it would end the caller too", s.name))
	}
	d.logf("closing terminal %s (pid %d)", s.name, s.pid)
	s.end()
	select {
	case <-s.done:
	default:
		return fmt.Errorf("%s (pid %d) did not end after SIGKILL", s.name, s.pid)
	}
	select {
	case <-s.forgotten:
	case <-time.After(forgetGrace):
		return fmt.Errorf("%s ended but the daemon still holds its name", s.name)
	}
	s.mu.Lock()
	code := s.exitCode
	s.mu.Unlock()
	return cl.c.write(frame{Type: "closed", ID: message.ID, Session: s.name, Code: code})
}

// forgetTerminal drops an ended shell. It leaves no ledger record, since there
// is nothing for `aterm resume` to resume.
func (d *daemon) forgetTerminal(s *ptySession) {
	d.mu.Lock()
	if d.terminals[s.name] == s {
		delete(d.terminals, s.name)
	}
	d.lastActive = time.Now()
	d.ended[s.name] = endedSession{code: s.exitCode, at: time.Now()}
	d.mu.Unlock()
	d.pushSessions()
}

// everySession is each seat and terminal, for what treats them alike: stopping.
func (d *daemon) everySession() []*ptySession {
	d.mu.Lock()
	defer d.mu.Unlock()
	all := make([]*ptySession, 0, len(d.sessions)+len(d.terminals))
	for _, s := range d.sessions {
		all = append(all, s)
	}
	for _, s := range d.terminals {
		all = append(all, s)
	}
	return all
}
