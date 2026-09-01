package main

import (
	"os"
	"strings"
	"testing"
)

func figureText(figure cardFigure) string {
	rows := make([][]rune, figureHeight)
	for index := range rows {
		rows[index] = []rune(strings.Repeat(" ", figureWidth))
	}
	for _, cell := range plotFigure(figure.Geometry) {
		rows[cell.Y][cell.X] = cell.Glyph
	}
	lines := make([]string, figureHeight)
	for index, row := range rows {
		lines[index] = string(row)
	}
	return strings.Join(lines, "\n")
}

// The figure is generated from `form.geometry` rather than drawn per role, so
// the properties worth asserting are of the generator. agentic-os#1255
func TestEveryGeometryDrawsSomethingInsideTheGrid(t *testing.T) {
	card := buildSessionCard(platformOverlay(t), launchPlan{})
	if len(card.Figures) < 2 {
		t.Fatalf("the platform overlay ships %d personalities", len(card.Figures))
	}
	drawn := map[string]string{}
	for _, figure := range card.Figures {
		cells := plotFigure(figure.Geometry)
		if len(cells) == 0 {
			t.Fatalf("%s drew nothing", figure.Geometry)
		}
		for _, cell := range cells {
			if cell.X < 0 || cell.X >= figureWidth || cell.Y < 0 || cell.Y >= figureHeight {
				t.Fatalf("%s drew outside the grid at %d,%d", figure.Geometry, cell.X, cell.Y)
			}
			if cell.Glyph == 0 || cell.Glyph == ' ' {
				t.Fatalf("%s inked a blank at %d,%d", figure.Geometry, cell.X, cell.Y)
			}
		}
		text := figureText(figure)
		if other, clash := drawn[text]; clash {
			t.Fatalf("%s and %s draw the same figure", figure.Geometry, other)
		}
		drawn[text] = figure.Geometry
	}
}

// A geometry the roster has not shipped yet still composes, which is the whole
// reason the grammar is read rather than the value matched.
func TestAnUnshippedGeometryStillDraws(t *testing.T) {
	cases := []string{"spiral-ribbons", "radial-arcs", "", "nonsense"}
	for _, geometry := range cases {
		if len(plotFigure(geometry)) == 0 {
			t.Fatalf("%q drew nothing", geometry)
		}
	}
	// The grammar composes, so a known arrangement with a new shape is not the
	// same picture as the fallback.
	if figureText(cardFigure{Geometry: "radial-arcs"}) == figureText(cardFigure{Geometry: "nonsense"}) {
		t.Fatal("a known arrangement should not fall back")
	}
}

func TestCardRoundTripsThroughTheSessionStage(t *testing.T) {
	document := platformOverlay(t)
	plan, err := buildLaunchPlan(
		document,
		launchRequest{Role: "platform", Seat: "claude", Expression: "acting", TerminalBin: "kitty"},
		t.TempDir(), "/stub/aterm", "/stub/agent-compose", "/stub/aos", true,
	)
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	index := indexOf(plan.Arguments, "--card")
	if index < 0 || index+1 >= len(plan.Arguments) {
		t.Fatalf("the plan carries no card: %v", plan.Arguments)
	}
	if indexOf(plan.Arguments, sessionCommand) > index {
		t.Fatal("--card belongs to the session stage, not the terminal")
	}
	options, err := parseSessionArgs(plan.Arguments[indexOf(plan.Arguments, sessionCommand)+1:])
	if err != nil {
		t.Fatalf("the session stage cannot read its own card: %v", err)
	}
	if options.Card.Annotation != plan.Identity.Annotation {
		t.Fatalf("card annotation = %q", options.Card.Annotation)
	}
	if len(options.Card.Figures) != len(document.Personalities) {
		t.Fatalf("card figures = %d, want %d", len(options.Card.Figures), len(document.Personalities))
	}
	if options.Card.Figures[0].Geometry != document.Personalities[0].Geometry {
		t.Fatalf("the card lost the geometry: %+v", options.Card.Figures[0])
	}
}

func TestRenderCardGrowsWithProgress(t *testing.T) {
	card := buildSessionCard(platformOverlay(t), launchPlan{Brand: launchBrand{Accent: "#9c8b31"}})
	empty := renderCard(card, 0)
	full := renderCard(card, 1)
	if empty == full {
		t.Fatal("the card should arrive rather than appear whole at progress 0")
	}
	if !strings.Contains(full, card.Annotation) {
		t.Fatalf("the card should name the seat:\n%s", full)
	}
	if strings.Count(full, "\n") != figureHeight {
		t.Fatalf("the card should be %d lines:\n%s", figureHeight, full)
	}
}

// Kai cut the card to the seat name, its colour, and the creature. agentic-os#1456
func TestTheCardCarriesNothingButTheNameAndTheCreature(t *testing.T) {
	card := buildSessionCard(platformOverlay(t), launchPlan{Brand: launchBrand{Accent: "#9c8b31"}})
	card.Seat = "claude"
	card.Tier = "frontier"
	card.Expression = "acting"
	card.Workspace = "a-workspace-label"
	card.Directory = "/a/directory/nobody/needs"
	card.Shadowed = true
	full := renderCard(card, 1)
	gone := []string{
		card.Seat, card.Tier, card.Expression, card.Workspace, card.Directory,
		"leased", "unleased",
	}
	for _, figure := range card.Figures {
		gone = append(gone, figure.Name, figure.Emoji)
	}
	for _, dropped := range gone {
		if strings.TrimSpace(dropped) == "" {
			continue
		}
		if strings.Contains(full, dropped) {
			t.Fatalf("the card should have dropped %q:\n%s", dropped, full)
		}
	}
	if !strings.Contains(full, card.Annotation) {
		t.Fatalf("the card should keep the seat name:\n%s", full)
	}
}

// Motion belongs in front of a person, never in a recording, a log, or CI.
func TestMotionIsOffWhenNobodyIsWatching(t *testing.T) {
	if cardMotionWanted(nil, false) {
		t.Fatal("a nil stdout is not a terminal")
	}
	if cardMotionWanted(os.Stdout, true) {
		t.Fatal("--no-motion should win")
	}
	t.Setenv(noMotionEnv, "1")
	if cardMotionWanted(os.Stdout, false) {
		t.Fatalf("%s should win", noMotionEnv)
	}
}

func TestDecodeSessionCardRejectsWhatItShould(t *testing.T) {
	for _, value := range []string{"", "not base64!", "e30"} {
		if _, err := decodeSessionCard(value); err == nil {
			t.Fatalf("%q should be rejected", value)
		}
	}
}
