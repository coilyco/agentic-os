---
name: tooling-tailscale
description: How Tailscale is deployed on the fleet. Native is the default everywhere. Containerizing it on Docker Desktop in userspace/SOCKS5 mode is the fallback for when a second host VPN fights over the route table. Use when checking tailnet reach, diagnosing a peer that will not resolve, or wiring Tailscale SSH. Triggers - tailscale, tailnet down, magicdns, ENOTFOUND, vpn coexistence, tailscale ssh, two vpns, wireguard utun conflict, ward-tailnet, tailscale-proxy socks5.
---

# Tailscale

## Native is the default

**Tailscale runs natively on the fleet most of the time.** The macOS app and
the Linux and Windows daemons hold the tailnet, MagicDNS resolves short peer
names against the tailnet search domain, and nothing about the normal path
involves Docker. Check it with `tailscale status`, which lists
every peer and marks the offline ones.

**Do not diagnose a tailnet problem by looking for a container.** The
containerized proxy below is a narrow fallback for one specific conflict, and
reaching for it first turns a thirty-second `tailscale status` into a hunt for
a Docker daemon that was never meant to be running. Docker Desktop being shut
down is the normal resting state on a host with no VPN conflict.

## MCP servers resolve peer names once, at launch

Every `tailnet_*` MCP server resolves its peer hostname when the harness starts
it, and **none of them retry**. A session that launches while Tailscale is
still settling gets `getaddrinfo ENOTFOUND kai-server` for every tailnet
server, and those servers stay dead for the whole session even after the
tailnet comes up seconds later.

So a wall of ENOTFOUND at session start is a **startup race**, not an outage.
Confirm which one you are looking at before repairing anything:

* `tailscale status` - is the peer actually present and not marked offline
* `dscacheutil -q host -a name kai-server` - does the short name resolve now

When both come back healthy, the tailnet is fine and the MCP servers simply
missed it. Restart the session to reconnect them rather than touching
Tailscale. Nothing you do to the daemon will revive a server that already
failed its one resolution attempt.

## The host-route conflict

Tailscale and a second WireGuard-based VPN (any commercial WireGuard client - Mullvad, Proton, and similar) cannot coexist natively on one macOS host. Both install a WireGuard-shaped `utun` interface and a default-route override, and macOS has no native arbitration. Whichever toggles last wins and the other silently breaks - no tailnet routing, no public egress, or DNS resolving through the wrong resolver and returning NXDOMAIN. The business-tier products show the same form because they ship the same WireGuard data plane underneath.

**Do not relitigate without new evidence.** Workarounds that did NOT work: pinning the route metric, manually adding scoped routes, running one at a time and toggling on demand (state survives toggles), Tailscale exit-node mode. The native answer is one VPN per host.

## The escape hatch: containerize Tailscale

**This section applies only when the conflict above is live.** A host running
one VPN keeps native Tailscale and never needs any of it.

Run Tailscale in a Docker container in userspace/SOCKS5 mode so its interface and routes live in the container namespace, never the host route table. The host VPN keeps the default route uncontested while you still get tailnet reach. The same Docker Desktop compose rule runs on macOS and Windows.

- [containerized setup](references/containerized-setup.md) - compose file, auth-key injection, SSH-through-SOCKS5 config, and the shared proxy model for host loopback and standalone AOS MCP connectivity.
- [sharp edges](references/sharp-edges.md) - tailnet identity, MagicDNS, IaC key minting, per-connection scope, throughput.

## Why this is here

A real conflict - Tailscale and a host VPN both grabbing the macOS route table - burned several debugging sessions, and the native-coexistence answer turned out to be "you can't." The containerized namespace is the only stable escape hatch found. Recorded here so the finding and the working setup are not re-derived under pressure the next time both VPNs need to run on one host.

## See also

- Official image: `https://hub.docker.com/r/tailscale/tailscale`, env-var reference at `https://tailscale.com/kb/1282/docker`.
- SSM params: `/coilysiren/tailscale-proxy/ts-authkey` (minted by `terraform/tailscale`), `/coilysiren/kai-server/tailnet-ip`. Inventory in `<personal-os-repo>/SSM.md`.
