package main

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func completionLines(t *testing.T, args ...string) []string {
	t.Helper()
	out := &bytes.Buffer{}
	writeCompletions(out, loadRosterFixture(t), args)
	text := strings.TrimSpace(out.String())
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

func TestCompletionOffersEveryLiveRoleFirst(t *testing.T) {
	lines := completionLines(t)
	slugs := make([]string, 0, len(lines))
	for _, line := range lines {
		slug, detail, found := strings.Cut(line, ":")
		if !found || detail == "" {
			t.Fatalf("every candidate needs a description: %q", line)
		}
		slugs = append(slugs, slug)
	}
	for _, want := range []string{"eng-platform", "sysadmin-senior", "scientist", "frontend-eng", "game-dev", "prod-director", "dev-advocate"} {
		if !contains(slugs, want) {
			t.Fatalf("completion should offer %q: %v", want, slugs)
		}
	}
	// The fixture archives analyst, and completing into a refusal is the one
	// thing completion exists to prevent.
	if contains(slugs, "analyst") {
		t.Fatalf("completion should not offer the archived role: %v", slugs)
	}
}

func TestCompletionOffersOnlyTheChosenRoleSeats(t *testing.T) {
	lines := completionLines(t, "sysadmin-senior")
	seats := make([]string, 0, len(lines))
	for _, line := range lines {
		seat, _, _ := strings.Cut(line, ":")
		seats = append(seats, seat)
	}
	if !contains(seats, "goose") {
		t.Fatalf("senior-sysadmin should offer its goose seat: %v", seats)
	}
	// goose belongs to senior-sysadmin alone, so it must not leak into another role.
	platform := completionLines(t, "eng-platform")
	for _, line := range platform {
		if strings.HasPrefix(line, "goose:") {
			t.Fatalf("goose is not a platform seat: %v", platform)
		}
	}
}

// A catalogue seat with no native harness cannot be launched, so completing it
// would offer a candidate the launcher then refuses.
func TestCompletionNeverOffersAnUnlaunchableSeat(t *testing.T) {
	for _, line := range completionLines(t, "frontend-eng") {
		if strings.HasPrefix(line, "penpot:") {
			t.Fatal("penpot has no native harness and must not be completable")
		}
	}
}

func TestCompletionIsSilentPastTheSeat(t *testing.T) {
	if lines := completionLines(t, "eng-platform", "claude"); lines != nil {
		t.Fatalf("harness arguments are not ours to complete: %v", lines)
	}
}

func TestCompletionIsSilentForARoleThatLeftTheRoster(t *testing.T) {
	if lines := completionLines(t, "engineer"); lines != nil {
		t.Fatalf("a stale role has no seats to offer: %v", lines)
	}
}

// Both shipped shell scripts split a candidate on its first colon, so a colon
// or newline in a description would truncate or forge a candidate.
func TestCompletionDetailSurvivesTheShellScripts(t *testing.T) {
	cases := map[string]string{
		"plain":              "plain",
		"with: a colon":      "with  a colon",
		"first\nsecond":      "first",
		"  padded  ":         "padded",
		"trailing\r\nsecond": "trailing",
	}
	for input, want := range cases {
		if got := completionDetail(input); got != want {
			t.Fatalf("completionDetail(%q) = %q, want %q", input, got, want)
		}
	}
	for _, line := range completionLines(t) {
		if strings.Count(line, ":") != 1 {
			t.Fatalf("a candidate must carry exactly one colon: %q", line)
		}
	}
}

func TestSeatDetailDegradesWithoutPronounsOrTier(t *testing.T) {
	cases := []struct {
		seat rosterSeat
		want string
	}{
		{rosterSeat{Name: "Angie", Pronouns: "she", Tier: "frontier"}, "Angie [she] // frontier"},
		{rosterSeat{Name: "Angie", Pronouns: "she"}, "Angie [she]"},
		{rosterSeat{Name: "Angie"}, "Angie"},
		{rosterSeat{Tier: "frontier"}, "frontier"},
		{rosterSeat{}, ""},
	}
	for _, testCase := range cases {
		if got := seatDetail(testCase.seat); got != testCase.want {
			t.Fatalf("seatDetail(%+v) = %q, want %q", testCase.seat, got, testCase.want)
		}
	}
}

// Completion runs on a keystroke, so an unreachable roster has to be silence
// rather than a diagnostic printed mid-word. See docs/aterm.md.
func TestCompletionStaysSilentWhenTheRosterIsUnreachable(t *testing.T) {
	var spawns []recordedSpawn
	deps := stubDeps(t, &spawns, true)
	deps.output = func(context.Context, string, ...string) ([]byte, error) {
		return nil, context.DeadlineExceeded
	}
	command := newCommand(deps)
	out := &bytes.Buffer{}
	command.Writer = out
	err := command.Run(context.Background(), []string{"aterm", "--generate-shell-completion"})
	if err != nil {
		t.Fatalf("completion must not fail the shell: %v", err)
	}
	if out.String() != "" {
		t.Fatalf("an unreachable roster should complete nothing: %q", out.String())
	}
}

// Enabling completion adds a `completion` subcommand, which must not capture
// the first positional the launcher reads as a role.
func TestCompletionSubcommandDoesNotShadowTheRolePositional(t *testing.T) {
	var spawns []recordedSpawn
	out, err := runAterm(t, stubDeps(t, &spawns, true), "--dry-run", "--json", "eng-platform", "claude")
	if err != nil {
		t.Fatalf("a role positional should still launch: %v", err)
	}
	if !strings.Contains(out, `"role": "eng-platform"`) {
		t.Fatalf("the role positional was lost: %s", out)
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func sessionCompletionLines(views ...sessionView) []string {
	out := &bytes.Buffer{}
	writeSessionCompletions(out, views)
	text := strings.TrimSpace(out.String())
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

func TestSessionCompletionOffersTheNameWithRoleAndIdentity(t *testing.T) {
	lines := sessionCompletionLines(
		sessionView{Name: "scientist-evie-cd12", Role: "scientist", Identity: "Evie"},
		sessionView{Name: "eng-platform-beetle-ox-dj89", Role: "eng-platform", Identity: "Beetle-Ox"},
	)
	want := []string{
		"eng-platform-beetle-ox-dj89:eng-platform // Beetle-Ox",
		"scientist-evie-cd12:scientist // Evie",
	}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("lines = %q, want %q", lines, want)
	}
}

func TestSessionCompletionDegradesAndNeverBreaksTheColonSplit(t *testing.T) {
	cases := []struct {
		view sessionView
		want string
	}{
		{sessionView{Name: "a", Role: "scientist"}, "a:scientist"},
		{sessionView{Name: "a", Identity: "Evie"}, "a:Evie"},
		{sessionView{Name: "a"}, "a"},
		{sessionView{Name: "a:b", Role: "scientist"}, `a\:b:scientist`},
		{sessionView{Name: "a", Role: "sci:entist", Identity: "Evie\nsecond line"}, "a:sci entist // Evie"},
	}
	for _, testCase := range cases {
		lines := sessionCompletionLines(testCase.view)
		if len(lines) != 1 || lines[0] != testCase.want {
			t.Fatalf("%+v -> %q, want %q", testCase.view, lines, testCase.want)
		}
	}
	if lines := sessionCompletionLines(sessionView{Role: "scientist"}); lines != nil {
		t.Fatalf("a session with no name is not completable: %q", lines)
	}
}

func headlessSession(t *testing.T, role, name, instance string) {
	t.Helper()
	options := sessionOptions{
		Daemon:   true,
		Headless: true,
		Card:     sessionCard{Role: role, Name: name, Seat: "claude", Instance: instance},
		Argv:     []string{"/bin/sh", "-c", `printf 'UP\033[?2004h\n'; cat`},
	}
	var stderr bytes.Buffer
	if code := runSession(options, strings.NewReader(""), io.Discard, &stderr); code != 0 {
		t.Fatalf("headless stage exited %d: %s", code, stderr.String())
	}
}

func completeThrough(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var spawns []recordedSpawn
	command := newCommand(stubDeps(t, &spawns, true))
	out := &bytes.Buffer{}
	command.Writer = out
	err := command.Run(context.Background(), append([]string{"aterm"}, append(args, "--generate-shell-completion")...))
	return out.String(), err
}

// Every command whose first argument is a live session offers the daemon's
// list, the one `aterm agents` prints, with no new verb on the daemon.
func TestEverySessionCommandCompletesTheLiveSessions(t *testing.T) {
	testDaemon(t)
	headlessSession(t, "eng-platform", "Beetle-Ox", "dj89")
	headlessSession(t, "scientist", "Evie", "cd12")
	for deadline := time.Now().Add(5 * time.Second); len(liveSessionViews(time.Second)) < 2; time.Sleep(50 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("the daemon never listed both sessions")
		}
	}
	want := "eng-platform-beetle-ox-dj89:eng-platform // Beetle-Ox\nscientist-evie-cd12:scientist // Evie\n"
	for _, sub := range []string{"attach", "send", "status", "close", "clear"} {
		got, err := completeThrough(t, sub)
		if err != nil {
			t.Fatalf("%s completion must not fail the shell: %v", sub, err)
		}
		if got != want {
			t.Fatalf("%s completed %q, want %q", sub, got, want)
		}
	}
}

func TestSessionCompletionIsSilentPastTheSessionArgument(t *testing.T) {
	testDaemon(t)
	headlessSession(t, "scientist", "Evie", "cd12")
	// For `send` the second argument is the message, which is not ours to complete.
	got, err := completeThrough(t, "send", "scientist-evie-cd12")
	if err != nil || got != "" {
		t.Fatalf("send past its target completed %q, %v", got, err)
	}
}

func TestSessionCompletionStaysSilentWithoutADaemon(t *testing.T) {
	t.Setenv(daemonSocketEnv, filepath.Join(t.TempDir(), "d.sock"))
	for _, sub := range []string{"attach", "send", "status", "close", "clear"} {
		got, err := completeThrough(t, sub)
		if err != nil || got != "" {
			t.Fatalf("%s with no daemon completed %q, %v", sub, got, err)
		}
	}
}
