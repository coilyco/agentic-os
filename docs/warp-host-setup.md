# Warp host setup

## Warp host setup

Per-host install and channel layout for Warp. The module model (apply/doctor, state layers, paths) lives in [warp.md](warp.md).

## Channels

Two Warp channels coexist on the Mac daily driver with separate bundle ids, URL schemes, and config dirs. Preview is the default-clicked terminal, Stable the named fallback.

- **Preview** (daily driver) - `/Applications/WarpPreview.app`, bundle `dev.warp.Warp-Preview`, scheme `warppreview://`, config dir `~/.warp-preview/`. Install: `ward pkg brew install --cask warp@preview --allow-untapped`.
- **Stable** (fallback) - `/Applications/Warp.app`, bundle `dev.warp.Warp-Stable`, scheme `warp://`, config dir `~/.warp/`. Install: `ward pkg brew install --cask warp --allow-untapped`.

Windows mirrors the split: Preview installs to `%LOCALAPPDATA%\Programs\WarpPreview` with config under `%LOCALAPPDATA%\warp\WarpPreview\`, Stable to `%LOCALAPPDATA%\Programs\Warp` with config under `%LOCALAPPDATA%\warp\Warp\`. `apply`/`doctor` auto-detect by install dir, preferring Preview, same as the Mac.

URL schemes are channel-specific by design: `warp://` always lands in Stable, `warppreview://` in Preview. There is no LaunchServices "default Warp" toggle that flips this, so tooling picks the channel by scheme at the call site.

Preview auto-symlinks `launch_configurations`, `tab_configs`, and `themes` into `~/.warp/` on first launch, so checked-in configs cover both channels for free. `settings.toml` is the one file that does not auto-share - the dual-channel install points Preview's `settings.toml` at the same source as Stable's.

## Install playbook (Mac daily driver)

1. `ward pkg brew install --cask warp@preview --allow-untapped` installs `/Applications/WarpPreview.app`.
2. Launch Preview once. It creates `~/.warp-preview/` and auto-symlinks the shared subdirs.
3. Point Preview's `settings.toml` at this repo's copy:
   ```sh
   rm ~/.warp-preview/settings.toml
   ln -s /Users/kai/projects/coilyco-flight-deck/agentic-os/warp/settings.toml ~/.warp-preview/settings.toml
   ```
4. Run `scripts/set-warp-default-editor.sh` to bind file-type defaults to Preview (markdown, python, go, the js/ts family, json, plain text, generic source UTI). It honors `WARP_DEFAULT_EDITOR_BUNDLE_ID` and `WARP_DEFAULT_EDITOR_APP_PATH` for per-host overrides.
5. Pin WarpPreview in the Dock, unpin Warp. Reach for it via Spotlight by typing `warppreview`.

Then `ward exec warp apply` reconciles the rest (settings, theme, launch configs, SQLite).

## Fallback to Stable

When a Preview regression breaks a workday:

- `open -a Warp` opens Stable by bundle name.
- `warp://` URIs always route to Stable - useful for known-good URI tests.
- `ward exec warp apply --channel stable` reconciles Stable's `~/.warp/` config and `dev.warp.Warp-Stable` SQLite directly (or export `WARP_CHANNEL=stable`). Without it, apply/doctor auto-detect and prefer Preview.
- `ward pkg brew upgrade --cask warp@preview --allow-untapped` picks up the next Preview release. Warp ships weekly.

## Subdirs

- `launch_configurations/` - one window, one-or-more tabs per YAML. URI `warppreview://launch/<name>`. `apply` symlinks each into the config dir and sweeps dangling links.
- `tab_configs/` - one tab per TOML, opens in the active window. URI `warppreview://tab_config/<name>`. The tab_config URI handler landed in Warp builds dated 2026-05-15 or later.
- `themes/` - color theme YAML files.
- `scripts/` - host-side helpers (`set-warp-default-editor.sh`).
- `settings.toml` - top-level settings, symlinked into both channels' config dirs.

## See also

- [warp.md](warp.md) - the Go module's model (apply/doctor, layers, paths).
- [tooling-warp skill](../.agents/skills/tooling-warp/SKILL.md) - agent-facing usage.
- the `ward agent <mode> work <ref> --new-tab` flow, which fires `warp://launch/...` from the CLI.
- the merged tab_config URI handler that motivated the Preview move.

## Warp mid-director mouse-tracking escape hatch

A recovery for a stuck xterm mouse-tracking flood inside a running warded director, without killing the director. See [warp.md](warp.md) for the config module this rides on, and the investigation notes in this doc.

## The failure

A warded director session (Claude Code, an Ink TUI, hosting child commands) can be poisoned by a child (a Linux tool under WSL, an Ink CLI) that enables xterm mouse-tracking mode 1003 and exits without restoring it. The Warp pane then floods with raw `^[[<35;X;YM` SGR motion reports (the `35` byte is 32 for motion + 3 for no button, which only mode 1003 emits). Nothing consumes the reports, so the pane fills with noise.

## Why the pwsh-prompt reset cannot heal it

The origin fix (commit `b562b4f`, `fix(warp): reset stuck mouse-tracking modes in pwsh prompt`) chains the outer host pwsh prompt to emit `ESC[?1000l..1006l` before each prompt, so a dirty-exiting child self-heals on the next prompt. Mouse-tracking (DECSET 1003) is state held in the Warp emulator - it only clears when a process writes DECRST bytes into the terminal output stream. A shell emits those only when it draws a prompt, and while the director TUI holds the terminal no prompting shell runs. The layers:

```
Warp pane -> pwsh (outer) -> Claude Code TUI (director) -> WSL/Ink child that dirties 1003
```

The child dirties the mode and returns control up to the director, which never yields to a prompt. The outer pwsh prompt only fires once the whole director exits - hence, historically, "drop the TUI." A precmd hook in the inner shell would fail the same way (the director sits between it and the pane), and a Warp keybinding that "sends text" writes the director's stdin, the wrong direction, and cannot reset the emulator.

## The fix: an emulator-level toggle keybinding

`warp apply` renders `keybindings.yaml` (a layer-2 rendered file, sibling of `settings.toml` in the config dir) with one managed binding:

```yaml
"workspace:toggle_mouse_reporting": "ctrl-shift-m"
```

`workspace:toggle_mouse_reporting` flips mouse reporting at the **Warp emulator layer** with zero cooperation from the director or any child. Press `ctrl-shift-m` while the flood is happening and it stops, TUI still running. This is the "reset without dropping the director" recovery.

It is a **toggle**, not a one-way disable: one press clears a stuck-on flood, a second press re-enables legit TUI mouse use. That is fine for an escape hatch.

The binding rides `warp apply` / `warp doctor` to every host, the same as `settings.toml`. `keybindings.yaml` did not exist on the Windows tower before this (zero custom bindings), so there was nothing to conflict with.

## Correct-layer note

The genuinely correct fix is upstream: the director TUI (Claude Code) re-asserting mouse state on redraw. That is not ours to patch. This keybinding is a mitigation.

A second lever was investigated and deferred: a dev-base `BASH_ENV`/EXIT-trap self-heal that emits DECRST on each `bash -c` teardown. It is mechanically easy but risky - a naive `printf` to stdout would inject escapes into Claude Code's captured command output fleet-wide, so it must emit to `/dev/tty` guarded by `[ -t 1 ]`, and that guard may mean it never fires for pipe-backed children (exactly the ones that matter). Blast radius vs uncertain payoff, so it needs an empirical spike of a live director session before it lands.
