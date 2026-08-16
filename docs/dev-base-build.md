# dev-base build

What the image build caches, pins, and resumes.

## dev-base build cache

Every multi-architecture payload and full-image build reuses a persistent
Buildx builder on its dedicated runner. The publish action creates a builder
only when it is absent and recreates it only when bootstrap fails.

Each language payload reads and writes a stable `type=registry` cache at
`agentic-os:lang-<language>-buildcache`. The full image uses
`agentic-os:buildcache`. Cache export uses `mode=max`, so intermediate language
layers remain reusable across workflow runs, runners, and days even though the
payload draft manifests are commit-scoped.

Cache writes use `ignore-error=true`. A registry cache fault therefore does not
discard an otherwise valid image, but the next run may be colder. The payload
draft manifest and the cache ref have separate jobs: the draft gives the full
job an exact same-commit input, while the stable cache accelerates later
builds.

Pull-request validation uses the persistent single-architecture
`aos-pr-builder` instead of registry cache export. Buildx Bake links the five
payload targets directly into `full`, keeps their outputs cache-only, and loads
only `full` for the smoke check. Cleanup removes local image tags and prunes
that builder to a configured maximum size.

See also:

* [dev-base image](dev-base-image.md)
* [PR build validation](pr-dev-base-build-validation.md)
* [release workflow](release.md)

## Pinned Rust toolchains

The Rust payload installs `stable` as its default and additionally bakes every
toolchain in `RUST_PINNED_VERSIONS`, a space-separated list declared in
[`docker-bake.hcl`](../docker/dev-base/docker-bake.hcl) and threaded to both
Dockerfiles.

This exists because a consumer pinning a channel in `rust-toolchain.toml`
otherwise downloads it from `static.rust-lang.org` on the first cargo call of
every CI run. That request has no cache, and when the runner's egress to that
host degrades, every Rust repo goes red on a network fault rather than on its
own code. galaxy-gen#84 is the worked example: three retries over ten minutes
all timed out from inside the runner while the same URL answered in 0.16s from
outside.

Each baked toolchain gets `clippy`, `rustfmt`, and the
`wasm32-unknown-unknown` target. **The components are the point** - baking a
bare toolchain still leaves rustup fetching components at first use, which is
the same download in a different place. The full image verifies each pin end to
end and fails if the list arrives empty, so an unplumbed build arg cannot make
that check a silent pass.

Adding a pin is one edit to the `RUST_PINNED_VERSIONS` default. Consumers keep
owning their own `rust-toolchain.toml`; this only decides what is already
resident when they ask.

## See also

* [dev-base image](dev-base-image.md)
* [CI parity](ci-in-dev-base.md)

## Dev-base publish resume

The dev-base publish graph is artifact-resumable. A workflow dispatch accepts
a commit SHA, optional tag, and artifact selection. It checks the target
registry manifest first and skips any payload or full image whose exact
checkpoint already exists.

Selecting `full` builds any missing language payloads before the fan-in image.
Selecting one language rebuilds only that payload. Stable per-language
`buildcache` refs remain independent of commit-scoped draft tags, so a resumed
or later build can reuse layers produced on another runner or day.

An `all` or `full` resume continues into the root release after the full draft
passes verification. A single-language resume stops after that payload. The
separate release dispatch remains available for idempotent retag or metadata
recovery and explicit version overrides.

## Verify before the tags move

Promote verifies the draft manifest by its source tag and only then stamps the
version tag and the moving `release` and `latest` aliases onto it. The draft is
already pushed and addressable at that point, so verification never needs the
release tags to exist. A verification failure therefore leaves the aliases on
the last image that passed, and promotion stays the final step, which is the
property that makes the job safe to retry.

Build mode verifies the draft it just pushed, because the image cannot be
verified before it exists. That ordering is safe because the build-mode publish
stamps only the commit-scoped draft tag, never a fleet alias.

## Cross-architecture verification

Both publish modes verify the full tier by running one container per published
platform, so the foreign-architecture leg needs a binfmt handler. Build mode
installs it while setting up the buildx builder. Promote mode skips that setup
and runs on the `docker` runner rather than `docker-build`, so it inherits no
handler from a build-mode run and installs binfmt itself before verifying.

See also:

* [dev-base container image](dev-base-image.md)
* [Release pipeline](release.md)
