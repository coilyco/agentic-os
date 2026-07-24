# dev-base build cache

How the dev-base publish jobs stay warm between runs (aos#491). Before this,
a cold multi-arch rebuild of the whole family under qemu routinely hit the
old shared 120-minute timeout, and the flake that cost a release
(agentic-os#490) rode on that coldness.

## The persistent builder is the disk cache

The buildx builder (`aosbuilder`, `docker-container` driver) keeps its layer
cache inside its own container on the runner's long-lived dind sidecar, so
the cache survives between runs exactly as long as the builder does. The old
bootstrap ran `docker buildx rm -f aosbuilder` before every create to dodge a
name collision left by crashed runs - which silently discarded the entire
local cache on every single run.

The [`actions/publish-dev-base-tier`](../actions/publish-dev-base-tier/action.yml)
composite now **reuses** the builder: it creates it only when absent (with a
`|| true` absorbing the create race between sibling tier jobs) and recreates
it only when the existing one genuinely fails to bootstrap. The crash-collision
case still self-heals; the warm cache stops being collateral.

Each language target assigns its commit-pinned `WARD_CONFIG_REF` only after the
shared package and language-toolchain layers. That ref changes on every aos
commit so the final `ward doctor` validates the exact bundled `.ward/` source.
Keeping the stamp beside the final validation gate preserves the expensive
layers across commits while still making every independent language image
commit-specific.

The language targets use identical common setup instructions over the same
Ubuntu parent. BuildKit can reuse those layers across targets without a
published shared runtime image.

## The registry cache is secondary, and loud when broken

Each tier also reads and writes a `type=registry` cache under its
`<tier>-buildcache` tag, which warms a fresh runner or a recreated builder.
The write keeps `ignore-error=true` - a registry hiccup must not fail an
otherwise-good push - but that made a permanently-cold cache invisible: if the
registry is unhealthy, the cache never populates, every run is cold, and each
cold run pushes even more multi-GB blobs at the struggling registry.

[`scripts/dev-base-build.py`](../scripts/dev-base-build.py) now probes the
buildcache manifest after each pushed tier and emits a `::warning::` in the
job log when the write did not land, after a bounded retry budget, so cache rot
is observed, not inferred from slow runs or a single transient registry miss.
The same helper also writes a per-tier step-summary cache plan before the
expensive build starts, including the cache key, registry source, destination,
and whether the source manifest was already present. That makes a cold cache
visible as a cache event, not just as a slow or failed build.

## See also

- [dev-base image topology](dev-base-image-tiering.md) - the graph these
  cached builds flow through.
- [release.md](release.md) - the per-tier publish jobs.
