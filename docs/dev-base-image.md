# dev-base container image

aos owns the agent dev env. ward consumes it by tag, so config cannot
drift.

This page describes the `dev-base-full` contract. The tier layout is in
[Tiered dev-base image split](dev-base-image-tiering.md).

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
`agentic-os:lang-node-${TAG}`, and so on. `promote.yml` first publishes
commit-scoped draft tags (`agentic-os:draft-${sha}`,
`agentic-os:core-draft-${sha}`, and so on). `release.yml` then retags that
same image family to one `vX.Y.Z`, the moving `:release` branch alias, and
the `:latest` compatibility alias. The `buildcache` tags hold the cache.
The old `agentic-os-<tier>` package names are retired.

## How it publishes

The publish jobs in [`.forgejo/workflows/promote.yml`](../.forgejo/workflows/promote.yml)
run after the repo gate and before `release` moves. They build one draft image
per tier with [`scripts/dev-base-build.py`](../scripts/dev-base-build.py) and
`--tier`, with `needs:` carrying the tier DAG (see the
[tiering doc](dev-base-image-tiering.md)). The release jobs in
[`.forgejo/workflows/release.yml`](../.forgejo/workflows/release.yml) compute
the public tag, retag the draft family to `vX.Y.Z`, `:release`, and `:latest`,
verify those manifests, then cut the public git/release tag. The core image
stamps `WARD_CONFIG_REF` from the current agentic-os commit at build time, so
ward launches against the bundled `.ward/` checkout rather than moving `main`.
The core build runs `ward doctor` after installing ward, rejecting a broken
bundle before the draft image publishes.

## Pinning a tool

Versions pin as `ARG`s: hand-edit and push to main to pin or roll back, else a
scheduled **auto-bump** refreshes stale pins ([auto-bump doc](dev-base-auto-bump.md)).
`GOLANGCI_LINT_VERSION` and `KDLFMT_VERSION` opt out, bumped by hand.

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
