# Independent language dev-base images

The dev-base family publishes language specialists and retains `dev-base-full`
as the compatibility surface.

## Published topology

* `lang-node` starts directly from Ubuntu and adds the common agent surface.
* `lang-go` starts directly from Ubuntu and adds Go.
* `lang-dotnet` starts directly from Ubuntu and adds the .NET SDK and ICU.
* `lang-rust` starts directly from Ubuntu and adds Rust, wasm, `trunk`, and the
  native alsa/udev/Wayland/XKB development contract.
* `lang-python` starts directly from Ubuntu and adds `python`, `pip`, and
  `pipenv`.
* `full` inherits the same-release `lang-rust` image and grafts the
  self-contained Go, .NET, and Python prefixes.

There is no published or internal runtime `core` target. The language targets
reuse [`install-common.sh`](../docker/dev-base/install-common.sh) as source,
not as an image parent. The hidden Ward/AOS builder produces only the two
binaries copied into each language target.

## Dependency graph

All language rows depend only on the draft plan and resume independently. The
publish matrix caps itself at one active row because every multi-architecture
export shares the persistent image builder. `full` waits for the matrix. Node
already arrives through the Rust image's complete common surface.

## Tag derivation

* `full` keeps the plain release tag.
* Each language tier prefixes the tag with its tier name.
* Each tier has an independent build-cache tag.
* No manifest file duplicates the graph. [`agentic_os/dev_base.py`](../agentic_os/dev_base.py)
  derives every ref from the declared specifications.

## Build context

Language targets share `docker/dev-base/` as a deliberately narrow context.
The allowlist-style `.dockerignore` sends only the Dockerfile, common installer,
runtime assets, and substrate repository list to the builder. The AOS source is
a separate named build context.

`full` retains its graft-only Dockerfile and local context. It consumes
same-release registry images through explicit build arguments.

## Release flow

* `promote.yml` gates `main` and fast-forwards `release`.
* `dev-base-publish.yml` runs up to four draft language exports on the dedicated
  image-build runner, matching its configured capacity, then publishes `full`.
* `release.yml` retags those draft manifests and creates the release only after
  every image succeeds.
* Manual dispatches can target one tier closure. See
  [publish resume](dev-base-publish-resume.md).

## See also

* [dev-base container images](dev-base-image.md)
* [Build cache](dev-base-build-cache.md)
