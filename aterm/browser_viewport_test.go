package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// argvBrowserBinary is an ATERM_BROWSER that writes the arguments it was started
// with to the returned file, then exits.
func argvBrowserBinary(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	record := filepath.Join(dir, "argv")
	script := filepath.Join(dir, "fake-chrome")
	body := "#!/bin/sh\nprintf '%s\\n' \"$@\" > '" + record + "'\nexit 0\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv(browserEnv, script)
	return record
}

// startedWith launches a fresh browser and returns the arguments Chromium received.
func startedWith(t *testing.T) []string {
	t.Helper()
	record := argvBrowserBinary(t)
	link, err := launchChromium(testHoldDir(t), "viewport", browserProfile{})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-link.exited:
	case <-time.After(5 * time.Second):
		t.Fatal("the fake browser should have exited")
	}
	raw, err := os.ReadFile(record)
	if err != nil {
		t.Fatalf("the fake browser did not record its arguments: %v", err)
	}
	return strings.Split(strings.TrimSpace(string(raw)), "\n")
}

func windowSize(args []string) []string {
	var found []string
	for _, arg := range args {
		if strings.HasPrefix(arg, "--window-size=") {
			found = append(found, arg)
		}
	}
	return found
}

func TestAFreshBrowserStartsAtAPortraitPhoneViewport(t *testing.T) {
	t.Setenv(browserViewportEnv, "")

	got := windowSize(startedWith(t))

	if len(got) != 1 || got[0] != "--window-size=390,844" {
		t.Fatalf("window size args = %v, want exactly --window-size=390,844", got)
	}
}

func TestTheViewportIsSettableByEnvironment(t *testing.T) {
	t.Setenv(browserViewportEnv, "1280x800")

	got := windowSize(startedWith(t))

	if len(got) != 1 || got[0] != "--window-size=1280,800" {
		t.Fatalf("window size args = %v, want the desktop size an operator asked for", got)
	}
}

func TestAMalformedOrOutOfRangeViewportFallsBackToThePhoneDefault(t *testing.T) {
	for _, raw := range []string{"banana", "390", "0x0", "100x100", "9000x9000", "390x", "x844", "-390x844"} {
		t.Setenv(browserViewportEnv, raw)

		if w, h := browserViewport(); w != 390 || h != 844 {
			t.Errorf("browserViewport(%q) = %dx%d, want 390x844", raw, w, h)
		}
	}
}

func TestTheViewportTakesAnUppercaseXAndSpaces(t *testing.T) {
	t.Setenv(browserViewportEnv, " 320 X 640 ")

	if w, h := browserViewport(); w != 320 || h != 640 {
		t.Fatalf("browserViewport = %dx%d, want 320x640", w, h)
	}
}
