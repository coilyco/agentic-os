# aguard

`aguard` is AOS's canonical guarded operator CLI. Packaged `specgen` discovers
the [`.specgen/`](../.specgen/README.md) project, materializes generated Go
out-of-band, and emits the `aguard` binary without committed Go build glue.
The [full dev-base image](dev-base-image.md) and native AOS releases build from
the same source and lock. Homebrew and Scoop install it beside `aos`.

## Authority boundary

The first snapshot carries the operator-facing Forgejo monolith, AWS, kubectl,
Tailscale, a combined Actions bridge, and runner-token fetch leaves. The Actions
bridge lives at `aguard ops actions` so its exec transport does not shadow the
spec-backed `aguard ops forgejo` group. The snapshot deliberately excludes the
`aos-agent` read, write, admin, and merge tiers plus the engineer-only AWS
overlay.

That split matters. Ward selects agent-role sources dynamically through
`.ward/roles.kdl`, while specgen merges every member with the same `wrap`
identity into one static binary. Ward retains that role orchestration only.
Human and operator automation use `aguard ops ...`. Copying every Ward role
source into `aguard` would silently union role authority.

## Source ownership

`.specgen/aguard/` owns the operator policy. `.ward/` owns the role-scoped Ward
bundle described in [ward specs](ward-specs.md). Neither tree is generated from
the other, and no drift check forces them to match. A policy that agents and
operators both need is reviewed in both authority contexts.

The Forgejo source is vendored from the pruned deployment contract, so
`aguard-lock` refreshes the dependency graph without reaching a live Forgejo
edge. The source Swagger remains plain JSON for review, while its generated API
lock is deterministic gzip at `forgejo.swagger.lock.json.gz`. Specgen decodes
that lock before use. The resulting `specverb.lock` pins cli-guard for
reproducible builds. The lock wrapper removes specgen's reproducible per-member
reference renders because AOS keeps maintained documentation under `docs/`.

## Development

`ward exec aguard-build` materializes `dist/aguard`. `ward exec aguard-run --`
passes subsequent arguments to the generated command. `ward exec aguard-lock`
is the only lock-writing step and uses the packaged `specgen` executable.

Actions log, list, and rerun leaves call packaged `agentic_os` Python modules.
The full image sets their module path. Native Aguard releases embed the same
bridge and set its module path at launch, so every leaf works outside a source
checkout without Ward, ward-kdl, or specgen at runtime.

## See also

* [Feature inventory](FEATURES.md)
* [Ward bundle](ward-specs.md)
* [Role surface tiers](role-surface-tiers.md)
