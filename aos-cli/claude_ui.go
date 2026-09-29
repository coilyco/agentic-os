package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/urfave/cli/v3"
)

// Claude Code theme and settings fragment rendered from the public person
// snapshot, moved here from agent-compose nativeui. docs/native-harness-config.md

const claudeUIBaseTheme = "dark"

// The subagent palette is eight fixed tokens. A role claims the nearest slot
// rather than adding one.
var claudeUISubagentSlots = map[string]string{
	"red_FOR_SUBAGENTS_ONLY":    "#dc2626",
	"blue_FOR_SUBAGENTS_ONLY":   "#6a9bcc",
	"green_FOR_SUBAGENTS_ONLY":  "#16a34a",
	"yellow_FOR_SUBAGENTS_ONLY": "#ca8a04",
	"purple_FOR_SUBAGENTS_ONLY": "#827dbd",
	"orange_FOR_SUBAGENTS_ONLY": "#d97757",
	"pink_FOR_SUBAGENTS_ONLY":   "#c46686",
	"cyan_FOR_SUBAGENTS_ONLY":   "#0891b2",
}

// claudeUISubagentStatusLine still names agent-compose until the subagent
// renderer moves here too (teable:coilyco/agent-compose#8199 item 8).
const claudeUISubagentStatusLine = "acompose statusline --subagent --color"

type claudeUISnapshot struct {
	RoleOrder     []string                       `json:"role_order"`
	Roles         map[string]claudeUIRole        `json:"roles"`
	Personalities map[string]claudeUIPersonality `json:"personalities"`
}

type claudeUIRole struct {
	DisplayName   string `json:"display_name"`
	FavoriteColor string `json:"favorite_color"`
	Identity      *struct {
		Name string `json:"name"`
	} `json:"identity"`
	Personalities []string `json:"personalities"`
	Purpose       string   `json:"purpose"`
}

type claudeUIPersonality struct {
	Color string   `json:"color"`
	Verbs []string `json:"verbs"`
}

type claudeUITheme struct {
	Name      string            `json:"name"`
	Base      string            `json:"base"`
	Overrides map[string]string `json:"overrides"`
}

type claudeUISettings struct {
	Theme        string `json:"theme"`
	SpinnerVerbs struct {
		Mode  string   `json:"mode"`
		Verbs []string `json:"verbs"`
	} `json:"spinnerVerbs"`
	SpinnerTipsEnabled bool `json:"spinnerTipsEnabled"`
	SpinnerTips        struct {
		ExcludeDefault bool     `json:"excludeDefault"`
		Tips           []string `json:"tips"`
	} `json:"spinnerTipsOverride"`
	SubagentStatusLine struct {
		Type    string `json:"type"`
		Command string `json:"command"`
	} `json:"subagentStatusLine"`
}

type claudeUIBundle struct {
	Role     string           `json:"role"`
	Slug     string           `json:"slug"`
	Theme    claudeUITheme    `json:"theme"`
	Settings claudeUISettings `json:"settings"`
}

