# AGENTS context inventory and tiers

`agents-context-inventory` inventories AGENTS-family context across the public AOS
substrate and a supplied managed fleet. It separates the aggregate duplication
corpus from product-specific clipping candidates.

The [context budget](context-budget.md) reuses its repository discovery and
explicit role-seat cascade helpers without relying on a routing board.

## Inputs

The substrate set comes from `aos-cli/repositories/substrate-repos.txt`. AOS does
not scan the projects directory to invent a fleet. Infrastructure supplies:

```text
# owner/name visibility
coilyco-flight-deck/agentic-os public
example/product private
```

Visibility is `public`, `private`, or `unknown`. Bare names work when one owner
matches. New manifests use `owner/name` so absent checkouts retain provenance.
AOS reads the supplied file and never imports private fleet config.

## Run it

```bash
just agents-context-inventory \
  --fleet-manifest /path/to/managed-repos.txt
```

Markdown is the default. `-- --format json` emits the machine contract and
`-- --output PATH` writes either render. `-- --check` fails on absent checkouts,
unknown visibility, or missing root `AGENTS.md`, while retaining the entry.

## Repository boundaries

The report keeps three surfaces distinct:

* **Substrate corpus** - AGENTS files named by the committed substrate manifest.
* **Product corpus** - the supplied managed fleet minus the substrate set.
* **AOSH** - separate, with `global_load: false`, never an implicit global source.
## Paragraph attribution

The tool hashes normalized paragraphs and emits no bodies. Duplicate ownership
prefers the AOS base, then another substrate, before AOSH or a product.

Every paragraph receives one deterministic classification and destination:

* **universal** - global person context.
* **role-specific** - role `COMPOSED.md`.
* **repo-specific-unconditional** - keep in the repository `AGENTS.md`.
* **task-specific** - ordinary skill.
* **generated-pointer** - validator or code.
* **duplicate** - deletion candidate.
* **documentation-only** - repository docs.

Infrastructure #607 lands each reviewed product change through its owner.

## Machine contract

JSON format `agentic-os.agents-context-inventory.v1` is stable and timestamp-free:

* repository aggregates, provenance, presence, root state, and document hashes
* product clipping candidates and recommended destinations
* an explicit AOSH non-global marker.

Context budgeting can import `discover_repositories`, `ContextSelection`, and
`active_cascade`. Adapters still own ingestion and non-AGENTS surfaces.

## Role-reachable context tiers (immediate + peripheral)

`check-context-budget` reports the **proactive** tier (eager prompt bytes, per
harness) documented in [context-budget.md](context-budget.md). A driver also
carries context it can reach one tool call away, cheap to provide but a bigger
haystack to pick wrongly from. Two reusable walkers measure it, per clone rather
than per harness, behind the same `count_tokens` chars/4 proxy.

## The two walkers

- **immediate** - a working-dir clone (`/workspace/<name>`), a grep surface not
  in the prompt. `immediate_walk(repo)` returns a `TierWalk(files, bytes, tokens)`
  over `git ls-files` (tracked only, so an untracked build/vendor tree does not
  inflate it). A tracked path that no longer reads is skipped, so the three
  figures stay coherent.
- **peripheral** - the reference repos (`/substrate/<name>`). `peripheral_walk(repos)`
  applies the same walker per repo and returns a `(total, per-repo)` pair. The
  caller passes the repo set - ward passes its substrate mirrors - so this layer
  never enumerates or names a substrate set of its own.

## Token meaning

For a whole-repo walk the token figure is an upper-bound proxy: a driver greps,
it does not ingest every tracked file. So file count and bytes carry the honest
cheap-to-provide signal, and tokens is the ceiling.

## The aos/ward split

These are measurement primitives for ward's role-aware three-tier probe
(
`docs/context-probe.md`), which calls them for tiers 2/3 and reuses the doc/skill
accounting for tier 1, then adds its container overlays. Per the
authoring-vs-rollout split, aos owns the generic **measurement** and ward owns
the **role / container / spec-schema** model on top. This layer stays
role-agnostic: the CLI flags take paths, never a role or a substrate set.

## CLI

`--immediate REPO` and `--peripheral REPO` are both repeatable. When present they
append a `role-reachable tiers` section below the per-harness blocks, one aligned
`files / bytes / ~tok` row per clone plus a `peripheral TOTAL`.
