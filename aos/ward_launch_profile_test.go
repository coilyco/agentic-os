package main

import "testing"

func TestLoadWardLaunchProfilesRejectsEmptyCodexInput(t *testing.T) {
	t.Parallel()
	_, err := loadWardLaunchProfiles([]byte(`{
  "format": "agentic-os.ward-launch-profiles.v1",
  "roles": {
    "director": {
      "codex": {
        "model": "gpt-5.6-sol",
        "reasoning_effort": "",
        "verbosity": "low"
      }
    }
  }
}`))
	if err == nil {
		t.Fatal("loadWardLaunchProfiles accepted an empty Codex input")
	}
}

func TestEmbeddedWardLaunchProfilesResolveCodexInputs(t *testing.T) {
	t.Parallel()
	inputs, err := wardLaunchInputsFor("director", "codex")
	if err != nil {
		t.Fatal(err)
	}
	if len(inputs) != 6 {
		t.Fatalf("Codex inputs = %v, want three non-empty config pairs", inputs)
	}
	for index := 1; index < len(inputs); index += 2 {
		if inputs[index] == "" || inputs[index][len(inputs[index])-1] == '=' {
			t.Fatalf("Codex input %q is empty", inputs[index])
		}
	}
}