func buildClaudeUI(snapshot claudeUISnapshot, roleName, spinnerMode string) (claudeUIBundle, error) {
	role, ok := snapshot.Roles[roleName]
	if !ok {
		return claudeUIBundle{}, fmt.Errorf("unknown role %q", roleName)
	}
	if len(role.Personalities) == 0 {
		return claudeUIBundle{}, fmt.Errorf("role %q has no personalities", roleName)
	}
	colors := make([]string, 0, len(role.Personalities))
	verbs := []string{}
	for _, name := range role.Personalities {
		personality, ok := snapshot.Personalities[name]
		if !ok {
			return claudeUIBundle{}, fmt.Errorf("role %q names missing personality %q", roleName, name)
		}
		colors = append(colors, personality.Color)
		verbs = append(verbs, personality.Verbs...)
	}
	overrides, err := claudeUIOverrides(role.FavoriteColor, colors)
	if err != nil {
		return claudeUIBundle{}, fmt.Errorf("role %q theme: %w", roleName, err)
	}
	displayName := role.DisplayName
	if role.Identity != nil && role.Identity.Name != "" {
		displayName = fmt.Sprintf("%s (%s)", role.DisplayName, role.Identity.Name)
	}
	if spinnerMode == "" {
		spinnerMode = "replace"
	}
	slug := "aos-" + roleName
	bundle := claudeUIBundle{
		Role:  roleName,
		Slug:  slug,
		Theme: claudeUITheme{Name: displayName, Base: claudeUIBaseTheme, Overrides: overrides},
	}
	bundle.Settings.Theme = "custom:" + slug
	bundle.Settings.SpinnerVerbs.Mode = spinnerMode
	bundle.Settings.SpinnerVerbs.Verbs = verbs
	bundle.Settings.SpinnerTipsEnabled = true
	bundle.Settings.SpinnerTips.Tips = claudeUITips(role)
	bundle.Settings.SubagentStatusLine.Type = "command"
	bundle.Settings.SubagentStatusLine.Command = claudeUISubagentStatusLine
	return bundle, nil
}

// claudeUITips carry doctrine rather than voice, because a tip lands while
// the reader is waiting.
func claudeUITips(role claudeUIRole) []string {
	tips := []string{role.Purpose}
	if role.Identity != nil && role.Identity.Name != "" {
		tips = append(tips, fmt.Sprintf(
			"%s holds the %s charter. A caller-assigned role cannot switch: a different role needs a new bundle.",
			role.Identity.Name, role.DisplayName,
		))
	}
	return append(tips, "Boundary: "+strings.Join(role.Personalities, ", ")+".")
}

func claudeUIOverrides(roleColor string, melded []string) (map[string]string, error) {
	overrides := map[string]string{
		"claude":           roleColor,
		"clawd_body":       roleColor,
		"briefLabelClaude": roleColor,
		"promptBorder":     roleColor,
		"skill":            melded[0],
		"autoAccept":       melded[0],
		"permission":       melded[len(melded)/2],
		"bashBorder":       melded[len(melded)/2],
		"suggestion":       melded[len(melded)-1],
		"remember":         melded[len(melded)-1],
	}
	for token, paired := range map[string]string{
		"claudeShimmer":       "claude",
		"promptBorderShimmer": "promptBorder",
		"permissionShimmer":   "permission",
	} {
		shimmer, err := claudeUIShimmer(overrides[paired])
		if err != nil {
			return nil, err
		}
		overrides[token] = shimmer
	}
	names := make([]string, 0, len(claudeUISubagentSlots))
	for name := range claudeUISubagentSlots {
		names = append(names, name)
	}
	sort.Strings(names)
	target, err := claudeUIOKLab(roleColor)
	if err != nil {
		return nil, err
	}
	best, bestDistance := "", math.Inf(1)
	for _, name := range names {
		lab, err := claudeUIOKLab(claudeUISubagentSlots[name])
		if err != nil {
			return nil, err
		}
		// Hue and chroma only: the slot is a palette pick, not a free color.
		if distance := math.Hypot(lab[1]-target[1], lab[2]-target[2]); distance < bestDistance {
			best, bestDistance = name, distance
		}
	}
	overrides[best] = roleColor
	return overrides, nil
}

// Shimmer raises lightness toward a ceiling and leaves hue and chroma alone.
func claudeUIShimmer(hex string) (string, error) {
	lab, err := claudeUIOKLab(hex)
	if err != nil {
		return "", err
	}
	lab[0] = math.Min(0.95, lab[0]+0.10)
	return claudeUIHex(lab), nil
}

