# AOS connectivity

## AOS standalone connectivity

AOS preserves the operator's MCP connectivity baseline when AOS owns the
container. This bootstrap behavior does not change the selected role's
authority.

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
This mode replaces `ward-tailnet` for these aliases. Bare `aos --role ...`
keeps network ownership.

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
tool discovery and network reach only. `--role` still selects context
and `--composed` still selects agent-compose projection.
