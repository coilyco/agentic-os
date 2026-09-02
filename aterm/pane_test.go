package main

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// fakeKitty answers remote control from a tab it holds in memory, so every test
// here exercises the real argv aterm would send without a kitty to send it to.
type fakeKitty struct {
	tab      kittyTab
	calls    [][]string
	refuses  bool
	sessions int
}

const paneTestSocket = "unix:/tmp/kitty-test"

// sessionWindow is the window aterm opened: its argv carries the session card,
// which is the only thing `off` needs to re-derive the plate.
func sessionWindow(t *testing.T, id int, role string, columns int) kittyWindow {
	t.Helper()
	encoded, err := encodeSessionCard(sessionCard{
		Format: cardFormat, Role: role, Seat: "claude", Annotation: "Angie [she]",
	})
	if err != nil {
		t.Fatalf("encode the session card: %v", err)
	}
	return kittyWindow{
		ID:      id,
		Columns: columns,
		Cmdline: []string{
			"/stub/aterm", sessionCommand, "--card", encoded, "--",
			"/stub/agent-compose", "launch", role, "claude",
		},
		UserVars: map[string]string{},
	}
}

func previewWindow(id int, tag string, columns int) kittyWindow {
	return kittyWindow{
		ID:       id,
		Columns:  columns,
		Cmdline:  []string{"/bin/sh", "-c", "sleep 60"},
		UserVars: map[string]string{paneVarName: tag},
	}
}

func (fake *fakeKitty) deps(t *testing.T) commandDeps {
	t.Helper()
	// The plate is cached under the user cache directory, so a temporary home
	// keeps these answers about the code rather than about this machine.
	t.Setenv("HOME", t.TempDir())
	t.Setenv(listenEnv, paneTestSocket)
	t.Setenv(windowIDEnv, "")
	return commandDeps{
		lookPath: func(name string) (string, error) { return "/stub/" + filepath.Base(name), nil },
		output: func(_ context.Context, _ string, args ...string) ([]byte, error) {
			fake.calls = append(fake.calls, args)
			if len(args) < 4 || args[0] != "@" || args[1] != "--to" {
				return nil, fmt.Errorf("unexpected argv: %v", args)
			}
			switch args[3] {
			case "ls":
				if fake.refuses {
					return nil, fmt.Errorf("Could not connect to specified socket")
				}
				fake.sessions++
				raw, err := json.Marshal([]kittyOSWindow{
					{ID: 1, IsActive: true, Tabs: []kittyTab{fake.tab}},
				})
				return raw, err
			case "launch":
				// kitty rearranges the tab on a split, and the measurement that
				// follows has to see that rather than the pre-split window.
				tag := ""
				for index, value := range args {
					if value == "--var" && index+1 < len(args) {
						tag = strings.TrimPrefix(args[index+1], paneVarName+"=")
					}
				}
				fake.tab.Layout = paneSplitLayout
				fake.tab.Windows[0].Columns = 83
				fake.tab.Windows = append(fake.tab.Windows, previewWindow(9, tag, 84))
				return []byte("9\n"), nil
			}
			return []byte(""), nil
		},
	}
}

func (fake *fakeKitty) sent(verb string) [][]string {
	matched := [][]string{}
	for _, call := range fake.calls {
		if len(call) > 3 && call[3] == verb {
			matched = append(matched, call)
		}
	}
	return matched
}

func runPane(t *testing.T, deps commandDeps, argv ...string) (string, error) {
	t.Helper()
	return runAterm(t, deps, append([]string{"pane"}, argv...)...)
}

func paneResult(t *testing.T, out string) paneReport {
	t.Helper()
	var report paneReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("pane --json did not emit JSON: %v\n%s", err, out)
	}
	if report.Format != paneFormat {
		t.Fatalf("format = %q", report.Format)
	}
	return report
}

func singleSession(t *testing.T, role string) *fakeKitty {
	t.Helper()
	return &fakeKitty{tab: kittyTab{
		ID: 1, IsActive: true, Layout: paneRestoreLayout,
		Windows: []kittyWindow{sessionWindow(t, 1, role, 168)},
	}}
}

func splitSession(t *testing.T, role, tag string) *fakeKitty {
	t.Helper()
	return &fakeKitty{tab: kittyTab{
		ID: 1, IsActive: true, Layout: paneSplitLayout,
		Windows: []kittyWindow{sessionWindow(t, 1, role, 83), previewWindow(9, tag, 84)},
	}}
}

