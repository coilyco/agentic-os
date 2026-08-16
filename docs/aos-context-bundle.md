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

AOS alone assembles the independent producers behind this handoff.

## Materialization

AOS starts the selected AOS image as a short-lived materializer. When
`--composed` is active, the helper:

1. asks `agent-compose bundle materialize` for the selected role and harness
2. independently verifies the returned immutable bundle
3. projects that exact verified bundle into an empty private home
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
  "agent": "codex",
  "repositories": ["coilyco-flight-deck/agentic-os", "coilysiren/lore"]
}
```

The manifest seals verified repository identities. See [repository residency](repo-layout.md).

## Lifetime

Completed bundles are content-addressed and made read-only under the user's AOS
cache. AOS reuses identical content. It does not delete a bundle when the host
command returns because a detached Ward container can outlive that process.

## Ward boundary

AOS first confirms that host Ward advertises `--context-bundle`. It invokes the
matching fixed workflow for `director`, `qa`, or `engineer`. Other safe roles
use `ward agent run --role <slug>` with the same immutable bundle.

Ward validates the directory before Docker starts, mounts it read-only,
revalidates it during bootstrap, copies accepted files into private agent HOME,
and appends Ward-owned authority context. Ward keeps bundle tools after the
image's existing PATH, so bundled tools cannot shadow image tools.

Ward maps sealed repositories read-only. See [repository residency](repo-layout.md).

The manifest cannot name permissions, credentials, host source paths, mount
modes, network access, or other capabilities. Ward's broker surface is fixed
and role-independent.

## Ownership

* AOS owns translation, staged-home validation, guarded assembly, and caching.
* Agent-compose owns role context and remains usable through `agent-compose`
  and `acompose`.
* umbra and specgen own generic guarded-tool generation.
* Ward owns runtime policy, Docker Compose, credentials, teardown, and
  authority in warded mode.

## See also

* [aos-cli.md](aos-cli.md) - public launch matrix.
* [aosguard.md](aosguard.md) - generated tool and skill.
