# Role surface tiers

The intended capability surface each `ward agent` container role carries, made
deliberate after the 2026-07-10 permissions rollback stripped the live-observe
surfaces below operational need (agentic-os#447). This is the reviewed
definition. A live surface that differs from it is drift to fix.

## Tiers

* **engineer** - forgejo read + actions read + write - lands code inside its clone, no live infra reach.
* **qa** - forgejo read + actions read + write - inspects a candidate and posts a verdict comment.
* **advisor** - personal overlay role, not shipped in the public bundle - forgejo read + actions read + aws/ssm + AOS private live-observe. The AOS proxy path is the documented default for that overlay.
* **director** - forgejo read + actions read + write + aws/ssm + kubectl + runner-token mint + AOS private live-observe - drives the headless lane and fronts incident recovery.
* **ops** - forgejo read + actions read + write + aws/ssm + kubectl + runner-token mint - live system maintenance, the widest tier.

Runner-token mint is `ward ops forgejo actions generate-runner-token`
([guardfile](../.ward/guardfile.forgejo.runnertoken.kdl)), whose absence
blocked the infrastructure#539 token restore. It stays out of the fleet-wide
[readactions overlay](../.ward/guardfile.forgejo.readactions.kdl): minting a
credential is director/ops reach, not a read.

## Layer ownership

Each capability is owned by exactly one layer, so a one-layer rollback reads
as drift against this map instead of silently disabling a role.

* **guardfile bindings** - [.ward/roles.kdl](../.ward/roles.kdl), authored here - which guarded verb families a shipped role mounts. The Forgejo read/write tiers and the ops guardfiles under [aws](../.ward/guardfile.aws.kdl) are the clearest examples. Ward does not select a private network or proxy.
* **role presets** - ward's tree - tagline, capabilities, modes, posture. Stripped from the aos overlay on 2026-07-10 (commit 566f42f) by design, never re-authored here.
* **image binaries** - [docker/dev-base/](dev-base-image-tiering.md), authored here - whether `aws`, `kubectl`, `helm`, and the Docker client exist on disk. The `ops` role remains a permission and bring-up boundary, not an image tag.
* **private live-observe composition** - [AOS](aos-private-network.md), authored here - advisor and director attach to the infrastructure-managed network and receive the remote-DNS proxy environment. Infrastructure converges the standing proxy and its reachability. Neither Ward nor the role guardfiles own a daemon, socket, TUN capability, network, proxy, or destination.

The observable failure mode is a launch-time error for a missing AOS private
network/proxy input, or a connection failure through that proxy after launch.
Neither is fixed by starting a daemon in the agent container.

## Live-check ownership

Infrastructure owns standing-proxy reachability. A director uses the AOS proxy
path for a read-only private MCP tool-inventory smoke test without logging an
endpoint, credentials, or service content. An unavailable proxy or private DNS
path is an infrastructure/AOS composition failure, not a Ward bring-up task.

## Deferred

Merge authority and commit-status read stay out of this map on purpose: the
ward-native PR workflow capability is foundational ward-side work, and the
specgen also-grant convenience is agentic-os#446.

## See also

* [ward-specs.md](ward-specs.md) - the bundle these guardfiles ship in.
* [dev-base-image-tiering.md](dev-base-image-tiering.md) - the image tier split.
* [ward-ops-forgejo-reference.md](ward-ops-forgejo-reference.md) - the committed forgejo surface render.
