package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// liveNames answers claim the way the daemon does: a name that is live gets the
// next slot suffix, and peek never holds anything.
func liveNames(live ...string) func(string, bool) string {
	taken := map[string]bool{}
	for _, name := range live {
		taken[name] = true
	}
	return func(base string, _ bool) string {
		return poolName(base, func(name string) bool { return taken[name] })
	}
}

var angie = identityPool{Base: "Angie", Adjectives: []string{"Fast", "Calm", "Warm"}}

// inOrder fixes the draw, so a test names the adjective it expects.
func inOrder(t *testing.T) {
	t.Helper()
	was := shuffleAdjectives
	shuffleAdjectives = func([]string) {}
	t.Cleanup(func() { shuffleAdjectives = was })
}

func TestChooseIdentityDrawsAFreeAdjective(t *testing.T) {
	inOrder(t)
	for _, test := range []struct {
		name string
		live []string
		want string
	}{
		{"none live takes the first drawn", nil, "Fast Angie"},
		{"a live adjective is skipped", []string{"eng-platform-fast-angie"}, "Calm Angie"},
		{"two live leave the third", []string{"eng-platform-fast-angie", "eng-platform-calm-angie"}, "Warm Angie"},
		{"the bare base name does not hold an adjective", []string{"eng-platform-angie"}, "Fast Angie"},
		{"every adjective live falls back to the slot suffix", []string{"eng-platform-fast-angie", "eng-platform-calm-angie", "eng-platform-warm-angie"}, ""},
	} {
		got, err := chooseIdentity(angie, "eng-platform", "", "", liveNames(test.live...))
		if err != nil || got != test.want {
			t.Errorf("%s: got %q, %v, want %q", test.name, got, err, test.want)
		}
	}
}

// With the real shuffle, a draw never meets a name a live session holds.
func TestAnUnfixedDrawNeverTakesALiveName(t *testing.T) {
	live := liveNames("eng-platform-fast-angie", "eng-platform-calm-angie")
	for range 200 {
		if got, err := chooseIdentity(angie, "eng-platform", "", "", live); err != nil || got != "Warm Angie" {
			t.Fatalf("got %q, %v, want the only free adjective", got, err)
		}
	}
}

func TestChooseIdentityWithNothingToChooseFrom(t *testing.T) {
	if got, err := chooseIdentity(identityPool{Base: "Angie"}, "eng-platform", "", "", liveNames()); err != nil || got != "" {
		t.Errorf("a roster with no adjectives: got %q, %v", got, err)
	}
	if got, err := chooseIdentity(angie, "eng-platform", "", "", nil); err != nil || got != "" {
		t.Errorf("no daemon to ask: got %q, %v", got, err)
	}
}

func TestChooseIdentityHonorsARequestedNameAndRefusesTheRest(t *testing.T) {
	if got, err := chooseIdentity(angie, "eng-platform", "Calm Angie", "", liveNames()); err != nil || got != "Calm Angie" {
		t.Errorf("got %q, %v", got, err)
	}
	if got, err := chooseIdentity(angie, "eng-platform", "Angie", "", liveNames()); err != nil || got != "" {
		t.Errorf("the base name needs no rename: got %q, %v", got, err)
	}
	for _, name := range []string{"Vera", "Fast Vera", "fast Angie", "Fast Calm Angie", "Angie Fast", "Fast  Angie", "Brisk Angie"} {
		_, err := chooseIdentity(angie, "eng-platform", name, "", liveNames())
		if err == nil || !strings.Contains(err.Error(), `not one of eng-platform's names ("Angie", or an adjective`) {
			t.Errorf("%q: got %v, want a refusal", name, err)
		}
	}
}

