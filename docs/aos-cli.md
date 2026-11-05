# AOS launcher and its release train

AOS exposes one launch shape with composed context and guarded tools always present.

The shared role slug selects context across capabilities, never transfers authority, and standalone AOS applies [bounded access gates](aos-cluster-access.md).

## Launch modes

Every AOS launch has two contexts: agent-compose verifies and projects the selected role into a private staged home, and AOS attaches standalone `aosguard` with its umbra credential mounts. The compatibility flags `--composed` and `--guarded` are still accepted, explicit false values included, but disable neither.

`aos` and `aoscompose` name the standalone container `<role>-<suffix>`, and `aoscomposed` stays compatible. `aoscompose` uses Docker host networking. First positional selects role, a second harness overrides the default, and auth is default-on: use `--auth=false` only for startup checks such as `aoscompose platform --version`.

## Routing

Every launch is a standalone container. The Ward route (`--warded`, `--agent-id`, and the `aosward` name) was retired with Ward (`COI-2727`). A stale `aosward` link refuses to run, and `--warded` no longer parses. The `acompose` subcommand refuses `--guarded` and `--agent` before materialization, naming the root form instead (agentic-os#810).

## Standalone contract

* Default image pulls each launch, custom images stay local, and standalone uses the native shadow: worktrees at `/workspace`, mapped CWD as workdir. HOME is copied to `/home/aos` from an allowlist, and composition hydrates the baked provider through `aos-substrate-cache`.
* [Authentication](aos-auth.md) fails closed before Docker and projects file-backed or Keychain credentials read-only, auth env names crossing unrendered under `--auth=true`.
* [Connectivity](aos-connectivity.md) keeps host networking, MCP, and tailnet behavior, and [kubeconfig](aos-cluster-access.md) mounts one source read-only.
* Host HOME, AWS, Git, and Docker stay out, so credentials use auth projection only.

Root performs bootstrap only and the harness runs as the host uid and gid. Composition verifies the immutable bundle with `project --scope home`, and `--no-substrate` omits unrelated reference trees.
## Validation and release

`just aos-test` runs Go, and the `aos-standalone-composition-smoke` verb covers the standalone lifecycle. The standalone smoke uses `--auth=false` and a version command, so it proves startup rather than authenticated inference, and the [auth contract](aos-auth.md) names the inference probe.

## aos CLI release

The portable CLI has an independent `aos-vMAJOR.MINOR.PATCH` clock inside the agentic-os repository. Root `v*` tags remain owned by the full dev-base image and hooks use `aos-precommit-v*`, so CLI delivery never waits on either.

## Automatic release

The promoted `release` branch drives `.github/workflows/aos-cli-release.yml`, whose path filter covers the shipped Go roots, AOSguard specs and bridges, the Specgen pin, the launch profiles stamped into the binaries, and release scripts, and manual dispatch against `release` is the retry path. A test derives the builder's inputs from `scripts/aos-release-build.sh` and fails when the filter omits one. The release job stages the embedded web client and tests the binary against it, computes the next CLI minor version without reading commit messages, cross-compiles and tag-stamps every native binary, packages each target bundle, renders checksums plus Homebrew and Scoop metadata, then creates the tag, the GitHub release and every asset, and commits the formula and manifest to `coilyco/homebrew-tap` and `coilyco/scoop-bucket`. The tag is created after the build, so a failed build leaves none behind. See [release.md](release.md) for the token and gate.

Assets group `aos-*`, `aos-bundle-*`, `aoscompose-*`, `aosguard-*`, and `aterm-*` per target, with `SHA256SUMS`, `aos.rb`, and `aos.json` covering the version-aligned set. `aterm` reads its own [target list](../aterm/release-targets.txt), which has no Windows entry, so the Scoop manifest installs everything but it.

## Install

Homebrew on macOS or Linux taps `coilyco/tap` (github.com/coilyco/homebrew-tap), Scoop on Windows adds `https://github.com/coilyco/scoop-bucket`. Exact commands are in [the README](../README.md).

Both put `aos`, `aoscompose`, `aoscomposed`, `aosguard`, and `aterm` on `PATH`. `aoscomposed` aliases `aoscompose`, `aosguard` carries the operator CLI and Actions bridge, and `aterm` is the [branded session launcher](aterm.md).

Publication mints one coilyco-ops App token per run from the `COILYCO_OPS_APP_ID` and `COILYCO_OPS_APP_PRIVATE_KEY_BASE64` Actions secrets, scoped to this repo, the tap, and the bucket.

Workflow dispatch accepts an existing or explicit `aos-v*` tag, or the operator selects patch, minor, or major. An existing tag or release is reused and a same-named asset is replaced, so a retry is idempotent.

## Actions secrets sync

`just sync-actions-secrets` copies Actions secrets from SSM to GitHub, in-process and never through disk or argv. GitHub does not let a secret be read back, so a stale value shows up as a dead workflow step.

- It plans `FLEET_SECRETS` on every repo in `WORKFLOW_REPOS` plus `MAPPING`, then sets each through `gh secret set` with the value on stdin. `gh` reads `GH_TOKEN`, and the identity needs Actions secrets write on the repo. `--target github` is accepted and is the only target.
- `--repo OWNER/REPO` (repeatable) limits the plan to one repo, because a coilyco-ops App installation token covers one. `--secret NAME` (repeatable) limits it to those names.
- `--dry-run` prints the `owner/repo: NAME <- ssm /path` plan and a count. `--check` lists, per repo, whether every planned name is present. It reads names, never values.

The plan carries `SENTRY_CI_DSN` on every repo, since the GitHub org plan tier is unverified. It carries `COILYCO_OPS_APP_ID` and `COILYCO_OPS_APP_PRIVATE_KEY_BASE64` on `agentic-os`, `umbra` and `agent-compose`, the repos whose workflows push to the tap, the bucket or a release. A workflow decodes the key and mints a short-lived installation token, so no long-lived token is stored.

The Forgejo-era PATs (`CI_RELEASE_TOKEN`, `TAP_WRITE_TOKEN`, `SCOOP_WRITE_TOKEN`, `DEPLOY_PUSH_TOKEN`, `REGISTRY_READ_TOKEN`) were never copied to GitHub and left the mapping with the Forgejo target. Secrets with no GitHub source yet (`REGISTRY_*`, `ECO_MOD_PACKAGE_TOKEN`, `MODIO_ACCESS_TOKEN`, `TEST_PYPI_TOKEN`, the Sentry browser DSNs) arrive with the repo that reads them.

## Local validation

`just aos-release-build` creates the binaries and `SHA256SUMS`. With `AOS_RELEASE_VERSION` set, `aos-release-package` renders local metadata and `aos-release-check` verifies checksums, versions, `--help`, an `aterm --dry-run` against roster and overlay fixtures, and that `aterm` refuses an off-roster role. The ordinary Go tests and pre-commit verbs remain the release gate.

## Signed macOS aterm

**The macOS `aterm` ships as a signed `Treehouse.app`, so its privacy grants survive an upgrade (COI-50).** macOS keys a bare executable's grant by path, and the Homebrew path carries the version. A bundle is keyed by identifier (`dev.coilyco.treehouse`), and a certificate signature stores identifier plus certificate where ad-hoc pins the build. `scripts/aterm-sign-darwin.sh` signs on the Linux release job with the pinned `rcodesign` (`scripts/ci/install-rcodesign.sh`), using `TREEHOUSE_CODESIGN_CERT_PEM` and `_KEY_PEM` synced from SSM `/coilysiren/codesign/aterm-cert` and `aterm-key`, and fails closed without them. The formula installs `aterm-darwin-arm64.app.tar.gz` and links `bin/aterm` into it. The release check reads the archive back, since `rcodesign` exits 0 for a bundle whose executable it never found. Rotating the certificate expires every grant. After two releases `codesign -d -r-` shows the same requirement with different `CDHash` values.
