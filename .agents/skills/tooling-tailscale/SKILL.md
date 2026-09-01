---
name: tooling-tailscale
description: Run Tailscale alongside a second host VPN by containerizing it on Docker Desktop for macOS or Windows - userspace/SOCKS5 mode keeps SSH and tailnet reach out of the host route table. Use when two VPNs fight over host routing, or wiring Tailscale SSH to a peer behind a host VPN. Triggers - tailscale, tailnet, vpn coexistence, tailscale ssh, two vpns, wireguard utun conflict, ward-tailnet, tailscale-proxy socks5.
---

# Tailscale

## The host-route conflict

Tailscale and a second WireGuard-based VPN (any commercial WireGuard client - Mullvad, Proton, and similar) cannot coexist natively on one macOS host. Both install a WireGuard-shaped `utun` interface and a default-route override, and macOS has no native arbitration. Whichever toggles last wins and the other silently breaks - no tailnet routing, no public egress, or DNS resolving through the wrong resolver and returning NXDOMAIN. The business-tier products show the same form because they ship the same WireGuard data plane underneath.

**Do not relitigate without new evidence.** Workarounds that did NOT work: pinning the route metric, manually adding scoped routes, running one at a time and toggling on demand (state survives toggles), Tailscale exit-node mode. The native answer is one VPN per host.

## The escape hatch: containerize Tailscale

Run Tailscale in a Docker container in userspace/SOCKS5 mode so its interface and routes live in the container namespace, never the host route table. The host VPN keeps the default route uncontested while you still get tailnet reach. The same Docker Desktop compose rule runs on macOS and Windows.

- [containerized setup](references/containerized-setup.md) - compose file, auth-key injection, SSH-through-SOCKS5 config, and the shared proxy model for host loopback and standalone AOS MCP connectivity.
- [sharp edges](references/sharp-edges.md) - tailnet identity, MagicDNS, IaC key minting, per-connection scope, throughput.

## Why this is here

A real conflict - Tailscale and a host VPN both grabbing the macOS route table - burned several debugging sessions, and the native-coexistence answer turned out to be "you can't." The containerized namespace is the only stable escape hatch found. Recorded here so the finding and the working setup are not re-derived under pressure the next time both VPNs need to run on one host.

## See also

- Official image: `https://hub.docker.com/r/tailscale/tailscale`, env-var reference at `https://tailscale.com/kb/1282/docker`.
- SSM params: `/coilysiren/tailscale-proxy/ts-authkey` (minted by `terraform/tailscale`), `/coilysiren/kai-server/tailnet-ip`. Inventory in `<personal-os-repo>/SSM.md`.
