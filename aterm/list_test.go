package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func rosterFixture(t *testing.T) rosterDocument {
	t.Helper()
	document, err := parseRoster(fixture(t, "roster.json"))
	if err != nil {
		t.Fatalf("parse the roster fixture: %v", err)
	}
	return document
}

// The machine form carries the launchable view, so a caller does not have to
// re-derive which catalogue seats `agent-compose launch` can actually start.
func TestListRosterProjectsOnlyLaunchableSeats(t *testing.T) {
	document := rosterFixture(t)
	listed := listRoster(document)
	if listed.Format != rosterFormat {
		t.Fatalf("format = %q, want %q", listed.Format, rosterFormat)
	}
	if len(listed.Roles) != len(document.Items) {
		t.Fatalf("roles = %d, want every live role %d", len(listed.Roles), len(document.Items))
	}
	for index, role := range listed.Roles {
		source := document.Items[index]
		if role.Slug != source.Slug || role.DisplayName != source.DisplayName {
			t.Fatalf("role %d = %q, want %q", index, role.Slug, source.Slug)
		}
		if role.Launchable != (len(role.Seats) > 0) {
			t.Fatalf("%s launchable = %v with %d seats", role.Slug, role.Launchable, len(role.Seats))
		}
		for _, seat := range role.Seats {
			if !isNativeHarness(seat.Harness) {
				t.Fatalf("%s lists non-native seat %q", role.Slug, seat.Harness)
			}
		}
		if len(role.Seats) == len(source.Seats) && len(source.Seats) > len(source.nativeSeats()) {
			t.Fatalf("%s kept a catalogue seat that cannot launch", role.Slug)
		}
	}
}

// Two writers over one roster, and the human one is the default.
func TestWriteRosterJSONParsesAndStaysBesideTheHumanForm(t *testing.T) {
	document := rosterFixture(t)
	machine := &bytes.Buffer{}
	if err := writeRosterJSON(machine, document); err != nil {
		t.Fatalf("write JSON roster: %v", err)
	}
	var decoded listedRoster
	if err := json.Unmarshal(machine.Bytes(), &decoded); err != nil {
		t.Fatalf("the JSON roster does not parse: %v", err)
	}
	if len(decoded.Roles) == 0 || decoded.Roles[0].Purpose == "" {
		t.Fatalf("decoded roster = %+v", decoded)
	}
	human := &bytes.Buffer{}
	if err := writeRoster(human, document); err != nil {
		t.Fatalf("write human roster: %v", err)
	}
	if strings.HasPrefix(strings.TrimSpace(human.String()), "{") {
		t.Fatal("the default --list should stay the human form")
	}
}

func TestListJSONThroughTheCommandAndTheFlagGuard(t *testing.T) {
	var spawns []recordedSpawn
	out, err := runAterm(t, stubDeps(t, &spawns, true), "--list", "--json")
	if err != nil {
		t.Fatalf("list --json: %v", err)
	}
	var decoded listedRoster
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("--list --json did not emit JSON: %v\n%s", err, out)
	}
	if len(decoded.Roles) == 0 {
		t.Fatal("--list --json emitted no roles")
	}
	// A flag that silently does nothing is worse than one that says so.
	_, err = runAterm(t, stubDeps(t, &spawns, true), "--json", "eng-platform", "claude")
	if err == nil || !strings.Contains(err.Error(), "--json applies to") {
		t.Fatalf("--json on a launch should refuse: %v", err)
	}
	if len(spawns) != 0 {
		t.Fatalf("nothing should have been spawned: %v", spawns)
	}
}
