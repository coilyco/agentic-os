package main

import (
	"strings"
	"testing"
)

func TestLoadHarnessLaunchProfilesUsesDefaultsAndRoleOverrides(t *testing.T) {
	t.Parallel()
	document, err := loadHarnessLaunchProfiles([]byte(`{
  "format": "agentic-os.harness-launch-profiles.v1",
  "defaults": {
    "codex": {
      "model": "default-model",
      "reasoning_effort": "medium",
      "verbosity": "low"
    }
  },
  "standalone_roles": {
    "director": {
      "codex": {
        "model": "role-model",
        "reasoning_effort": "high"
      }
    }
  }
}`))
	if err != nil {
		t.Fatal(err)
	}
	if document.Defaults["codex"].Model != "default-model" {
		t.Fatalf("default profile was not decoded: %+v", document.Defaults["codex"])
	}
	if document.StandaloneRoles["director"]["codex"].Model != "role-model" {
		t.Fatalf(
			"standalone role profile was not decoded: %+v",
			document.StandaloneRoles["director"]["codex"],
		)
	}
}

func TestLoadHarnessLaunchProfilesRejectsMalformedRegistry(t *testing.T) {
	t.Parallel()
	for _, data := range [][]byte{
		[]byte(`{}`),
		[]byte(`{"format":"agentic-os.harness-launch-profiles.v1","defaults":{}}`),
		[]byte(`{"format":"agentic-os.harness-launch-profiles.v1","defaults":{"other":{"model":"x"}}}`),
		[]byte(`{"format":"agentic-os.harness-launch-profiles.v1","defaults":{"codex":{"model":""}}}`),
	} {
		if _, err := loadHarnessLaunchProfiles(data); err == nil {
			t.Fatalf("loadHarnessLaunchProfiles accepted %q", data)
		}
	}
}

func TestEmbeddedHarnessLaunchProfilesProduceExplicitWardEnvironment(t *testing.T) {
	t.Parallel()
	environment, err := wardLaunchEnvironmentFor("codex")
	if err != nil {
		t.Fatal(err)
	}
	rendered := strings.Join(environment, " ")
	for _, key := range []string{
		"WARD_CODEX_MODEL=",
		"WARD_CODEX_REASONING_EFFORT=",
		"WARD_CODEX_VERBOSITY=",
	} {
		if !strings.Contains(rendered, key) {
			t.Fatalf("Ward environment %q does not contain %q", rendered, key)
		}
	}
	for _, entry := range environment {
		if strings.HasSuffix(entry, "=") {
			t.Fatalf("Ward environment entry %q has an empty value", entry)
		}
	}
}

func TestStandaloneRoleTuningDoesNotChangeWardEnvironment(t *testing.T) {
	t.Parallel()
	director, err := standaloneHarnessLaunchProfileFor("director", "codex")
	if err != nil {
		t.Fatal(err)
	}
	engineer, err := standaloneHarnessLaunchProfileFor("engineer", "codex")
	if err != nil {
		t.Fatal(err)
	}
	if director == engineer {
		t.Fatal("embedded standalone role tuning collapsed to one profile")
	}

	want, err := wardLaunchEnvironmentFor("codex")
	if err != nil {
		t.Fatal(err)
	}
	for _, role := range []string{"director", "engineer", "qa"} {
		plan, err := buildWardLaunchPlan(integratedLaunchOptions{
			Role: role, Agent: "codex", Image: "aos:test",
		}, "")
		if err != nil {
			t.Fatal(err)
		}
		got := plan.Environment
		if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
			t.Fatalf("%s Ward environment = %v, want harness defaults %v", role, got, want)
		}
		if strings.Contains(strings.Join(plan.Args, " "), "--config") {
			t.Fatalf("%s Ward args contain a role-local config flag: %v", role, plan.Args)
		}
	}
}
