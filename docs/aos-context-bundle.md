# AOS context bundle and connectivity

Ward accepts one narrow, provider-neutral directory:

```text
context-bundle.json
home/<selected instruction and skill roots>
bin/aosguard
```

AOS alone assembles the independent producers behind this handoff.

## Materialization

AOS starts the selected image as a short-lived materializer. Under
`--composed` it asks `agent-compose bundle materialize` for the role and
harness, independently verifies the returned immutable bundle, projects that
exact bundle into an empty private home, removes `.agent-compose/` bookkeeping,
and validates that only the selected instruction and skill roots remain.

Under `--guarded` it adds umbra's generated `aosguard` skill to the selected
skill root and copies the binary under `bin/`. Guarded-only mode writes a small
instruction naming the attached tool and stating that the shared role slug
grants no authority.

AOS writes the strict `ward.context-bundle.v1` manifest last, carrying
`format`, `role`, `agent`, and a `repositories` list. The manifest seals
verified repository identities. See [repository residency](repo-layout.md).

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

AOS owns translation, staged-home validation, guarded assembly, and caching.
Agent-compose owns role context and stays usable through `agent-compose` and
`acompose`. umbra owns generic guarded-tool generation, and Ward owns
runtime policy, Compose, credentials, teardown, and warded-mode authority.

## AOS standalone connectivity

AOS preserves the operator's MCP connectivity baseline when AOS owns the
container. This bootstrap behavior applies without `--warded` and does not
change the selected role's authority.

## MCP inventory

AOS discovers the resolved host `~/.mcporter/mcporter.json` when that file is
present. The launcher mounts only that inventory read-only. It does not mount
host HOME, OAuth caches, or unrelated harness configuration.

Container bootstrap uses the AOS projector after projecting harness defaults.
Claude and Codex receive native MCP registrations. Every harness receives the
mcporter inventory as an on-demand CLI fallback. The same projector backs
[native environment convergence](aos-convergence.md), so staged and host
registries share one validation and rendering contract.

## Tailnet HTTP endpoints

AOS joins the shared `ward-tailnet` Docker network when it exists and exports
`AOS_TAILNET_SOCKS5=socks5h://tailscale-proxy:1055` for tools that support
SOCKS without a global proxy.

It also resolves each HTTP MCP hostname on the host. For an address inside a
Tailscale range it rewrites that endpoint in the ephemeral inventory to a
private loopback port, starts an unprivileged TCP bridge, and sends bridge
traffic through `tailscale-proxy:1055`. The bridge sends the resolved tailnet
address to the proxy, so it does not depend on proxy-side MagicDNS, and public
endpoints keep their URLs.

Bridging supports `http` only. AOS rejects a detected tailnet `https` endpoint
rather than silently breaking TLS hostname validation.

## Host network

`aoscompose` and `aoscomposed` launch with Docker host networking. Private tools
should bind `127.0.0.1:<port>`, which opens at that URL from the host browser.
This mode replaces `ward-tailnet` for these aliases. Bare `aos --role ...` and
`aosward` keep network ownership.

## Outage behavior

MCP registration remains visible when the standing proxy is cycling. Calls to
tailnet-only endpoints fail until infrastructure restores the proxy. A running
standalone agent can use the bridge after the proxy returns because each bridge
dials the proxy for every new connection.

The infrastructure tailscale-proxy role owns proxy convergence. AOS neither
starts nor repairs that service.

## Kubeconfig coexistence

An authorized standalone launch can add the
[operator-selected kubeconfig mount](aos-cluster-access.md) while retaining the
same MCP inventory, `ward-tailnet` attachment, SOCKS environment, and endpoint
forwarders. Kubeconfig projection does not infer cluster transport from the
credential document.

The host inventory is operator configuration, not role policy. Projection adds
tool discovery and network reach only. `--role` still selects context,
`--composed` still selects agent-compose projection, and Ward remains the only
owner of Ward runtime policy when `--warded` is present.
