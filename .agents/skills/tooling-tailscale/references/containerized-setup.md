# Containerized Tailscale setup

Docker Desktop on macOS and Windows runs Linux containers inside a VM, so a containerized Tailscale builds its interface and routes inside the container's network namespace, never the host route table. The host VPN keeps the default route uncontested while the proxy provides tailnet reach.

Use userspace mode, not TUN: no `/dev/net/tun`, no `NET_ADMIN`, no route. Tailscale exposes a SOCKS5 proxy and you route specific connections through it.

## compose

```yaml
services:
  tailscale:
    image: tailscale/tailscale:latest
    container_name: tailscale-proxy        # fixed name shared with ward
    hostname: tailscale-proxy              # becomes the tailnet node name
    environment:
      TS_AUTHKEY: ${TS_AUTHKEY}            # reusable + ephemeral key, tag:proxy, minted by terraform/tailscale
      TS_USERSPACE: "true"
      TS_SOCKS5_SERVER: 0.0.0.0:1055
    ports:
      - "127.0.0.1:1055:1055"             # SOCKS5, published to host loopback only
    restart: unless-stopped
    networks: [ward-tailnet]               # in-VM consumers reach the box by name here
networks:
  ward-tailnet:
    name: ward-tailnet                     # fixed docker name - shared contract, do not rename
```

This is the standing, shared box. It runs once with `restart: unless-stopped`, and the infra sibling (the `tailscale-proxy` ansible role in `coilyco-flight-deck/infrastructure`) converges it, not ward. The compose lives at `ansible/roles/tailscale-proxy/files/compose.yaml` and is authoritative. This excerpt focuses on its Tailscale service.

Inject the auth key without writing it to disk:

```bash
TS_AUTHKEY="$(ward ops aws ssm get-parameter \
  --name /coilysiren/tailscale-proxy/ts-authkey --with-decryption \
  --query 'Parameter.Value' --output text)" docker compose up -d
```

## Two consumers, one box

The standing box serves SOCKS5 on `0.0.0.0:1055` to two distinct callers, and the compose above wires both:

- **Host tools** reach it on the published `127.0.0.1:1055` loopback. SSH and any host-side client point at that port (see below). The bind stays loopback-only, so nothing off the host can dial it.
- **In-VM containers** that join the `ward-tailnet` network reach it by name as `socks5h://tailscale-proxy:1055`. The fixed docker name is what lets an AOS live-observe surface resolve the box without an IP or SSM lookup. `socks5h` (not `socks5`) hands the hostname to tailscaled to resolve tailnet-side, which is what makes by-name dialing of tailnet peers work from inside the surface.

The second consumer is AOS advisor/director composition. Infrastructure never
starts a proxy per surface. AOS attaches the surface and exports its proxy
environment, while Ward stays transport-neutral and receives no network or
proxy default.

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
ward ops aws ssm get-parameter --name /coilysiren/kai-server/tailnet-ip \
  --with-decryption --query 'Parameter.Value' --output text
```

`ssh kai-server-proxied` now flows ssh -> SOCKS5 (container) -> tailscale userspace -> tailnet -> peer. The host route table is never touched, so the host VPN keeps full control of public egress.
