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
	// statePrompt is a screen holding the session for a person: a card, a menu, a sign-in.
	statePrompt = "prompt"
)

// What the screen detector names, the finer reading behind the state above. Every one
// but busy, idle, starting and unknown holds the seat for a person (COI-3788).
const (
	screenChoice     = "choice"
	screenPermission = "permission"
	screenMCP        = "mcp_menu"
	screenResume     = "resume_picker"
	screenLogin      = "login"
	screenUnknown    = "unknown"
)

// busyWindow is how recent output counts as work. A harness repaints its
// spinner many times a second, so a few seconds of silence is not working.
const busyWindow = 3 * time.Second

// screenSignature is one screen a harness draws while it waits on a person. Every mark
// must match the screen text. The first signature that matches names the screen.
type screenSignature struct {
	kind  string
	marks []*regexp.Regexp
}

func signature(kind string, marks ...string) screenSignature {
	compiled := make([]*regexp.Regexp, len(marks))
	for index, mark := range marks {
		compiled[index] = regexp.MustCompile(mark)
	}
	return screenSignature{kind: kind, marks: compiled}
}

// heldScreens are each harness's words while it waits on a person, tried in order. A
// reworded one reads unknown until added. Claude's come from testdata/claude-screens.
var heldScreens = map[string][]screenSignature{
	"claude": {
		signature(screenPermission, `(?i)do you want to (proceed|make this edit|create|allow|run)`),
		// The menu and one server's page under it. A heading row, so the slash-command
		// hint "Manage MCP servers" never reads as the menu.
		signature(screenMCP, `(?m)^\s{0,4}MCP servers?$`, `(?i)esc to (close|go back)`),
		signature(screenResume, `(?m)^\s{0,4}Resume session\b`, `(?i)esc to close`),
		// A short terminal scrolls the heading away, and the footer stays on the last rows.
		signature(screenMCP, `(?i)enter to view`, `(?i)esc to close`),
		signature(screenResume, `(?i)enter to resume`, `(?i)esc to close`),
		signature(screenLogin, `(?i)select login method|paste code here if prompted|oauth error:|login successful|choose the text style that looks best`),
		signature(screenChoice, `(?i)enter to (select|confirm)`),
		// The review page that ends a multi-question AskUserQuestion draws no footer,
		// and Enter on it submits the answers. testdata/claude-screens/review.txt
		signature(screenChoice, `(?i)ready to submit your answers`),
		signature(screenChoice, `(?i)esc to cancel`),
	},
	"codex": {
		signature(screenPermission, `(?i)would you like to (run|make|allow)`),
		signature(screenPermission, `(?i)yes, proceed`),
	},
	"opencode": {
		signature(screenPermission, `(?i)permission required`),
		signature(screenPermission, `(?i)allow once`),
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

// idleScreens are each harness's empty input box, so idle is a reading and not the
// absence of one. A quiet screen none of these match is unknown.
var idleScreens = map[string]*regexp.Regexp{
	// The ruled box, or the older boxed one with its shortcut hint.
	"claude":   regexp.MustCompile(`(?m)^\s*─{8,}\n❯|\? for shortcuts`),
	"codex":    regexp.MustCompile(`(?m)^\s*› .*(\n.*){0,3}\n\s*\? for shortcuts`),
	"opencode": regexp.MustCompile(`ctrl\+p commands`),
	"goose":    regexp.MustCompile(`(?m)^>\s+Enter to send`),
}

// heldScreen is what holds the seat for a person, or "" when nothing does.
func heldScreen(seat, text string) string {
	for _, sig := range heldScreens[seat] {
		matched := true
		for _, mark := range sig.marks {
			if !mark.MatchString(text) {
				matched = false
				break
			}
		}
		if matched {
			return sig.kind
		}
	}
	return ""
}

// showsCard is whether the screen holds the seat for a person, so Enter would answer it.
func showsCard(seat, text string) bool { return heldScreen(seat, text) != "" }

// detectScreen names what a seat's screen shows. A held screen reads even before the
// seat is ready, since a sign-in screen is why one never becomes ready.
func detectScreen(seat string, lines []string, ready bool, quiet time.Duration) string {
	text := strings.Join(lines, "\n")
	if kind := heldScreen(seat, text); kind != "" {
		return kind
	}
	if !ready {
		return stateStarting
	}
	if quiet < busyWindow {
		return stateBusy
	}
	if mark, ok := busyScreens[seat]; ok && mark.MatchString(text) {
		return stateBusy
	}
	if mark, ok := idleScreens[seat]; ok && mark.MatchString(text) {
		return stateIdle
	}
	return screenUnknown
}

// stateOf folds a screen reading into the coarse state, where unknown stays idle as it
// always read.
func stateOf(kind string, ready bool) string {
	switch {
	case !ready:
		return stateStarting
	case kind == stateBusy:
		return stateBusy
	case kind == stateIdle || kind == screenUnknown:
		return stateIdle
	}
	return statePrompt
}

// classify names what a session is doing from its screen lines, its seat, whether
// it can take a message yet, and how long it has been quiet.
func classify(seat string, lines []string, ready bool, quiet time.Duration) string {
	return stateOf(detectScreen(seat, lines, ready, quiet), ready)
}

// sessionStatus is the read-only answer to `aterm status`: what the session is
// doing and what its screen shows, without typing into it or focusing a window.
type sessionStatus struct {
	Name  string `json:"name"`
	State string `json:"state"`
	// TUI is the detector's finer reading. See detectScreen.
	TUI   string `json:"tui,omitempty"`
	Ready bool   `json:"ready"`
	// QuietSeconds is the time since the session last wrote, and InputSeconds the
	// time since anyone typed into it, or -1 when no one has.
	QuietSeconds int  `json:"quiet_seconds"`
	InputSeconds int  `json:"input_seconds"`
	Drafted      bool `json:"kai_drafting"`
	Pending      int  `json:"pending"`
	Clients      int  `json:"clients"`
	Rows         int  `json:"rows"`
	Cols         int  `json:"cols"`
	// Prompt is the screen around a permission or choice card, when one is up.
	Prompt []string `json:"prompt,omitempty"`
	Screen []string `json:"screen"`
	// Awake is the host's keep-awake, not this session's.
	Awake *awakeState `json:"host_awake,omitempty"`
}

const (
	defaultStatusLines = 30
	maxStatusLines     = 200
	// promptBefore and promptAfter bound the rows kept around a prompt's first line.
	promptBefore = 4
	promptAfter  = 12
)

// promptLines is the rows around the first one a held screen's mark matches, or nil.
func promptLines(seat string, lines []string) []string {
	if !showsCard(seat, strings.Join(lines, "\n")) {
		return nil
	}
	for index, line := range lines {
		for _, sig := range heldScreens[seat] {
			for _, mark := range sig.marks {
				if mark.MatchString(line) {
					return lines[max(index-promptBefore, 0):min(index+promptAfter, len(lines))]
				}
			}
		}
	}
	return nil
}

// tailLines is the last n rows, with n held between 1 and maxStatusLines.
func tailLines(lines []string, n int) []string {
	if n <= 0 {
		n = defaultStatusLines
	}
	n = min(n, maxStatusLines)
	return append([]string{}, lines[max(len(lines)-n, 0):]...)
}
