package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/urfave/cli/v3"
)

const (
	defaultConvergeConfig = ".config/aos/converge.yaml"
	defaultCatalogueState = ".local/state/aos"
	defaultManifestPath   = ".config/aos/catalogues.json"
	catalogueManifestKind = "aos.catalogues.v1"
)

type environmentConfig struct {
	Catalogues *catalogueConfig `yaml:"catalogues"`
	MCP        *mcpConfig       `yaml:"mcp"`
}

type catalogueConfig struct {
	StateDir string   `yaml:"state_dir"`
	Manifest string   `yaml:"manifest"`
	CacheTTL string   `yaml:"cache_ttl"`
	Sources  []string `yaml:"sources"`
}

type mcpConfig struct {
	Inventory     string `yaml:"inventory"`
	ProjectNative *bool  `yaml:"project_native"`
}

type environmentConvergeOptions struct {
	ConfigPath string
	Home       string
	Check      bool
	Now        func() time.Time
	// Environment judges the ${VAR} headers of the Claude registry, nil keeps them all.
	Environment environmentLookup
	// ExportCredentials fills Environment before an apply. Check never calls it,
	// since check makes no network access.
	ExportCredentials func(context.Context)
}

type environmentConvergeResult struct {
	Catalogues int
	MCPServers int
	Changed    bool
	Warnings   []string
	Configured bool
}

type catalogueManifest struct {
	Format     string                   `json:"format"`
	Catalogues []catalogueManifestEntry `json:"catalogues"`
}

type catalogueManifestEntry struct {
	Source string `json:"source"`
	Path   string `json:"path"`
	Commit string `json:"commit"`
}

func runEnvironmentConverge(ctx context.Context, cmd *cli.Command) error {
	result, err := convergeEnvironment(ctx, environmentConvergeOptions{
		ConfigPath:  cmd.String("config"),
		Home:        cmd.String("home"),
		Check:       cmd.Bool("check"),
		Environment: os.LookupEnv,
		ExportCredentials: func(ctx context.Context) {
			applyNativeLaunchCredentials(ctx, os.Stderr, "")
		},
	})
	if err != nil {
		return err
	}
	if !result.Configured {
		fmt.Println("AOS convergence: no config, nothing to do")
		return nil
	}
	status := "current"
	if result.Changed {
		status = "changed"
		if cmd.Bool("check") {
			status = "drifted"
		}
	}
	fmt.Printf(
		"AOS convergence: %s (%d catalogues, %d MCP servers)\n",
		status,
		result.Catalogues,
		result.MCPServers,
	)
	for _, warning := range result.Warnings {
		fmt.Fprintf(os.Stderr, "warning: %s\n", warning)
	}
	if cmd.Bool("check") && result.Changed {
		return cli.Exit("AOS convergence drift detected", 1)
	}
	return nil
}

func convergeEnvironment(
	ctx context.Context,
	opts environmentConvergeOptions,
) (environmentConvergeResult, error) {
	home, err := resolveConvergeHome(opts.Home)
	if err != nil {
		return environmentConvergeResult{}, err
	}
	explicitConfig := strings.TrimSpace(opts.ConfigPath) != ""
	configPath := strings.TrimSpace(opts.ConfigPath)
	if configPath == "" {
		configPath = filepath.Join(home, filepath.FromSlash(defaultConvergeConfig))
	} else {
		configPath, err = resolveConvergePath(configPath, home, "")
		if err != nil {
			return environmentConvergeResult{}, fmt.Errorf("resolve AOS convergence config: %w", err)
		}
	}
	raw, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) && !explicitConfig {
			return environmentConvergeResult{}, nil
		}
		return environmentConvergeResult{}, fmt.Errorf("read AOS convergence config: %w", err)
	}
	var config environmentConfig
	if err := yaml.UnmarshalWithOptions(raw, &config, yaml.Strict()); err != nil {
		return environmentConvergeResult{}, fmt.Errorf("parse AOS convergence config: %w", err)
	}
	result := environmentConvergeResult{Configured: true}
	configDir := filepath.Dir(configPath)

	if config.Catalogues != nil {
		catalogueResult, err := convergeCatalogues(
			ctx,
			*config.Catalogues,
			home,
			configDir,
			opts.Check,
			opts.Now,
		)
		if err != nil {
			return environmentConvergeResult{}, err
		}
		result.Catalogues = len(catalogueResult.Entries)
		result.Changed = result.Changed || catalogueResult.Changed
		result.Warnings = append(result.Warnings, catalogueResult.Warnings...)
	}
	if config.MCP != nil {
		inventory, err := resolveRequiredConfigPath(
			config.MCP.Inventory,
			home,
			configDir,
			"MCP inventory",
		)
		if err != nil {
			return environmentConvergeResult{}, err
		}
		projectNative := true
		if config.MCP.ProjectNative != nil {
			projectNative = *config.MCP.ProjectNative
		}
		if projectNative && !opts.Check && opts.ExportCredentials != nil {
			// Before the projection, which leaves off a header whose variable is unset.
			opts.ExportCredentials(ctx)
		}
		mcpResult, err := projectNativeMCP(nativeMCPOptions{
			Inventory:     inventory,
			Home:          home,
			Check:         opts.Check,
			ProjectNative: projectNative,
			Environment:   opts.Environment,
		})
		if err != nil {
			return environmentConvergeResult{}, err
		}
		result.MCPServers = mcpResult.Servers
		result.Changed = result.Changed || mcpResult.Changed
		result.Warnings = append(result.Warnings, mcpResult.HeaderOmissions...)
	}
	return result, nil
}

