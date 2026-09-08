# dev-base image

AOS releases one multi-architecture full image. Five language payloads are
build artifacts preserving parallelism, cross-run caching, and a shallow
fan-in, and are not consumer images.

## Build graph

[`docker/dev-base/Dockerfile`](../docker/dev-base/Dockerfile) owns five
independent Ubuntu payload targets: `lang-node`, `lang-go`, `lang-dotnet`,
`lang-rust`, and `lang-python`, each carrying only its toolchain plus the
architecture metadata the full build needs. The matrix builds all five in
parallel across four runners, and commit-scoped payload manifests transport one
exact build into the full-image job.

[`docker/dev-base/full/Dockerfile`](../docker/dev-base/full/Dockerfile) starts
from the same-commit Rust payload, grafts the other four, and installs the
shared agent and operator surface once. Only the full image carries the
entrypoint, common verification, release-pinned `aos`, `aosguard`,
agent-compose, harnesses, operator CLIs, and full-only gate tools such as
`golangci-lint`, `trufflehog`, and `kdlfmt`.

The full image includes `kubectl`, but image builds own no host mounts,
kubeconfig, or cluster transport: the standalone launcher may add an
[operator-selected kubeconfig](aos-cluster-access.md) for an authorized role.

## Build and publication

`just dev-base-build` executes the declarative
[`docker-bake.hcl`](../docker/dev-base/docker-bake.hcl), building the five
payload targets as cache-only dependencies and loading only
`agentic-os:dev-base-local` for smoke verification.

[`dev-base-publish.yml`](../.forgejo/workflows/dev-base-publish.yml) skips
unless the promoted diff touches `docker/`. Its matrix publishes commit-scoped
payload drafts, the full job consumes those exact manifests and publishes
`draft-${sha}`, and a successful full draft calls the root release workflow.

[`release.yml`](../.forgejo/workflows/release.yml) promotes only the full
manifest to its next minor tag, `release`, and `latest`, and manual dispatch
reuses it for retries and overrides. Payload drafts and cache refs are internal
transport with no release alias or compatibility contract. Writes are asserted
rather than assumed: each payload and the release promotion are re-read from the
registry rather than taken from a job colour, each payload gets one bounded
retry, and `apt-get` retries via
[`prepare-build-stage.sh`](../docker/dev-base/prepare-build-stage.sh).

Pull requests touching `docker/` build the complete source graph through
[`actions/dev-base-build`](../actions/dev-base-build/action.yml) with no
registry credential, publishing nothing. See
[PR build validation](ci-in-dev-base.md).

Every managed version has one default `ARG` across the two Dockerfiles: a
language pin in its payload target, and shared agents, internal tools, operator
CLIs, and full-only gates in the full Dockerfile, so changing them reuses cached
payloads rather than rebuilding toolchains. Every pin is manual, and nothing
resolves them against upstream or fails when one falls behind, so a pin is only
as current as the last person who checked it. The image owns this deployment's
Git identity and maps it onto Ward's `WARD_GIT_*` contract, without redefining
Ward policy, AOSguard policy, or agent-compose source data.
## dev-base build cache

Every multi-architecture payload and full-image build reuses a persistent Buildx
builder on its dedicated runner, created only when absent and recreated only
when bootstrap fails.

Each payload reads and writes a stable `type=registry` cache at
`agentic-os:lang-<language>-buildcache`, and the full image uses
`agentic-os:buildcache`. Export uses `mode=max`, so layers stay reusable across
runs and days even though draft manifests are commit-scoped, and
`ignore-error=true` on writes makes a cache fault cost a colder next run rather
than a valid image. The draft gives the full job an exact same-commit input
while the stable cache accelerates later builds.

Pull-request validation uses the single-architecture `aos-pr-builder` instead
of registry export: Bake links the five payload targets into `full`, keeps
their outputs cache-only, and loads only `full` for the smoke check.

## Pinned Rust toolchains

The Rust payload installs `stable` as its default and bakes every toolchain in
`RUST_PINNED_VERSIONS`, declared in
[`docker-bake.hcl`](../docker/dev-base/docker-bake.hcl) and threaded to both
Dockerfiles.

Without it, a consumer pinning a channel in `rust-toolchain.toml` downloads it
from `static.rust-lang.org` on the first cargo call of every CI run. That
request has no cache, so when runner egress to that host degrades every Rust
repo goes red on a network fault rather than on its own code. galaxy-gen#84 is
the worked example: three retries over ten minutes timed out from inside the
runner while the same URL answered in 0.16s from outside.

Each baked toolchain gets `clippy`, `rustfmt`, and the
`wasm32-unknown-unknown` target. **The components are the point**, since a bare
toolchain still leaves rustup fetching them at first use. The full image
verifies each pin end to end and fails on an empty list, so an unplumbed build
arg cannot pass silently. Adding a pin is one edit to the default, and
consumers keep owning their own `rust-toolchain.toml`.

## Resume, verify, promote

The publish graph is artifact-resumable. A dispatch accepts a commit SHA,
optional tag, and artifact selection, checks the target registry manifest
first, and skips any payload or full image whose exact checkpoint exists.
Selecting `full` builds missing payloads before the fan-in image, and selecting
one language rebuilds only that payload. An `all` or `full` resume continues
into the root release after the full draft verifies, while a single-language
resume stops there.

Promote verifies the draft manifest by its source tag, then stamps the version
tag and the moving `release` and `latest` aliases onto it. The draft is already
addressable, so verification never needs the release tags to exist, and a
failure leaves the aliases on the last image that passed. Promotion staying
last is what makes the job safe to retry. Build mode verifies the draft it just
pushed, which is safe because it stamps only the commit-scoped tag.

Both modes verify the full tier by running one container per published
platform, so the foreign-architecture leg needs a binfmt handler. Build mode
installs it with the buildx builder, and promote mode runs on the `docker`
runner, inherits none, and installs it itself.
