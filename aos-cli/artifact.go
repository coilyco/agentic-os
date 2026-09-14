// Scaffolds a self-contained web artifact over the coilyco kit, and tells a
// generated project when the kit it vendored has moved. See the template README.
package main

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/urfave/cli/v3"
)

//go:embed artifact_assets/coilyco-kit.css artifact_assets/coilyco-kit.json
var artifactAssets embed.FS

//go:embed artifact_template/*
var artifactTemplate embed.FS

const (
	kitCSSName  = "coilyco-kit.css"
	kitPinName  = "coilyco-kit.json"
	kitVendorIn = "vendor"
)

// kitPin is the consumer's copy of the kit's identity. The token table is
// deliberately not stored: a pin is for comparison, not for re-deriving.
type kitPin struct {
	Schema string `json:"schema"`
	Hash   string `json:"hash"`
	Count  int    `json:"count"`
	// The stylesheet's own digest. Without it the pin describes a file the
	// check never opens, so a deleted or edited CSS still reported current.
	CSS string `json:"css_sha256,omitempty"`
}

func sha256File(path string) (string, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), nil
}

var artifactNameRe = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

func embeddedKitPin() (kitPin, error) {
	raw, err := artifactAssets.ReadFile("artifact_assets/" + kitPinName)
	if err != nil {
		return kitPin{}, fmt.Errorf("read embedded kit pin: %w", err)
	}
	var pin kitPin
	if err := json.Unmarshal(raw, &pin); err != nil {
		return kitPin{}, fmt.Errorf("parse embedded kit pin: %w", err)
	}
	css, err := artifactAssets.ReadFile("artifact_assets/" + kitCSSName)
	if err != nil {
		return kitPin{}, fmt.Errorf("read embedded kit css: %w", err)
	}
	sum := sha256.Sum256(css)
	pin.CSS = hex.EncodeToString(sum[:])
	return pin, nil
}

func readProjectPin(dir string) (kitPin, error) {
	path := filepath.Join(dir, kitVendorIn, kitPinName)
	raw, err := os.ReadFile(path)
	if err != nil {
		return kitPin{}, fmt.Errorf("read %s: %w", path, err)
	}
	var pin kitPin
	if err := json.Unmarshal(raw, &pin); err != nil {
		return kitPin{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return pin, nil
}

// artifactScaffold refuses an existing directory rather than merging into one:
// a half-written project is harder to diagnose than a refusal.
func artifactScaffold(root, name string) error {
	if _, err := os.Stat(root); err == nil {
		return fmt.Errorf("%s already exists", root)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect %s: %w", root, err)
	}
	if err := os.MkdirAll(filepath.Join(root, kitVendorIn), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", root, err)
	}
	entries, err := fs.ReadDir(artifactTemplate, "artifact_template")
	if err != nil {
		return fmt.Errorf("read template: %w", err)
	}
	for _, entry := range entries {
		raw, err := artifactTemplate.ReadFile("artifact_template/" + entry.Name())
		if err != nil {
			return fmt.Errorf("read template %s: %w", entry.Name(), err)
		}
		// The suffix keeps this repo's doc and comment rules off files bound
		// for another repo. A leading underscore restores a dotfile.
		out := strings.TrimSuffix(entry.Name(), ".tmpl")
		if trimmed := strings.TrimPrefix(out, "_"); trimmed != out {
			out = "." + trimmed
		}
		body := strings.ReplaceAll(string(raw), "{{NAME}}", name)
		if err := os.WriteFile(filepath.Join(root, out), []byte(body), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", out, err)
		}
	}
	css, err := artifactAssets.ReadFile("artifact_assets/" + kitCSSName)
	if err != nil {
		return fmt.Errorf("read embedded %s: %w", kitCSSName, err)
	}
	if err := os.WriteFile(filepath.Join(root, kitVendorIn, kitCSSName), css, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", kitCSSName, err)
	}
	pin, err := embeddedKitPin()
	if err != nil {
		return err
	}
	body, err := json.MarshalIndent(pin, "", "  ")
	if err != nil {
		return fmt.Errorf("encode kit pin: %w", err)
	}
	target := filepath.Join(root, kitVendorIn, kitPinName)
	if err := os.WriteFile(target, append(body, '\n'), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", target, err)
	}
	return nil
}

func runArtifactNew(_ *cli.Command, args []string, stdout *os.File) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: aos artifact new <name>")
	}
	name := strings.TrimSpace(args[0])
	if !artifactNameRe.MatchString(name) {
		return fmt.Errorf(
			"name %q: use lowercase letters, digits and hyphens, starting with a letter",
			name,
		)
	}
	root, err := filepath.Abs(name)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", name, err)
	}
	if err := artifactScaffold(root, name); err != nil {
		return err
	}
	pin, err := embeddedKitPin()
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "created %s\n", root)
	fmt.Fprintf(stdout, "kit pin %s %s (%d primitives)\n", pin.Schema, pin.Hash, pin.Count)
	fmt.Fprintf(stdout, "\n  cd %s && npm install && npm run dev\n", name)
	return nil
}

