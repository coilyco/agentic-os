# Dev container and hosted CI

The custom [dev-base image](dev-base-image.md) is being retired for two standard
environments. Kai decided this on 2026-10-25 (COI-2953): dev-base was viable only
while one machine built the image and ran the jobs that used it. Builds now move
to GitHub-hosted runners, so a multi-gigabyte image pulled by every job is the
cost to remove. Both picks below came from Jev, and the reasoning is the author's.

## CI: GitHub-hosted runner, no container image

Jobs run on plain `ubuntu-24.04`. Language versions come from each repo's own
version files through the standard setup actions, and
[`actions/setup-ci`](../actions/setup-ci/action.yml) installs what the runner
image lacks. The live example is [`.github/workflows/ci.yml`](../.github/workflows/ci.yml).

- **Runner already ships** - Go, Node, Python, Rust with rustup, .NET, Docker,
  kubectl, Helm 3, AWS CLI, GitHub CLI, git-lfs, yq and jq.
- **setup-ci adds** - `just`, `uv`, `pre-commit`, `trufflehog`, `golangci-lint`,
  `kdlfmt`, `yq` and `umbra`, each pinned with a SHA-256 value. Pick a subset with
  the `tools` input. The default is `just uv pre-commit`.
- **agent-compose** - the aterm roster guard reads it. `scripts/ci/install-agent-compose.sh`
  installs a pinned release and its roster from GitHub, checked against SHA-256 values.
- **Per-language setup stays in the workflow** - `actions/setup-go` with
  `go-version-file`, `actions/setup-node` with `node-version-file`, and so on.
  The repo owns its versions, not a fleet image.
- **Cache both** - `~/.cache/uv`, and `~/.cache/pre-commit` keyed on
  `.pre-commit-config.yaml`. dev-base pre-warmed hook environments. A hosted
  runner has no such cache, so the first run builds the Go and Rust hooks.

## Agent and warded containers: the devcontainer spec

[`.devcontainer/devcontainer.json`](../.devcontainer/devcontainer.json) builds on
`mcr.microsoft.com/devcontainers/base:ubuntu-24.04` (272 MB compressed, amd64
and arm64) and layers features on it.

- **Official features** - go, node, dotnet, rust, aws-cli, github-cli, git-lfs,
  kubectl-helm-minikube and docker-outside-of-docker.
- **`agentic-os-tools`** - the same installer CI uses, as a feature.
- **`agentic-os-media`** - ffmpeg and ImageMagick.
- **`agentic-os-agent`** - `aos`, `aosguard`, agent-compose, the claude, codex,
  goose, opencode and mcporter harnesses, the Claude managed settings, and the
  guarded `git`, `gh`, `rm`, `find` and `mv` shims. The shims build in place from
  sources the `aos` release bundle carries ([`stage-shim-src.sh`](../scripts/stage-shim-src.sh)).
  The feature's build fails if a shim is not first on PATH or a guarded
  `git commit --no-verify` is not refused.

The user is root because `aos` replaces the entrypoint, bootstraps as root, then
drops to the uid it was given. Build where it will run, then point `aos` at the
tag, which skips its pull for any image but the default:

```
devcontainer build --workspace-folder . --image-name aos-devcontainer:local
aos --role <role> --image aos-devcontainer:local acompose -- <command>
```

## What this gives up

- **Parity by construction** - dev-base made CI and agents share an image, so
  drift failed on a pull request (agentic-os#328). Now CI runs on the runner image
  and agents in the dev container. The replacement guard builds the dev container
  and runs the gate verb in it, on `.devcontainer/` changes and nightly.
- **Baked Rust toolchains** - dev-base installed pinned channels so runners never
  fetched them (galaxy-gen#84). The dev container installs one Rust version.
- **Hook environment cache** - an agent's first `pre-commit run` builds hooks.
- **Helm** - dev-base pinned Helm 4, the hosted runner has Helm 3.

## Pins

Every download in the tools installer carries a SHA-256 value taken from the
upstream release's own checksums. A bump is one edit to the table in
[`install.sh`](../.devcontainer/features/agentic-os-tools/install.sh). Check both
architectures without installing: `VERIFY_ONLY=1 ARCH=amd64 bash install.sh`, then
`ARCH=arm64`. `PLAN=1` prints the table.

The agent feature downloads the `aos` bundle and agent-compose from Forgejo for
now. GitHub has no agent-compose release past v1.32.0 and its `aos` repo is
private. Both URLs are feature options, and they move with the release trains.

## Not ported yet

- Composed `aos` launches, which read the baked mirror in `/opt/substrate-seed`.
- A tailscale client, pre-warmed hook environments, and the WASM tools.
- Publishing the features to a registry, so other repos can reference them.
- Deleting `docker/dev-base/`, its release train and the doctrine that names it.

## Porting a workflow

[`workflow_contract.json`](../agentic_os/workflow_contract.json) owns the runner
labels. Kai decided on 2026-10-25 that no job uses a self-hosted runner
(COI-2967), so every Forgejo label maps to a hosted one. `rollout`, `website` and
`deploy` map too but are redesigns, not ports, and print a `TODO`.

- **Script** - `port-forgejo-workflow .forgejo/workflows/ci.yml` writes
  `.github/workflows/ci.yml`. It maps `runs-on`, deletes `container:`, and adds the
  pinned `actions/setup-ci` as the first step. Comments stay. It never touches
  step bodies or secrets, and refuses to overwrite without `--force`.
- **By hand after** - `actions/setup-go` with `go-version-file`, a `~/.cache/pre-commit`
  cache, and `workflow_dispatch`. Leave out a `just pre-commit` step until the repo's
  hook pin moves to GitHub (COI-2943), since a hosted runner cannot reach Forgejo.
- **Hook** - `github-workflow-contract` fails a tracked `.github/workflows` file with
  a label outside the contract, a runner group, a `runs-on` expression, or a scalar
  naming `forgejo.coilysiren.me`. A comment may name it. It never reads
  `.forgejo/workflows`, so a repo keeps both until the flip.
- **Not ported** - the composite actions that call the Forgejo API (`tag-bump`,
  `create-release`, `upload-release-asset`, `bump-formula`).

## Until the flip: CI in dev-base

Forgejo CI still runs each job in the moving `:release` dev-base image through the
app's own `just` verbs, so a green run means a headless agent can land the change.

- **Convention** - `container: forgejo.coilysiren.me/coilyco/agentic-os:release`,
  never `:latest`. [`ci-in-dev-base-example.yml`](ci-in-dev-base-example.yml) is
  the copy-paste start, and `.forgejo/workflows/ci.yml` is the live gate.
- **Required status** - workflow `ci`, job `gate`, so branch protection can
  require `ci / gate`. `dev-base-pr / build` runs only for changes under `docker/`.
- **Authoring and rollout** - aos authors the convention and publishes the alias.
  An infrastructure rollout templates the `:release` ref into each app's workflow.
