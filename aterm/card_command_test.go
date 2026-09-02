package main

import (
	"strings"
	"testing"
)

func TestCardVerbRendersTheSessionItIsInside(t *testing.T) {
	card := buildSessionCard(platformOverlay(t), launchPlan{Brand: launchBrand{Accent: "#9c8b31"}})
	payload, err := encodeSessionCard(card)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	t.Setenv(cardEnv, payload)
	var spawns []recordedSpawn
	out, err := runAterm(t, stubDeps(t, &spawns, true), "card")
	if err != nil {
		t.Fatalf("card: %v\n%s", err, out)
	}
	if !strings.Contains(out, card.Annotation) {
		t.Fatalf("the card should name the seat:\n%s", out)
	}
	if len(spawns) != 0 {
		t.Fatalf("the card verb opens no window, got %d", len(spawns))
	}
}

// The verb reads the resolved payload rather than re-resolving through
// agent-compose, which is what keeps it working from inside a shadow.
func TestCardVerbRefusesOutsideASession(t *testing.T) {
	t.Setenv(cardEnv, "")
	var spawns []recordedSpawn
	_, err := runAterm(t, stubDeps(t, &spawns, true), "card")
	if err == nil {
		t.Fatal("the card verb should refuse when the session carries no card")
	}
	if code := exitCodeFor(err); code != exitMissing {
		t.Fatalf("exit code = %d, want %d: %v", code, exitMissing, err)
	}
}

func TestSessionCarriesTheCardToTheHarness(t *testing.T) {
	card := buildSessionCard(platformOverlay(t), launchPlan{Brand: launchBrand{Accent: "#9c8b31"}})
	payload, err := encodeSessionCard(card)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	options, err := parseSessionArgs([]string{"--card", payload, "--", "true"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if options.CardPayload != payload {
		t.Fatalf("the session dropped the payload it was handed")
	}
}
