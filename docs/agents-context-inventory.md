# AGENTS context inventory

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
ward exec agents-context-inventory -- \
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

* repository aggregates, provenance, presence, root state, and document hashes;
* product clipping candidates and recommended destinations;
* an explicit AOSH non-global marker.

Context budgeting can import `discover_repositories`, `ContextSelection`, and
`active_cascade`. Adapters still own ingestion and non-AGENTS surfaces.
