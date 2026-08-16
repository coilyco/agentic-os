# Pull-request dev-base build validation

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
all payloads and installing the shared surface. That gate includes
`WARD_DOCTOR_ALLOW_PLACEHOLDERS=1 ward doctor`, followed by the
complete-toolchain smoke.

## Required status

`ci / gate` remains the universal required status. `dev-base-pr / build` is a
conditional status for Docker-changing pull requests, not a universal branch
protection context.

## See also

* [dev-base image](dev-base-image.md)
* [dev-base build cache](dev-base-build.md)
* [CI parity](ci-in-dev-base.md)
* [release pipeline](release.md)
