# dev-base container image

aos owns the agent dev env. ward consumes it by tag, so config cannot
drift.

This page covers the language specialists and the `dev-base-full` compatibility
surface. See [tiering](dev-base-image-tiering.md).

## What ships

[`docker/dev-base/core/Dockerfile`](../docker/dev-base/core/Dockerfile) layers
the root runtime tier on `ubuntu:24.04`. The sibling tier Dockerfiles live in
[`docker/dev-base/`](../docker/dev-base/) as one folder per tier.

* **core** - common development tools, Node-backed agent harnesses, operational CLIs, platform assets, and ward.
* **language specialists** - Node, Go, .NET 10 + ICU, Rust + wasm + `trunk`, and Python + pip + `pipenv`.
* **shared CLIs** - aws, Homebrew, claude, mcporter, opencode, codex, goose, gh, helm, kubectl, yq, Docker, `tailscale`, and `tailscaled`. Core supplies them to every parallel `lang-*` image.
* **gate tools** - golangci-lint, trufflehog, and kdlfmt.
* **native libs (full only)** - alsa/udev/wayland/xkbcommon + pkg-config.
* **platform seed** - substrate mirrors, self-name/status assets, and the shell entrypoint.

Tools under `/usr/local`, `/home/linuxbrew/.linuxbrew`, or `/opt` run as any uid. ward owns `run-as-uid`, mounts, and `~/.aws`. Root bootstrap seeds `/home/ubuntu/.ward/audit` as uid 1000 and avoids root-owned audit state.

## Naming and tags

Published as one package,
`forgejo.coilysiren.me/coilyco-flight-deck/agentic-os`, with variants as tags.
`full` is the plain default tag, and the folder name prefixes every other tier's
tag. `full` uses `agentic-os:${TAG}`. Other refs use
`agentic-os:<tier>-${TAG}` for `core`, `lang-node`, `lang-go`, `lang-dotnet`,
`lang-rust`, and `lang-python`. The family publishes no `ops` or `agent` tag.
`dev-base-publish.yml` first
publishes draft tags (`agentic-os:draft-${sha}`, `agentic-os:core-draft-${sha}`,
and so on) on the promoted SHA. Its manual dispatch path can resume one tier
closure at a time. `release.yml` retags that family to `vX.Y.Z`, `:release`,
and `:latest` after each draft tag appears. The `buildcache` tags hold the
cache.

## How it publishes

The publish jobs in [`.forgejo/workflows/dev-base-publish.yml`](../.forgejo/workflows/dev-base-publish.yml)
run after `release` already moved. They build one draft per tier with
[`scripts/dev-base-build.py`](../scripts/dev-base-build.py); `needs:` carries
the tier DAG (see the [tiering doc](dev-base-image-tiering.md)). The manual
retry workflow in [`.forgejo/workflows/release.yml`](../.forgejo/workflows/release.yml)
re-runs the same retag path. The core image
sets its commit-pinned `WARD_CONFIG_REF` in the final validation layer. That
lets `ward doctor` validate the bundled `.ward/` checkout without invalidating
the cached toolchain layers on every commit.

See [publish resume](dev-base-publish-resume.md).

## Pinning a tool

Versions pin as `ARG`s: hand-edit and push, else **auto-bump** refreshes stale
pins ([auto-bump doc](dev-base-auto-bump.md)).
`GOLANGCI_LINT_VERSION`, `KDLFMT_VERSION`, `TRUNK_VERSION`, and `WARD_VERSION` opt out. Ward stays
manual while raw releases stage: aos advances prod/N-1 after real-bundle
validation.

## Pulling it

```bash
docker pull forgejo.coilysiren.me/coilyco-flight-deck/agentic-os:release
```

Needs a `docker login`. `ward container up/exec` (ward#98) is the entry point.

## Not here

* Mount and compose logic, `ward container` verbs, and the mount-eligibility manifest - ward#98 and aos#222.
* `coily` (retired, folded into `ward ops`) and running services - not shipped.
* Standalone `ops` and `agent` images - capabilities live in each language specialist instead.
* `docker buildx` and `wasm-pack` - job-local.
* Tailnet startup, auth, and socket wiring - ward owns bring-up.

## See also

* [release.md](release.md) - the pipeline this rides on. [FEATURES.md](FEATURES.md).
* [Language-specialist dev-base images](dev-base-image-tiering.md) - the inheritance and compatibility fan-in model.
