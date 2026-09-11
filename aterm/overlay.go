package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	overlayFormat = "agent-compose.overlay.v1"
	overlaySchema = 1
)

type overlaySeat struct {
	Key       string `json:"key"`
	Harness   string `json:"harness"`
	Name      string `json:"name"`
	Pronouns  string `json:"pronouns"`
	Tier      string `json:"tier"`
	LegalName string `json:"legal_name,omitempty"`
}

// Names run widest-reading first, the emoji's own name then the roster's reading.
type overlayEmblem struct {
	Names []string `json:"names"`
	Emoji string   `json:"emoji"`
}

// The creature the emblem belongs to, decoded whole rather than sampled so a
// dropped upstream field fails here. See docs/aterm.md.
type overlayBody struct {
	Archetype  string `json:"archetype"`
	Attachment string `json:"attachment"`
}

type overlaySoundMark struct {
	Timbre  string `json:"timbre"`
	Contour string `json:"contour"`
	Pulse   string `json:"pulse"`
}

type overlayPersonality struct {
	Name      string           `json:"name"`
	Species   string           `json:"species,omitempty"`
	Color     string           `json:"color"`
	Motif     string           `json:"motif"`
	Geometry  string           `json:"geometry"`
	Emblem    overlayEmblem    `json:"emblem"`
	Body      overlayBody      `json:"body"`
	SoundMark overlaySoundMark `json:"sound_mark"`
}

// What a session says as it closes, so a hold banner is not invented.
type overlayOutro struct {
	Clean   string `json:"clean,omitempty"`
	Failure string `json:"failure,omitempty"`
}

type overlayDocument struct {
	Format          string      `json:"format"`
	SchemaVersion   int         `json:"schema_version"`
	Person          string      `json:"person"`
	Role            string      `json:"role"`
	RoleDisplayName string      `json:"role_display_name"`
	Purpose         string      `json:"purpose"`
	Stance          string      `json:"stance"`
	Seat            overlaySeat `json:"seat"`
	Annotation      string      `json:"annotation"`
	// The four-part sentence a seat answers "who are you" with, and the
	// creature pair inside it. Derived from the meld: agent-compose#7485.
	Identity      string        `json:"identity,omitempty"`
	Creature      string        `json:"creature,omitempty"`
	Outro         *overlayOutro `json:"outro,omitempty"`
	Expression    string        `json:"expression"`
	FavoriteColor string        `json:"favorite_color"`
	// Solved across the whole roster upstream, which one overlay cannot do.
	Background    string               `json:"background"`
	Personalities []overlayPersonality `json:"personalities"`
}

func loadOverlay(
	ctx context.Context,
	deps commandDeps,
	agentCompose string,
	role string,
	seat string,
	expression string,
) (overlayDocument, error) {
	command := []string{
		agentCompose, "overlay",
		"--role", role,
		"--seat", seat,
		"--expression", expression,
		"--json",
	}
	raw, err := whileWaiting2(deps.notice, command, func() ([]byte, error) {
		return deps.output(ctx, command[0], command[1:]...)
	})
	if err != nil {
		return overlayDocument{}, fmt.Errorf("load the identity overlay: %w", err)
	}
	return parseOverlay(raw, role, seat, expression)
}

func parseOverlay(raw []byte, role, seat, expression string) (overlayDocument, error) {
	var document overlayDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		return overlayDocument{}, fmt.Errorf("decode the identity overlay: %w", err)
	}
	if document.Format != overlayFormat || document.SchemaVersion != overlaySchema {
		return overlayDocument{}, fmt.Errorf(
			"the identity overlay has unsupported contract %q schema %d",
			document.Format,
			document.SchemaVersion,
		)
	}
	if document.Role != strings.TrimSpace(role) {
		return overlayDocument{}, fmt.Errorf(
			"the identity overlay role %q does not match requested role %q",
			document.Role,
			role,
		)
	}
	if document.Seat.Harness != strings.TrimSpace(seat) {
		return overlayDocument{}, fmt.Errorf(
			"the identity overlay seat %q does not match requested seat %q",
			document.Seat.Harness,
			seat,
		)
	}
	if document.Expression != strings.TrimSpace(expression) {
		return overlayDocument{}, fmt.Errorf(
			"the identity overlay expression %q does not match requested expression %q",
			document.Expression,
			expression,
		)
	}
	if strings.TrimSpace(document.Person) == "" || strings.TrimSpace(document.Seat.Name) == "" {
		return overlayDocument{}, fmt.Errorf("the identity overlay is missing person or seat name")
	}
	if !hexColorPattern.MatchString(document.FavoriteColor) {
		return overlayDocument{}, fmt.Errorf(
			"the identity overlay favorite color %q is not #RRGGBB",
			document.FavoriteColor,
		)
	}
	return document, nil
}

// seatAnnotation prefers the string agent-compose composed, so the window title,
// the harness prompt box, and the status row all read the same identity.
func seatAnnotation(document overlayDocument) string {
	if annotation := strings.TrimSpace(document.Annotation); annotation != "" {
		return annotation
	}
	name := seatName(document)
	if name == "" {
		return ""
	}
	label := strings.TrimSpace(document.RoleDisplayName)
	if label == "" {
		label = strings.TrimSpace(document.Role)
	}
	if label != "" {
		name += " (" + label + ")"
	}
	return name
}

// seatName is the annotation without its role, for a title that already names
// the role in its own segment.
func seatName(document overlayDocument) string {
	name := strings.TrimSpace(document.Seat.Name)
	if name == "" {
		return ""
	}
	subject, _, _ := strings.Cut(strings.TrimSpace(document.Seat.Pronouns), "/")
	if subject = strings.TrimSpace(subject); subject != "" {
		name += " [" + subject + "]"
	}
	return name
}
