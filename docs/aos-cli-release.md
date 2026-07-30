---
doc_goal: Explain the standalone aos CLI release, artifact, and package-manager contract.
---
# aos CLI release

The portable CLI has an independent `aos-vMAJOR.MINOR.PATCH` clock inside the
agentic-os Forgejo repository. Root `v*` tags remain owned by the full
dev-base image, while hooks use `aos-precommit-v*`, so CLI delivery never waits
on either train.

## Automatic release

Every `main` push runs the release-impact classifier.
`.forgejo/workflows/aos-cli-release.yml` queues a release only for shipped Go
sources and manifests, AOSguard specs and bridges, the Specgen pin, or release
build/package scripts. Docs, tests, Alacritty, and workflow-only changes skip
it. Manual dispatch overrides the decision. The release job:

1. installs the validated workflow Ward, then runs Python, Go, and pre-commit validation
2. bumps the CLI minor version without reading commit-message signals
3. cross-compiles matching `aos`, `aosguard`, and `agent-terminal` binaries for
   every target in `aos/release-targets.txt`
4. stamps the same tag into all three binaries
5. renders checksums, Homebrew, and Scoop metadata
6. creates or reuses the Forgejo release
7. replaces every release asset from the clean `dist/` directory
8. updates the tap and bucket when their write tokens are present

Release assets group `aos-*`, `aosguard-*`, and `agent-terminal-*` on Darwin
arm64, Linux amd64 and arm64, and Windows amd64. `SHA256SUMS`, `aos.rb`, and
`aos.json` cover the whole version-aligned set.

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

Both package managers install `aos`, `aosguard`, and `agent-terminal` on
`PATH`. The `aosguard` release binary contains its generated operator CLI and
the Forgejo Actions bridge. The `agent-terminal` binary contains the Alacritty
renderer only. Agent-compose remains the provider of
`agent-compose.overlay.v1` and must be installed separately on native director
hosts. See the [native launcher walkthrough](agent-terminal-native.md) for its
dependencies, upgrades, rollback, and version reporting.

Forgejo also serves every checksummed binary directly from the
[agentic-os releases](https://forgejo.coilysiren.me/coilyco-flight-deck/agentic-os/releases).

The Homebrew and Scoop publication steps consume the repo-scoped
`TAP_WRITE_TOKEN` and `SCOOP_WRITE_TOKEN` Actions secrets. Their existing
SSM-backed token family is synchronized into the AOS repository by
`ward exec sync-actions-secrets`. An operator supplies the attended
`FORGEJO_ADMIN_TOKEN` in memory when the legacy SSM admin token is unavailable.

## Manual retry

Workflow dispatch accepts an existing or explicit `aos-v*` tag. With no tag,
the operator selects patch, minor, or major. Existing releases and same-named
assets are reused and replaced, making a retry idempotent.

## Local validation

`ward exec aos-release-build` creates the binaries and `SHA256SUMS`. With
`AOS_RELEASE_VERSION` set, `aos-release-package` renders local metadata and
`aos-release-check` verifies checksums, versions, `--help`, and an
`agent-terminal --dry-run` against a renderer-neutral overlay fixture. The
ordinary Go tests, repository test, and pre-commit verbs remain the release
gate. `ward exec release-impact -- --surface aos-cli --base REF --head REF`
prints the decision and matching paths without publishing.
