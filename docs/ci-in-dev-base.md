# CI parity in dev-base

Every app's CI runs **inside the moving `:release` [dev-base image](dev-base-image.md),
through the same `just` verbs the agents run**, so a green CI means "a
headless agent can actually land this," not "it passed in some other
environment" (agentic-os#328).

The current contract follows the one promoted full dev-base image. The same
image contains every supported language toolchain.

## The motivating failure

On an app, CI was green while **every** dispatched agent run died. The two
environments had diverged: CI ran a bare runner + `setup-uv` (which fetches its
own writable Python, green), while the agent ran in dev-base where a root-owned
`UV_PYTHON_INSTALL_DIR` blocked `uv run` (red, invisibly). CI was validating a
world no agent lives in. Pinning CI to the same image the agents use makes those
environment regressions fail **loudly, on a PR**, before any container dispatches.

## The convention

Each app CI job runs in the dev-base container and invokes the app's own gate
verbs instead of hand-rolled `uv run` steps:

- `container: forgejo.coilysiren.me/coilyco/agentic-os:release`
- `just test` / `just lint` / `just smoke` - the same verbs a
  dispatched agent runs. Each app defines these in its own `justfile`.

[`ci-in-dev-base-example.yml`](ci-in-dev-base-example.yml) is a copy-paste
starting point for an app's `.forgejo/workflows/*.yml`.

This repo's live companion is [`.forgejo/workflows/ci.yml`](../.forgejo/workflows/ci.yml).
It keeps the workflow name `ci` and the job name `gate`, so Forgejo branch
protection can require the `ci / gate` status context on `pull-request-and-merge`
repos. The live gate resolves the generated Ward tag attached to Ward's
promoted `release` branch, installs that immutable ref, then runs `pytest` plus
`pre-commit run --all-files`. If tag publication temporarily lags promotion,
the resolver falls back to the literal `release` ref.

This repo also exposes `dev-base-pr / build` only for pull requests with a
change under `docker/`. It builds the complete one-architecture Bake graph.
The job has no registry
credential and never pushes. See
[pull-request dev-base validation](ci-in-dev-base.md).

## Promoted moving alias

CI uses the moving `:release` alias, never the compatibility-only `:latest`
alias. Each run consumes the newest successfully promoted full image, matching
the default used by the AOS launcher.

## The release source of truth

The release pipeline promotes the successfully published full image to
`agentic-os:release`, so consumers share one registry-owned source of truth.
[dev-base-image.md](dev-base-image.md) covers how that alias publishes.

## Authoring vs rollout

aos **authors** the convention and publishes the alias. It does not reach into
other repos. An infrastructure rollout templates the literal `:release` ref
into each app's workflow, per the authoring-vs-rollout law. The registry resolves
that alias when the app CI starts.

## Rollout unit

Each app's `.forgejo/workflows/*.yml` is the rollout unit, with a single app
as first mover on its own adoption issue. The blocking dependency was the
dev-base image fix (agentic-os#327): with it fixed, parity holds green.

## Pull-request dev-base build validation

Forgejo starts `dev-base-pr / build` only when a pull request changes a path
under `docker/`. The workflow uses Forgejo's native `paths` filter, so no
repository script walks the diff or re-implements path matching. Pull requests
without Docker changes do not enqueue the heavy job.

## Build-only boundary

[`actions/dev-base-build`](../actions/dev-base-build/action.yml) has no registry
token input. It installs Docker and Buildx, then executes the declarative
[`docker-bake.hcl`](../docker/dev-base/docker-bake.hcl) for one platform. The
Bake graph links every language payload into the full image. Payload results stay
in BuildKit's cache-only exporter, and only the full image is loaded for the
shared smoke check.

The action does not log in, pass `--push`, or install QEMU. It removes local
tags and prunes the persistent `aos-pr-builder` cache to its configured maximum
after every run.

## Publication parity

PR validation and publication share the Dockerfiles, named contexts, Docker
bootstrap, builder setup, and
[`verify-full.sh`](../actions/publish-dev-base/scripts/verify-full.sh).
Publication keeps the extra registry login, per-artifact registry cache, and
multi-architecture push. Forgejo owns its dependency order directly: the
language matrix completes before the full job through `needs`.

Language targets validate their isolated toolchains. The full image runs
[`verify-common.sh`](../docker/dev-base/verify-common.sh) once after grafting
all payloads and installing the shared surface. That gate runs the
complete-toolchain smoke.

## Required status

`ci / gate` remains the universal required status. `dev-base-pr / build` is a
conditional status for Docker-changing pull requests, not a universal branch
protection context.
