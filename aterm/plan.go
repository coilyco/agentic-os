package main

import (
	"context"
	"fmt"
	"strings"
)

const launchFormat = "aterm.launch.v1"

type launchRequest struct {
	Role             string
	Seat             string
	Expression       string
	TaskTitle        string
	WorkingDirectory string
	AgentComposeBin  string
	AOSBin           string
	TerminalBin      string
	Workspace        string
	StartAs          string
	FontSize         string
	NoMotion         bool
	Sound            bool
	Extra            []string
	Hold             bool
	Creature         creaturePlate
}

type launchIdentity struct {
	Person          string `json:"person"`
	Role            string `json:"role"`
	RoleDisplayName string `json:"role_display_name"`
	Seat            string `json:"seat"`
	Name            string `json:"name"`
	Pronouns        string `json:"pronouns"`
	Annotation      string `json:"annotation"`
	Expression      string `json:"expression"`
	FavoriteColor   string `json:"favorite_color"`
}

type launchPlan struct {
	Format           string         `json:"format"`
	Identity         launchIdentity `json:"identity"`
	Brand            launchBrand    `json:"brand"`
	WorkingDirectory string         `json:"working_directory"`
	Workspace        string         `json:"workspace"`
	Card             sessionCard    `json:"card"`
	Creature         creaturePlate  `json:"creature"`
	Shadowed         bool           `json:"shadowed"`
	Child            []string       `json:"child"`
	Executable       string         `json:"executable"`
	Arguments        []string       `json:"arguments"`
}

// composeChild mirrors the acompose shell function so the window runs the same
// runtime the bare command does. See agentic-os/shell/common.sh.
func composeChild(request launchRequest, agentCompose, aos string, shadowed bool) []string {
	launch := []string{agentCompose, "launch", request.Role, request.Seat}
	launch = append(launch, request.Extra...)
	if !shadowed {
		return launch
	}
	child := []string{
		aos, "_native-shadow",
		"--harness", request.Seat,
		"--role", request.Role,
		"--assigned-role", "--",
	}
	return append(child, launch...)
}

// nativeShadowAvailable probes the same way the acompose shell function does. An
// AOS without the shadow verb still launches, just without a leased workspace.
func nativeShadowAvailable(ctx context.Context, deps commandDeps, aos string) bool {
	command := []string{aos, "_native-shadow", "--probe"}
	return whileWaiting(deps.notice, command, func() error {
		return deps.run(ctx, command[0], command[1:]...)
	}) == nil
}

func buildLaunchPlan(
	document overlayDocument,
	request launchRequest,
	cwd string,
	self string,
	agentCompose string,
	aos string,
	shadowed bool,
) (launchPlan, error) {
	brand, err := buildBrand(document, request.TaskTitle, request.Workspace)
	if err != nil {
		return launchPlan{}, err
	}
	child := composeChild(request, agentCompose, aos, shadowed)
	// kitty's --title permanently fixes the OS window title against the child,
	// which is the job Alacritty needed a separate dynamic_title=false for.
	arguments := []string{
		"--title", brand.Title,
		"--directory", cwd,
		"--start-as", request.StartAs,
		"-o", fmt.Sprintf("font_size=%s", request.FontSize),
		"-o", "background_opacity=1.0",
		// A kitty outliving its last window blanks a bundle reopen. See docs/aterm.md.
		"-o", "macos_quit_when_last_window_closed=yes",
		"-o", fmt.Sprintf("background=%s", brand.Background),
		"-o", fmt.Sprintf("cursor=%s", brand.Accent),
		"-o", fmt.Sprintf("selection_background=%s", brand.Accent),
		"-o", fmt.Sprintf("selection_foreground=%s", brand.SelectionText),
	}
	// kitty draws the window background back over the image at this fraction,
	// which is what holds the creature under the text. See docs/aterm.md.
	if request.Creature.Path != "" {
		arguments = append(arguments,
			"-o", fmt.Sprintf("background_image=%s", request.Creature.Path),
			"-o", "background_image_layout=cscaled",
			"-o", "background_image_linear=yes",
			"-o", fmt.Sprintf("background_tint=%s", request.Creature.Tint),
		)
	}
	plan := launchPlan{
		Format: launchFormat,
		Identity: launchIdentity{
			Person:          document.Person,
			Role:            document.Role,
			RoleDisplayName: document.RoleDisplayName,
			Seat:            document.Seat.Harness,
			Name:            document.Seat.Name,
			Pronouns:        document.Seat.Pronouns,
			Annotation:      seatAnnotation(document),
			Expression:      document.Expression,
			FavoriteColor:   brand.Accent,
		},
		Brand:            brand,
		WorkingDirectory: cwd,
		Workspace:        request.Workspace,
		Creature:         request.Creature,
		Shadowed:         shadowed,
		Child:            child,
		Executable:       strings.TrimSpace(request.TerminalBin),
	}
	plan.Card = buildSessionCard(document, plan)
	// The card renders inside the window, so it travels to the session stage
	// rather than being drawn by the launcher. See docs/aterm.md.
	encoded, err := encodeSessionCard(plan.Card)
	if err != nil {
		return launchPlan{}, err
	}
	session := []string{self, sessionCommand}
	if request.Hold {
		session = append(session, "--hold")
	}
	if request.NoMotion {
		session = append(session, "--no-motion")
	}
	if request.Sound {
		session = append(session, "--sound")
	}
	session = append(session, "--card", encoded, "--")
	// kitty takes the program as trailing arguments, with no -e separator.
	plan.Arguments = append(arguments, append(session, child...)...)
	return plan, nil
}
