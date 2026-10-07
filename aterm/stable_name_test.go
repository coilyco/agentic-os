package main

import (
	"encoding/json"
	"fmt"
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
			carriesName := slices.Contains(plan.Child, sessionName("Angie", "eng-platform", ""))
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
		{"Beetle-Ox", "eng-platform", "2", "eng-platform-beetle-ox-2"},
		{"Valerie", "sysadmin-junior", "3", "sysadmin-junior-valerie-3"},
		{"Vera", "", "2", ""},
	} {
		if got := sessionName(testCase.name, testCase.role, testCase.instance); got != testCase.want {
			t.Fatalf("sessionName(%q, %q, %q) = %q, want %q",
				testCase.name, testCase.role, testCase.instance, got, testCase.want)
		}
	}
}

func TestPoolNameTakesTheFirstFreeSlotAndFreedNamesReturn(t *testing.T) {
	base := "eng-platform-beetle-ox"
	live := map[string]bool{}
	taken := func(name string) bool { return live[name] }
	for _, step := range []struct {
		release string
		want    string
	}{
		{"", base},
		{"", base + "-2"},
		{"", base + "-3"},
		// The bare name freeing up is the first a relaunch gets back, ahead of -4.
		{base, base},
		{base + "-2", base + "-2"},
	} {
		delete(live, step.release)
		got := poolName(base, taken)
		if got != step.want {
			t.Fatalf("after releasing %q the pool granted %q, want %q", step.release, got, step.want)
		}
		live[got] = true
	}
}

func TestPoolSlotIsWhatPastTheBase(t *testing.T) {
	for name, want := range map[string]string{
		"eng-platform-beetle-ox":   "",
		"eng-platform-beetle-ox-2": "2",
		"eng-platform-beetle-ox-9": "9",
	} {
		if got := poolSlot("eng-platform-beetle-ox", name); got != want {
			t.Fatalf("poolSlot(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestLaunchPlanCarriesTheShadowIDToTheShadowAndNotTheName(t *testing.T) {
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
	if plan.Card.Instance != "" || plan.Identity.Instance != "" {
		t.Fatalf("the shadow code must not reach the name: card %q, identity %q",
			plan.Card.Instance, plan.Identity.Instance)
	}
}

func TestLaunchPlanTakesTheNameTheDaemonGrants(t *testing.T) {
	var spawns []recordedSpawn
	deps := stubDeps(t, &spawns, true)
	var asked []string
	deps.claim = func(base string, peek bool) string {
		asked = append(asked, fmt.Sprintf("%s peek=%v", base, peek))
		return base + "-2"
	}
	out, err := runAterm(t, deps, "--dry-run", "--json", "eng-platform", "claude")
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	var plan launchPlan
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatalf("decode plan: %v", err)
	}
	if want := []string{"eng-platform-angie peek=true"}; !slices.Equal(asked, want) {
		t.Fatalf("a dry run should peek without holding the name: asked %v, want %v", asked, want)
	}
	if plan.Card.Instance != "2" || plan.Identity.Instance != "2" {
		t.Fatalf("the granted slot should ride the card: card %q, identity %q",
			plan.Card.Instance, plan.Identity.Instance)
	}
	if !slices.Contains(plan.Child, "eng-platform-angie-2") {
		t.Fatalf("the harness should be named for the granted slot: %v", plan.Child)
	}
	if at := slices.Index(plan.Child, "--session-id"); at < 0 || plan.Child[at+1] != stubInstance {
		t.Fatalf("the shadow keeps its own code beside a pooled name: %v", plan.Child)
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

// COI-2433. The record names the slot the session last held, and the pool may grant
// another now, so claude is named for the grant rather than the record.
func TestResumeNamesClaudeForTheGrantedPoolNameNotTheRecordedOne(t *testing.T) {
	id := newConversationID()
	entry := ledgerEntry{Name: "eng-platform-angie-2", Role: "eng-platform", Seat: "claude", Cwd: "/gone", Conversation: id}
	for _, granted := range []string{"eng-platform-angie", "eng-platform-angie-3"} {
		t.Run(granted, func(t *testing.T) {
			var spawns []recordedSpawn
			deps := stubDeps(t, &spawns, true)
			deps.claim = func(string, bool) string { return granted }
			args, err := resumeArgs(entry, false, false)
			if err != nil {
				t.Fatal(err)
			}
			out, err := runAterm(t, deps, append([]string{"--dry-run", "--json"}, args...)...)
			if err != nil {
				t.Fatalf("dry run: %v", err)
			}
			var plan launchPlan
			if err := json.Unmarshal([]byte(out), &plan); err != nil {
				t.Fatalf("decode plan: %v", err)
			}
			var names []string
			for at, argument := range plan.Child {
				if argument == "--name" && at+1 < len(plan.Child) {
					names = append(names, plan.Child[at+1])
				}
			}
			if want := []string{granted}; !slices.Equal(names, want) {
				t.Fatalf("claude should carry the one name the daemon granted: got %v, want %v in %v", names, want, plan.Child)
			}
			if at := slices.Index(plan.Child, "--resume"); at < 0 || plan.Child[at+1] != id {
				t.Fatalf("the conversation must still be resumed by its id: %v", plan.Child)
			}
			if slices.Contains(plan.Child, "--session-id") && slices.Index(plan.Child, "--session-id") > slices.Index(plan.Child, "--") {
				t.Fatalf("a resume must not mint a second conversation id: %v", plan.Child)
			}
		})
	}
}
