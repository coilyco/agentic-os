# aterm macOS app bundles

Opening a role from Spotlight, the Dock, or Finder. The window each bundle
opens is [the native agent terminal](aterm.md).

`aterm bundles` writes one `.app` per live role into `~/Applications`, so a role opens from Spotlight, the Dock, or Finder with no terminal to type in. Each wraps `aterm <role>`, so it opens the window above.

```text
just aterm-bundles                   # write them
just aterm-bundles --dry-run         # what would land, rendered
```

**A Finder launch carries none of your shell's PATH.** It starts at `/usr/bin:/bin:/usr/sbin:/sbin`, where none of `aos`, `agent-compose`, kitty, or the harness lives. Pinning the first three through the env vars `aterm` reads is not enough, since `agent-compose launch` resolves the harness itself, so a bundle stopping there reached shadow init and died on `claude` not found. The wrapper rebuilds `PATH` from a login shell, current as tools move, over a baked copy for a profile exporting none. The baked copy is the half that outlives the shell it was taken from, so a versioned Homebrew Cellar entry in it is a dead directory after the next upgrade of that formula: the bake trades `<prefix>/Cellar/<formula>/<version>` for the `<prefix>/opt/<formula>` symlink Homebrew repoints instead, collapsing two installed versions of one formula to the single path they both stand for. A formula with no `opt` path is unlinked and reachable only through the Cellar, so that one keeps its version rather than losing the only route to it. agentic-os#1440

**A window belongs to the bundle holding the binary that drew it.** Calling kitty where it lives credited every window to kitty in the Dock, the switcher, and Mission Control. Each bundle links the terminal in beside its launcher and opens through it, so those surfaces read the role, and `LSUIElement` went with it: an accessory app cannot own the front window. A shell launch prefers the role's installed bundle too. The plist names the host's architecture, or LaunchServices runs it translated and every child inherits that (#1291). A launch failing before a window opens has nothing to hold the error, so the wrapper alerts through `osascript`.

**A bundle is only as new as the `aterm` it calls.** Generation writes the wrapper, so a name or `PATH` fix lands on regeneration while every window option comes from the binary it invokes. Those moving separately once made half a fix look whole, so generation warns when the builds differ.

**A bundle is named for who answers**, the person and role rather than the harness: `Vera // Systems Administrator`. A POSIX filename cannot hold a slash, so it is stored with ` :: `, which macOS renders as one, and only the directory carries it: the executable stays a plain `aterm-<role>`.

**Roles come from the live roster**, the read the launcher and its completion use, so no second list goes stale. A bundle is recognized by a marker inside it rather than its name, so a renamed scheme reports what the last run wrote instead of orphaning it. What this run no longer writes is reported rather than deleted, an app it did not write is never overwritten, and every target is checked first, so a refusal cannot half-regenerate the set. Bundles are per-role on the launch profiles' seat, each Finder-tagged `acompose` so one Spotlight word finds them. `--tag` renames it. kitty outlives its last window by macOS convention, which left the bundle registered and the next click reopening nothing, so `aterm` quits it with the window.

## Running through VibeTunnel

Every session `aterm` opens, from a Dock bundle or the shell, runs through [VibeTunnel](https://vibetunnel.sh/)'s `vt`, so the same terminal also shows in a browser. **`_session` puts the harness behind `vt -S`, and `--no-vibetunnel` (or `ATERM_NO_VIBETUNNEL`) opts out.** The launcher only passes `--vibetunnel` to `_session`. `_session` looks `vt` up on the window's own PATH after the card has drawn, so the browser sees the harness and not the animation. `--dry-run` shows the state on a `vibetunnel` row. `plan.Child` stays the compose command, so the release check that asserts against it does not change.

**`-S` is required.** Without it `vt` re-runs the command through the interactive shell, which re-quotes the argv and sources the rc files a second time. With it the argv reaches the harness untouched, including the shadow child's own `--`. That was run against VibeTunnel 1.0.0-beta.18. `vt` reads no `--` of its own, so the child's first word must not start with a dash. `vt` exits with the child's code, so `_session` still holds the window on a failure.

**A window drops `VIBETUNNEL_SESSION_ID`.** `vt` refuses to start inside a session it opened and exits 1. An aterm launched from a wrapped shell would open a window whose harness never starts. `spawnWindow` filters the marker on every launch, shadow or not.

**A missing `vt` costs the browser view, never the session.** `_session` prints one line and runs the harness directly. `aterm doctor` reports a `vibetunnel` row as a warning when `vt` is missing or `vt status` does not print `Running: Yes`, and never fails the launch chain on it. The status text is vt's own words, so the check is one substring.

**The server's bind address and authentication are the operator's config, not aterm's.** aterm starts no server and sets no credential. It only decides whether a session is visible to the one that is running.
