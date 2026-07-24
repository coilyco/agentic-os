# dev-base container image

AOS publishes one development image. `agentic-os:${TAG}` is the full surface,
and `agentic-os:release` is the moving default used by the AOS launcher and CI.
There are no language-specialist, core, ops, or agent image tags.

## What ships

[`docker/dev-base/Dockerfile`](../docker/dev-base/Dockerfile) owns the single
`dev-base-full` target. It installs the common agent surface once, then adds
every supported language toolchain:

* Node and the JavaScript agent tools
* Go
* .NET and ICU
* Rust, wasm, `trunk`, and native Bevy/Winit development libraries
* Python, pip, and `pipenv`

The image also carries `aos`, Ward for role-scoped orchestration, `aguard` for
operator commands, and the repository's packaged Aguard Python bridges.
Full-only gate tools include `golangci-lint`, `trufflehog`, and `kdlfmt`.

## Build and publication

`ward exec dev-base-build` builds `agentic-os:dev-base-local`.
[`scripts/dev-base-build.py`](../scripts/dev-base-build.py) supplies the AOS
CLI, Aguard spec, and Aguard Python package as named Docker contexts.

[`dev-base-publish.yml`](../.forgejo/workflows/dev-base-publish.yml) builds one
multi-architecture draft manifest after the release branch advances.
[`release.yml`](../.forgejo/workflows/release.yml) promotes that manifest to the
versioned tag plus `release` and `latest`. Both paths check the target manifest
before work, so a retry skips an image that already landed. See
[publish resume](dev-base-publish-resume.md).

## Pinning a tool

Every managed version has one default `ARG` in the Dockerfile.
`ward exec dep-bump -- check` compares those pins with upstream releases.
`ward exec dep-bump -- apply --arg NAME --version VERSION` rewrites one owning
declaration.

## Pulling an image

Consumers pin a versioned ref for reproducibility. Humans and AOS may use the
moving `release` alias for the current promoted image. CI and deployment
configuration own their exact pin. The image repository never reaches upward
into a consumer repository for runtime configuration.

## Not here

* Fleet rollout belongs in infrastructure/ansible.
* A repository-owned shared runtime image is intentionally absent.
* Infrastructure owns private-network startup, authentication, and sockets. AOS composes role-scoped client access.
* Standalone `ops` and `agent` images are intentionally absent.

Source ownership follows the tool boundary. The image pins versions but does
not redefine Ward configuration, Aguard policy, or agent-compose source data.

## See also

* [CI parity](ci-in-dev-base.md)
* [Build cache](dev-base-build-cache.md)
* [Release workflow](release.md)
