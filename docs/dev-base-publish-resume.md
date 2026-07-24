# Dev-base publish resume

The dev-base publish path is image-resumable. A workflow dispatch accepts a
commit SHA and optional tag, checks the target registry manifest first, and
skips the full image when that exact checkpoint already exists.

The release retry accepts the same commit-derived source tag and promotes only
the full manifest. Registry login, manifest inspection, build, and promote
operations all use bounded retry budgets.

See also:

* [dev-base container image](dev-base-image.md)
* [Release pipeline](release.md)
