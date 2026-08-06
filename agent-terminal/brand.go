package main

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	baseBackground  = "#101216"
	lightForeground = "#f5f7fa"
	backgroundTint  = 0.08
	maxTitleRunes   = 120
)

var hexColorPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

type overlaySeat struct {
	Harness  string `json:"harness"`
	Name     string `json:"name"`
	Pronouns string `json:"pronouns"`
}

type overlayEmblem struct {
	Glyph string `json:"glyph"`
}

type overlayPersonality struct {
	Name   string        `json:"name"`
	Color  string        `json:"color"`
	Emblem overlayEmblem `json:"emblem"`
}

type overlayDocument struct {
	Format        string               `json:"format"`
	SchemaVersion int                  `json:"schema_version"`
	Person        string               `json:"person"`
	Role          string               `json:"role"`
	Purpose       string               `json:"purpose"`
	Seat          overlaySeat          `json:"seat"`
	Expression    string               `json:"expression"`
	FavoriteColor string               `json:"favorite_color"`
	Personalities []overlayPersonality `json:"personalities"`
}

type launchRequest struct {
	Role             string
	Seat             string
	Expression       string
	TaskTitle        string
	WorkingDirectory string
	AgentComposeBin  string
	AOSComposeBin    string
	AlacrittyBin     string
	Child            []string
	DryRun           bool
}

type launchIdentity struct {
	Person        string `json:"person"`
	Role          string `json:"role"`
	Seat          string `json:"seat"`
	Name          string `json:"name"`
	Pronouns      string `json:"pronouns"`
	Expression    string `json:"expression"`
	FavoriteColor string `json:"favorite_color"`
}

type launchBrand struct {
	Title         string `json:"title"`
	Accent        string `json:"accent"`
	Background    string `json:"background"`
	SelectionText string `json:"selection_text"`
}

type launchPlan struct {
	Format           string         `json:"format"`
	Identity         launchIdentity `json:"identity"`
	Brand            launchBrand    `json:"brand"`
	WorkingDirectory string         `json:"working_directory"`
	Executable       string         `json:"executable"`
	Arguments        []string       `json:"arguments"`
}

func parseOverlay(raw []byte, request launchRequest) (overlayDocument, error) {
	var document overlayDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		return overlayDocument{}, fmt.Errorf("decode identity overlay: %w", err)
	}
	if document.Format != overlayFormat || document.SchemaVersion != overlaySchema {
		return overlayDocument{}, fmt.Errorf(
			"identity overlay has unsupported contract %q schema %d",
			document.Format,
			document.SchemaVersion,
		)
	}
	if document.Role != strings.TrimSpace(request.Role) {
		return overlayDocument{}, fmt.Errorf(
			"identity overlay role %q does not match requested role %q",
			document.Role,
			request.Role,
		)
	}
	if document.Seat.Harness != strings.TrimSpace(request.Seat) {
		return overlayDocument{}, fmt.Errorf(
			"identity overlay seat %q does not match requested seat %q",
			document.Seat.Harness,
			request.Seat,
		)
	}
	if document.Expression != strings.TrimSpace(request.Expression) {
		return overlayDocument{}, fmt.Errorf(
			"identity overlay expression %q does not match requested expression %q",
			document.Expression,
			request.Expression,
		)
	}
	if strings.TrimSpace(document.Person) == "" || strings.TrimSpace(document.Seat.Name) == "" {
		return overlayDocument{}, fmt.Errorf("identity overlay is missing person or seat name")
	}
	if !hexColorPattern.MatchString(document.FavoriteColor) {
		return overlayDocument{}, fmt.Errorf(
			"identity overlay favorite color %q is not #RRGGBB",
			document.FavoriteColor,
		)
	}
	return document, nil
}

