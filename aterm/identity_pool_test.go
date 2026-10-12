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

var angiePool = []string{"Angie", "Bea", "Cy"}

func TestChooseIdentityTakesTheFirstFreeName(t *testing.T) {
	for _, test := range []struct {
		name string
		live []string
		want string
	}{
		{"none live keeps the primary", nil, ""},
		{"primary live takes the first alternate", []string{"eng-platform-angie"}, "Bea"},
		{"two live take the second alternate", []string{"eng-platform-angie", "eng-platform-bea"}, "Cy"},
		{"a freed primary is reused before an alternate", []string{"eng-platform-bea"}, ""},
		{"an exhausted pool falls back to the slot suffix", []string{"eng-platform-angie", "eng-platform-bea", "eng-platform-cy"}, ""},
	} {
		got, err := chooseIdentity(angiePool, "eng-platform", "", liveNames(test.live...))
		if err != nil || got != test.want {
			t.Errorf("%s: got %q, %v, want %q", test.name, got, err, test.want)
		}
	}
}

func TestChooseIdentityWithNothingToChooseFrom(t *testing.T) {
	if got, err := chooseIdentity([]string{"Angie"}, "eng-platform", "", liveNames("eng-platform-angie")); err != nil || got != "" {
		t.Errorf("a one-name pool: got %q, %v", got, err)
	}
	if got, err := chooseIdentity(angiePool, "eng-platform", "", nil); err != nil || got != "" {
		t.Errorf("no daemon to ask: got %q, %v", got, err)
	}
}

func TestChooseIdentityHonorsARequestedNameInThePool(t *testing.T) {
	got, err := chooseIdentity(angiePool, "eng-platform", "Cy", liveNames())
	if err != nil || got != "Cy" {
		t.Errorf("got %q, %v", got, err)
	}
	if got, err := chooseIdentity(angiePool, "eng-platform", "Angie", liveNames()); err != nil || got != "" {
		t.Errorf("the primary needs no rename: got %q, %v", got, err)
	}
	if _, err := chooseIdentity(angiePool, "eng-platform", "Vera", liveNames()); err == nil ||
		!strings.Contains(err.Error(), "not one of eng-platform's names (Angie, Bea, Cy)") {
		t.Errorf("another role's name: %v", err)
	}
}

func TestIdentityPoolReadsTheRoster(t *testing.T) {
	roster := rosterDocument{Items: []rosterRole{
		{Slug: "eng-platform", Identity: rosterIdentity{Name: "Angie", Alternates: []string{"Bea"}}},
		{Slug: "scientist", Identity: rosterIdentity{Name: "Moss"}},
	}}
	if got := identityPool(roster, "eng-platform"); strings.Join(got, ",") != "Angie,Bea" {
		t.Errorf("got %v", got)
	}
	if got := identityPool(roster, "nobody"); got != nil {
		t.Errorf("got %v for a role that is not there", got)
	}
}

func TestTheChildCarriesTheChosenNameOnlyWhenThereIsOne(t *testing.T) {
	request := launchRequest{Role: "eng-platform", Seat: "claude", ShadowID: "ab84"}
	for _, shadowed := range []bool{true, false} {
		plain := composeChild(request, "/ac", "/aos", shadowed)
		if plain[0] == "/usr/bin/env" {
			t.Errorf("shadowed=%v: an unnamed launch gained an env prefix: %v", shadowed, plain)
		}
		request.Identity = "Bea"
		named := composeChild(request, "/ac", "/aos", shadowed)
		if named[0] != "/usr/bin/env" || named[1] != "AGENT_COMPOSE_IDENTITY=Bea" {
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
			for _, item := range document["items"].([]any) {
				role := item.(map[string]any)
				if role["slug"] == "eng-platform" {
					role["identity"].(map[string]any)["alternates"] = []string{"Bea", "Cy"}
				}
			}
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

func TestASecondInstanceOfARoleIsLaunchedUnderItsOwnName(t *testing.T) {
	first := dryRunPlan(t, nil, "eng-platform", "claude")
	if first.Identity.Name != "Angie" || first.Identity.Instance != "" {
		t.Fatalf("the first instance is %q slot %q", first.Identity.Name, first.Identity.Instance)
	}
	second := dryRunPlan(t, []string{"eng-platform-angie"}, "eng-platform", "claude")
	if second.Identity.Name != "Bea" || second.Identity.Instance != "" {
		t.Fatalf("the second instance is %q slot %q, want Bea with no slot suffix", second.Identity.Name, second.Identity.Instance)
	}
	if second.Child[0] != "/usr/bin/env" || second.Child[1] != "AGENT_COMPOSE_IDENTITY=Bea" {
		t.Errorf("the launch does not carry the name: %v", second.Child)
	}
	third := dryRunPlan(t, []string{"eng-platform-angie", "eng-platform-bea"}, "eng-platform", "claude")
	if third.Identity.Name != "Cy" {
		t.Errorf("the third instance is %q", third.Identity.Name)
	}
	if second.Card.Name != "Bea" {
		t.Errorf("the card and so the stamp say %q, want Bea", second.Card.Name)
	}
}

func TestIdentityFlagNamesTheInstance(t *testing.T) {
	plan := dryRunPlan(t, nil, "--identity", "Cy", "eng-platform", "claude")
	if plan.Identity.Name != "Cy" || plan.Child[1] != "AGENT_COMPOSE_IDENTITY=Cy" {
		t.Errorf("got %q, %v", plan.Identity.Name, plan.Child)
	}
}

// An agent-compose that predates the variable renders the primary again, so the
// launch must not claim a name the card does not show.
func TestAnOldAgentComposeFallsBackToTheSlotSuffix(t *testing.T) {
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
			for _, item := range document["items"].([]any) {
				if role := item.(map[string]any); role["slug"] == "eng-platform" {
					role["identity"].(map[string]any)["alternates"] = []string{"Bea"}
				}
			}
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