type catalogueConvergeResult struct {
	Entries  []catalogueManifestEntry
	Changed  bool
	Warnings []string
}

func convergeCatalogues(
	ctx context.Context,
	config catalogueConfig,
	home string,
	configDir string,
	check bool,
	now func() time.Time,
) (catalogueConvergeResult, error) {
	stateDir := config.StateDir
	if strings.TrimSpace(stateDir) == "" {
		stateDir = filepath.Join(home, filepath.FromSlash(defaultCatalogueState))
	}
	resolvedState, err := resolveConvergePath(stateDir, home, configDir)
	if err != nil {
		return catalogueConvergeResult{}, fmt.Errorf("resolve catalogue state directory: %w", err)
	}
	manifestPath := config.Manifest
	if strings.TrimSpace(manifestPath) == "" {
		manifestPath = filepath.Join(home, filepath.FromSlash(defaultManifestPath))
	}
	resolvedManifest, err := resolveConvergePath(manifestPath, home, configDir)
	if err != nil {
		return catalogueConvergeResult{}, fmt.Errorf("resolve catalogue manifest: %w", err)
	}
	ttl := defaultCatalogueTTL
	if strings.TrimSpace(config.CacheTTL) != "" {
		ttl, err = time.ParseDuration(config.CacheTTL)
		if err != nil {
			return catalogueConvergeResult{}, fmt.Errorf("parse catalogue cache_ttl: %w", err)
		}
	}
	if ttl < 0 {
		return catalogueConvergeResult{}, fmt.Errorf("catalogue cache_ttl cannot be negative")
	}
	if now == nil {
		now = time.Now
	}
	sources := make([]catalogueSource, 0, len(config.Sources))
	for _, locator := range config.Sources {
		source, err := parseCatalogueLocator(locator)
		if err != nil {
			return catalogueConvergeResult{}, err
		}
		sources = append(sources, source)
	}
	options := catalogueHydrateOptions{
		StateDir: resolvedState,
		TTL:      ttl,
		Now:      now,
		Check:    check,
	}
	hydrated, drift, err := hydrateCatalogues(ctx, sources, options)
	if err != nil {
		return catalogueConvergeResult{}, err
	}
	entries := make([]catalogueManifestEntry, 0, len(hydrated))
	var warnings []string
	for _, item := range hydrated {
		entries = append(entries, catalogueManifestEntry{
			Source: item.Source.Locator,
			Path:   item.Path,
			Commit: item.Commit,
		})
		if item.Warning != "" {
			warnings = append(warnings, item.Warning)
		}
	}
	payload, err := json.MarshalIndent(catalogueManifest{
		Format:     catalogueManifestKind,
		Catalogues: entries,
	}, "", "  ")
	if err != nil {
		return catalogueConvergeResult{}, fmt.Errorf("render catalogue manifest: %w", err)
	}
	payload = append(payload, '\n')
	manifestChanged := !fileHasBytes(resolvedManifest, payload)
	if !check && manifestChanged {
		if err := writeAtomicFile(resolvedManifest, payload, 0o644); err != nil {
			return catalogueConvergeResult{}, fmt.Errorf("write catalogue manifest: %w", err)
		}
	}
	return catalogueConvergeResult{
		Entries:  entries,
		Changed:  drift || manifestChanged,
		Warnings: warnings,
	}, nil
}

func resolveConvergeHome(raw string) (string, error) {
	home := strings.TrimSpace(raw)
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home: %w", err)
		}
	}
	absolute, err := filepath.Abs(home)
	if err != nil {
		return "", fmt.Errorf("resolve home: %w", err)
	}
	return filepath.Clean(absolute), nil
}

func resolveRequiredConfigPath(raw, home, base, label string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", fmt.Errorf("%s path is required", label)
	}
	resolved, err := resolveConvergePath(raw, home, base)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", label, err)
	}
	return resolved, nil
}

func resolveConvergePath(raw, home, base string) (string, error) {
	value := strings.TrimSpace(strings.ReplaceAll(raw, "${HOME}", home))
	switch {
	case value == "~":
		value = home
	case strings.HasPrefix(value, "~/") || strings.HasPrefix(value, `~\`):
		value = filepath.Join(home, value[2:])
	}
	if !filepath.IsAbs(value) {
		if strings.TrimSpace(base) == "" {
			base = "."
		}
		value = filepath.Join(base, value)
	}
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	return filepath.Clean(absolute), nil
}

func fileHasBytes(path string, desired []byte) bool {
	current, err := os.ReadFile(path)
	return err == nil && bytes.Equal(current, desired)
}

func writeAtomicFile(path string, payload []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".aos-converge-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if _, err := temp.Write(payload); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Chmod(mode); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		_ = os.Remove(path)
	}
	return os.Rename(tempPath, path)
}
