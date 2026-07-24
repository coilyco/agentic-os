# Fixed Goose context snapshot

The context-budget report measures the committed
`ops / operational-decision / goose` lane before and after the ordinary-skill
refactor. One representative open-source harness keeps the measurement useful
without building short-lived adapters for every harness.

The command asks the local `agent-compose` binary to build the exact
`native-skills` bundle, then reads that verified bundle. It never invokes Goose,
inference, a backend model, hardware, an endpoint, or another live service. The
AOS checkout is the default provider, repository, and CWD. `--provider`,
`--repo`, and `--cwd` make those inputs explicit for fixtures or reproduction.

## Measurement boundary

The snapshot separates:

* **Eager context** - the projected global `.goosehints`, the
  [AGENTS inventory](agents-context-inventory.md) global and root-to-CWD
  cascade, and selected ordinary, personality, role-composed, and optional
  plugin skill frontmatter.
* **Lazy context** - selected skill bodies and resources, optional plugin
  bodies and resources, and the count of deferred mcporter registrations.
* **MCP schemas** - zero eager schemas. The mcporter inventory is recorded as
  deferred registrations without copying configuration or querying a server.

The YAML artifact groups components first by **eager** or **lazy** delivery,
then by component kind. Each row records its owner, stable source path,
delivery path, bytes, token proxy, and content hash. Stable ordering and the
absence of timestamps and absolute source locators make identical inputs
produce the same payload hash.

The shared AGENTS inventory supplies the cascade and preserves a source that
arrives through both global and repository delivery paths as two occurrences.

The durable artifact retains only the deferred MCP count. Server names,
configuration, endpoints, and schemas stay in the private inventory that owns
them. The checked-in
[pre-refactor snapshot](context-budget-goose-before.yaml) is the comparison
point.

## Capture and compare

Capture a replacement pre-refactor snapshot:

```sh
ward exec context-budget -- --goose \
  --snapshot docs/context-budget-goose-before.yaml
```

After the refactor, render the component and total delta:

```sh
ward exec context-budget -- --goose \
  --compare docs/context-budget-goose-before.yaml \
  --snapshot /tmp/goose-context-after.yaml
```

`--skill-root` adds a harness or plugin skill root when the projected runtime
declares one outside the verified bundle. Duplicate skill ids, a missing fixed
route, malformed provider, wrong bundle role, unsafe bundle entry point, or
mismatched comparison lane fail closed.

This mode is not a cross-harness comparison or budget gate. A broader adapter
matrix becomes justified only when the Goose result identifies a decision
another harness must resolve.
