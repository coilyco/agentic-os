---
doc_goal: Explain the standalone aos CLI release, artifact, and package-manager contract.
---
# aos CLI release

The portable CLI has an independent `aos-vMAJOR.MINOR.PATCH` clock inside the
agentic-os Forgejo repository. Root `v*` tags remain owned by the full
dev-base image, while hooks use `aos-precommit-v*`, so CLI delivery never waits
on either train.

## Automatic release

The promoted `release` branch drives `.forgejo/workflows/aos-cli-release.yml`.
Its native path filter covers the shipped Go roots, AOSguard specs and bridges,
the CLI-owned Specgen pin, and release build/package scripts. Unrelated changes
do not start the workflow. Manual dispatch remains the retry and override path.
The release job:

1. installs the validated workflow Ward, then runs Python, Go, and pre-commit validation
2. bumps the CLI minor version without reading commit-message signals
3. cross-compiles matching `aos`, `aoscompose`, `aosward`, `aosguard`, `agent-terminal`, and `aosterm` binaries
4. stamps the same tag into every native binary
5. packages `aos`, `aosguard`, the skill, bridges, and manifests into each target bundle
6. renders checksums, Homebrew, and Scoop metadata
7. creates or reuses the Forgejo release
8. replaces every release asset from the clean `dist/` directory
9. updates the tap and bucket when their write tokens are present

Release assets group `aos-*`, `aos-bundle-*`, `aoscompose-*`, `aosward-*`,
`aosguard-*`, `agent-terminal-*`, and `aosterm-*` on every target.
`SHA256SUMS`, `aos.rb`, and `aos.json` cover the version-aligned set.

## Install

Homebrew on macOS or Linux:

```sh
brew tap coilyco-flight-deck/tap https://forgejo.coilysiren.me/coilyco-flight-deck/homebrew-tap.git
brew install coilyco-flight-deck/tap/aos
```

Scoop on Windows:

```sh
scoop bucket add coilyco https://forgejo.coilysiren.me/coilyco-flight-deck/scoop-bucket.git
scoop install coilyco/aos
```

Both package managers install `aos`, `aoscompose`, `aoscomposed`, `aosward`,
`aosguard`, `agent-terminal`, and `aosterm` on `PATH`. `aoscomposed` is a
package alias for the canonical `aoscompose` artifact, while `aosward` forces
warded launch mode from its executable name. The `aosguard` release binary
contains its generated operator CLI and the Forgejo Actions bridge. `aosterm`
is the Alacritty wrapper around `aoscompose`. `agent-terminal` is compatible.
See the [native launcher walkthrough](agent-terminal-native.md) for runtime
dependencies, upgrades, rollback, and version reporting.

Forgejo also serves every checksummed binary from the [agentic-os releases](https://forgejo.coilysiren.me/coilyco-flight-deck/agentic-os/releases).

The Homebrew and Scoop publication steps consume the repo-scoped
`TAP_WRITE_TOKEN` and `SCOOP_WRITE_TOKEN` Actions secrets. Their existing
SSM-backed token family is synchronized into the AOS repository by
`ward exec sync-actions-secrets`. An operator supplies the attended
`FORGEJO_ADMIN_TOKEN` in memory.

## Manual retry

Workflow dispatch accepts an existing or explicit `aos-v*` tag. With no tag,
the operator selects patch, minor, or major. Existing releases and same-named
assets are reused and replaced, making a retry idempotent.

## Local validation

`ward exec aos-release-build` creates the binaries and `SHA256SUMS`. With
`AOS_RELEASE_VERSION` set, `aos-release-package` renders local metadata and
`aos-release-check` verifies checksums, versions, `--help`, and an
`aosterm --dry-run` against a renderer-neutral overlay fixture. The
ordinary Go tests, repository test, and pre-commit verbs remain the release gate.
Forgejo evaluates release paths directly from the promoted diff.
