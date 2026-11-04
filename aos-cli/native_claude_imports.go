package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// nativeClaudeApprovedImports are the only external files Kai approved for every native
// session, standing, on 2026-11-04. They are home-relative so the policy names no host.
var nativeClaudeApprovedImports = []string{
	"projects/coilyco/agentic-os/AGENTS.md",
	"projects/coilyco/agentic-os-kai/AGENTS.md",
}

// nativeClaudeImportToken is an `@/abs/path` or `@~/path` import, as claude reads one.
var nativeClaudeImportToken = regexp.MustCompile(`(?:^|[\s(])@(~?/[^\s)]+)`)

// nativeClaudeImportDepth bounds how far an approved file's own imports are followed.
const nativeClaudeImportDepth = 3

// nativeClaudeImportsApproved is whether every external file the sources import, followed
// through each other, is one of the approved two. Missing sources import nothing.
func nativeClaudeImportsApproved(home string, ignored []string, sources ...string) bool {
	approved := map[string]bool{}
	for _, relative := range nativeClaudeApprovedImports {
		for _, form := range nativeClaudeTrustPaths([]string{filepath.Join(home, relative)}) {
			approved[form] = true
		}
	}
	seen := map[string]bool{}
	var walk func(path string, depth int) bool
	walk = func(path string, depth int) bool {
		if seen[path] {
			return true
		}
		seen[path] = true
		raw, err := os.ReadFile(path)
		if err != nil {
			return true
		}
		for _, match := range nativeClaudeImportToken.FindAllStringSubmatch(string(raw), -1) {
			target := filepath.Clean(strings.Replace(match[1], "~/", home+"/", 1))
			if underAny(target, ignored) {
				continue
			}
			if !approved[target] {
				return false
			}
			if depth < nativeClaudeImportDepth && !walk(target, depth+1) {
				return false
			}
		}
		return true
	}
	for _, source := range sources {
		if filepath.IsAbs(source) && !walk(source, 0) {
			return false
		}
	}
	return true
}

func underAny(path string, roots []string) bool {
	for _, root := range roots {
		if root != "" && (path == root || strings.HasPrefix(path, root+string(os.PathSeparator))) {
			return true
		}
	}
	return false
}
