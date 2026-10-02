package main

import (
	"regexp"
	"strings"
	"time"
)

// A session's state, as the screen and the output clock read it. See
// docs/aterm-daemon.md.
const (
	stateStarting = "starting"
	stateBusy     = "busy"
	stateIdle     = "idle"
	// statePrompt is a permission or choice prompt holding the session for a person.
	statePrompt = "prompt"
)

// busyWindow is how recent output counts as work. A harness repaints its
// spinner many times a second, so a few seconds of silence is not working.
const busyWindow = 3 * time.Second

// promptScreens are the harnesses' own words while they wait on a person, so a
// reworded one reads as idle until it is added here.
var promptScreens = map[string][]*regexp.Regexp{
	"claude": {
		regexp.MustCompile(`(?i)do you want to (proceed|make this edit|create|allow|run)`),
		regexp.MustCompile(`(?i)esc to cancel`),
		regexp.MustCompile(`(?i)enter to select`),
	},
	"codex": {
		regexp.MustCompile(`(?i)would you like to (run|make|allow)`),
		regexp.MustCompile(`(?i)yes, proceed`),
	},
	"opencode": {
		regexp.MustCompile(`(?i)permission required`),
		regexp.MustCompile(`(?i)allow once`),
	},
}

// busyScreens are the texts a harness shows only while a turn runs, for the
// stretch where a tool is silent and nothing repaints.
var busyScreens = map[string]*regexp.Regexp{
	// The claude spinner reads `(12m 25s · ↓ 44.5k tokens)`, read off a live session.
	"claude":   regexp.MustCompile(`(?i)esc to interrupt|\(\d+m? ?\d*s · [↑↓] [0-9.,]+k? tokens\)`),
	"codex":    regexp.MustCompile(`(?i)esc to interrupt`),
	"opencode": regexp.MustCompile(`(?i)esc interrupt`),
}

// classify names what a session is doing from its screen lines, its seat, whether
// it can take a message yet, and how long it has been quiet.
func classify(seat string, lines []string, ready bool, quiet time.Duration) string {
	if !ready {
		return stateStarting
	}
	text := strings.Join(lines, "\n")
	for _, mark := range promptScreens[seat] {
		if mark.MatchString(text) {
			return statePrompt
		}
	}
	if quiet < busyWindow {
		return stateBusy
	}
	if mark, ok := busyScreens[seat]; ok && mark.MatchString(text) {
		return stateBusy
	}
	return stateIdle
}
