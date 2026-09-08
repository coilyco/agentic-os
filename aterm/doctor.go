package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/urfave/cli/v3"
)

const doctorFormat = "aterm.doctor.v1"

const (
	doctorOK   = "ok"
	doctorWarn = "warn"
	doctorFail = "fail"
)

var doctorMark = map[string]string{
	doctorOK:   lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render("ok  "),
	doctorWarn: lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Render("warn"),
	doctorFail: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("1")).Render("FAIL"),
}

type doctorCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

type doctorReport struct {
	Format string        `json:"format"`
	Checks []doctorCheck `json:"checks"`
	Ready  bool          `json:"ready"`
}

func (report *doctorReport) add(name, status, detail string, arguments ...any) {
	if len(arguments) > 0 {
		detail = fmt.Sprintf(detail, arguments...)
	}
	report.Checks = append(report.Checks, doctorCheck{Name: name, Status: status, Detail: detail})
}

// runDoctor walks the whole launch chain in one command, because a launch that
// produces no window otherwise sends the operator probing it by hand.
func runDoctor(ctx context.Context, deps commandDeps, cmd *cli.Command) error {
	report := doctorReport{Format: doctorFormat}
	checkNativeSession(&report)
	checkProjectsRoot(&report, cmd.String("working-directory"))
	roster := checkAgentCompose(ctx, deps, cmd, &report)
	checkTerminal(ctx, deps, cmd, &report)
	checkAOS(ctx, deps, cmd, &report, roster)
	report.Ready = true
	for _, check := range report.Checks {
		if check.Status == doctorFail {
			report.Ready = false
		}
	}
	if err := writeDoctorReport(cmd.Root().Writer, report, cmd.Bool("json")); err != nil {
		return err
	}
	if !report.Ready {
		return withExit(exitFailure, fmt.Errorf("doctor found a launch this host cannot make"))
	}
	return nil
}

func checkNativeSession(report *doctorReport) {
	launch := readCanonicalLaunch()
	switch {
	case !launch.inShadow():
		report.add("native session", doctorOK, "not inside one, so a launch can open its own")
	case launch.complete():
		report.add("native session", doctorOK,
			"inside %s, and a launch opens on the canonical %s", launch.Session, launch.Home)
	default:
		report.add("native session", doctorFail,
			"inside %s, and the aos that opened it publishes no canonical home, so a launch is refused",
			launch.Session)
	}
}

func checkProjectsRoot(report *doctorReport, configured string) {
	resolved, err := validateWorkingDirectory(configured)
	if err != nil {
		report.add("working directory", doctorFail, "%v", err)
		return
	}
	if strings.TrimSpace(os.Getenv(defaultWorkingEnvVar)) == "" {
		report.add("working directory", doctorWarn,
			"%s, but %s is unset so this is the fallback", resolved, defaultWorkingEnvVar)
		return
	}
	report.add("working directory", doctorOK, "%s", resolved)
}

func checkAgentCompose(
	ctx context.Context,
	deps commandDeps,
	cmd *cli.Command,
	report *doctorReport,
) rosterDocument {
	agentCompose, err := requireBinary(deps.lookPath, cmd.String("agent-compose-bin"))
	if err != nil {
		report.add("agent-compose", doctorFail, "%v", err)
		report.add("roster", doctorFail, "not read, agent-compose is missing")
		report.add("overlay", doctorFail, "not read, agent-compose is missing")
		return rosterDocument{}
	}
	report.add("agent-compose", doctorOK, "%s", agentCompose)
	roster, err := loadRoster(ctx, deps, agentCompose)
	if err != nil {
		report.add("roster", doctorFail, "%v", err)
		report.add("overlay", doctorFail, "not read, the roster failed")
		return rosterDocument{}
	}
	launchable := 0
	for _, role := range roster.Items {
		if len(role.nativeSeats()) > 0 {
			launchable++
		}
	}
	status := doctorOK
	if launchable == 0 {
		status = doctorFail
	}
	report.add("roster", status, "%s, %d role(s), %d with a launchable native seat",
		roster.Format, len(roster.Items), launchable)
	checkOverlay(ctx, deps, cmd, report, agentCompose, roster)
	return roster
}

