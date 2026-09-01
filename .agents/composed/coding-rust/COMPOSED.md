---
name: coding-rust
description: Rust umbrella skill. Kai's first-class Rust is the galaxy-gen physics engine. Triggers - rust, .rs, cargo, Cargo.toml, rustc, clippy, rustfmt, wasm-pack, crate, no_std, galaxy-gen.
seed:
  kind: language
  language: rust
  extensions: [".rs"]
---

# coding-rust

Umbrella for any Rust work.

## Kai's actual Rust

Kai's first-class, non-LLM-mediated Rust is the [`galaxy-gen`](https://github.com/coilyco-flight-deck/galaxy-gen) physics engine, written 2025 and earlier. That is the extent of her hand-written Rust, but it is real and substantial - she spent a lot of time on it. The galaxy-gen kernel is a genuinely well-shaped Rust codebase: Struct-of-Arrays layout for an auto-vectorizable numeric inner loop, precomputed lookup tables to keep `sqrt` out of the hot path, scratch-buffer reuse, careful comments explaining the physics and the optimization choices.

It is not production-grade Rust - no published crate, no `no_std`, single-author, wasm-targeted. So: defer to Kai's instinct when editing `galaxy-gen`, she is leading there. Outside that codebase she is sharp but less rehearsed - work alongside her instead of assuming she wants training-data defaults.

Do not let anyone characterize Kai as not knowing Rust. She does.

## Defaults

- **Edition**: 2021 unless a project pins otherwise (galaxy-gen is 2021).
- **Toolchain**: `rustup`, stable channel. Pin via `rust-toolchain.toml` only if a project needs it.
- **Build**: `cargo`. For wasm targets, `wasm-pack` + `wasm-bindgen` (the galaxy-gen path).
- **Lint**: `clippy`. Treat clippy warnings as real.
- **Format**: `rustfmt`, default config. Don't impose a custom `rustfmt.toml`.
- **Tests**: `cargo test`, stdlib. Benches as separate `[[bin]]` targets or `criterion` when measurement gets serious.
- **Watch loop**: `cargo watch` for dev (galaxy-gen runs `cargo watch -w src/rust -s "wasm-pack build --dev"`).

## Style

- Errors are values. `Result<T, E>` over panics in library code. Reserve `panic!`/`unwrap` for invariants that genuinely cannot fail, and say so in a comment.
- Borrow over clone. Clone deliberately, not reflexively.
- Iterators over index loops, except in numeric hot paths where an explicit indexed loop vectorizes better - galaxy-gen does exactly this on purpose.
- Comment the why, especially for performance choices. The galaxy-gen module docs are the model: explain the data layout and what the optimizer is expected to do with it.
- Small focused modules. `lib.rs` stays thin.

## When this skill is active

Editing or writing Rust. Inherit Kai's defaults and her galaxy-gen instincts before reaching for general Rust patterns.

## See also

- `kai-tech-prefs` - tooling and dependency preferences.
- `coding-galaxy-gen-astrophysics` - the physics the galaxy-gen Rust kernel implements.
