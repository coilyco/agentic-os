package main

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func TestHasNameFlagSeesEveryWayACallerNamesASession(t *testing.T) {
	for arguments, want := range map[string]bool{
		"--resume": false, "--name x": true, "-n x": true, "--name=x": true, "--model sonnet --name x": true,
	} {
		if got := hasNameFlag(strings.Fields(arguments)); got != want {
			t.Fatalf("hasNameFlag(%q) = %v, want %v", arguments, got, want)
		}
	}
}
func TestLaunchPlanNamesOnlyClaudeSessionsTheCallerLeftUnnamed(t *testing.T) {
	for _, testCase := range []struct {
		name  string
		args  []string
		named bool
	}{
		{"claude", []string{"eng-platform", "claude"}, true},
		{"claude opted out", []string{"--no-stable-name", "eng-platform", "claude"}, false},
		{"claude with the caller's own name", []string{"eng-platform", "claude", "--", "--name", "mine"}, false},
		{"another seat", []string{"prod-director", "codex"}, false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			var spawns []recordedSpawn
			args := append([]string{"--dry-run", "--json"}, testCase.args...)
			out, err := runAterm(t, stubDeps(t, &spawns, true), args...)
			if err != nil {
				t.Fatalf("dry run: %v", err)
			}
			var plan launchPlan
			if err := json.Unmarshal([]byte(out), &plan); err != nil {
				t.Fatalf("decode plan: %v", err)
			}
			if plan.StableName != testCase.named {
				t.Fatalf("plan.StableName = %v, want %v", plan.StableName, testCase.named)
			}
			// agent-compose drops its own name when it is handed one, so the flag
			// has to reach it ahead of the caller's own arguments.
			carriesName := slices.Contains(plan.Child, sessionName("Angie", "eng-platform", stubInstance))
			if carriesName != testCase.named {
				t.Fatalf("the child should carry the name only when named: %v", plan.Child)
			}
		})
	}
}

func TestSessionNameIsForWhoAnswersAndWhichInstance(t *testing.T) {
	for _, testCase := range []struct {
		name     string
		role     string
		instance string
		want     string
	}{
		{"Vera", "sysadmin-senior", "", "sysadmin-senior-vera"},
		{"Beetle-Ox", "eng-platform", "ab84", "eng-platform-beetle-ox-ab84"},
		{"Valerie", "sysadmin-junior", "zv88", "sysadmin-junior-valerie-zv88"},
		{"Vera", "", "ab84", ""},
	} {
		if got := sessionName(testCase.name, testCase.role, testCase.instance); got != testCase.want {
			t.Fatalf("sessionName(%q, %q, %q) = %q, want %q",
				testCase.name, testCase.role, testCase.instance, got, testCase.want)
		}
	}
}

func TestTwoLaunchesOfOneRoleGetTwoNames(t *testing.T) {
	if sessionName("Angie", "eng-platform", "ab84") == sessionName("Angie", "eng-platform", "tu78") {
		t.Fatal("two instances of one role must not share a name, or neither is addressable")
	}
}

func TestLaunchPlanCarriesTheMintedInstanceToTheShadowAndTheCard(t *testing.T) {
	var spawns []recordedSpawn
	out, err := runAterm(t, stubDeps(t, &spawns, true), "--dry-run", "--json", "prod-director", "codex")
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	var plan launchPlan
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatalf("decode plan: %v", err)
	}
	at := slices.Index(plan.Child, "--session-id")
	if at < 0 || plan.Child[at+1] != stubInstance || slices.Index(plan.Child, "--") < at {
		t.Fatalf("the shadow should be asked for %s ahead of its `--`: %v", stubInstance, plan.Child)
	}
	if plan.Card.Instance != stubInstance || plan.Identity.Instance != stubInstance {
		t.Fatalf("card instance = %q, identity instance = %q, want %s",
			plan.Card.Instance, plan.Identity.Instance, stubInstance)
	}
}

func TestLaunchPlanWithoutAMintingAOSStaysUnsuffixed(t *testing.T) {
	var spawns []recordedSpawn
	out, err := runAterm(t, stubDeps(t, &spawns, false), "--dry-run", "--json", "eng-platform", "claude")
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	var plan launchPlan
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatalf("decode plan: %v", err)
	}
	if plan.Card.Instance != "" || slices.Contains(withoutConversation(plan.Child), "--session-id") {
		t.Fatalf("an aos that cannot mint should leave no instance: %+v", plan.Child)
	}
	if !slices.Contains(plan.Child, sessionName("Angie", "eng-platform", "")) {
		t.Fatalf("the unsuffixed name should still reach claude: %v", plan.Child)
	}
}
