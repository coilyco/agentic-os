# aguard

`aguard` is AOS's standalone guarded operator CLI. Packaged `specgen` discovers
the [`.specgen/`](../.specgen/README.md) project, materializes generated Go
out-of-band, and emits the `aguard` binary without committed Go build glue.

## Authority boundary

The first snapshot carries the operator-facing Forgejo monolith, AWS, kubectl,
Tailscale, a combined Actions bridge, and runner-token fetch leaves. The Actions
bridge lives at `aguard ops actions` so its exec transport does not shadow the
spec-backed `aguard ops forgejo` group. The snapshot deliberately excludes the
`aos-agent` read, write, admin, and merge tiers plus the engineer-only AWS
overlay.

That split matters. Ward selects those role sources dynamically through
`.ward/roles.kdl`, while specgen merges every member with the same `wrap`
identity into one static binary. Copying every Ward source into `aguard` would
silently union role authority.

## Source ownership

`.specgen/aguard/` is an independent point-in-time snapshot. `.ward/` continues
to own the live Ward bundle described in [ward specs](ward-specs.md). Neither
tree is generated from the other, and no drift check forces them to match.
Moving a policy change between them is a deliberate review event.

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

Actions log, list, and rerun leaves call the tracked scripts beneath
`.specgen/aguard/scripts/`. Run `aguard` from the AOS repository root so those
paths and their `agentic_os` Python modules resolve.

## See also

* [Feature inventory](FEATURES.md)
* [Ward bundle](ward-specs.md)
* [Role surface tiers](role-surface-tiers.md)
