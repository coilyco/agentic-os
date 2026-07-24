# dev-base build cache

The full multi-architecture image reuses a persistent Buildx builder on the
dedicated image runner. The publish action creates the builder only when it is
absent and recreates it only when bootstrap fails.

The image also reads and writes a `type=registry` cache at
`agentic-os:buildcache`. A cache write uses `ignore-error=true`, so a registry
cache fault does not discard an otherwise valid image. The build helper probes
the cache manifest afterward and emits a warning when the write did not land.

The workflow step summary records the cache key, source, destination, and
source-manifest provenance before the expensive build starts. A cold cache is
therefore visible as a cache event instead of only as a slow build.

See also:

* [dev-base image](dev-base-image.md)
* [release workflow](release.md)