// The whole point of the verb: a window left split by an agent that has since
// died still restores, because nothing `on` recorded is needed to do it.
func TestPaneOffRestoresWithoutAnythingOnLeftBehind(t *testing.T) {
	fake := splitSession(t, "platform", defaultPaneTag)
	out, err := runPane(t, fake.deps(t), "off", "--json")
	if err != nil {
		t.Fatalf("pane off: %v\n%s", err, out)
	}
	report := paneResult(t, out)
	if report.Role != "platform" {
		t.Fatalf("role = %q, want the slug read back out of the session card", report.Role)
	}
	if !report.Changed {
		t.Fatal("pane off closed no pane when one was open")
	}
	want := bakeCreaturePlate("platform", creaturePresence, creatureWholeWindow).Path
	if want == "" {
		t.Fatal("the platform role baked no plate, so this test proves nothing")
	}
	if report.Plate != want {
		t.Fatalf("restored %q, want the whole-window plate %q", report.Plate, want)
	}
	background := fake.sent("set-background-image")
	if len(background) != 1 {
		t.Fatalf("set the background %d time(s), want once", len(background))
	}
	if got := background[0][len(background[0])-1]; got != want {
		t.Fatalf("kitty was handed %q, want %q", got, want)
	}
	if len(fake.sent("close-window")) != 1 {
		t.Fatal("pane off left the preview pane open")
	}
	if layouts := fake.sent("goto-layout"); len(layouts) != 1 || layouts[0][4] != paneRestoreLayout {
		t.Fatalf("goto-layout calls = %v, want one %s", layouts, paneRestoreLayout)
	}
}

// A second `off` is the ordinary case, not a failure: the caller does not know
// whether the pane is still there, which is why it is running the verb.
func TestPaneOffIsIdempotentWithNoPaneOpen(t *testing.T) {
	fake := singleSession(t, "platform")
	out, err := runPane(t, fake.deps(t), "off", "--json")
	if err != nil {
		t.Fatalf("pane off with no pane: %v\n%s", err, out)
	}
	if report := paneResult(t, out); report.Changed {
		t.Fatal("pane off reported a change with no pane open")
	}
	if closes := fake.sent("close-window"); len(closes) != 0 {
		t.Fatalf("asked kitty to close %d window(s) that do not exist", len(closes))
	}
	if len(fake.sent("set-background-image")) != 1 {
		t.Fatal("pane off should restore the creature whether or not a pane was open")
	}
}

// --configured rewrites what new windows inherit, so the blast radius would
// escape the session this command is scoped to.
func TestPaneNeverConfiguresTheBackgroundGlobally(t *testing.T) {
	for _, argv := range [][]string{{"off"}, {"on", "--", "/bin/sh", "-c", "sleep 60"}} {
		fake := singleSession(t, "platform")
		if _, err := runPane(t, fake.deps(t), argv...); err != nil {
			t.Fatalf("pane %v: %v", argv, err)
		}
		for _, call := range fake.calls {
			for _, value := range call {
				if value == "--configured" {
					t.Fatalf("pane %v passed --configured: %v", argv, call)
				}
			}
		}
	}
}

func TestPaneOnSplitsTaggedAndMovesTheCreature(t *testing.T) {
	fake := singleSession(t, "platform")
	out, err := runPane(t, fake.deps(t), "on", "--json", "--", "/bin/sh", "-c", "sleep 60")
	if err != nil {
		t.Fatalf("pane on: %v\n%s", err, out)
	}
	report := paneResult(t, out)
	if report.Share < 0.45 || report.Share > 0.55 {
		t.Fatalf("measured share %.3f, want about half the window", report.Share)
	}
	whole := bakeCreaturePlate("platform", creaturePresence, creatureWholeWindow).Path
	if report.Plate == whole {
		t.Fatal("the split plate is the launch plate, so the creature never moved")
	}
	launches := fake.sent("launch")
	if len(launches) != 1 {
		t.Fatalf("launched %d pane(s), want one", len(launches))
	}
	launch := strings.Join(launches[0], " ")
	for _, want := range []string{"--dont-take-focus", "--var " + paneVarName + "=" + defaultPaneTag, "sleep 60"} {
		if !strings.Contains(launch, want) {
			t.Fatalf("launch argv %q is missing %q", launch, want)
		}
	}
	if layouts := fake.sent("goto-layout"); len(layouts) != 1 || layouts[0][4] != paneSplitLayout {
		t.Fatalf("goto-layout calls = %v, want one %s before the launch", layouts, paneSplitLayout)
	}
}

