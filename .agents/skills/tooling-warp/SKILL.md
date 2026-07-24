---
name: tooling-warp
description: Warp is Kai's terminal on every host. Block-mode shell with left-side vertical tabs, one shell per tab. Preview is the Mac daily driver, Stable is the fallback.
---

# Warp

The terminal on every host. Block-mode UI with left-side vertical tabs.

## Use when

Editing Warp config, debugging UI noise, adjusting tab behavior, configuring shells per-OS, tuning Warp AI/agent surfaces, or wiring `launch_configurations` / `tab_configs` URI handlers.

## Triggers

warp, Warp, WarpPreview, ~/.warp, ~/.warp-preview, warp settings, settings.toml, vertical tabs, warp tabs, launch configuration, tab config, warp blocks, warp ai, warp agent, warpify, subshell, startup_config, warp://, warppreview://.

## Map

- [channels](references/channels.md) - Stable vs Preview, bundle ids, URL schemes, why Preview is the daily driver.
- [config-and-sync](references/config-and-sync.md) - canonical repo paths, the symlink layout, and the cloud-sync gotcha.
- [tabs-and-settings](references/tabs-and-settings.md) - why Warp, tab discipline, and `[appearance.vertical_tabs]` settings that must not regress.
- [portability-and-launch](references/portability-and-launch.md) - host-absolute path caveat, launch_configurations / tab_configs URIs, and common edits.
- [internals](references/internals.md) - AI noise-cut, `settings.toml` <-> SQLite model, schema-enum debugging, inline rendering.
- [internals-debug](references/internals-debug.md) - SQLite peeking, enum-source trust order, schema gotchas.

## Start dir override

`StartupDir` (where a fresh tab opens) defaults to the projects root, one level above the repo's workspace dir. Set `WARP_STARTUP_DIR` to an absolute path in the env before `ward exec warp apply` to pin it per host - e.g. a specific repo instead of the root. It is operator-local preference, read from the local env and never embedded in the tracked templates. One override feeds every surface at once: fresh Bash and Zsh sessions landing at `$HOME`, the pwsh profile auto-cd, Warp's `settings.toml` `custom_dir` (global/new_tab/new_window), and `startup_config.toml`. `doctor` reads the same env, so a host with the override set still passes clean. On Windows, `setx WARP_STARTUP_DIR <path>` persists it across sessions.

## See also

- [`docs/warp-host-setup.md`](../../../docs/warp-host-setup.md) - install playbook for the Mac daily driver (brew install Preview, swap `settings.toml` symlink, run `scripts/set-warp-default-editor.sh` to rebind file-type defaults via `duti` + `lsregister`, Dock/Spotlight discipline). The script honors `WARP_DEFAULT_EDITOR_BUNDLE_ID` and `WARP_DEFAULT_EDITOR_APP_PATH` for per-host overrides.
- automate the `warp/launch_configurations/*` and `warp/tab_configs/*` symlink walk into `~/.warp/`.
- dual-channel install (the Preview promotion).
- `ward agent <mode> work <ref> --new-tab`, the consumer of the Warp tab_config URI handler.
- merged tab_config URI handler that motivated moving to Preview as the daily driver.
- terminal file-viewer wrapper functions (`bat`, `view`, `open`, etc.).
- Brewfile catalog of modern CLI tools.
