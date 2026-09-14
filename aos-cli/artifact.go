// Scaffolds a self-contained web artifact over the coilyco kit, and tells a
// generated project when the kit it vendored has moved. See the template README.
package main

import (
	"context"
	"embed"
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
	for _, asset := range []string{kitCSSName, kitPinName} {
		raw, err := artifactAssets.ReadFile("artifact_assets/" + asset)
		if err != nil {
			return fmt.Errorf("read embedded %s: %w", asset, err)
		}
		target := filepath.Join(root, kitVendorIn, asset)
		if err := os.WriteFile(target, raw, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", target, err)
		}
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
	return artifactCheckResult{
		Project: project,
		Current: current,
		// Compared on the hash alone. A count cannot stand in for it, because
		// one literal swapped for another moves the hash and no count at all.
		Stale: project.Hash != current.Hash,
	}, nil
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
		Usage: "scaffold and check a self-contained web artifact built over the coilyco kit",
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