// The creature has to land inside what the session keeps, which is the whole
// reason the plate is recomposed rather than left alone.
func TestSplitPlateKeepsTheCreatureInsideTheSurvivingPane(t *testing.T) {
	const share = 0.5
	art := creatureArt(roleIcon("platform"))
	if art == nil {
		t.Fatal("the platform icon carries no art")
	}
	source := decodePlateArt(t, art)
	plate := composeCreaturePlate(source, share)
	bounds := plate.Bounds()
	limit := int(float64(bounds.Dx()) * share)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := limit; x < bounds.Max.X; x++ {
			if _, _, _, alpha := plate.At(x, y).RGBA(); alpha != 0 {
				t.Fatalf("ink at %d,%d crosses the divider at %d of %d", x, y, limit, bounds.Dx())
			}
		}
	}
}

func TestPaneRefusesASecondPaneOnTheSameTag(t *testing.T) {
	fake := splitSession(t, "platform", defaultPaneTag)
	out, err := runPane(t, fake.deps(t), "on", "--", "/bin/sh")
	if err == nil {
		t.Fatalf("pane on opened a second pane on the same tag: %s", out)
	}
	if !strings.Contains(err.Error(), "already open") {
		t.Fatalf("error %q does not name the open pane", err)
	}
	if len(fake.sent("launch")) != 0 {
		t.Fatal("pane on launched despite refusing")
	}
}

func TestPanePreflightNamesAnUnsetSocket(t *testing.T) {
	fake := singleSession(t, "platform")
	deps := fake.deps(t)
	t.Setenv(listenEnv, "")
	out, err := runPane(t, deps, "off")
	if err == nil {
		t.Fatalf("pane off ran without a socket: %s", out)
	}
	for _, want := range []string{listenEnv, "allow_remote_control"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("blocker %q does not name %q", err, want)
		}
	}
	if exitCodeFor(err) != exitUsage {
		t.Fatalf("exit code = %d, want %d", exitCodeFor(err), exitUsage)
	}
}

func TestPanePreflightNamesRefusedRemoteControl(t *testing.T) {
	fake := singleSession(t, "platform")
	fake.refuses = true
	out, err := runPane(t, fake.deps(t), "off")
	if err == nil {
		t.Fatalf("pane off ran against a kitty that refused: %s", out)
	}
	if !strings.Contains(err.Error(), "allow_remote_control") {
		t.Fatalf("blocker %q does not name what governs it", err)
	}
}

func TestPaneNamesAWindowWithNoSessionCard(t *testing.T) {
	fake := &fakeKitty{tab: kittyTab{
		ID: 1, IsActive: true, Windows: []kittyWindow{previewWindow(3, "other", 168)},
	}}
	out, err := runPane(t, fake.deps(t), "off")
	if err == nil {
		t.Fatalf("pane off guessed a role: %s", out)
	}
	if !strings.Contains(err.Error(), "--role") {
		t.Fatalf("error %q does not name the way out", err)
	}
}

func TestPaneTakesAnExplicitRoleOverTheCard(t *testing.T) {
	fake := splitSession(t, "platform", defaultPaneTag)
	out, err := runPane(t, fake.deps(t), "off", "--json", "--role", "frontend")
	if err != nil {
		t.Fatalf("pane off --role: %v\n%s", err, out)
	}
	if report := paneResult(t, out); report.Role != "frontend" {
		t.Fatalf("role = %q, want the explicit flag to win", report.Role)
	}
}

// A zoom keybinding selects `stack`, where every window reports the full width,
// so the ratio would shrink a creature whose session still fills the window.
func TestSharePrefersTheWholeWindowOnAnUnmeasurableLayout(t *testing.T) {
	stacked := kittyTab{
		Layout: "stack",
		Windows: []kittyWindow{
			{Columns: 168, UserVars: map[string]string{}},
			{Columns: 168, UserVars: map[string]string{paneVarName: defaultPaneTag}},
		},
	}
	if got := survivingShare(stacked, defaultPaneTag); got != creatureWholeWindow {
		t.Fatalf("share under stack = %.3f, want the whole window", got)
	}
	split := stacked
	split.Layout = paneSplitLayout
	split.Windows = []kittyWindow{
		{Columns: 83, UserVars: map[string]string{}},
		{Columns: 84, UserVars: map[string]string{paneVarName: defaultPaneTag}},
	}
	if got := survivingShare(split, defaultPaneTag); got < 0.45 || got > 0.55 {
		t.Fatalf("share under %s = %.3f, want about half", paneSplitLayout, got)
	}
}
