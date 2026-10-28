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

The portable CLI has an independent `aos-vMAJOR.MINOR.PATCH` clock inside the agentic-os Forgejo repository. Root `v*` tags remain owned by the full dev-base image and hooks use `aos-precommit-v*`, so CLI delivery never waits on either.

## Automatic release

The promoted `release` branch drives `.forgejo/workflows/aos-cli-release.yml`, whose path filter covers the shipped Go roots, AOSguard specs and bridges, the Specgen pin, the launch profiles stamped into the binaries, and release scripts, and manual dispatch is the retry path. A test derives the builder's inputs from `scripts/aos-release-build.sh` and fails when the filter omits one. The release job validates through Ward, bumps the CLI minor version without reading commit messages, cross-compiles and tag-stamps every native binary, packages each target bundle, renders checksums plus Homebrew and Scoop metadata, creates or reuses the Forgejo release, replaces every asset from a clean `dist/`, then waits for the GitHub copy of those assets (the formula and manifest name GitHub URLs, and the ser8 mirror copies them across) and updates the tap and bucket when their write tokens exist.

Assets group `aos-*`, `aos-bundle-*`, `aoscompose-*`, `aosguard-*`, and `aterm-*` per target, with `SHA256SUMS`, `aos.rb`, and `aos.json` covering the version-aligned set. `aterm` reads its own [target list](../aterm/release-targets.txt), which has no Windows entry, so the Scoop manifest installs everything but it.

## Install

Homebrew on macOS or Linux taps `coilyco/tap` (github.com/coilyco/homebrew-tap), Scoop on Windows adds `https://github.com/coilyco/scoop-bucket`, since Forgejo is tailnet-only. Exact commands are in [the README](../README.md).

Both put `aos`, `aoscompose`, `aoscomposed`, `aosguard`, and `aterm` on `PATH`. `aoscomposed` aliases `aoscompose`, `aosguard` carries the operator CLI and Actions bridge, and `aterm` is the [branded session launcher](aterm.md).

Publication consumes the repo-scoped `TAP_WRITE_TOKEN` and `SCOOP_WRITE_TOKEN` Actions secrets, synchronized from SSM by `just sync-actions-secrets`, and an operator supplies the attended `FORGEJO_ADMIN_TOKEN` in memory.

Workflow dispatch accepts an existing or explicit `aos-v*` tag, or the operator selects patch, minor, or major. Existing releases and same-named assets are reused and replaced, so a retry is idempotent.

## Actions secrets sync

`just sync-actions-secrets` copies Actions secrets from SSM to a forge, in-process and never through disk or argv. Neither forge lets a secret be read back, so a stale value shows up as a dead workflow step.

- `--target forgejo` - the default until the COI-2927 cutover. Writes the `MAPPING` table with the attended `FORGEJO_ADMIN_TOKEN`.
- `--target github` - plans `FLEET_SECRETS` on every repo in `WORKFLOW_REPOS` plus `GITHUB_MAPPING`, then sets each through `gh secret set` with the value on stdin. `gh` reads `GH_TOKEN`, and the identity needs Actions secrets write on the repo. `--repo OWNER/REPO` (repeatable) limits the plan to one repo, because a coilyco-ops App installation token covers one.
- `--dry-run` prints the `owner/repo: NAME <- ssm /path` plan and a count. `--check` (GitHub only) lists, per repo, whether every planned name is present. It reads names, never values.

The GitHub plan carries `SENTRY_CI_DSN` on every repo, since the GitHub org plan tier is unverified. It carries `COILYCO_OPS_APP_ID` and `COILYCO_OPS_APP_PRIVATE_KEY_BASE64` on `agentic-os`, `umbra` and `agent-compose`, the repos whose workflows push to the tap, the bucket or a release. A workflow decodes the key and mints a short-lived installation token, so no long-lived token is stored. The Forgejo PATs in `MAPPING` are never copied to GitHub.

`WORKFLOW_REPOS` is the set of Forgejo repos that carry `.forgejo/workflows`. Remove a repo when its Forgejo copy is archived. Secrets with no GitHub source yet (`REGISTRY_*`, `ECO_MOD_PACKAGE_TOKEN`, `MODIO_ACCESS_TOKEN`, `TEST_PYPI_TOKEN`, the Sentry browser DSNs) arrive with the flip of the repo that reads them.

## Local validation

`just aos-release-build` creates the binaries and `SHA256SUMS`. With `AOS_RELEASE_VERSION` set, `aos-release-package` renders local metadata and `aos-release-check` verifies checksums, versions, `--help`, an `aterm --dry-run` against roster and overlay fixtures, and that `aterm` refuses an off-roster role. The ordinary Go tests and pre-commit verbs remain the release gate.
