package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

const (
	cardFormat     = "aterm.card.v1"
	cardMotionSpan = 400 * time.Millisecond
	cardFrames     = 10
	cardGap        = "  "
	noMotionEnv    = "ATERM_NO_MOTION"
	cardEnv        = "ATERM_CARD"
)

type cardFigure struct {
	Name     string `json:"name"`
	Color    string `json:"color"`
	Motif    string `json:"motif"`
	Emoji    string `json:"emoji"`
	Geometry string `json:"geometry"`
	Timbre   string `json:"timbre"`
	Contour  string `json:"contour"`
	Pulse    string `json:"pulse"`
}

type sessionCard struct {
	Format     string `json:"format"`
	Annotation string `json:"annotation"`
	// Role is the slug, not the display name, because it is what re-derives
	// the creature plate after the launch. See docs/aterm-pane.md.
	Role       string       `json:"role"`
	Seat       string       `json:"seat"`
	Tier       string       `json:"tier"`
	Expression string       `json:"expression"`
	Workspace  string       `json:"workspace"`
	Directory  string       `json:"directory"`
	Shadowed   bool         `json:"shadowed"`
	Accent     string       `json:"accent"`
	Figures    []cardFigure `json:"figures"`
}

func buildSessionCard(document overlayDocument, plan launchPlan) sessionCard {
	card := sessionCard{
		Format:     cardFormat,
		Annotation: plan.Identity.Annotation,
		Role:       plan.Identity.Role,
		Seat:       plan.Identity.Seat,
		Tier:       document.Seat.Tier,
		Expression: plan.Identity.Expression,
		Workspace:  plan.Workspace,
		Directory:  plan.WorkingDirectory,
		Shadowed:   plan.Shadowed,
		Accent:     plan.Brand.Accent,
	}
	for _, personality := range document.Personalities {
		card.Figures = append(card.Figures, cardFigure{
			Name:     personality.Name,
			Color:    personality.Color,
			Motif:    personality.Motif,
			Emoji:    personality.Emblem.Emoji,
			Geometry: personality.Geometry,
			Timbre:   personality.SoundMark.Timbre,
			Contour:  personality.SoundMark.Contour,
			Pulse:    personality.SoundMark.Pulse,
		})
	}
	return card
}

func encodeSessionCard(card sessionCard) (string, error) {
	encoded, err := json.Marshal(card)
	if err != nil {
		return "", fmt.Errorf("marshal the session card: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(encoded), nil
}

func decodeSessionCard(value string) (sessionCard, error) {
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(value))
	if err != nil {
		return sessionCard{}, fmt.Errorf("decode the session card: %w", err)
	}
	var card sessionCard
	if err := json.Unmarshal(raw, &card); err != nil {
		return sessionCard{}, fmt.Errorf("parse the session card: %w", err)
	}
	if card.Format != cardFormat {
		return sessionCard{}, fmt.Errorf("session card has unsupported contract %q", card.Format)
	}
	return card, nil
}

// renderCard draws the card at a fraction of its motion, 0 for nothing drawn
// yet and 1 for the whole figure. See docs/aterm.md.
func renderCard(card sessionCard, progress float64) string {
	panels := make([][]string, 0, len(card.Figures))
	for _, figure := range card.Figures {
		panels = append(panels, renderFigure(figure, progress))
	}
	lines := make([]string, figureHeight)
	for row := 0; row < figureHeight; row++ {
		parts := make([]string, 0, len(panels))
		for _, panel := range panels {
			parts = append(parts, panel[row])
		}
		lines[row] = strings.Join(parts, cardGap)
	}
	lines[0] += cardGap + cardGap + cardName(card)
	return strings.Join(lines, "\n") + "\n"
}

// cardName is the whole of the card's text. Kai cut the seat, tier, expression,
// workspace, directory, shadow line, and personality legend. agentic-os#1456
func cardName(card sessionCard) string {
	return lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(card.Accent)).
		Render(card.Annotation)
}

func renderFigure(figure cardFigure, progress float64) []string {
	ordered := plotFigure(figure.Geometry)
	shown := int(float64(len(ordered))*progress + 0.5)
	if shown > len(ordered) {
		shown = len(ordered)
	}
	ink := lipgloss.NewStyle()
	if hexColorPattern.MatchString(figure.Color) {
		ink = ink.Foreground(lipgloss.Color(figure.Color))
	}
	texture := lipgloss.NewStyle().Faint(true)
	grid := make([][]string, figureHeight)
	for row := range grid {
		grid[row] = make([]string, figureWidth)
		for column := range grid[row] {
			grid[row][column] = texture.Render(string(figureTexture(figure.Motif, column, row)))
		}
	}
	for _, cell := range ordered[:shown] {
		grid[cell.Y][cell.X] = ink.Render(string(cell.Glyph))
	}
	lines := make([]string, figureHeight)
	for row := range grid {
		lines[row] = strings.Join(grid[row], "")
	}
	return lines
}

// The texture is sparse on purpose. A motif is a hint under the figure rather
// than a second figure competing with it.
func figureTexture(motif string, x, y int) rune {
	if (x*3+y*5)%7 != 0 {
		return ' '
	}
	return motifStipple(motif)
}

// playCard animates the card in under its own motion, then leaves the finished
// card on screen and hands the window to the harness.
func playCard(writer io.Writer, card sessionCard, animate bool) {
	if len(card.Figures) == 0 {
		return
	}
	if !animate {
		fmt.Fprint(writer, renderCard(card, 1))
		return
	}
	height := figureHeight + 1
	for frame := 1; frame <= cardFrames; frame++ {
		fmt.Fprint(writer, renderCard(card, float64(frame)/cardFrames))
		if frame == cardFrames {
			break
		}
		fmt.Fprintf(writer, "\033[%dA", height)
		time.Sleep(cardMotionSpan / cardFrames)
	}
}

// cardMotionWanted is off wherever the animation would land in a recording, a
// log, or CI rather than in front of a person.
func cardMotionWanted(stdout *os.File, disabled bool) bool {
	if disabled || strings.TrimSpace(os.Getenv(noMotionEnv)) != "" {
		return false
	}
	return characterDevices(stdout)
}
