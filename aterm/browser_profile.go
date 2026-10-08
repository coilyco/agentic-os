package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

// playwrightConfigEnv names the directory holding the roles' Playwright MCP
// configs, ahead of the agentic-os-kai checkout under the projects root.
const playwrightConfigEnv = "ATERM_PLAYWRIGHT_CONFIG_DIR"

const tempProfileNote = "A temporary profile on the host."

// browserProfile is where one browser keeps its profile. The zero value is a
// throwaway directory that the launcher makes and removes.
type browserProfile struct {
	// dir is a role's Playwright profile. It is never deleted.
	dir string
	// note is the reason a live browser_state carries.
	note string
	// release hands a claimed profile back, nil for a throwaway.
	release func()
}

// profileClaims says which session holds each role profile, since Chromium
// lets one process at a time use a profile directory. The zero value is ready.
type profileClaims struct {
	mu      sync.Mutex
	holders map[string]string
}

// claim takes dir for session, or names the session already holding it.
func (c *profileClaims) claim(dir, session string) (holder string, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.holders == nil {
		c.holders = map[string]string{}
	}
	if held := c.holders[dir]; held != "" {
		return held, false
	}
	c.holders[dir] = session
	return "", true
}

func (c *profileClaims) release(dir, session string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.holders[dir] == session {
		delete(c.holders, dir)
	}
}

// playwrightConfigDir is where `local_coilyco_playwright_<role>.json` lives.
// The files are Kai's per-host input, so they are read here and never embedded.
func playwrightConfigDir() string {
	if named := strings.TrimSpace(os.Getenv(playwrightConfigEnv)); named != "" {
		return named
	}
	return filepath.Join(defaultWorkingDirectory(), "coilyco", "agentic-os-kai", "config")
}

// rolePlaywrightDir is the role's Playwright userDataDir, empty with the
// reason when the role has none to share.
func rolePlaywrightDir(role string) (dir, why string) {
	if !safeRoleSlug(role) {
		return "", "this session has no role"
	}
	raw, err := os.ReadFile(filepath.Join(playwrightConfigDir(), "local_coilyco_playwright_"+role+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return "", fmt.Sprintf("No Playwright profile is configured for the %s role.", role)
	}
	if err != nil {
		return "", fmt.Sprintf("The %s role's Playwright config could not be read: %v.", role, err)
	}
	var config struct {
		Browser struct {
			Isolated    bool   `json:"isolated"`
			UserDataDir string `json:"userDataDir"`
		} `json:"browser"`
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		return "", fmt.Sprintf("The %s role's Playwright config is not valid JSON: %v.", role, err)
	}
	switch {
	case config.Browser.Isolated:
		return "", fmt.Sprintf("The %s role's Playwright browser is isolated, so it keeps no profile.", role)
	case config.Browser.UserDataDir == "":
		return "", fmt.Sprintf("The %s role's Playwright config names no userDataDir.", role)
	case !filepath.IsAbs(config.Browser.UserDataDir):
		return "", fmt.Sprintf("The %s role's Playwright userDataDir is not an absolute path.", role)
	}
	return filepath.Clean(config.Browser.UserDataDir), ""
}

// profileLockPID is the live pid in dir's SingletonLock (a symlink to
// "host-pid"), 0 when free. A dead pid is a stale lock Chromium clears itself.
func profileLockPID(dir string) int {
	target, err := os.Readlink(filepath.Join(dir, "SingletonLock"))
	if err != nil {
		return 0
	}
	cut := strings.LastIndexByte(target, '-')
	pid, err := strconv.Atoi(target[cut+1:])
	if cut < 0 || err != nil || pid <= 0 {
		return 0
	}
	if err := syscall.Kill(pid, 0); err != nil && !errors.Is(err, syscall.EPERM) {
		return 0
	}
	return pid
}

// chooseProfile is the role's Playwright profile when nothing holds it, else
// a temporary one with the reason why not.
func (d *daemon) chooseProfile(session string) browserProfile {
	var role string
	if s := d.session(session); s != nil {
		role = s.role
	}
	dir, why := rolePlaywrightDir(role)
	if dir == "" {
		return browserProfile{note: tempProfileNote + " " + why}
	}
	if holder, ok := d.browsers.claims.claim(dir, session); !ok {
		return browserProfile{note: fmt.Sprintf("%s The %s role's Playwright profile is in use by session %s, so its logins are not here.", tempProfileNote, role, holder)}
	}
	release := func() { d.browsers.claims.release(dir, session) }
	if pid := profileLockPID(dir); pid != 0 {
		release()
		return browserProfile{note: fmt.Sprintf("%s The %s role's Playwright profile is locked by process %d outside aterm, so its logins are not here.", tempProfileNote, role, pid)}
	}
	return browserProfile{dir: dir, note: fmt.Sprintf("The %s role's Playwright profile, so its logins are here.", role), release: release}
}
