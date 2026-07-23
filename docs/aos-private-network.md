---
doc_goal: Define AOS's role-scoped private live-observe client composition.
---
# AOS private live-observe composition

Advisor and director are the only AOS roles that receive private-network
access. AOS requires the operator-local `AOS_PRIVATE_NETWORK` and
`AOS_PRIVATE_PROXY` configuration inputs for those roles. It attaches the
container to the former and exports the latter as `ALL_PROXY`, `HTTP_PROXY`,
and `HTTPS_PROXY`, including lowercase variants.

The proxy must use the `socks5h` scheme. That hands private hostname lookup to
the proxy, rather than requiring the container's ordinary DNS resolver to know
the private name.

## Ownership

Infrastructure converges the standing private-network proxy, its credentials,
and its reachability. AOS owns the role selection and conversion of its two
configuration inputs into Docker network and client-environment arguments.
The concrete values live in the AOS configuration contract, not in Ward.

Ward receives no private network, host, port, proxy protocol, destination,
daemon, socket, or lifecycle default. AOS never starts a local daemon, mounts a
socket, or requests TUN or `NET_ADMIN`.

## Failure modes

A missing or malformed AOS input stops an advisor or director launch before
Docker starts. A post-launch connection or private-DNS failure means the
infrastructure-managed proxy or its AOS composition is unavailable. Neither
condition is repaired by starting a daemon in the agent container.

## Safe smoke test

A director can initialize the registered private Bluesky MCP endpoint and list
only its tool names. The expected observation is the exact read-only tool
inventory. Do not print the endpoint, credentials, environment, or Bluesky
content.

## See also

* [AOS composed-container CLI](aos-cli.md) - launcher and base runtime contract.
* [Role surface tiers](role-surface-tiers.md) - role ownership boundary.
* [Features](FEATURES.md) - shipped capability inventory.
