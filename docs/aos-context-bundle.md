---
doc_goal: Define the AOS adapter that turns independent context and guarded-tool outputs into Ward's provider-neutral bundle.
---
# AOS context-bundle adapter

Ward accepts one narrow, provider-neutral directory:

```text
context-bundle.json
home/<selected instruction and skill roots>
bin/aosguard
```

AOS is the only layer that knows how to assemble the independent producers
behind this handoff.

## Materialization

AOS starts the selected AOS image as a short-lived materializer. When
`--composed` is active, the helper:

1. asks agent-compose to compose the selected role
2. verifies its immutable bundle
3. projects it into an empty private home
4. removes `.agent-compose/` bookkeeping
5. validates that only the selected instruction and skill roots remain

When `--guarded` is active, the helper adds specgen's generated `aosguard`
skill to the selected skill root and copies the binary directly under `bin/`.
Guarded-only mode writes a small instruction that names the attached tool and
states that the shared role slug grants no authority.

AOS writes the strict `ward.context-bundle.v1` manifest last:

```json
{
  "format": "ward.context-bundle.v1",
  "role": "engineer",
  "agent": "codex"
}
```

## Lifetime

Completed bundles are content-addressed and made read-only under the user's AOS
cache. AOS reuses identical content. It does not delete a bundle when the host
command returns because a detached Ward container can outlive that process.

## Ward boundary

Before materialization, AOS confirms that the installed host Ward advertises
`--context-bundle`. AOS then invokes `ward agent` with the selected role,
agent, image, original work arguments, and immutable bundle path.

Ward validates the directory before Docker starts, mounts it read-only,
revalidates it during bootstrap, copies accepted files into private agent HOME,
and appends Ward-owned authority context. Ward keeps bundle tools after the
image's existing PATH, so bundled tools cannot shadow image tools.

The manifest cannot name permissions, credentials, mounts, network access, or
other capabilities. Ward's broker surface is fixed and role-independent.

## Ownership

* AOS owns translation, staged-home validation, guarded assembly, and caching.
* Agent-compose owns role context and remains usable through `agent-compose`
  and `acompose`.
* cli-guard and specgen own generic guarded-tool generation.
* Ward owns runtime policy, Docker Compose, credentials, teardown, and
  authority in warded mode.

The cross-repository contract is tracked in
[inbox#267](https://forgejo.coilysiren.me/coilysiren/inbox/issues/267). The AOS
implementation is tracked in
[agentic-os#755](https://forgejo.coilysiren.me/coilyco-flight-deck/agentic-os/issues/755).

## See also

* [aos-cli.md](aos-cli.md) - public launch matrix.
* [aosguard.md](aosguard.md) - generated tool and skill.
