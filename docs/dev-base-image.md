# dev-base container images

The dev-base family publishes independent language specialists plus the
`dev-base-full` compatibility surface. See [image topology](dev-base-image-tiering.md).

## What ships

[`docker/dev-base/Dockerfile`](../docker/dev-base/Dockerfile) is a multi-target
Dockerfile. Every `dev-base-lang-*` target starts with its own
`FROM ubuntu:<version>` instruction. The language images do not inherit a
repository-owned base image or another language target.

[`docker/dev-base/install-common.sh`](../docker/dev-base/install-common.sh)
owns the repeated source-level setup for agent harnesses, operational CLIs,
platform assets, `aos`, agent-compose, ward, and the substrate seed. Reusing
that installer keeps the independently built images aligned without turning
the common surface into a published image contract.

The published specialists are Node, Go, .NET + ICU, Rust + wasm + `trunk`, and
Python + pip + `pipenv`. `full` inherits the same-release Rust image and grafts
the self-contained Go, .NET, and Python toolchain prefixes. Rust supplies the
native Bevy/Winit development libraries.

## Naming and tags

One repository release tag drives the family:

* `full` uses the plain `agentic-os:${TAG}` ref.
* Language images use `agentic-os:<tier>-${TAG}`.
* Draft, `release`, `latest`, and build-cache tags follow the same rule.
* The family publishes no `core`, `ops`, or `agent` tag.

The default AOS launcher image remains `agentic-os:release`.

## How it publishes

[`.forgejo/workflows/dev-base-publish.yml`](../.forgejo/workflows/dev-base-publish.yml)
builds the language images through a matrix capped at one active export on the
shared image builder. Every matrix row starts from Ubuntu and can rerun
independently. The `full` job waits for the language matrix.

[`release.yml`](../.forgejo/workflows/release.yml) retags the already-published
draft manifests to the versioned tag plus `release` and `latest`. The helper
checks target manifests before work, so a manual retry resumes at the selected
tier without rebuilding completed outputs. See [publish resume](dev-base-publish-resume.md).

## Pinning a tool

Tool versions have one default declaration in the Dockerfile that owns the
installation. Shared and language versions live in the multi-target Dockerfile.
Full-only gate tools live in `full/Dockerfile`.

`ward exec dep-bump -- check` compares managed pins with upstream releases.
`ward exec dep-bump -- apply --arg NAME --version VERSION` rewrites one owning
default. The scheduled dependency workflow performs the same operation and
runs the repository gate before pushing.

## Pulling an image

Consumers pin a versioned ref for reproducibility. Humans and AOS may use the
moving `release` alias for the current promoted image. CI and deployment
configuration own their exact pin. The image repository never reaches upward
into a consumer repository for runtime configuration.

## Not here

* Fleet rollout belongs in infrastructure/ansible.
* A repository-owned shared runtime image is intentionally absent.
* Standalone `ops` and `agent` images are intentionally absent.

## See also

* [Independent language image topology](dev-base-image-tiering.md)
* [CI parity](ci-in-dev-base.md)
* [Build cache](dev-base-build-cache.md)
* [Release workflow](release.md)
