# Role-intent harness selection

AOS publishes a model-opaque default harness for every supported role-intent lane.
Harness selection answers which work surface receives a task. Backend model, server, score, fallback, and hardware selection remain separate concerns.

## Confirmed board

The committed board contains thirteen roles and twenty-one lanes:

* **engineer** - `autonomous-coding` uses `openhands`.
* **director** - `strategic-planning` uses `plandex`.
* **qa** - `code-review` uses `aider`.
* **advisor** - `research-synthesis` uses `hermes`.
* **ops** - `ops-investigation` uses `holmesgpt`. `operational-decision` uses `goose`.
* **pm** - `strategic-planning` uses `hermes`. `project-coordination` uses `plane`.
* **designer** - `product-shaping` uses `penpot`. `design-production` uses `aosx`.
* **social** - `message-composition` uses `mixpost`. `channel-publishing` uses `elizaos`.
* **community** - Both intents use the exclusive `sirens-echo` harness.
* **sales** - `research-synthesis` uses `hermes`. `conversation-management` uses `elizaos`.
* **customer-success** - `knowledge-retrieval` and `conversation-management` both use `rasa`.
* **ceo** - `strategic-planning` uses `hermes`.
* **technical-writer** - `knowledge-retrieval` uses `anythingllm`. `message-composition` uses `openwebui`.

Engineer is the sole unattended lane. Every role declares one or two intents,
and every lane has exactly one selected harness.

## Resolve a lane

The released `aos` binary resolves a general role plus explicit intent:

```bash
aos --role director lane-default --intent strategic-planning
```

It emits only `role`, `intent`, `harness`, and the derived logical `route`.
`--profile PATH` writes the same choice to an AOS-owned local profile. See
[local lane profiles](local-lane-profiles.md). The compatibility command:

```bash
aos --role director harness-default --intent strategic-planning
```

It still prints only `plandex`. The resolver never auto-executes a terminal
harness or product surface. A launcher or profile invokes the selected adapter.

## Ownership and synchronization

AOS owns the public
[agent and harness capability registry](../.agents/harnesses.yaml), including
descriptions, links, intents, optional roles, and role eligibility. The
Community allowlist admits only `sirens-echo` without changing shared
intents. AOS also owns the hand-maintained
[role-intent-harness board](../.agents/role-harnesses.yaml). Role semantics and
harness choice are launcher policy, independent of model scoring and hardware.

AOS also owns the generated `intent` children inside each canonical role in
[`.agents/roles.kdl`](../.agents/roles.kdl), the committed agent-compose
provider projection. Ward never parses these composition routes.

The sync also writes [`role-harnesses.json`](../aos/role-harnesses.json) as the
compiled view embedded by the standalone `aos` binary. The JSON does not become
a second hand-owned source. The drift check requires both generated views to
match the AOS capability registry joined with the same AOS routing board.
Run `ward exec sync-harness-board` after the AOS registry or routing board changes. Run
`ward exec sync-harness-board -- --check` for a read-only drift check. Local
pre-commit performs the same self-contained check.

The AOS registry and routing board are always required. Malformed or incomplete
sources fail closed. Each generated KDL region is marker-bounded so
the sync preserves hand-owned composed-skill bindings.
The KDL carries role, intent, and harness identity. The JSON adds schema counts
and role-source provenance. Neither copies backend routing data.

## See also

* [AOS composed-container CLI](aos-cli.md) - released launcher behavior.
* [Local lane profiles](local-lane-profiles.md) - adapter and merge contract.
* [AOSH projections](ward-local-models.md) - authoring-time sync boundaries.
* [Role-composed skills](role-composed-skills.md) - role-scoped knowledge.
