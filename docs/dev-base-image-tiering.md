# Language-specialist dev-base images

The dev-base family publishes language specialists and retains `dev-base-full`
as its compatibility surface.

## Tier layout

* `core` - Ubuntu, common tooling, Node-backed agent harnesses, operational CLIs, assets, entrypoint, substrate seed, and the hidden `ward` builder.
* `lang-node` - the Node specialist tag over the common substrate.
* `lang-go` - the shared specialist surface plus Go.
* `lang-dotnet` - the shared specialist surface plus the .NET SDK and ICU.
* `lang-rust` - the shared specialist surface plus Rust, wasm, `trunk`, and the native alsa/udev/Wayland/XKB build contract.
* `lang-python` - the shared specialist surface plus `python`, `pip`, and `pipenv`.
* `full` - the compatibility fan-in and gate tools, inheriting the complete Rust/native surface.

The family publishes no `ops` or `agent` image. Operational tooling and agent
harnesses are capabilities of each language specialist, not standalone
specialists of their own.

## Dependency graph

* `core` is the shared substrate. Node lives there because the agent harnesses require its runtime.
* Every `lang-*` image is a direct, parallel child of `core`.
* `full` builds from the same-release `lang-rust` image, so its Rust, wasm, Trunk, and native Bevy/Winit dependencies are inherited as one contract. Node already arrives through `core`.
* `full` grafts the self-contained Go, .NET, and Python prefixes. [`agentic_os/dev_base.py`](../agentic_os/dev_base.py) emits the required image build args.
* The hidden builder stage stays inside `core` so ward still compiles per target platform.

## Tag derivation

* One repo release tag drives the whole family.
* Every tier publishes under the `agentic-os` package. `full` keeps the plain tag and other tiers prefix it with their name.
* The release helper in [`scripts/dev-base-build.py`](../scripts/dev-base-build.py) derives the plan from the declared tier graph.
* Every ref derives from `{registry base, tier, tag}`. No manifest JSON can drift.

## Release flow

* `promote.yml` gates `main` and fast-forwards `release`. Draft image publishing is separate.
* `dev-base-publish.yml` keeps `plan-draft` on the general `docker` runner and sends every image-building job to the dedicated `docker-build` lane. That lane's 130-minute runner cap encloses each workflow-level 120-minute wall. Each draft tier runs through [`actions/publish-dev-base-tier`](../actions/publish-dev-base-tier/action.yml), and `needs:` carries the graph above.
* Each job verifies its manifests. Builder and layer cache persist between runs: [dev-base build cache](dev-base-build-cache.md).
* `release.yml` provides manual retag retries and never runs on push.
* Manual workflow dispatches can target one tier closure at a time. See [publish resume](dev-base-publish-resume.md).

## ARG ownership

* `UV_VERSION` and `WARD_VERSION` live in `core`.
* Node, agent-harness, and operational CLI version arguments live in `core`.
* `GO_VERSION` lives in `lang-go`.
* `DOTNET_VERSION` lives in `lang-dotnet`.
* `TRUNK_VERSION` lives in `lang-rust`.
* `lang-python` uses the Python runtimes and `uv` supplied by `core` and owns its `pipenv` installation.
* `GOLANGCI_LINT_VERSION`, `TRUFFLEHOG_VERSION`, and `KDLFMT_VERSION` live in `full`.

## See also

* [dev-base container image](dev-base-image.md) - the current published contract.
* [CI parity in dev-base](ci-in-dev-base.md) - the pinned consumer side.
* [FEATURES.md](FEATURES.md) - the feature inventory this lands in.
