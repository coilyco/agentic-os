---
doc_goal: Explain native agent-terminal installation and version lifecycle.
---
# Native `agent-terminal`

`agent-terminal` ships on the AOS `aos-vMAJOR.MINOR.PATCH` release train. The
same release defines its `agent-compose.overlay.v1` renderer contract and
stamps `aos`, `aoscomposed`, `aosward`, `aosguard`, and `agent-terminal` with one version.

## Requirements

The launcher needs these separate native dependencies on `PATH`:

* `agent-compose` supplies the renderer-neutral identity overlay
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

Both packages place `agent-terminal` on `PATH` beside `aos`, `aoscomposed`,
`aosward`, and `aosguard`.
A clean installation can run `agent-terminal --help` without an agentic-os
checkout. A dry run additionally needs `agent-compose` to return the selected
overlay, but does not need Alacritty.

Every versioned binary and `SHA256SUMS` is also available from the
[agentic-os releases](https://forgejo.coilysiren.me/coilyco-flight-deck/agentic-os/releases).

## Upgrade

`brew upgrade aos` upgrades the Homebrew installation. `scoop update aos`
upgrades the Scoop installation. Each update replaces all five AOS tools as
one version-aligned set.

## Rollback

Select an earlier `aos-v*` release and install its matching `aos`, `aoscomposed`,
`aosward`, `aosguard`, and `agent-terminal` artifacts together. Verify all five against that
release's `SHA256SUMS` before replacing the binaries on `PATH`. Do not mix
launcher and AOS versions because the AOS release defines the renderer
contract.

## Version reporting

```text
aos version
aoscomposed version
aosward version
aosguard --version
agent-terminal --version
```

All five outputs must name the same `aos-vMAJOR.MINOR.PATCH` release.

## See also

* [Branded Alacritty directors](alacritty-directors.md) - launch and renderer
  behavior.
* [AOS CLI release](aos-cli-release.md) - release automation and artifact
  validation.
