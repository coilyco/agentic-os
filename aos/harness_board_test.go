package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

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

func TestEveryEmbeddedLaneHasDeterministicProjection(t *testing.T) {
	t.Parallel()
	board, err := loadHarnessBoard(embeddedHarnessBoard)
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, role := range board.Roles {
		for _, expected := range role.Intents {
			lane, resolveErr := resolveLaneDefault(board, role.Role, expected.Intent)
			if resolveErr != nil {
				t.Fatal(resolveErr)
			}
			if lane.Role != role.Role || lane.Intent != expected.Intent {
				t.Fatalf("projection provenance changed: %#v", lane)
			}
			if lane.Harness != expected.Harness {
				t.Fatalf("projection harness changed: %#v", lane)
			}
			if lane.Route != role.Role+"/"+expected.Intent {
				t.Fatalf("projection route is not deterministic: %#v", lane)
			}
			seen++
		}
	}
	if seen != board.LaneCount {
		t.Fatalf("resolved %d lanes, want %d", seen, board.LaneCount)
	}
}

func TestCommunityLaneProjections(t *testing.T) {
	t.Parallel()
	board, err := loadHarnessBoard(embeddedHarnessBoard)
	if err != nil {
		t.Fatal(err)
	}
	for intent, harness := range map[string]string{
		"knowledge-retrieval":     "sirens-echo",
		"conversation-management": "sirens-echo",
	} {
		lane, resolveErr := resolveLaneDefault(board, "community", intent)
		if resolveErr != nil {
			t.Fatal(resolveErr)
		}
		if lane.Harness != harness || lane.Route != "community/"+intent {
			t.Fatalf("%s projection = %#v", intent, lane)
		}
	}
}

func TestLaneDefaultCommandEmitsOnlyModelOpaqueControlData(t *testing.T) {
	t.Parallel()
	command := newCommand()
	var output bytes.Buffer
	command.Writer = &output
	err := command.Run(
		context.Background(),
		[]string{
			"aos", "--role", "community", "lane-default",
			"--intent", "knowledge-retrieval",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	var projection map[string]string
	if err := json.Unmarshal(output.Bytes(), &projection); err != nil {
		t.Fatal(err)
	}
	if len(projection) != 4 {
		t.Fatalf("lane-default emitted extra fields: %#v", projection)
	}
	if projection["role"] != "community" ||
		projection["intent"] != "knowledge-retrieval" ||
		projection["harness"] != "sirens-echo" ||
		projection["route"] != "community/knowledge-retrieval" {
		t.Fatalf("lane-default projection = %#v", projection)
	}
}

func TestLocalLaneProfileIsIdempotentAndPreservesUserContent(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "rasa", "community.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	original := []byte(`{
  "format": "agentic-os.local-lane-profile.v1",
  "prompt": "Member-facing words stay here.",
  "request": {
    "timeout": 30
  }
}
`)
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	lane := laneProjection{
		Role:    "community",
		Intent:  "knowledge-retrieval",
		Harness: "rasa",
		Route:   "community/knowledge-retrieval",
	}
	if err := writeLocalLaneProfile(path, lane); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeLocalLaneProfile(path, lane); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("idempotent profile write changed the file")
	}
	var profile map[string]any
	if err := json.Unmarshal(second, &profile); err != nil {
		t.Fatal(err)
	}
	if profile["prompt"] != "Member-facing words stay here." {
		t.Fatalf("user-owned profile content changed: %#v", profile)
	}
	request := profile["request"].(map[string]any)
	if request["provider"] != "agent-proxy" ||
		request["model"] != "community/knowledge-retrieval" ||
		request["timeout"] != float64(30) {
		t.Fatalf("local request profile = %#v", request)
	}
	if profile["prompt"] == lane.Route {
		t.Fatal("logical route entered prompt content")
	}
}

func TestLocalLaneProfileRefusesForeignFile(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "profile.json")
	original := []byte("{\"format\":\"other.profile/v1\",\"user\":\"content\"}\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	err := writeLocalLaneProfile(path, laneProjection{
		Role: "community", Intent: "knowledge-retrieval",
		Harness: "rasa", Route: "community/knowledge-retrieval",
	})
	if err == nil {
		t.Fatal("foreign profile was overwritten")
	}
	current, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !bytes.Equal(current, original) {
		t.Fatal("foreign profile changed after refusal")
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
