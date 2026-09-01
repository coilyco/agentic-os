package main

import (
	"fmt"
	"os"
	"strings"
)

// aos sets this exactly when the process is running inside a per-session
// shadow, so it is the declared marker rather than a derived one.
const nativeSessionEnv = "AOS_NATIVE_SESSION"

func nativeSession() string {
	return strings.TrimSpace(os.Getenv(nativeSessionEnv))
}

// refuseNestedLaunch stops a launch whose window could only show an error, for
// the reason docs/aterm.md gives. agentic-os#1460
func refuseNestedLaunch() error {
	session := nativeSession()
	if session == "" {
		return nil
	}
	return withExit(exitNested, fmt.Errorf(
		"refusing to launch from inside native session %s: the window inherits HOME=%s, so aos "+
			"resolves its projects root under that home while the repository plan reached "+
			"through it names a different one, and the session dies before the harness starts. "+
			"Launch from the role's Dock bundle, or from a terminal outside this session",
		session, sessionHome(),
	))
}

func sessionHome() string {
	if home := strings.TrimSpace(os.Getenv("HOME")); home != "" {
		return home
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "the session home"
	}
	return home
}
