package main

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
)

// templateData is the substitution set; all paths use forward slashes.
type templateData struct {
	RepoRoot      string
	WorkspaceDir  string
	StartupDir    string
	ThemePath     string
	TabConfigPath string
	WallpaperPath string
}

// newTemplateData builds the substitution set from resolved host paths.
func newTemplateData(h *HostPaths) templateData {
	return templateData{
		RepoRoot:      filepath.ToSlash(h.RepoRoot),
		WorkspaceDir:  filepath.ToSlash(h.WorkspaceDir),
		StartupDir:    filepath.ToSlash(h.StartupDir),
		ThemePath:     filepath.ToSlash(h.ThemePath),
		TabConfigPath: filepath.ToSlash(h.TabConfigPath),
		WallpaperPath: filepath.ToSlash(h.WallpaperPath),
	}
}

// render executes one embedded template against the host substitution set.
func render(templateName string, data templateData) ([]byte, error) {
	raw, err := templatesFS.ReadFile("templates/" + templateName)
	if err != nil {
		return nil, fmt.Errorf("reading embedded template %s: %w", templateName, err)
	}
	tmpl, err := template.New(templateName).Option("missingkey=error").Parse(string(raw))
	if err != nil {
		return nil, fmt.Errorf("parsing template %s: %w", templateName, err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("rendering template %s: %w", templateName, err)
	}
	return buf.Bytes(), nil
}

// resolvePwshProfile asks pwsh 7 for $PROFILE. Windows only.
func resolvePwshProfile() (string, error) {
	candidates := []string{
		"pwsh", "pwsh.exe",
		`C:\Program Files\PowerShell\7\pwsh.exe`,
		`C:\Program Files\PowerShell\7-preview\pwsh.exe`,
	}
	for _, exe := range candidates {
		out, err := exec.Command(exe, "-NoProfile", "-Command", "$PROFILE").Output()
		if err == nil {
			p := strings.TrimSpace(string(out))
			if p != "" {
				return p, nil
			}
		}
	}
	return "", fmt.Errorf("could not resolve pwsh $PROFILE (is PowerShell 7 installed and on PATH?)")
}
