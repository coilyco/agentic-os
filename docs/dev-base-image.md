# dev-base container image

aos owns the agent dev env. ward consumes it by tag, so config cannot
drift.

This page covers `dev-base-full`. See [tiering](dev-base-image-tiering.md).

## What ships

[`docker/dev-base/core/Dockerfile`](../docker/dev-base/core/Dockerfile) layers
the root runtime tier on `ubuntu:24.04`. The sibling tier Dockerfiles live in
[`docker/dev-base/`](../docker/dev-base/) as one folder per tier.

- **core toolchain** - `uv`, pre-commit, Python, shellcheck, git, git-lfs, build-essential, Rust, and ward.
- **language/runtime tiers** - Node, Go, and .NET 10 + ICU.
- **ops / agent CLIs** - aws cli, Homebrew, claude, mcporter, opencode, codex, goose, gh, helm, kubectl, yq, Docker CLI, and the Tailscale client plus `tailscaled` daemon binary.
- **gate tools** - golangci-lint, trufflehog, and kdlfmt.
- **platform seed** - the substrate mirrors, the baked agent self-name / status-line assets, and the container shell entrypoint that seeds `AOS_REPO_ROOT` plus `WARD_CONFIG_REF` before the read-only director shell starts.

Tools under `/usr/local`, `/home/linuxbrew/.linuxbrew`, or `/opt` run as any uid. ward owns `run-as-uid`, mounts, and `~/.aws`. Root bootstrap seeds `/home/ubuntu/.ward/audit` as uid 1000 and avoids root-owned audit state.

## Naming and tags

Published as one package,
`forgejo.coilysiren.me/coilyco-flight-deck/agentic-os`, with variants as tags.
`full` is the plain default tag, and the folder name prefixes every other tier's
tag, so the published refs are `agentic-os:${TAG}`, `agentic-os:core-${TAG}`,
`agentic-os:lang-node-${TAG}`, and so on. `dev-base-publish.yml` first
publishes draft tags (`agentic-os:draft-${sha}`, `agentic-os:core-draft-${sha}`,
and so on) on the promoted SHA. Its manual dispatch path can resume one tier
closure at a time. `release.yml` retags that family to `vX.Y.Z`, `:release`,
and `:latest` after each draft tag appears. The `buildcache` tags hold the
cache.
The old `agentic-os-<tier>` package names are retired.

## How it publishes

The publish jobs in [`.forgejo/workflows/dev-base-publish.yml`](../.forgejo/workflows/dev-base-publish.yml)
run after `release` already moved. They build one draft per tier with
[`scripts/dev-base-build.py`](../scripts/dev-base-build.py); `needs:` carries
the tier DAG (see the [tiering doc](dev-base-image-tiering.md)). The manual
retry workflow in [`.forgejo/workflows/release.yml`](../.forgejo/workflows/release.yml)
retags the already-published draft family to `vX.Y.Z`, `:release`, and
`:latest`, waiting for each draft source tag before retagging. The core image
stamps `WARD_CONFIG_REF` from the current commit, so ward launches against the
bundled `.ward/` checkout. The core build runs `ward doctor` first, rejecting
a broken bundle before the draft image publishes.

See [publish resume](dev-base-publish-resume.md).

## Pinning a tool

Versions pin as `ARG`s: hand-edit and push, else **auto-bump** refreshes stale
pins ([auto-bump doc](dev-base-auto-bump.md)).
`GOLANGCI_LINT_VERSION`, `KDLFMT_VERSION`, and `WARD_VERSION` opt out. Ward stays
manual while raw releases stage: aos advances prod/N-1 after real-bundle
validation.

## Pulling it

```bash
docker pull forgejo.coilysiren.me/coilyco-flight-deck/agentic-os:release
```

Needs a `docker login`. `ward container up/exec` (ward#98) is the entry point.

## Not here

- Mount / compose logic and `ward container` verbs - ward#98. The mount-eligibility manifest - aos#222.
- `coily` (retired, folded into `ward ops`) and running services - not shipped.
- `docker buildx` and `wasm-pack` - job-local publish or toolchain steps.
- Tailnet daemon startup, auth, and socket wiring - ward owns bring-up, even though the image now ships both Tailscale binaries.

## See also

- [release.md](release.md) - the pipeline this rides on. [FEATURES.md](FEATURES.md).
- [Tiered dev-base image split](dev-base-image-tiering.md) - the implemented fan-out
  and fan-in model that preserves `dev-base-full` as the default.
