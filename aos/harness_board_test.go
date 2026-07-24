package main

import "testing"

func TestEmbeddedHarnessBoardIsValid(t *testing.T) {
	t.Parallel()
	if _, err := loadHarnessBoard(embeddedHarnessBoard); err != nil {
		t.Fatal(err)
	}
}

func TestResolveHarnessDefaultReturnsOnlyTheSelectedHarness(t *testing.T) {
	t.Parallel()
	board := harnessBoard{
		Roles: []harnessRole{{
			Role: "example-role",
			Intents: []harnessLane{{
				Intent:  "example-intent",
				Harness: "example-harness",
			}},
		}},
	}
	got, err := resolveHarnessDefault(board, "example-role", "example-intent")
	if err != nil {
		t.Fatal(err)
	}
	if got != "example-harness" {
		t.Fatalf("resolveHarnessDefault() = %q, want example-harness", got)
	}
}

func TestResolveHarnessDefaultRejectsUnknownLane(t *testing.T) {
	t.Parallel()
	board := harnessBoard{Roles: []harnessRole{{
		Role: "example-role",
		Intents: []harnessLane{{
			Intent:  "known-intent",
			Harness: "example-harness",
		}},
	}}}
	if _, err := resolveHarnessDefault(board, "example-role", "unknown-intent"); err == nil {
		t.Fatal("unknown role-intent lane resolved")
	}
}

func TestHarnessBoardRejectsBackendFields(t *testing.T) {
	t.Parallel()
	data := []byte(`{
		"format": "agentic-os.role-harness-board.v1",
		"role_source": "source",
		"role_count": 1,
		"lane_count": 1,
		"roles": [{
			"role": "engineer",
			"intents": [{
				"intent": "autonomous-coding",
				"harness": "openhands",
				"model": "must-not-cross"
			}]
		}]
	}`)
	if _, err := loadHarnessBoard(data); err == nil {
		t.Fatal("backend model field crossed into the launcher contract")
	}
}
