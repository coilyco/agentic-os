# aosguard

`aosguard` is AOS's guarded operator CLI. The name is intentionally
AOS-specific. cli-guard and specgen remain generic products, while AOS owns
this concrete policy snapshot, release name, and launch integration.

Packaged `specgen` discovers the [`.specgen/`](../.specgen/README.md) project,
materializes generated Go out of band, and emits the `aosguard` binary without
committed Go build glue. The full dev-base image and native AOS releases build
from the same source and lock. Homebrew and Scoop install it beside `aos`.

## Authority boundary

The snapshot carries operator-facing Forgejo, AWS, kubectl, Tailscale, Actions,
and runner-token leaves. AWS SSM permits single reads, file-backed writes, and
named deletions. Actions lives at `aosguard ops actions` so its exec transport
does not shadow `aosguard ops forgejo`. The snapshot excludes Ward role policy.
Forgejo metadata includes
`repo-topic replace-all <owner> <repo>`, whose repeated `--topics` values
become the repository's complete topic set.

Ward's fixed broker and AOSguard's static operator surface are independent.
Specgen merges AOSguard policy into one static binary. Neither surface imports
role-derived grants from the other.

## Generated skill

Specgen renders one concise native skill plus a complete lazy command index:

```text
aosguard/SKILL.md
aosguard/references/commands.yaml
```

The full image builds those files beside the binary. `aos --guarded` projects
the skill into the selected agent's skill root. In warded mode, AOS also puts
the binary under the generic bundle's `bin/` directory. Ward mounts it
read-only after the image's PATH, so it cannot shadow an image tool.

The skill grants no permission. The running `aosguard --help`, nested group
help, and `describe` surfaces remain authoritative.

## Source ownership

`.specgen/aosguard/` owns the operator policy. Ward owns its broker internally.
Neither product reads policy from the other, and no drift check forces them to
match.

The Forgejo source is vendored from the pruned deployment contract, so
`aosguard-lock` refreshes the dependency graph without reaching a live Forgejo
edge. The source Swagger and its generated API lock use deterministic gzip at
`forgejo.swagger.v1.json.gz` and `forgejo.swagger.lock.json.gz`. Specgen
decodes each before use. The resulting `specverb.lock` pins cli-guard for
reproducible builds.

Specgen no longer emits per-member Markdown. `aosguard-lock` instead refreshes
the native skill under ignored `dist/skills/`, while maintained product
documentation stays under `docs/`.

## Development

`ward exec aosguard-build` materializes `dist/aosguard` and refreshes the
generated skill. `ward exec aosguard-run --` passes subsequent arguments to the
generated command. `ward exec aosguard-lock` is the only lock-writing step and
uses the packaged `specgen` executable.

Actions leaves call packaged `agentic_os` Python modules. Native `aosguard`
releases embed the same bridge, so every leaf works outside a source checkout.

Cross-repository composition is tracked in
[inbox#267](https://forgejo.coilysiren.me/coilysiren/inbox/issues/267), with AOS
implementation in [agentic-os#755](https://forgejo.coilysiren.me/coilyco-flight-deck/agentic-os/issues/755).

## See also

* [AOS launch CLI](aos-cli.md)
* [Feature inventory](FEATURES.md)
* [AOS and Ward boundary](ward-specs.md)
