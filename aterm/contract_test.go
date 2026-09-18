package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"testing"
)

// The fixtures prove aterm still works on the roster it was written for, which
// is not the failure that happens. Upstream drift is. See docs/aterm.md.
const liveRosterEnv = "ATERM_LIVE_ROSTER"

// liveDeps talks to the real agent-compose. A host without it skips, unless
// ATERM_LIVE_ROSTER says this host was supposed to have one.
func liveDeps(t *testing.T) (commandDeps, string) {
	t.Helper()
	deps := systemDeps()
	deps.notice = nil
	agentCompose, err := requireBinary(deps.lookPath, defaultOverlayBin)
	if err != nil {
		if strings.TrimSpace(os.Getenv(liveRosterEnv)) != "" {
			t.Fatalf("%s is set but agent-compose is not on PATH: %v", liveRosterEnv, err)
		}
		t.Skipf("no agent-compose on PATH, so the live roster cannot be read. Set %s to make this a failure.", liveRosterEnv)
	}
	return deps, agentCompose
}

func liveRoster(t *testing.T) (commandDeps, string, rosterDocument) {
	t.Helper()
	deps, agentCompose := liveDeps(t)
	roster, err := loadRoster(context.Background(), deps, agentCompose)
	if err != nil {
		t.Fatalf("read the live roster: %v", err)
	}
	return deps, agentCompose, roster
}

func liveOverlay(t *testing.T, deps commandDeps, agentCompose, role, seat string) ([]byte, overlayDocument) {
	t.Helper()
	command := []string{agentCompose, "overlay", "--role", role, "--seat", seat,
		"--expression", defaultExpression, "--json"}
	raw, err := deps.output(context.Background(), command[0], command[1:]...)
	if err != nil {
		t.Fatalf("read the %s %s overlay: %v", role, seat, err)
	}
	document, err := parseOverlay(raw, role, seat, defaultExpression)
	if err != nil {
		t.Fatalf("parse the %s %s overlay: %v", role, seat, err)
	}
	return raw, document
}

// The check that would have caught this whole backlog: anything the overlay
// ships and the struct drops is a failure on the day it is added.
func TestLiveOverlayDiscardsNoField(t *testing.T) {
	deps, agentCompose, roster := liveRoster(t)
	// An entry here needs a reason, not a shrug. These five come out once the
	// released roster drops them too. Why, and when: agentic-os#1285.
	waived := map[string]string{}
	for index := 0; index < 4; index++ {
		for _, path := range []string{
			"emblem.glyph", "emblem.name",
			"form.silhouette", "form.geometry", "form.motion",
		} {
			waived[fmt.Sprintf("personalities[%d].%s", index, path)] =
				"retired by agent-compose#364, still shipped by an older installed roster"
		}
	}
	for _, role := range roster.Items {
		for _, seat := range role.nativeSeats() {
			name := role.Slug + "/" + seat.Harness
			t.Run(name, func(t *testing.T) {
				raw, document := liveOverlay(t, deps, agentCompose, role.Slug, seat.Harness)
				encoded, err := json.Marshal(document)
				if err != nil {
					t.Fatalf("re-encode: %v", err)
				}
				shipped := leaves(t, raw)
				decoded := leaves(t, encoded)
				dropped := []string{}
				for path, want := range shipped {
					if reason, excused := waived[path]; excused {
						t.Logf("waived %s: %s", path, reason)
						continue
					}
					got, present := decoded[path]
					if !present {
						dropped = append(dropped, path+" (dropped)")
						continue
					}
					if got != want {
						dropped = append(dropped, fmt.Sprintf("%s (%v, want %v)", path, got, want))
					}
				}
				sort.Strings(dropped)
				for _, entry := range dropped {
					t.Errorf("the live overlay ships %s and aterm does not keep it", entry)
				}
			})
		}
	}
}

func TestLiveRosterEveryLaunchableSeatResolvesToAPlan(t *testing.T) {
	deps, agentCompose, roster := liveRoster(t)
	launchable := 0
	for _, role := range roster.Items {
		for _, seat := range role.nativeSeats() {
			launchable++
			t.Run(role.Slug+"/"+seat.Harness, func(t *testing.T) {
				_, document := liveOverlay(t, deps, agentCompose, role.Slug, seat.Harness)
				plan, err := buildLaunchPlan(document, launchRequest{
					Role: role.Slug, Seat: seat.Harness, Expression: defaultExpression,
					Workspace: "repo@branch", TerminalBin: defaultTerminalBin,
				}, t.TempDir(), "/stub/aterm", agentCompose, "/stub/aos", true)
				if err != nil {
					t.Fatalf("build a plan: %v", err)
				}
				if !strings.HasPrefix(plan.Brand.Title, "repo@branch"+titleSeparator) {
					t.Fatalf("title = %q, want the workspace first", plan.Brand.Title)
				}
				if !hexColorPattern.MatchString(plan.Brand.Background) {
					t.Fatalf("background = %q", plan.Brand.Background)
				}
				if len(plan.Card.Figures) != len(document.Personalities) {
					t.Fatalf("card carries %d of %d personalities",
						len(plan.Card.Figures), len(document.Personalities))
				}
			})
		}
	}
	if launchable == 0 {
		t.Fatal("the live roster has no launchable seat at all")
	}
}

