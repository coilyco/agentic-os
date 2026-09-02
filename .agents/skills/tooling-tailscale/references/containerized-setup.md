# Containerized Tailscale setup

Docker Desktop on macOS and Windows runs Linux containers inside a VM, so a containerized Tailscale builds its interface and routes inside the container's network namespace, never the host route table. The host VPN keeps the default route uncontested while the proxy provides tailnet reach.

Use userspace mode, not TUN: no `/dev/net/tun`, no `NET_ADMIN`, no route. Tailscale exposes a SOCKS5 proxy and you route specific connections through it.

## compose

```yaml
services:
  tailscale:
    image: tailscale/tailscale:latest
    container_name: tailscale-proxy        # fixed proxy name used by standalone AOS
    hostname: tailscale-proxy              # becomes the tailnet node name
    environment:
      TS_AUTHKEY: ${TS_AUTHKEY}            # reusable + ephemeral key, tag:proxy, minted by terraform/tailscale
      TS_USERSPACE: "true"
      TS_SOCKS5_SERVER: 0.0.0.0:1055
    ports:
      - "127.0.0.1:1055:1055"             # SOCKS5, published to host loopback only
    restart: unless-stopped
    networks: [ward-tailnet]               # shared Docker network for in-VM consumers
networks:
  ward-tailnet:
    name: ward-tailnet                     # existing shared Docker name, do not rename
```

This is the standing, shared box. It runs once with `restart: unless-stopped`, and the infra sibling (the `tailscale-proxy` ansible role in `coilyco-bridge/infrastructure`) converges it, not ward. The compose lives at `ansible/roles/tailscale-proxy/files/compose.yaml` and is authoritative. This excerpt focuses on its Tailscale service.

Inject the auth key without writing it to disk:

```bash
TS_AUTHKEY="$(aosguard ops aws ssm get-parameter \
  --name /coilysiren/tailscale-proxy/ts-authkey --with-decryption \
  --query 'Parameter.Value' --output text)" docker compose up -d
```

## Consumers

The standing box serves SOCKS5 on `0.0.0.0:1055` to callers that opt in, and the compose above wires the two supported paths:

- **Host tools** reach it on the published `127.0.0.1:1055` loopback. SSH and any host-side client point at that port (see below). The bind stays loopback-only, so nothing off the host can dial it.
- **Standalone AOS launches** that join the `ward-tailnet` network reach it by name as `socks5h://tailscale-proxy:1055`. AOS exports that proxy URL for tools that explicitly support SOCKS, and its bounded MCP bridge sends tailnet HTTP endpoint traffic through this proxy when the host inventory resolves an MCP endpoint into a Tailscale range. `socks5h` (not `socks5`) hands the hostname to tailscaled to resolve tailnet-side when a caller uses by-name dialing.

The infrastructure `tailscale-proxy` role converges the standing userspace proxy. Standalone AOS consumes the existing proxy and network for its bounded MCP connectivity path. AOS neither starts nor repairs the proxy.

## SSH through the proxy

```
# ~/.ssh/config
Host kai-server-proxied
  HostName <RESOLVED_TAILNET_IP>
  User kai
  ProxyCommand nc -X 5 -x 127.0.0.1:1055 %h %p
```

Resolve the peer IP at runtime rather than hardcoding it:

```bash
aosguard ops aws ssm get-parameter --name /coilysiren/kai-server/tailnet-ip \
  --with-decryption --query 'Parameter.Value' --output text
```

`ssh kai-server-proxied` now flows ssh -> SOCKS5 (container) -> tailscale userspace -> tailnet -> peer. The host route table is never touched, so the host VPN keeps full control of public egress.