func checkOverlay(
	ctx context.Context,
	deps commandDeps,
	cmd *cli.Command,
	report *doctorReport,
	agentCompose string,
	roster rosterDocument,
) {
	for _, role := range roster.Items {
		seats := role.nativeSeats()
		if len(seats) == 0 {
			continue
		}
		expression := strings.TrimSpace(cmd.String("expression"))
		document, err := loadOverlay(ctx, deps, agentCompose, role.Slug, seats[0].Harness, expression)
		if err != nil {
			report.add("overlay", doctorFail, "%v", err)
			return
		}
		report.add("overlay", doctorOK, "%s schema %d, read for %s %s",
			overlayFormat, overlaySchema, role.Slug, seats[0].Harness)
		checkSensoryVocabulary(report, document)
		return
	}
	report.add("overlay", doctorFail, "no live role has a launchable native seat to read one for")
}

// The identity card, the launch motion, and the sound mark are built from these
// fields, so a roster that stops shipping them degrades quietly. agentic-os#1251
func checkSensoryVocabulary(report *doctorReport, document overlayDocument) {
	missing := []string{}
	for _, personality := range document.Personalities {
		for name, value := range map[string]string{
			"color":      personality.Color,
			"motif":      personality.Motif,
			"emblem":     personality.Emblem.Emoji,
			"geometry":   personality.Geometry,
			"body":       personality.Body.Archetype,
			"sound_mark": personality.SoundMark.Timbre,
		} {
			if strings.TrimSpace(value) == "" {
				missing = append(missing, personality.Name+"."+name)
			}
		}
	}
	if len(missing) > 0 {
		report.add("identity vocabulary", doctorWarn,
			"%d field(s) shipped empty: %s", len(missing), strings.Join(missing, ", "))
		return
	}
	report.add("identity vocabulary", doctorOK,
		"%d personality(s), every sensory field present", len(document.Personalities))
}

func checkTerminal(
	ctx context.Context,
	deps commandDeps,
	cmd *cli.Command,
	report *doctorReport,
) {
	name := strings.TrimSpace(cmd.String("terminal-bin"))
	terminal, err := requireBinary(deps.lookPath, name)
	if err != nil {
		report.add("terminal", doctorFail, "%v", err)
		report.add("terminal config", doctorFail, "not read, %s is missing", name)
		return
	}
	version := ""
	if raw, err := deps.output(ctx, terminal, "--version"); err == nil {
		version = " // " + firstLine(string(raw))
	}
	report.add("terminal", doctorOK, "%s%s", terminal, version)
	// The config parse is kitty's own, so a terminal that does not answer is a
	// warning rather than a verdict about the config. See docs/aterm.md.
	if err := deps.run(ctx, terminal, "+runpy", "raise SystemExit(0)"); err != nil {
		report.add("terminal config", doctorWarn,
			"%s could not be asked to parse its config: %v", name, err)
		return
	}
	report.add("terminal config", doctorOK, "%s parses its resolved config", name)
	checkFont(ctx, deps, report)
}

// The Sombra baseline names no font family on purpose, so the usual answer is
// the terminal's own default. A host config that names one is worth checking.
func checkFont(ctx context.Context, deps commandDeps, report *doctorReport) {
	path := terminalConfigPath()
	family := configuredFontFamily(path)
	if family == "" {
		report.add("font", doctorOK, "the terminal's own default, no font_family configured")
		return
	}
	lister, err := requireBinary(deps.lookPath, "fc-list")
	if err != nil {
		report.add("font", doctorWarn, "%q configured, not verified without fc-list", family)
		return
	}
	raw, err := deps.output(ctx, lister, family)
	if err != nil || strings.TrimSpace(string(raw)) == "" {
		report.add("font", doctorFail, "%q is configured in %s and is not installed", family, path)
		return
	}
	report.add("font", doctorOK, "%q is installed", family)
}

