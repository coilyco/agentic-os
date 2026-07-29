# dev-base image family

AOS publishes five language-specialist images and one full compatibility image.
Every specialist starts directly from `ubuntu:24.04`. `agentic-os:${TAG}` is
the full surface, and `agentic-os:release` remains the moving default used by
the AOS launcher and CI.

## What ships

[`docker/dev-base/Dockerfile`](../docker/dev-base/Dockerfile) owns the five
independent language targets. Each target runs the same
[`install-common.sh`](../docker/dev-base/install-common.sh) source and adds one
toolchain:

* `lang-node`
* `lang-go`
* `lang-dotnet`
* `lang-rust`
* `lang-python`

Every image carries Node because the common agent harnesses require it. The
specialist name identifies the additional development toolchain.

The common surface also carries `aos`, Ward for fixed workflow orchestration,
`aosguard` for operator commands, agent-compose with its embedded `person:kai`
source, and the repository's packaged aosguard Python bridges. Each image build renders
aosguard's native agent skill, renders an agent-compose roster, and checks the
generated person snapshot plus personality definitions. Linuxbrew is not part
of any image.

[`docker/dev-base/full/Dockerfile`](../docker/dev-base/full/Dockerfile) starts
from the same-release Rust image and grafts Go, .NET, and Python tooling from
their same-release images. Node is already present in the common surface. The
full-only gate tools are `golangci-lint`, `trufflehog`, and `kdlfmt`.

## Build and publication

`ward exec dev-base-build` builds all five local specialists, then
`agentic-os:dev-base-local`.
[`scripts/dev-base-build.py`](../scripts/dev-base-build.py) supplies the AOS
CLI, aosguard spec, and aosguard Python package as named Docker contexts.

[`dev-base-publish.yml`](../.forgejo/workflows/dev-base-publish.yml) builds the
five multi-architecture specialists in a four-wide matrix, then builds the full
fan-in image. [`release.yml`](../.forgejo/workflows/release.yml) promotes every
manifest to its versioned and moving tags. Language tags use
`lang-<language>-<tag>`. The full image keeps the plain `<tag>`, `release`, and
`latest` names. Both paths check each target manifest before work, so a retry
skips an image that already landed. See [publish resume](dev-base-publish-resume.md).

## Pinning a tool

Every managed version has one default `ARG` across the two Dockerfiles.
`ward exec dep-bump -- check` compares those pins with upstream releases.
`ward exec dep-bump -- apply --arg NAME --version VERSION` rewrites one owning
declaration.

Source ownership follows the tool boundary. The image owns this deployment's
Git identity and its entrypoint maps that identity onto Ward's provider-neutral
`WARD_GIT_*` environment contract before container bootstrap. The image does
not redefine Ward policy, aosguard policy, or agent-compose source data.

## See also

* [CI parity](ci-in-dev-base.md)
* [Build cache](dev-base-build-cache.md)
* [Release workflow](release.md)