func claudeUIOKLab(hex string) ([3]float64, error) {
	h := strings.TrimPrefix(hex, "#")
	var r, g, b int
	if len(h) != 6 {
		return [3]float64{}, fmt.Errorf("color %q must be #rrggbb", hex)
	}
	if _, err := fmt.Sscanf(h, "%02x%02x%02x", &r, &g, &b); err != nil {
		return [3]float64{}, fmt.Errorf("color %q must be #rrggbb: %w", hex, err)
	}
	lr, lg, lb := claudeUILinear(r), claudeUILinear(g), claudeUILinear(b)
	l := math.Cbrt(0.4122214708*lr + 0.5363325363*lg + 0.0514459929*lb)
	m := math.Cbrt(0.2119034982*lr + 0.6806995451*lg + 0.1073969566*lb)
	s := math.Cbrt(0.0883024619*lr + 0.2817188376*lg + 0.6299787005*lb)
	return [3]float64{
		0.2104542553*l + 0.7936177850*m - 0.0040720468*s,
		1.9779984951*l - 2.4285922050*m + 0.4505937099*s,
		0.0259040371*l + 0.7827717662*m - 0.8086757660*s,
	}, nil
}

func claudeUIHex(lab [3]float64) string {
	cube := func(v float64) float64 { return v * v * v }
	l := cube(lab[0] + 0.3963377774*lab[1] + 0.2158037573*lab[2])
	m := cube(lab[0] - 0.1055613458*lab[1] - 0.0638541728*lab[2])
	s := cube(lab[0] - 0.0894841775*lab[1] - 1.2914855480*lab[2])
	return fmt.Sprintf("#%02x%02x%02x",
		claudeUIGamma(4.0767416621*l-3.3077115913*m+0.2309699292*s),
		claudeUIGamma(-1.2684380046*l+2.6097574011*m-0.3413193965*s),
		claudeUIGamma(-0.0041960863*l-0.7034186147*m+1.7076147010*s))
}

func claudeUILinear(channel int) float64 {
	c := float64(channel) / 255
	if c <= 0.04045 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}

func claudeUIGamma(c float64) int {
	c = math.Max(0, math.Min(1, c))
	if c <= 0.0031308 {
		c *= 12.92
	} else {
		c = 1.055*math.Pow(c, 1/2.4) - 0.055
	}
	return int(math.Round(c * 255))
}

// loadClaudeUISnapshot reads the person snapshot from agent-compose, which
// owns composition. aos renders the Claude surface from it.
func loadClaudeUISnapshot(ctx context.Context) (claudeUISnapshot, error) {
	var snapshot claudeUISnapshot
	out, err := exec.CommandContext(ctx, "agent-compose", "catalog", "snapshot").Output()
	if err != nil {
		return snapshot, fmt.Errorf("agent-compose catalog snapshot: %w", err)
	}
	if err := json.Unmarshal(out, &snapshot); err != nil {
		return snapshot, fmt.Errorf("parse person snapshot: %w", err)
	}
	return snapshot, nil
}

// writeClaudeUI writes <dir>/themes/<slug>.json and <dir>/settings.<role>.json,
// the layout agent-compose native-ui --out used.
func writeClaudeUI(dir string, bundle claudeUIBundle) error {
	if err := os.MkdirAll(filepath.Join(dir, "themes"), 0o755); err != nil {
		return err
	}
	for path, value := range map[string]any{
		filepath.Join(dir, "themes", bundle.Slug+".json"):   bundle.Theme,
		filepath.Join(dir, "settings."+bundle.Role+".json"): bundle.Settings,
	} {
		raw, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(path, append(raw, '\n'), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func runClaudeUI(ctx context.Context, cmd *cli.Command) error {
	snapshot, err := loadClaudeUISnapshot(ctx)
	if err != nil {
		return err
	}
	roles := snapshot.RoleOrder
	if role := cmd.String("role"); role != "" {
		roles = []string{role}
	}
	bundles := make([]claudeUIBundle, 0, len(roles))
	for _, role := range roles {
		bundle, err := buildClaudeUI(snapshot, role, cmd.String("spinner-mode"))
		if err != nil {
			return err
		}
		bundles = append(bundles, bundle)
	}
	if out := cmd.String("out"); out != "" {
		for _, bundle := range bundles {
			if err := writeClaudeUI(out, bundle); err != nil {
				return err
			}
		}
		return nil
	}
	raw, err := json.MarshalIndent(bundles, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Println(string(raw))
	return err
}
