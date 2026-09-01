---
name: tooling-warp
description: Operate and diagnose the repo-managed Warp terminal configuration. Triggers - Warp, WarpPreview, warp settings, tabs, warp://, warppreview://.
---

# Warp

Use this thin router for the Warp configuration shipped by AOS. The repo
docs own platform layout, settings, and failure details.

## Core loop

* Run `just warp doctor` to inspect the chosen channel without changing
  it.
* Run `just warp apply` after an approved config change.
* Set `WARP_CHANNEL=preview|stable` or pass `--channel` when auto-detection
  should not choose. Preview is preferred when both channels exist.
* Set `WARP_STARTUP_DIR` to an absolute operator-local path when new tabs
  should open somewhere other than the projects root.

## Routing facts

* `warppreview://` opens Preview and `warp://` opens Stable.
* `launch_configurations` open windows. `tab_configs` open tabs.
* The `warp/` module and embedded templates are canonical. Do not hand-edit a
  rendered host copy.
* Ward owns reconciliation and verification. The skill does not reproduce
  settings keys or SQLite internals.

## Read next

* [`references/warp.md`](references/warp.md) - state layers, commands, paths, and
  sharp edges.
* [`references/warp-host-setup.md`](references/warp-host-setup.md) - channel
  installation and host setup.
* [`references/warp-host-setup.md`](references/warp-host-setup.md) -
  recovery from a stuck mouse-tracking flood.