func terminalConfigPath() string {
	if directory := strings.TrimSpace(os.Getenv("KITTY_CONFIG_DIRECTORY")); directory != "" {
		return filepath.Join(directory, "kitty.conf")
	}
	config := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME"))
	if config == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		config = filepath.Join(home, ".config")
	}
	return filepath.Join(config, "kitty", "kitty.conf")
}

func configuredFontFamily(path string) string {
	if path == "" {
		return ""
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	family := ""
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		// Last setting wins in kitty, matching how it reads its own config.
		if len(fields) > 1 && fields[0] == "font_family" {
			family = strings.Join(fields[1:], " ")
		}
	}
	return family
}

func checkAOS(
	ctx context.Context,
	deps commandDeps,
	cmd *cli.Command,
	report *doctorReport,
	roster rosterDocument,
) {
	aos, err := requireBinary(deps.lookPath, cmd.String("aos-bin"))
	if err != nil {
		report.add("aos", doctorWarn, "%v, so every launch runs unleased", err)
		report.add("session shadow", doctorWarn, "unleased, aos is missing")
		report.add("launch profiles", doctorWarn, "not read, aos is missing")
		return
	}
	report.add("aos", doctorOK, "%s", aos)
	// nativeShadowAvailable degrades silently and correctly, which is exactly
	// why doctor is where it becomes legible. agentic-os#1257
	if nativeShadowAvailable(ctx, deps, aos) {
		report.add("session shadow", doctorOK, "leased, the window gets its own worktree")
	} else {
		report.add("session shadow", doctorWarn,
			"unleased, `%s _native-shadow --probe` failed, so the window shares this "+
				"checkout and its launch skips daily host convergence", aos)
	}
	checkLaunchProfiles(ctx, deps, cmd, report, aos, roster)
}

func checkLaunchProfiles(
	ctx context.Context,
	deps commandDeps,
	cmd *cli.Command,
	report *doctorReport,
	aos string,
	roster rosterDocument,
) {
	if len(roster.Items) == 0 {
		report.add("launch profiles", doctorFail, "not read, the roster is empty")
		return
	}
	resolved := 0
	broken := []string{}
	for _, role := range roster.Items {
		if len(role.nativeSeats()) == 0 {
			continue
		}
		raw, err := deps.output(ctx, aos, "_launch-agent", role.Slug)
		if err != nil {
			broken = append(broken, role.Slug+": "+firstLine(err.Error()))
			continue
		}
		seat := strings.TrimSpace(string(raw))
		if !isNativeHarness(seat) || !seatInRole(seat, role) {
			broken = append(broken, fmt.Sprintf("%s: %q is not one of its launchable seats", role.Slug, seat))
			continue
		}
		resolved++
	}
	if len(broken) > 0 {
		report.add("launch profiles", doctorFail,
			"%d resolved, %d did not: %s", resolved, len(broken), strings.Join(broken, "; "))
		return
	}
	report.add("launch profiles", doctorOK, "%d role(s) resolve to a launchable seat", resolved)
}

func firstLine(value string) string {
	value = strings.TrimSpace(value)
	if index := strings.IndexAny(value, "\r\n"); index >= 0 {
		return strings.TrimSpace(value[:index])
	}
	return value
}

func writeDoctorReport(writer io.Writer, report doctorReport, asJSON bool) error {
	if asJSON {
		encoded, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal the doctor report: %w", err)
		}
		_, err = fmt.Fprintf(writer, "%s\n", encoded)
		return err
	}
	width := 0
	for _, check := range report.Checks {
		if len(check.Name) > width {
			width = len(check.Name)
		}
	}
	label := lipgloss.NewStyle().Width(width + 2)
	for _, check := range report.Checks {
		fmt.Fprintf(writer, "%s  %s%s\n", doctorMark[check.Status], label.Render(check.Name), check.Detail)
	}
	if report.Ready {
		_, err := fmt.Fprintln(writer, "\naterm can open a window on this host.")
		return err
	}
	_, err := fmt.Fprintln(writer, "\naterm cannot open a window on this host.")
	return err
}
