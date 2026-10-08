package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// roleConfigs points the daemon at a temp config directory holding one role's
// Playwright config, userDataDir to be filled in by the caller.
func roleConfigs(t *testing.T, role, body string) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv(playwrightConfigEnv, dir)
	if body != "" {
		if err := os.WriteFile(filepath.Join(dir, "local_coilyco_playwright_"+role+".json"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func profileConfig(userDataDir string) string {
	return fmt.Sprintf(`{"browser":{"isolated":false,"userDataDir":%q,"launchOptions":{"headless":true}}}`, userDataDir)
}

func profileDaemon(roles map[string]string) *daemon {
	d := &daemon{sessions: map[string]*ptySession{}}
	for name, role := range roles {
		d.sessions[name] = &ptySession{name: name, role: role}
	}
	return d
}

func TestRolePlaywrightDirReadsTheRolesConfigAndRefusesWhatCannotBeShared(t *testing.T) {
	good := t.TempDir()
	cases := []struct {
		name, role, body, wantDir, wantWhy string
	}{
		{"configured", "eng-platform", profileConfig(good), good, ""},
		{"no file", "eng-platform", "", "", "No Playwright profile is configured for the eng-platform role"},
		{"isolated", "eng-platform", `{"browser":{"isolated":true,"userDataDir":"/x"}}`, "", "isolated"},
		{"no dir", "eng-platform", `{"browser":{}}`, "", "names no userDataDir"},
		{"relative", "eng-platform", profileConfig("profiles/x"), "", "not an absolute path"},
		{"broken json", "eng-platform", `{`, "", "not valid JSON"},
		{"unsafe role", "../etc", "", "", "no role"},
		{"no role", "", "", "", "no role"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			roleConfigs(t, tc.role, tc.body)
			dir, why := rolePlaywrightDir(tc.role)
			if dir != tc.wantDir || !strings.Contains(why, tc.wantWhy) {
				t.Fatalf("dir %q why %q, want dir %q why containing %q", dir, why, tc.wantDir, tc.wantWhy)
			}
		})
	}
}

func TestChooseProfileGivesTheFirstSessionOfARoleItsProfileAndTheSecondATemporaryOne(t *testing.T) {
	profile := filepath.Join(t.TempDir(), "role-profile")
	roleConfigs(t, "eng-platform", profileConfig(profile))
	d := profileDaemon(map[string]string{"eng-platform-a": "eng-platform", "eng-platform-b": "eng-platform", "scientist-c": "scientist"})

	first := d.chooseProfile("eng-platform-a")
	if first.dir != profile || first.release == nil || !strings.Contains(first.note, "its logins are here") {
		t.Fatalf("the first session should get the role profile: %+v", first)
	}
	second := d.chooseProfile("eng-platform-b")
	if second.dir != "" || !strings.Contains(second.note, "in use by session eng-platform-a") {
		t.Fatalf("the second session should get a temporary profile naming the first: %+v", second)
	}
	other := d.chooseProfile("scientist-c")
	if other.dir != "" || !strings.Contains(other.note, "No Playwright profile is configured for the scientist role") {
		t.Fatalf("a role without a config should say so: %+v", other)
	}

	first.release()
	if again := d.chooseProfile("eng-platform-b"); again.dir != profile {
		t.Fatalf("the profile should be free once the first session's browser ends: %+v", again)
	}
}

func TestChooseProfileFallsBackWhenAnotherProcessHoldsTheProfileLock(t *testing.T) {
	profile := t.TempDir()
	roleConfigs(t, "eng-platform", profileConfig(profile))
	d := profileDaemon(map[string]string{"eng-platform-a": "eng-platform"})
	lock := filepath.Join(profile, "SingletonLock")

	if err := os.Symlink(fmt.Sprintf("somehost-%d", os.Getpid()), lock); err != nil {
		t.Fatal(err)
	}
	held := d.chooseProfile("eng-platform-a")
	if held.dir != "" || !strings.Contains(held.note, fmt.Sprintf("locked by process %d outside aterm", os.Getpid())) {
		t.Fatalf("a live lock should send the session to a temporary profile: %+v", held)
	}
	if holder, ok := d.browsers.claims.claim(profile, "probe"); !ok {
		t.Fatalf("a refused profile must not stay claimed by %s", holder)
	}
	d.browsers.claims.release(profile, "probe")

	// A stale lock names a pid that is gone, which Chromium clears itself.
	_ = os.Remove(lock)
	if err := os.Symlink("somehost-999999999", lock); err != nil {
		t.Fatal(err)
	}
	if stale := d.chooseProfile("eng-platform-a"); stale.dir != profile {
		t.Fatalf("a stale lock should not keep the profile out of use: %+v", stale)
	}
}

// fakeBrowserBinary is an ATERM_BROWSER that exits at once, so launchChromium's
// profile handling is judged without a real Chromium.
func fakeBrowserBinary(t *testing.T) {
	t.Helper()
	script := filepath.Join(t.TempDir(), "fake-chrome")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv(browserEnv, script)
}

func TestLaunchChromiumKeepsARoleProfileAndReleasesItWhenTheBrowserExits(t *testing.T) {
	fakeBrowserBinary(t)
	profile := filepath.Join(t.TempDir(), "role-profile")
	var claims profileClaims
	if _, ok := claims.claim(profile, "eng-platform-a"); !ok {
		t.Fatal("claim")
	}
	link, err := launchChromium(t.TempDir(), "eng-platform-a", browserProfile{
		dir: profile, note: "kept", release: func() { claims.release(profile, "eng-platform-a") },
	})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-link.exited:
	case <-time.After(5 * time.Second):
		t.Fatal("the fake browser should have exited")
	}
	if _, err := os.Stat(profile); err != nil {
		t.Fatalf("a role profile must outlive its browser: %v", err)
	}
	if holder, ok := claims.claim(profile, "next"); !ok {
		t.Fatalf("the profile should be released once the browser exits, held by %q", holder)
	}
	if link.note != "kept" {
		t.Fatalf("note = %q", link.note)
	}
}

func TestLaunchChromiumRemovesAThrowawayProfileAndReleasesAClaimWhenItFailsToStart(t *testing.T) {
	fakeBrowserBinary(t)
	dir := testHoldDir(t)
	link, err := launchChromium(dir, "scientist-c", browserProfile{})
	if err != nil {
		t.Fatal(err)
	}
	<-link.exited
	if left, _ := filepath.Glob(filepath.Join(dir, "*")); len(left) != 0 {
		t.Fatalf("a throwaway profile should go with its browser: %v", left)
	}

	t.Setenv(browserEnv, filepath.Join(dir, "does-not-exist"))
	released := false
	_, err = launchChromium(dir, "eng-platform-a", browserProfile{dir: filepath.Join(dir, "p"), release: func() { released = true }})
	if err == nil || !released {
		t.Fatalf("a browser that cannot start must give the profile back, err %v released %v", err, released)
	}
}

// A cookie set in one launch on a role profile is there in the next, which is
// what carries a login from the role's Playwright profile into the stream.
func TestRealChromiumRoleProfileKeepsACookieAcrossLaunches(t *testing.T) {
	if chromiumPath() == "" {
		t.Skip("no Chromium or Chrome on this host")
	}
	profile := filepath.Join(t.TempDir(), "role-profile")
	hold := testHoldDir(t)
	run := func(use func(ctx context.Context, cdp *cdpClient)) {
		t.Helper()
		link, err := launchChromium(hold, "probe", browserProfile{dir: profile})
		if err != nil {
			t.Fatalf("launch: %v", err)
		}
		cdp := newCDPClient(link.r, link.w, func(string, string, json.RawMessage) {})
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		use(ctx, cdp)
		link.stop()
	}
	run(func(ctx context.Context, cdp *cdpClient) {
		cookie := map[string]any{"name": "login", "value": "kept", "domain": "example.com", "path": "/", "expires": float64(time.Now().Add(24 * time.Hour).Unix())}
		if _, err := cdp.call(ctx, "", "Storage.setCookies", map[string]any{"cookies": []any{cookie}}); err != nil {
			t.Fatalf("set cookie: %v", err)
		}
	})
	run(func(ctx context.Context, cdp *cdpClient) {
		raw, err := cdp.call(ctx, "", "Storage.getCookies", nil)
		if err != nil || !strings.Contains(string(raw), `"value":"kept"`) {
			t.Fatalf("the second launch should find the login: %v %s", err, raw)
		}
	})
}
