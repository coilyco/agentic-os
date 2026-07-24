# Dev-base publish resume

The dev-base publish path is tier-resumable. A workflow_dispatch rerun targets
one tier closure at a time with `sha`, `tier`, and `tag` inputs, checks the
target registry manifest first, and skips any tier that already matches
exactly.

A `release.yml` dispatch can use the same tier selector to revalidate one
closure without forcing the final release job. `tier=all` keeps the full
release gate intact.

The retry budget is bounded around registry login, manifest inspection,
cache probe, and source-image wait operations. The build itself still fails
promptly on deterministic errors, and the durable checkpoint is the registry
package tag itself.

See also:

- [dev-base container image](dev-base-image.md)
- [Independent dev-base image topology](dev-base-image-tiering.md)
- [Release pipeline](release.md) for the retag path that adds `source-tag`.
