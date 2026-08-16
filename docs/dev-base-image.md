# dev-base image

AOS releases one multi-architecture full image. Five language payloads are
build artifacts that preserve parallelism, cross-run caching, and a shallow
full-image fan-in. They are not consumer images.

## Build graph

[`docker/dev-base/Dockerfile`](../docker/dev-base/Dockerfile) owns the five
independent Ubuntu payload targets:

* `lang-node`
* `lang-go`
* `lang-dotnet`
* `lang-rust`
* `lang-python`

Each payload contains only its language toolchain and the architecture metadata
needed by the full build. The Forgejo matrix builds all five in parallel, with
four runners active at once. Stable per-language registry cache refs preserve
BuildKit layers across runners and across days.

[`docker/dev-base/full/Dockerfile`](../docker/dev-base/full/Dockerfile) starts
from the same-commit Rust payload and grafts Node, Go, .NET, and Python from the
others. Only the full image carries the entrypoint, common verification,
release-pinned `aos`, Ward, `aosguard`, agent-compose, harnesses, operator CLIs,
and gate tools such as `golangci-lint`, `trufflehog`, and `kdlfmt`.

`AOS_VERSION` pins one `aos-bundle` release carrying `aos`, `aosguard`, its
skill, the operator Python modules, and the repo lists, so the image builds
nothing from this tree and `docker/**` is a complete path filter. Image builds
own no host mounts, kubeconfig, or cluster transport. The AOS launcher may add
an [operator kubeconfig](aos-kubeconfig.md) for a role.

## Build and publication

`ward exec dev-base-build` executes the declarative
[`docker-bake.hcl`](../docker/dev-base/docker-bake.hcl). BuildKit builds the five
payload targets as cache-only dependencies and loads only
`agentic-os:dev-base-local` for smoke verification.

[`dev-base-publish.yml`](../.forgejo/workflows/dev-base-publish.yml) skips unless the promoted diff changes a path under `docker/`. Its matrix publishes
commit-scoped payload drafts, then the full job consumes those exact manifests
and publishes `draft-${sha}`. A successful full draft calls the root release
workflow. Manual dispatch can resume one payload or the complete full closure,
and a resumed full closure finishes the same release chain.

[`release.yml`](../.forgejo/workflows/release.yml) promotes only the full
manifest to its next minor tag, `release`, and `latest`. Manual dispatch reuses
it for retries and version overrides. Payload drafts and cache refs are internal
build transport with no release alias or compatibility contract. See [publish resume](dev-base-publish-resume.md).

Pull requests with a `docker/` change build the complete source graph through
[`actions/dev-base-build`](../actions/dev-base-build/action.yml). PR validation
has no registry credential and publishes nothing. See
[PR build validation](pr-dev-base-build-validation.md).

## Pinning a tool

Every managed version has one default `ARG` across the two Dockerfiles. A
language pin lives in its payload target. Shared agents, internal tools,
operator CLIs, and full-only gates live in the full Dockerfile, so changing
them reuses cached language payloads instead of rebuilding their toolchains.

Every pin is manual. Nothing resolves these versions against upstream and
nothing fails when one falls behind, so a pin is only as current as the last
person who checked it - an explicit inventory responsibility. Rust also bakes
the channels consumers pin, see [pinned toolchains](dev-base-rust-pins.md).

Source ownership follows the tool boundary. The image owns this deployment's
Git identity and maps it onto Ward's provider-neutral `WARD_GIT_*` environment
contract. It does not redefine Ward policy, AOSguard policy, or agent-compose
source data.

## See also

* [CI parity](ci-in-dev-base.md)
* [Build cache](dev-base-build-cache.md)
* [PR build validation](pr-dev-base-build-validation.md)
* [Release workflow](release.md)
