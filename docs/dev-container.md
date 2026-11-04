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
  installs a pinned release and its roster from the directory the public fetch-release action
  filled from the private treehouse-servers release, checked against SHA-256 values.
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
  guarded `git`, `rm`, `find` and `mv` shims. The shims build in place from
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

The agent feature's `aos` bundle and agent-compose download URLs still point at
Forgejo, which was deleted on 2026-11-01 (COI-2931). Both URLs are feature
options, and they move with the release trains.

## Not ported yet

- Composed `aos` launches, which read the baked mirror in `/opt/substrate-seed`.
- A tailscale client, pre-warmed hook environments, and the WASM tools.
- Publishing the features to a registry, so other repos can reference them.
- Deleting `docker/dev-base/`, its release train and the doctrine that names it.

## Porting a workflow

[`workflow_contract.json`](../agentic_os/workflow_contract.json) owns the runner
labels. No job uses a self-hosted runner (COI-2967, COI-3042), so the hook rejects
`kai-server` and every other label outside the hosted list.

- **By hand** - put `actions/setup-ci` first, then `actions/setup-go` with
  `go-version-file` where the repo builds Go, a `~/.cache/pre-commit` cache, and
  `workflow_dispatch`.
- **Hook** - `github-workflow-contract` fails a tracked `.github/workflows` file with
  a label outside the contract, a runner group, a `runs-on` expression, or a scalar
  naming `forgejo.coilysiren.me`. A comment may name it.
