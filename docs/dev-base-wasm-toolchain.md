# Pinned WASM toolchain

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
