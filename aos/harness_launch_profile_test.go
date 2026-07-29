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
  "roles": {
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
	if document.Roles["director"]["codex"].Model != "role-model" {
		t.Fatalf("role profile was not decoded: %+v", document.Roles["director"]["codex"])
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

func TestEmbeddedHarnessLaunchProfilesProduceExplicitWardInputs(t *testing.T) {
	t.Parallel()
	inputs, err := wardLaunchInputsFor("director", "codex")
	if err != nil {
		t.Fatal(err)
	}
	rendered := strings.Join(inputs, " ")
	for _, key := range []string{
		"agent.codex.model=",
		"agent.codex.effort=",
		"agent.codex.verbosity=",
	} {
		if !strings.Contains(rendered, key) {
			t.Fatalf("Ward inputs %q do not contain %q", rendered, key)
		}
	}
}
