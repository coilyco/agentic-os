package main

import (
	"fmt"
	"os"
	"strings"
)

// aos sets these exactly when the process is running inside a per-session
// shadow, so they are declared markers rather than derived ones.
const (
	nativeSessionEnv         = "AOS_NATIVE_SESSION"
	nativeSessionRootEnv     = "AOS_NATIVE_SESSION_ROOT"
	nativeSessionProjectsEnv = "AOS_NATIVE_SESSION_PROJECTS"
	canonicalHomeEnv         = "AOS_NATIVE_CANONICAL_HOME"
	canonicalProjectsEnv     = "AOS_NATIVE_CANONICAL_PROJECTS"
	// A launched window is a session rather than a subagent of the one that
	// opened it, and inheriting this marker turns its transcript off.
	childSessionEnv = "CLAUDE_CODE_CHILD_SESSION"
)

// canonicalLaunch is what a session inside a shadow needs in order to open one
// that is not. See docs/aterm.md.
type canonicalLaunch struct {
	Session  string
	Root     string
	Home     string
	Projects string
}

func readCanonicalLaunch() canonicalLaunch {
	return canonicalLaunch{
		Session:  strings.TrimSpace(os.Getenv(nativeSessionEnv)),
		Root:     strings.TrimSpace(os.Getenv(nativeSessionRootEnv)),
		Home:     strings.TrimSpace(os.Getenv(canonicalHomeEnv)),
		Projects: strings.TrimSpace(os.Getenv(canonicalProjectsEnv)),
	}
}

func (launch canonicalLaunch) inShadow() bool { return launch.Session != "" }

func (launch canonicalLaunch) complete() bool {
	return launch.Root != "" && launch.Home != "" && launch.Projects != ""
}

// refuseNestedLaunch stops a launch whose window could only show an error. An
// aos too old to publish the canonical values is the remaining case.
func refuseNestedLaunch() error {
	launch := readCanonicalLaunch()
	if !launch.inShadow() || launch.complete() {
		return nil
	}
	return withExit(exitNested, fmt.Errorf(
		"refusing to launch from inside native session %s: the aos that opened it does not "+
			"publish %s, %s, and %s, so the new window would inherit this session's home and "+
			"die before the harness starts. Upgrade aos, or launch from the role's Dock bundle",
		launch.Session, nativeSessionRootEnv, canonicalHomeEnv, canonicalProjectsEnv,
	))
}

// canonicalEnviron drops every value pointing into this session's shadow and
// restores the canonical home and projects root. See docs/aterm.md.
func canonicalEnviron(environ []string, launch canonicalLaunch) []string {
	if !launch.inShadow() || !launch.complete() {
		return nil
	}
	kept := make([]string, 0, len(environ)+2)
	for _, entry := range environ {
		name, value, split := strings.Cut(entry, "=")
		if !split {
			continue
		}
		switch name {
		case nativeSessionEnv, nativeSessionRootEnv, nativeSessionProjectsEnv,
			canonicalHomeEnv, canonicalProjectsEnv, childSessionEnv:
			continue
		case "PATH":
			kept = append(kept, "PATH="+withoutShadowPath(value, launch.Root))
			continue
		}
		if underRoot(value, launch.Root) {
			continue
		}
		kept = append(kept, entry)
	}
	return append(kept, "HOME="+launch.Home, defaultWorkingEnvVar+"="+launch.Projects)
}

// underRoot matches a whole path rather than a substring, so a value that
// merely mentions the root is kept.
func underRoot(value, root string) bool {
	return value == root || strings.HasPrefix(value, root+string(os.PathSeparator))
}

func withoutShadowPath(value, root string) string {
	entries := strings.Split(value, string(os.PathListSeparator))
	kept := make([]string, 0, len(entries))
	for _, entry := range entries {
		if underRoot(entry, root) {
			continue
		}
		kept = append(kept, entry)
	}
	return strings.Join(kept, string(os.PathListSeparator))
}
