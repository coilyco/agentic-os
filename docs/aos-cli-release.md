---
doc_goal: Explain the standalone aos CLI release, artifact, and package-manager contract.
---
# aos CLI release

The portable CLI has an independent `aos-vMAJOR.MINOR.PATCH` clock inside the
agentic-os Forgejo repository. Root `v*` tags remain owned by hook pins and the
full dev-base image, so CLI delivery never waits on image promotion.

## Automatic release

Every push to canonical `main` queues
`.forgejo/workflows/aos-cli-release.yml` without cancellation. The job:

1. runs Python, Go, and pre-commit validation
2. bumps the CLI minor version without reading commit-message signals
3. cross-compiles every target in `aos/release-targets.txt`
4. stamps the tag into `aos version`
5. renders checksums, Homebrew, and Scoop metadata
6. creates or reuses the Forgejo release
7. replaces every release asset from the clean `dist/` directory
8. updates the tap and bucket when their write tokens are present

Release assets are `aos-darwin-arm64`, `aos-linux-amd64`,
`aos-linux-arm64`, `aos-windows-amd64.exe`, `SHA256SUMS`, `aos.rb`, and
`aos.json`.

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
`aos-release-check` verifies it without creating a tag. The ordinary
`aos-test`, repository test, and pre-commit verbs remain the release gate.

## See also

* [aos-cli.md](aos-cli.md) - launch and substrate contract.
* [release.md](release.md) - root hook and dev-base release train.
* [FEATURES.md](FEATURES.md) - shipped inventory.
