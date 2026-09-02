package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

const planFieldWidth = 13

var (
	planLabelStyle = lipgloss.NewStyle().Faint(true).Width(planFieldWidth)
	planValueStyle = lipgloss.NewStyle()
)

// renderPlan is the operator's half of --dry-run. Deciding whether a brand
// looks right means seeing the color, not reading its hex. See docs/aterm.md.
func renderPlan(writer io.Writer, document overlayDocument, plan launchPlan) error {
	accent := lipgloss.Color(plan.Brand.Accent)
	lines := &strings.Builder{}
	fmt.Fprintf(lines, "%s\n", lipgloss.NewStyle().Bold(true).Foreground(accent).
		Render(plan.Identity.Annotation))
	fields := [][2]string{
		{"seat", seatLine(document, plan)},
		{"expression", plan.Identity.Expression},
		{"workspace", planWorkspace(plan)},
		{"directory", plan.WorkingDirectory},
		{"shadow", shadowLine(plan.Shadowed)},
		{"personality", personalityLine(document)},
		{"brand", brandLine(plan.Brand)},
		{"creature", creatureLine(plan.Creature)},
		{"title", plan.Brand.Title},
		{"child", strings.Join(plan.Child, " ")},
	}
	for _, field := range fields {
		if strings.TrimSpace(field[1]) == "" {
			continue
		}
		fmt.Fprintf(lines, "  %s%s\n", planLabelStyle.Render(field[0]), planValueStyle.Render(field[1]))
	}
	// The card is what the window opens with, so a dry run that cannot show it
	// cannot answer whether the identity looks right.
	if len(plan.Card.Figures) > 0 {
		fmt.Fprintf(lines, "\n%s", renderCard(plan.Card, 1))
	}
	_, err := io.WriteString(writer, lines.String())
	return err
}

func seatLine(document overlayDocument, plan launchPlan) string {
	seat := plan.Identity.Seat
	if tier := strings.TrimSpace(document.Seat.Tier); tier != "" {
		seat += " // " + tier
	}
	return seat
}

func planWorkspace(plan launchPlan) string {
	if plan.Workspace == "" {
		return "(none, the directory is not a checkout)"
	}
	return plan.Workspace
}

func shadowLine(shadowed bool) string {
	if shadowed {
		return "leased"
	}
	return "none, the window runs Agent Compose directly"
}

// personalityLine is the first place the sensory identity shows up: each
// personality in its own color, behind its own emblem.
func personalityLine(document overlayDocument) string {
	parts := make([]string, 0, len(document.Personalities))
	for _, personality := range document.Personalities {
		label := strings.TrimSpace(personality.Emblem.Emoji + " " + personality.Name)
		if hexColorPattern.MatchString(personality.Color) {
			label = lipgloss.NewStyle().Foreground(lipgloss.Color(personality.Color)).Render(label)
		}
		parts = append(parts, label)
	}
	return strings.Join(parts, "  ")
}

// creatureLine says where the plate is, since deciding whether the background
// looks right means opening the file the window will draw.
func creatureLine(plate creaturePlate) string {
	if plate.Path == "" {
		return "none, the window background stays flat"
	}
	return fmt.Sprintf("%s   %s tint", plate.Path, plate.Tint)
}

func brandLine(brand launchBrand) string {
	return strings.TrimSpace(fmt.Sprintf(
		"%s %s accent   %s %s background",
		swatch(brand.Accent), brand.Accent,
		swatch(brand.Background), brand.Background,
	))
}

// A 16-color terminal renders #9c8b31 as bright red, which is worse than no
// swatch when the whole point is deciding whether the brand looks right.
func swatch(color string) string {
	if !hexColorPattern.MatchString(color) || lipgloss.ColorProfile() > termenv.ANSI256 {
		return ""
	}
	return lipgloss.NewStyle().Background(lipgloss.Color(color)).Render("  ")
}
