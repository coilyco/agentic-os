---
doc_goal: Explain native aosterm installation and version lifecycle.
---
# Native `aosterm`

`aosterm` ships on the AOS `aos-vMAJOR.MINOR.PATCH` release train beside its
compatibility name, `agent-terminal`. The same release defines its
`agent-compose.overlay.v1` renderer contract and stamps every native binary
with one version.

## Requirements

The launcher needs these separate native dependencies on `PATH`:

* `agent-compose` supplies the renderer-neutral identity overlay
* `aoscompose` supplies the composed standalone launch
* Alacritty receives the translated title, color, and child command arguments

Neither dependency is bundled into the launcher. Packages contain no
host-specific path or fleet configuration.

## Install

Homebrew installs the native set on macOS:

```sh
brew tap coilyco-flight-deck/tap https://forgejo.coilysiren.me/coilyco-flight-deck/homebrew-tap.git
brew install coilyco-flight-deck/tap/aos
```

Scoop installs the native set on Windows:

```sh
scoop bucket add coilyco https://forgejo.coilysiren.me/coilyco-flight-deck/scoop-bucket.git
scoop install coilyco/aos
```

Both packages place `aosterm` and `agent-terminal` on `PATH` beside `aos`,
`aoscompose`, `aoscomposed`, `aosward`, and `aosguard`. A clean installation
can run either terminal command's `--help` without an agentic-os checkout. A dry
run additionally needs `agent-compose` and `aoscompose`, but not Alacritty.

Every versioned binary and `SHA256SUMS` is also available from the
[agentic-os releases](https://forgejo.coilysiren.me/coilyco-flight-deck/agentic-os/releases).

## Upgrade

`brew upgrade aos` upgrades the Homebrew installation. `scoop update aos`
upgrades the Scoop installation. Each update replaces the version-aligned
native command set together.

## Rollback

Select an earlier `aos-v*` release and install its matching artifacts together.
Verify them against that release's `SHA256SUMS` before replacing binaries on
`PATH`. Do not mix launcher and AOS versions because the AOS release defines
the renderer contract.

## Version reporting

```text
aos version
aoscompose version
aoscomposed version
aosward version
aosguard --version
agent-terminal --version
aosterm --version
```

All outputs must name the same `aos-vMAJOR.MINOR.PATCH` release.

## See also

* [Branded Alacritty directors](alacritty-directors.md) - launch and renderer
  behavior.
* [AOS CLI release](aos-cli-release.md) - release automation and artifact
  validation.
