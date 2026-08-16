# Warp config module

## Warp config module

The `warp/` Go module establishes and verifies Kai's Warp terminal config across hosts. Run it through ward, never bare `go`:

- `ward exec warp apply` - host-aware, idempotent: reconcile every state layer.
- `ward exec warp doctor` - verify only: PASS/FAIL per check, no mutation.

The repo is the source of truth. `apply` pushes the repo's intent onto the host, `doctor` reports drift without touching anything. Both resolve the repo root by walking up from cwd to the first `.git`, so they work from any subdir.

## State layers

`apply` reconciles four kinds of state, each with a matching `doctor` check:

- **Rendered files** - `settings.toml`, `keybindings.yaml`, the theme YAML, `tab_configs/startup_config.toml`. Embedded templates rendered to real files (not symlinks) in the Warp config dir. Drift = content mismatch, except [volatile keys](warp.md) doctor skips.
- **Launch-config symlinks** - every `warp/launch_configurations/*.yaml` is symlinked into the config dir's `launch_configurations/`, and dangling links are swept. See [launch configs](#launch-configs) below.
- **SQLite settings** - keys in Warp's `warp.sqlite` with no TOML surface (see `mapping.go`), plus a Windows-only [default-shell pref](warp-default-shell.md). Skipped until Warp inits the DB.
- **Wallpaper** - existence check only.

## Per-OS config dir

`paths.go` resolves the layout per OS. The config dir is:

- **macOS** - channel-aware. Preview (default) at `~/.warp-preview/` with SQLite under `dev.warp.Warp-Preview`, Stable (fallback) at `~/.warp/` with SQLite under `dev.warp.Warp-Stable`.
- **Windows** - channel-aware: `%LOCALAPPDATA%\warp\WarpPreview\` or `...\warp\Warp\`, config in `<channel>\config\`, SQLite in `<channel>\data\`. Themes scan `%APPDATA%` (Roaming).
- **Linux** - `~/.config/warp-terminal/`.

On macOS and Windows the two channels coexist, so `apply`/`doctor` pick one and target its config dir and SQLite together. Selection: `--channel preview|stable` (or `WARP_CHANNEL` env) wins, else auto-detect by installed app (macOS `/Applications`, Windows `%LOCALAPPDATA%\Programs`), **preferring Preview**. The resolved channel is echoed in the header line. Linux is single-channel and ignores the flag.

`WorkspaceDir` is the parent of the repo root. `StartupDir` (where a fresh tab opens) is one level above that on every OS.

## Launch configs

Launch configs are mirrored, not rendered: the repo dir is the source, the config dir holds a symlink per `*.yaml`. `apply` creates any missing link, repoints a stale one, and sweeps links that no longer resolve. It is idempotent - a link already pointing at the right source is left alone - and it never touches real files or healthy unrelated links. A pre-existing real file at a target is backed up to `<name>.bak` before linking.

The walk exists because hand-made links went orphaned: a new or moved launch config stayed invisible to Warp until someone ran `ln -s` by hand. Implementation in `launch.go`.

`tab_configs/` entries beyond the rendered `startup_config.toml` remain hand-placed real files, not part of this walk.

## Sharp edges

- **Preview vs Stable** - two channels coexist with separate config dirs and URL schemes. `warppreview://` always lands in Preview, `warp://` in Stable.
- **Stuck mouse-tracking flood** - a child that dirties DECSET 1003 floods the pane. `ctrl-shift-m` clears it mid-director, no TUI kill. See [warp-host-setup.md](warp-host-setup.md).
- **SQLite needs init** - the SQLite layer skips until Warp has launched once and created `warp.sqlite`. A fresh host shows SKIP, not FAIL.
- **Windows symlinks** - need Developer Mode or elevation for the launch-config walk, same caveat as the skills sweep.

## See also

- [warp-host-setup.md](warp-host-setup.md) - channel split, install playbook, URI scheme.
- [tooling-warp skill](../.agents/skills/tooling-warp/SKILL.md) - agent-facing usage.
- [README.md](../README.md) - repo intro.

## Warp-owned volatile settings

`ward exec warp apply` renders `settings.toml` from `warp/templates/settings.toml.tmpl` and `ward exec warp doctor` checks the on-disk file against that same render. The default check is byte-for-byte: any difference is drift and FAILs.

A few keys break that model. The running Warp persists live UI and cloud-account state back into `settings.toml` on every launch, overwriting whatever `apply` wrote. Pinning them in the template guarantees perpetual doctor drift no matter what value the template carries.

## The volatile keys

The allowlist is `volatileSettingsKeys` in `warp/main.go`:

- `zoom_level` (under `[appearance.window]`) - Warp writes the live window zoom. The template pins a value, but Warp rewrites it to whatever the window is currently at.
- `cloud_conversation_storage_enabled` (under `[agents]`) - Warp mirrors cloud account state here on every launch, regardless of `is_settings_sync_enabled`. The only way to actually pin it false is the Warp UI toggle, which then makes Warp write `false` itself.

## How doctor handles them

For each volatile key, doctor neutralizes the key's **value** (not the line) in both the canonical render and the on-disk file before comparing, via `reconcileVolatile` / `neutralizeKey` in `warp/main.go`. A value-only difference therefore no longer counts as drift, and doctor emits a `NOTE` showing `template=<x> live=<y>` instead of a `FAIL`.

Only the scalar value is exempt. The key line itself must still be present and in place, so the template keeps emitting both keys (and `cloud_conversation_storage_enabled` keeps its documenting comment). Structural drift - a missing key line, a moved section header, a renamed key - still FAILs. The neutralization is also prefix-safe: `zoom_level` never matches a longer key like `max_zoom_level`.

## Adding a key

Add the TOML key name to `volatileSettingsKeys` in `warp/main.go`. Only do this for keys Warp genuinely owns and rewrites from live state - everything else should stay an enforced byte-for-byte match. See docs/warp.md for the original motivation.
