---
name: coding-go
description: Umbrella for Go work across repos. Carries Kai's defaults (Go 1.22+, urfave/cli over cobra/kong, Charm stack for TUI, slog for logging) so agents inherit them before reaching for generic Go.
seed:
  kind: language
  language: go
  extensions: [".go"]
---

# coding-go

Umbrella for any Go work.

## Triggers

go, golang, .go, go.mod, go.sum, gopls, urfave, cobra, kong, charm, bubbletea, lipgloss, gum, glow, slog, gofmt, goroutine, channel.

## Defaults

- **Version**: Go 1.22+ unless project pins lower.
- **Modules**: always. No GOPATH-era patterns.
- **CLI framework**: `urfave/cli` first (Kai is a maintainer), cobra/kong only if a project already commits to them. See `kai-tech-prefs` (in agentic-os-kai).
- **TUI**: Charm stack (bubbletea, lipgloss, gum, glow, huh). Same skill prefs.
- **Logging**: `log/slog` stdlib. Structured, leveled.
- **Tests**: stdlib `testing`. Reach for `testify` only when assertions get repetitive.
- **Lint**: `golangci-lint`.

## Style

- Errors are values. Wrap with `fmt.Errorf("context: %w", err)`. Don't swallow.
- Small interfaces. Define at the consumer, not the producer.
- No god structs. If a struct has fields for two concerns, split.
- Avoid `init()` for anything besides flag/format registration.
- `context.Context` first parameter on any I/O-bound function.

## When this skill is active

Editing or writing Go. Inherit Kai's defaults before reaching for general Go patterns.

## See also

- [`coding-shape-tui`](../coding-shape-tui/SKILL.md) - building TUIs (Charm stack).