// A catalogue seat outside the native set is real and unlaunchable, and the
// refusal has to say which of the two it failed.
func TestLiveRosterRefusesEveryNonNativeSeat(t *testing.T) {
	deps, _, roster := liveRoster(t)
	deps.tty = func() bool { return false }
	checked := 0
	for _, role := range roster.Items {
		for _, seat := range role.Seats {
			if isNativeHarness(seat.Harness) {
				continue
			}
			checked++
			_, _, _, err := resolveInvocation(context.Background(), deps, "/stub/aos", roster,
				[]string{role.Slug, seat.Harness})
			if err == nil {
				t.Fatalf("%s %s is not launchable and was accepted", role.Slug, seat.Harness)
			}
			if exitCodeFor(err) != exitOffRoster {
				t.Fatalf("%s %s refused with exit %d", role.Slug, seat.Harness, exitCodeFor(err))
			}
			if !strings.Contains(err.Error(), "no native harness to launch") {
				t.Fatalf("%s %s refused with %q", role.Slug, seat.Harness, err)
			}
		}
	}
	if checked == 0 {
		t.Skip("the live roster ships no unlaunchable catalogue seat to refuse")
	}
}

// A ratchet rather than a target. agent-compose#358 raises the floor; this
// stops it falling, which is the part aterm can hold on its own.
const backgroundSeparationFloor = 3.0

func TestLiveRoleBackgroundsStaySeparable(t *testing.T) {
	deps, agentCompose, roster := liveRoster(t)
	backgrounds := map[string][3]uint8{}
	for _, role := range roster.Items {
		seats := role.nativeSeats()
		if len(seats) == 0 {
			continue
		}
		_, document := liveOverlay(t, deps, agentCompose, role.Slug, seats[0].Harness)
		brand, err := buildBrand(document, "", "repo@branch")
		if err != nil {
			t.Fatalf("brand %s: %v", role.Slug, err)
		}
		parsed, err := parseHex(brand.Background)
		if err != nil {
			t.Fatalf("parse %s background: %v", role.Slug, err)
		}
		backgrounds[role.Slug] = parsed
	}
	slugs := make([]string, 0, len(backgrounds))
	for slug := range backgrounds {
		slugs = append(slugs, slug)
	}
	sort.Strings(slugs)
	// agent-compose#500 color_twin: an exact dE 0.00 match is a declared twin,
	// not a collision the solver failed to separate.
	worst, worstPair := math.MaxFloat64, ""
	for first := 0; first < len(slugs); first++ {
		for second := first + 1; second < len(slugs); second++ {
			distance := labDistance(backgrounds[slugs[first]], backgrounds[slugs[second]])
			if distance == 0 {
				t.Logf("twinned background pair %s/%s at dE 0.00, exempt from the floor", slugs[first], slugs[second])
				continue
			}
			if distance < worst {
				worst, worstPair = distance, slugs[first]+"/"+slugs[second]
			}
		}
	}
	t.Logf("closest non-twinned background pair %s at dE %.2f across %d roles", worstPair, worst, len(slugs))
	if worst < backgroundSeparationFloor {
		t.Fatalf("%s are dE %.2f apart, under the %.1f floor",
			worstPair, worst, backgroundSeparationFloor)
	}
}

// The default seat is configuration aos owns, so this asks aos rather than
// parsing the profiles a second time.
func TestLiveDefaultSeatBelongsToItsRole(t *testing.T) {
	deps, _, roster := liveRoster(t)
	aos, err := requireBinary(deps.lookPath, defaultAOSBin)
	if err != nil {
		t.Skipf("no aos on PATH, so the launch profiles cannot be resolved: %v", err)
	}
	asked, refused := 0, 0
	firstRefusal := ""
	for _, role := range roster.Items {
		if len(role.nativeSeats()) == 0 {
			continue
		}
		asked++
		raw, err := deps.output(context.Background(), aos, "_launch-agent", role.Slug)
		if err != nil {
			refused++
			if firstRefusal == "" {
				firstRefusal = err.Error()
			}
			continue
		}
		seat := strings.TrimSpace(string(raw))
		if !isNativeHarness(seat) || !seatInRole(seat, role) {
			t.Fatalf("%s pins default seat %q, which it cannot launch", role.Slug, seat)
		}
	}
	// aterm degrades to the catalogue order against an aos too old for the
	// verb, so this does too. Refusing some roles and not others is the defect.
	if refused == asked && asked > 0 {
		t.Skipf("this aos does not answer `_launch-agent`, so it is too old for the verb: %s", firstRefusal)
	}
	if refused > 0 {
		t.Fatalf("%d of %d roles have no default agent, first: %s", refused, asked, firstRefusal)
	}
}
