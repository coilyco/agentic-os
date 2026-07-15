# Dev-base publish resume

The dev-base publish path is tier-resumable. A workflow_dispatch rerun targets
one tier closure at a time with `sha`, `tier`, and `tag` inputs, checks the
target registry manifest first, and skips any tier that already matches
exactly.

The retry budget is bounded around registry login, manifest inspection,
buildx push, cache probe, and retag operations. A transient registry hiccup
can heal without hiding a real publish failure, and the durable checkpoint is
the registry package tag itself.

See also:

- [dev-base container image](dev-base-image.md)
- [Tiered dev-base image split](dev-base-image-tiering.md)
- [Release pipeline](release.md) for the retag path that adds `source-tag`.
