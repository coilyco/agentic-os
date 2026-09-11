# Pinned and vendored build inputs

Two cases of the same rule: the input is carried, never fetched mid-build.
How `coilyco-bridge/deploy` gets a copy of this repo's Forgejo operator policy
and why it arrives as a push, then why the WASM toolchain is baked into the
dev-base image.

## Why a push

Config lives at the lowest layer that fully determines it, is consumed only by
that layer or higher, and is never fetched downward. A shipped product does not
reach up into a reference repo for its own runtime config.

A CI-time fetch is the same coupling wearing a different hat: it makes deploy's
build depend on reaching this repo. So the bytes arrive by a push that deploy
then reviews and commits, which is the authoring-vs-rollout split already in
force everywhere else. Authored here, rolled out by a push.

## What moves

`.forgejo/workflows/vendor-forgejo-policy.yml` fires on a `release` push that
touches either file, and runs `scripts/ci/vendor-forgejo-policy.sh`:

- `.umbra/guardfiles/aosguard/forgejo.kdl` - the operator policy
- `.umbra/guardfiles/aosguard/forgejo.swagger.v1.json.gz` - the pruned spec

Both land in `services/forgejo-mcp/vendor/aosguard/` beside a `SOURCE` file
naming the commit they came from. The pin is what makes the copy auditable:
deploy answers "which policy is this" by reading it rather than guessing.

The script pushes a branch and opens a pull request. It never merges. Deploy
reviews and lands its own vendored copy, so nothing here writes deploy's `main`.

## The credential, and what happens without it

The push needs `DEPLOY_WRITE_TOKEN`, an Actions secret carrying write to
`coilyco-bridge/deploy`. Minting and placing it is an operator step on a hosted
surface, so it is not part of this change.

Until it exists the workflow is **inert rather than broken**: the script warns
and exits 0 on an absent token, the same guard `aos-cli-release.sh` uses for its
tap and scoop pushes. A run reports the skip in its log rather than failing the
release path.

That guard is the thing worth not losing. A vendoring job that fails loudly on
every release would get disabled. One that silently pushed with a token nobody
reviewed would be worse. `tests/test_vendor_forgejo_policy.py` holds it in place.

## Once it lands in deploy

`services/forgejo-mcp/forgejo.mcp.kdl` can `inherit` the vendored guardfile
instead of restating grants. That is the deduplication `agentic-os#1365` was
originally asking after, and it is available only for this rung: the sirens-echo
tier fixes every path to one repository and cannot inherit a parameterized one.

## The pinned WASM toolchain

`wasm-pack`, `wasm-opt`, and the `wasm-bindgen` CLI are all baked into the Rust
payload of [the dev-base image](dev-base-image.md) for one reason: wasm-pack
downloads whichever of them PATH does not already supply, in the middle of a
build, from GitHub releases, with nothing bounding the fetch.

That makes it the same failure documented for pinned toolchains in
[dev-base-image.md](dev-base-image.md#pinned-rust-toolchains) - a repo going red
on runner egress rather than on its own code. galaxy-gen#89 is the wasm-bindgen
instance: six runs spent the runner's entire 30m budget on that one download,
each having compiled the crate itself in under seven seconds.

## The version pin that is not free to float

`WASM_BINDGEN_VERSION` differs from the other pins beside it. wasm-pack compares
the CLI it finds against the `wasm-bindgen` **crate** version a consumer locks,
and downloads the matching CLI when the two differ. A stale pin therefore does
not fail - it silently restores the stall this bake removes.

So this pin tracks consumers rather than leading them. Bump it when a consumer's
locked crate moves. A caret dependency such as galaxy-gen's
`wasm-bindgen = "^0.2"` moves on any `cargo update` with no deliberate edit, so
the drift can arrive without anyone choosing it.

A consumer that would rather have drift fail loudly than silently re-download
can build with `wasm-pack build --mode no-install`, which makes wasm-pack use
the PATH binaries and error on a mismatch instead of fetching.

## Arch coverage

`WASM_BINDGEN_ARCH` resolves in
[`prepare-build-stage.sh`](../docker/dev-base/prepare-build-stage.sh) beside
every other tool's arch var. Upstream ships `x86_64` and `aarch64`
`unknown-linux-musl` tarballs, so both build arches take the musl build and
neither depends on the host glibc.

Only the binaries wasm-pack invokes are kept: `wasm-bindgen` for build and
`wasm-bindgen-test-runner` for `wasm-pack test`. `wasm2es6js` ships in the same
tarball and wasm-pack never calls it.
