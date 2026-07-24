---
name: coding-shape-tui
description: Category umbrella for building terminal UIs. Charm stack is the default - bubbletea, lipgloss, gum, glow, huh. Fall back to tview/promptui only when a project already commits to them.
---

# coding-shape-tui

Umbrella for any work that ships a terminal UI as the primary surface.

## Triggers

tui, terminal ui, charm, charmbracelet, bubbletea, lipgloss, gum, glow, huh, freeze, vhs, soft-serve, wish, tview, promptui, blessed, ink, textual.

## Framework defaults by language

- **Go** → Charm stack:
  - `bubbletea` - TUI framework (Elm architecture).
  - `lipgloss` - styling (border, padding, color).
  - `huh` - forms.
  - `gum` - shell-script prompts/styling (no Go binary needed).
  - `glow` - markdown render.
  - `vhs` - terminal recording for demos.
- **Python** → `textual` (Textualize) for full TUIs, `rich` for output styling without an interactive loop.
- **Node/TypeScript** → `ink` (React-shaped, but acceptable in this niche).

Reach for tview/promptui only when an existing project already commits to them.

## Why Charm

Cotton-candy aesthetic, MIT-licensed, very actively maintained, plays nicely with `urfave/cli` for the imperative-CLI-with-occasional-TUI shape. See `kai-tech-prefs` (in agentic-os-kai).

## Design principles

- **TTY-aware.** Detect non-interactive mode (pipe, CI) and degrade to plain text. Never wedge a script.
- **Keyboard-first.** Mouse support is bonus, not load-bearing.
- **Quit gracefully.** Ctrl+C and `q` should always work.
- **Color via lipgloss/rich.** Never hand-roll ANSI escape codes.
- **Width-responsive.** Don't assume 80 columns. Test at 40 and 200.

## When this skill is active

Designing a TUI, adding an interactive surface to an existing CLI, or styling terminal output beyond plain print.

## See also

- [`coding-go`](../coding-go/SKILL.md) - host language for Charm.