// A resume prefers the seat's old name, but a name the roster has retired is dropped.
func TestChooseIdentityPrefersAResumedNameOnlyWhileItIsAllowedAndFree(t *testing.T) {
	inOrder(t)
	for _, test := range []struct {
		name      string
		preferred string
		live      []string
		want      string
	}{
		{"an allowed free name is kept", "Warm Angie", nil, "Warm Angie"},
		{"the base name keeps the bare form", "Angie", nil, ""},
		{"a name a live seat holds is not taken twice", "Warm Angie", []string{"eng-platform-warm-angie"}, "Fast Angie"},
		{"a retired alternate is dropped for a draw", "Bea", nil, "Fast Angie"},
		{"a retired adjective is dropped for a draw", "Brisk Angie", nil, "Fast Angie"},
	} {
		got, err := chooseIdentity(angie, "eng-platform", "", test.preferred, liveNames(test.live...))
		if err != nil || got != test.want {
			t.Errorf("%s: got %q, %v, want %q", test.name, got, err, test.want)
		}
	}
}

// A base name is one opaque string, so a single word works the same as a pair.
func TestAnAdjectiveSitsBeforeAnyBaseName(t *testing.T) {
	for _, base := range []string{"Beetle-Ox", "Beetle", "Mister Beetle"} {
		pool := identityPool{Base: base, Adjectives: []string{"Fast"}}
		if !pool.allows("Fast "+base) || !pool.allows(base) || pool.allows("Slow "+base) {
			t.Errorf("%q: the pool reads the base name's parts", base)
		}
	}
}

func TestIdentityPoolReadsTheRoster(t *testing.T) {
	roster := rosterDocument{
		Items: []rosterRole{
			{Slug: "eng-platform", Identity: rosterIdentity{Name: "Angie"}},
			{Slug: "scientist", Identity: rosterIdentity{Name: "Moss"}},
		},
		IdentityAdjectives: []string{"Fast", "Calm"},
	}
	if got := identityPoolOf(roster, "eng-platform"); got.Base != "Angie" || strings.Join(got.Adjectives, ",") != "Fast,Calm" {
		t.Errorf("got %+v", got)
	}
	if got := identityPoolOf(roster, "nobody"); got.Base != "" || got.allows("Fast ") || got.allows("") {
		t.Errorf("got %+v for a role that is not there", got)
	}
}

func TestTheChildCarriesTheChosenNameOnlyWhenThereIsOne(t *testing.T) {
	request := launchRequest{Role: "eng-platform", Seat: "claude", ShadowID: "ab84"}
	for _, shadowed := range []bool{true, false} {
		plain := composeChild(request, "/ac", "/aos", shadowed)
		if plain[0] == "/usr/bin/env" {
			t.Errorf("shadowed=%v: an unnamed launch gained an env prefix: %v", shadowed, plain)
		}
		request.Identity = "Fast Angie"
		named := composeChild(request, "/ac", "/aos", shadowed)
		if named[0] != "/usr/bin/env" || named[1] != "AGENT_COMPOSE_IDENTITY=Fast Angie" {
			t.Errorf("shadowed=%v: got %v", shadowed, named)
		}
		request.Identity = ""
	}
}

// aterm launches with a dry run twice here, once per live set, and reads the
// plan back as a client would.
func dryRunPlan(t *testing.T, live []string, argv ...string) launchPlan {
	t.Helper()
	var spawns []recordedSpawn
	deps := stubDeps(t, &spawns, true)
	inner := deps.output
	deps.output = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "catalog" {
			var document map[string]any
			if err := json.Unmarshal(fixture(t, "roster.json"), &document); err != nil {
				return nil, err
			}
			document["identity_adjectives"] = []string{"Fast", "Calm", "Warm"}
			return json.Marshal(document)
		}
		if name == "/usr/bin/env" && len(args) > 2 && strings.HasPrefix(args[0], identityEnv+"=") {
			raw, err := inner(ctx, args[1], args[2:]...)
			if err != nil {
				return nil, err
			}
			var document map[string]any
			if err := json.Unmarshal(raw, &document); err != nil {
				return nil, err
			}
			document["seat"].(map[string]any)["name"] = strings.TrimPrefix(args[0], identityEnv+"=")
			return json.Marshal(document)
		}
		return inner(ctx, name, args...)
	}
	deps.claim = liveNames(live...)
	out, err := runAterm(t, deps, append([]string{"--dry-run", "--json"}, argv...)...)
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	var plan launchPlan
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatalf("read the plan: %v\n%s", err, out)
	}
	return plan
}

