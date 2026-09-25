# aterm macOS app bundles

Opening a role from Spotlight, the Dock, or Finder. The window each bundle
opens is [the native agent terminal](aterm.md).

`aterm bundles` writes one `.app` per live role into `~/Desktop`, so a role opens from Spotlight, the Dock, or Finder with no terminal to type in. Each wraps `aterm <role>`, so it opens the window above.

```text
just aterm-bundles                   # write them
just aterm-bundles --dry-run         # what would land, rendered
```

**A Finder launch carries none of your shell's PATH.** It starts at `/usr/bin:/bin:/usr/sbin:/sbin`, where none of `aos`, `agent-compose`, kitty, or the harness lives. Pinning the first three through the env vars `aterm` reads is not enough, since `agent-compose launch` resolves the harness itself, so a bundle stopping there reached shadow init and died on `claude` not found. The wrapper rebuilds `PATH` from a login shell, current as tools move, over a baked copy for a profile exporting none. The baked copy is the half that outlives the shell it was taken from, so a versioned Homebrew Cellar entry in it is a dead directory after the next upgrade of that formula: the bake trades `<prefix>/Cellar/<formula>/<version>` for the `<prefix>/opt/<formula>` symlink Homebrew repoints instead, collapsing two installed versions of one formula to the single path they both stand for. A formula with no `opt` path is unlinked and reachable only through the Cellar, so that one keeps its version rather than losing the only route to it. agentic-os#1440

**A window belongs to the bundle holding the binary that drew it.** Calling kitty where it lives credited every window to kitty in the Dock, the switcher, and Mission Control. Each bundle links the terminal in beside its launcher and opens through it, so those surfaces read the role, and `LSUIElement` went with it: an accessory app cannot own the front window. A shell launch prefers the role's installed bundle too. The plist names the host's architecture, or LaunchServices runs it translated and every child inherits that (#1291). A launch failing before a window opens has nothing to hold the error, so the wrapper alerts through `osascript`.

**`aterm bundles --check` reports drift and writes nothing, so a convergence tool can gate the write on it.** A write clears each `.app` before rebuilding it, and a bundle launch is what starts the daily local convergence, so an unconditional write would delete the launcher that is running it. `--check` compares each bundle's launcher, `Info.plist`, terminal link, icon, and Finder tag against what a write would produce now, prints `current`, `drifted <path>: <parts>`, or `missing`, and exits 7 when any differs, where a failure exits 1. A stale bundle is listed and never counts as drift. It takes neither `--dry-run` nor `--json`. The build is baked in the plist, so every release shows as drift once.

**A bundle is only as new as the `aterm` it calls.** Generation writes the wrapper, so a name or `PATH` fix lands on regeneration while every window option comes from the binary it invokes. Those moving separately once made half a fix look whole, so generation warns when the builds differ.

**A bundle is named for the role alone**, the roster's display name: `Platform Engineer`. The person moves to Get Info, and the executable stays a plain `aterm-<role>`.

**Roles come from the live roster**, the read the launcher and its completion use, so no second list goes stale. A bundle is recognized by a marker inside it rather than its name, so a renamed scheme reports what the last run wrote instead of orphaning it. What this run no longer writes is reported, and removed only under `--prune` after every write lands, an app it did not write is never overwritten, and every target is checked first, so a refusal cannot half-regenerate the set. Bundles are per-role on the launch profiles' seat, each Finder-tagged `acompose` so one Spotlight word finds them. `--tag` renames it. kitty outlives its last window by macOS convention, which left the bundle registered and the next click reopening nothing, so `aterm` quits it with the window.
