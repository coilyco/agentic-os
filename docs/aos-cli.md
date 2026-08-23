# AOS launcher and its release train

AOS exposes one launch shape with composed context and guarded tools always present.

The shared role slug selects context across capabilities, never transfers
authority, and standalone AOS applies
[bounded access gates](aos-cluster-access.md).

## Launch modes

Every AOS launch has two contexts: agent-compose verifies and projects the
selected role into a private staged home, and AOS attaches standalone `aosguard`
with its specgen credential mounts. The compatibility flags `--composed` and
`--guarded` are still accepted, explicit false values included, but disable
neither.

`aos` and `aoscompose` name the standalone container `<role>-<suffix>`, and
`aoscomposed` stays compatible. `aoscompose` uses Docker host networking. First
positional selects role, a second harness overrides the default, and auth is
default-on: use `--auth=false` only for startup checks such as `aoscompose
platform --version`.

`aosward` is the same executable with warded mode forced, equal to `aos
--warded` and not disablable by `--warded=false`. It takes the ordinary flags
plus a trailing issue reference, and uses Ward's generic runner and broker for
Compose, lifecycle, and [credential handoff](aos-cluster-access.md).

## Routing

In warded mode, arguments after `--` go to Ward with the image, agent, role,
workspace request, and context bundle. Harness model and effort settings never
change composition. AOS gives Agent Compose the role, delivery mode, and the
role's first roster-supported compatibility tier, identical across seats and
unrelated to model or context-window size. Agent Compose owns identity and seat
context, and Ward cannot change privileged surface. See the
[context-bundle adapter](aos-context-bundle.md).

Ward ships the `director`, `qa`, and `engineer` repository workflows. Other safe
roles use its [generic read-only command](aos-roles-and-voice.md). AOS rejects
incompatible agents and translated Ward flags before starting a container.

**The root action routes to Ward, the `acompose` subcommand does not.** It once
took `--warded` and exited zero, so a standalone container could read as
Ward-brokered. It now refuses that flag, `--guarded`, and `--agent` before
materialization, naming the root form instead (agentic-os#810).

## Standalone contract

* Default image pulls each launch, custom images stay local, and standalone uses
  the native shadow: worktrees at `/workspace`, mapped CWD as workdir. HOME is
  copied to `/home/aos` from an allowlist, and composition hydrates the baked
  provider through `aos-substrate-cache`.
* [Authentication](aos-auth.md) fails closed before Docker and projects
  file-backed or Keychain credentials read-only, auth env names crossing
  unrendered under `--auth=true`.
* [Connectivity](aos-context-bundle.md) keeps host networking, MCP, and tailnet
  behavior, and [kubeconfig](aos-cluster-access.md) mounts one source read-only.
* Host HOME, AWS, Git, and Docker stay out, so credentials use auth projection
  only.

Root performs bootstrap only and the harness runs as the host uid and gid.
Composition verifies the immutable bundle with `project --scope home`, and
`--no-substrate` omits unrelated reference trees.
## Validation and release

`just aos-test` runs Go, and the `aos-composition-dry-run`,
`aos-composition-smoke`, and `aos-standalone-composition-smoke` verbs cover both
lifecycle shapes. The standalone smoke uses `--auth=false` and a version command,
so it proves startup rather than authenticated inference, and the
[auth contract](aos-auth.md) names the inference probe.

## aos CLI release

The portable CLI has an independent `aos-vMAJOR.MINOR.PATCH` clock inside the
agentic-os Forgejo repository. Root `v*` tags remain owned by the full dev-base
image and hooks use `aos-precommit-v*`, so CLI delivery never waits on either.

## Automatic release

The promoted `release` branch drives `.forgejo/workflows/aos-cli-release.yml`,
whose path filter covers the shipped Go roots, AOSguard specs and bridges, the
Specgen pin, and release scripts, and manual dispatch is the retry path. The
release job validates through Ward, bumps the CLI minor version without reading
commit messages, cross-compiles and tag-stamps every native binary, packages each
target bundle, renders checksums plus Homebrew and Scoop metadata, creates or
reuses the Forgejo release, replaces every asset from a clean `dist/`, and
updates the tap and bucket when their write tokens exist.

Assets group `aos-*`, `aos-bundle-*`, `aoscompose-*`, `aosward-*`, `aosguard-*`,
`agent-terminal-*`, and `aosterm-*` per target, with `SHA256SUMS`, `aos.rb`, and
`aos.json` covering the version-aligned set.

## Install

Homebrew on macOS or Linux taps `coilyco-flight-deck/tap`, Scoop on Windows adds
the `coilyco` bucket, and Forgejo also serves every checksummed binary from
[releases](https://forgejo.coilysiren.me/coilyco-flight-deck/agentic-os/releases).
Exact commands are in [the README](../README.md).

Both put `aos`, `aoscompose`, `aoscomposed`, `aosward`, `aosguard`,
`agent-terminal`, and `aosterm` on `PATH`. `aoscomposed` aliases `aoscompose`,
`aosward` forces warded mode from its executable name, `aosguard` carries the
operator CLI and Actions bridge, and `aosterm` is the Alacritty wrapper. Runtime
dependencies: [native launcher walkthrough](agent-terminal-native.md).

Publication consumes the repo-scoped `TAP_WRITE_TOKEN` and `SCOOP_WRITE_TOKEN`
Actions secrets, synchronized from SSM by `just sync-actions-secrets`, and an
operator supplies the attended `FORGEJO_ADMIN_TOKEN` in memory.

Workflow dispatch accepts an existing or explicit `aos-v*` tag, or the operator
selects patch, minor, or major. Existing releases and same-named assets are
reused and replaced, so a retry is idempotent.

## Local validation

`just aos-release-build` creates the binaries and `SHA256SUMS`. With
`AOS_RELEASE_VERSION` set, `aos-release-package` renders local metadata and
`aos-release-check` verifies checksums, versions, `--help`, and an `aosterm
--dry-run` against an overlay fixture. The ordinary Go tests and pre-commit
verbs remain the release gate.