// artifactCheckResult separates the two answers a reader needs: whether the
// vendored kit has moved, and whether the pin could be read at all.
type artifactCheckResult struct {
	Project kitPin
	Current kitPin
	Stale   bool
	// Set when the stylesheet on disk is not the one the pin describes, absence
	// included. A pin that never opens its own file passed a deleted one.
	CSSProblem string
}

func artifactCheck(dir string) (artifactCheckResult, error) {
	current, err := embeddedKitPin()
	if err != nil {
		return artifactCheckResult{}, err
	}
	project, err := readProjectPin(dir)
	if err != nil {
		return artifactCheckResult{}, err
	}
	if project.Schema != current.Schema {
		return artifactCheckResult{}, fmt.Errorf(
			"pin schema %q is not %q, so the two cannot be compared",
			project.Schema, current.Schema,
		)
	}
	result := artifactCheckResult{
		Project: project,
		Current: current,
		// Compared on the hash alone. A count cannot stand in for it, because
		// one literal swapped for another moves the hash and no count at all.
		Stale: project.Hash != current.Hash,
	}
	// The pin describes a stylesheet, so the check has to open it.
	cssPath := filepath.Join(dir, kitVendorIn, kitCSSName)
	switch actual, err := sha256File(cssPath); {
	case os.IsNotExist(err):
		result.CSSProblem = fmt.Sprintf("%s is absent", filepath.Join(kitVendorIn, kitCSSName))
	case err != nil:
		result.CSSProblem = fmt.Sprintf("%s could not be read: %v", kitCSSName, err)
	case project.CSS == "":
		result.CSSProblem = fmt.Sprintf(
			"%s records no css_sha256, so this project predates the digest and its "+
				"stylesheet cannot be verified; re-scaffold to pick one up", kitPinName)
	case actual != project.CSS:
		result.CSSProblem = fmt.Sprintf(
			"%s does not match the digest its own pin records, so it was edited or "+
				"replaced after scaffolding", filepath.Join(kitVendorIn, kitCSSName))
	}
	return result, nil
}

func runArtifactCheck(_ *cli.Command, args []string, stdout *os.File) error {
	dir := "."
	if len(args) == 1 {
		dir = args[0]
	} else if len(args) > 1 {
		return fmt.Errorf("usage: aos artifact check [dir]")
	}
	result, err := artifactCheck(dir)
	if err != nil {
		return err
	}
	if result.CSSProblem != "" {
		return fmt.Errorf("vendored kit is not intact: %s", result.CSSProblem)
	}
	if !result.Stale {
		fmt.Fprintf(stdout, "kit pin current: %s (%d primitives)\n",
			result.Current.Hash, result.Current.Count)
		return nil
	}
	return fmt.Errorf(
		"vendored kit is stale: project pins %s (%d primitives), this aos ships %s (%d). "+
			"Re-run `aos artifact new` into a scratch directory and copy %s/%s across, "+
			"then re-check the page against the moved tokens",
		result.Project.Hash, result.Project.Count,
		result.Current.Hash, result.Current.Count,
		kitVendorIn, kitCSSName,
	)
}

func artifactCommand() *cli.Command {
	return &cli.Command{
		Name:  "artifact",
		Usage: "scaffold a web page over the coilyco kit that fetches nothing at runtime",
		Commands: []*cli.Command{
			{
				Name:      "new",
				Usage:     "scaffold a new artifact project",
				ArgsUsage: "<name>",
				Action: func(_ context.Context, cmd *cli.Command) error {
					return runArtifactNew(cmd, cmd.Args().Slice(), os.Stdout)
				},
			},
			{
				Name:      "check",
				Usage:     "report whether the project's vendored kit has moved",
				ArgsUsage: "[dir]",
				Action: func(_ context.Context, cmd *cli.Command) error {
					return runArtifactCheck(cmd, cmd.Args().Slice(), os.Stdout)
				},
			},
		},
	}
}