func TestEverySeatOfARoleIsLaunchedUnderAnAdjective(t *testing.T) {
	inOrder(t)
	first := dryRunPlan(t, nil, "eng-platform", "claude")
	if first.Identity.Name != "Fast Angie" || first.Identity.Instance != "" {
		t.Fatalf("the first seat is %q slot %q", first.Identity.Name, first.Identity.Instance)
	}
	if first.Child[0] != "/usr/bin/env" || first.Child[1] != "AGENT_COMPOSE_IDENTITY=Fast Angie" {
		t.Errorf("the launch does not carry the name: %v", first.Child)
	}
	second := dryRunPlan(t, []string{"eng-platform-fast-angie"}, "eng-platform", "claude")
	if second.Identity.Name != "Calm Angie" || second.Identity.Instance != "" {
		t.Fatalf("the second seat is %q slot %q, want Calm Angie with no slot suffix", second.Identity.Name, second.Identity.Instance)
	}
	if second.Card.Name != "Calm Angie" {
		t.Errorf("the card and so the stamp say %q, want Calm Angie", second.Card.Name)
	}
}

func TestIdentityFlagNamesTheSeat(t *testing.T) {
	plan := dryRunPlan(t, nil, "--identity", "Warm Angie", "eng-platform", "claude")
	if plan.Identity.Name != "Warm Angie" || plan.Child[1] != "AGENT_COMPOSE_IDENTITY=Warm Angie" {
		t.Errorf("got %q, %v", plan.Identity.Name, plan.Child)
	}
}

func TestPreferIdentityKeepsAResumedSeatsNameAndDropsARetiredOne(t *testing.T) {
	inOrder(t)
	if plan := dryRunPlan(t, nil, "--prefer-identity", "Warm Angie", "eng-platform", "claude"); plan.Identity.Name != "Warm Angie" {
		t.Errorf("a resumed seat came back as %q", plan.Identity.Name)
	}
	if plan := dryRunPlan(t, nil, "--prefer-identity", "Bea", "eng-platform", "claude"); plan.Identity.Name != "Fast Angie" {
		t.Errorf("a retired alternate gave %q, want a drawn Fast Angie", plan.Identity.Name)
	}
}

// An agent-compose that predates the variable renders the base name again, so the
// launch must not claim a name the card does not show.
func TestAnOldAgentComposeFallsBackToTheSlotSuffix(t *testing.T) {
	inOrder(t)
	var spawns []recordedSpawn
	deps := stubDeps(t, &spawns, true)
	inner := deps.output
	deps.output = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if name == "/usr/bin/env" && len(args) > 2 {
			return inner(ctx, args[1], args[2:]...)
		}
		if len(args) > 0 && args[0] == "catalog" {
			var document map[string]any
			if err := json.Unmarshal(fixture(t, "roster.json"), &document); err != nil {
				return nil, err
			}
			document["identity_adjectives"] = []string{"Fast"}
			return json.Marshal(document)
		}
		return inner(ctx, name, args...)
	}
	deps.claim = liveNames("eng-platform-angie")
	out, err := runAterm(t, deps, "--dry-run", "--json", "eng-platform", "claude")
	if err != nil {
		t.Fatal(err)
	}
	var plan launchPlan
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatal(err)
	}
	if plan.Identity.Name != "Angie" || plan.Identity.Instance != "2" {
		t.Errorf("got %q slot %q, want Angie slot 2", plan.Identity.Name, plan.Identity.Instance)
	}
	if plan.Child[0] == "/usr/bin/env" {
		t.Errorf("the launch claims a name the card does not show: %v", plan.Child)
	}
}