func buildLaunchPlan(
	document overlayDocument,
	request launchRequest,
	cwd string,
) (launchPlan, error) {
	title, err := buildTitle(document, request.TaskTitle)
	if err != nil {
		return launchPlan{}, err
	}
	accent := strings.ToLower(document.FavoriteColor)
	background, err := mixHex(baseBackground, accent, backgroundTint)
	if err != nil {
		return launchPlan{}, fmt.Errorf("derive background tint: %w", err)
	}
	selectionText, err := mostReadable(accent, baseBackground, lightForeground)
	if err != nil {
		return launchPlan{}, fmt.Errorf("derive selection text: %w", err)
	}
	arguments := []string{
		"--title", title,
		"--working-directory", cwd,
		"-o", "window.dynamic_title=false",
		"-o", "window.opacity=1.0",
		"-o", fmt.Sprintf(`colors.primary.background="%s"`, background),
		"-o", fmt.Sprintf(`colors.cursor.cursor="%s"`, accent),
		"-o", fmt.Sprintf(`colors.selection.background="%s"`, accent),
		"-o", fmt.Sprintf(`colors.selection.text="%s"`, selectionText),
		"-e",
	}
	arguments = append(arguments, request.Child...)
	return launchPlan{
		Format: launchFormat,
		Identity: launchIdentity{
			Person:        document.Person,
			Role:          document.Role,
			Seat:          document.Seat.Harness,
			Name:          document.Seat.Name,
			Pronouns:      document.Seat.Pronouns,
			Expression:    document.Expression,
			FavoriteColor: accent,
		},
		Brand: launchBrand{
			Title:         title,
			Accent:        accent,
			Background:    background,
			SelectionText: selectionText,
		},
		WorkingDirectory: cwd,
		Executable:       strings.TrimSpace(request.AlacrittyBin),
		Arguments:        arguments,
	}, nil
}

func buildTitle(document overlayDocument, taskTitle string) (string, error) {
	parts := make([]string, 0, len(document.Personalities))
	for _, personality := range document.Personalities {
		glyph := strings.TrimSpace(personality.Emblem.Glyph)
		if glyph != "" {
			parts = append(parts, glyph)
		}
	}
	name := strings.TrimSpace(document.Seat.Name)
	title := strings.TrimSpace(strings.Join(parts, " ") + " " + name)
	title += " · " + strings.TrimSpace(document.Expression)
	taskTitle = strings.TrimSpace(taskTitle)
	if taskTitle != "" {
		if containsControl(taskTitle) {
			return "", fmt.Errorf("task title contains a control character")
		}
		title += " · " + taskTitle
	}
	if containsControl(title) {
		return "", fmt.Errorf("derived title contains a control character")
	}
	if utf8.RuneCountInString(title) > maxTitleRunes {
		runes := []rune(title)
		title = string(runes[:maxTitleRunes-1]) + "…"
	}
	return title, nil
}

func containsControl(value string) bool {
	for _, char := range value {
		if unicode.IsControl(char) {
			return true
		}
	}
	return false
}

func mixHex(base, tint string, amount float64) (string, error) {
	baseRGB, err := parseHex(base)
	if err != nil {
		return "", err
	}
	tintRGB, err := parseHex(tint)
	if err != nil {
		return "", err
	}
	if amount < 0 || amount > 1 {
		return "", fmt.Errorf("mix amount %f is outside 0..1", amount)
	}
	mixed := [3]uint8{}
	for index := range mixed {
		value := float64(baseRGB[index])*(1-amount) + float64(tintRGB[index])*amount
		mixed[index] = uint8(math.Round(value))
	}
	return formatHex(mixed), nil
}

func mostReadable(background, dark, light string) (string, error) {
	backgroundRGB, err := parseHex(background)
	if err != nil {
		return "", err
	}
	darkRGB, err := parseHex(dark)
	if err != nil {
		return "", err
	}
	lightRGB, err := parseHex(light)
	if err != nil {
		return "", err
	}
	if contrastRatio(backgroundRGB, darkRGB) >= contrastRatio(backgroundRGB, lightRGB) {
		return strings.ToLower(dark), nil
	}
	return strings.ToLower(light), nil
}

func parseHex(value string) ([3]uint8, error) {
	if !hexColorPattern.MatchString(value) {
		return [3]uint8{}, fmt.Errorf("color %q is not #RRGGBB", value)
	}
	result := [3]uint8{}
	for index := range result {
		component, err := strconv.ParseUint(value[1+index*2:3+index*2], 16, 8)
		if err != nil {
			return [3]uint8{}, fmt.Errorf("parse color %q: %w", value, err)
		}
		result[index] = uint8(component)
	}
	return result, nil
}

func formatHex(value [3]uint8) string {
	return fmt.Sprintf("#%02x%02x%02x", value[0], value[1], value[2])
}

func contrastRatio(first, second [3]uint8) float64 {
	firstLuminance := relativeLuminance(first)
	secondLuminance := relativeLuminance(second)
	if firstLuminance < secondLuminance {
		firstLuminance, secondLuminance = secondLuminance, firstLuminance
	}
	return (firstLuminance + 0.05) / (secondLuminance + 0.05)
}

func relativeLuminance(value [3]uint8) float64 {
	channel := func(component uint8) float64 {
		normalized := float64(component) / 255
		if normalized <= 0.04045 {
			return normalized / 12.92
		}
		return math.Pow((normalized+0.055)/1.055, 2.4)
	}
	return 0.2126*channel(value[0]) + 0.7152*channel(value[1]) + 0.0722*channel(value[2])
}
