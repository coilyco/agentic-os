# AOS cluster access

The two credentials a warded session needs to reach the cluster.

## AOS ward credentials

## AOS to Ward credential handoff

AOS owns the coilyco deployment source for Ward's Forgejo broker credential.
Ward owns the broker, its authorization policy, lifecycle, and agent isolation.
Ward never learns how the deployment obtains the credential.

## Launch behavior

For an integrated `aos --warded` launch:

1. AOS uses a non-empty host `FORGEJO_TOKEN` as an explicit override.
2. Otherwise, AOS reads the deployment parameter through the host AWS session.
3. AOS supplies the value only in Ward's privileged process environment.
4. Ward keeps the raw value in its sibling broker.
5. Ward removes the value from the selected agent harness environment.

The credential stays in process memory. It never enters argv, dry-run output,
the context bundle, tracked configuration, or agent-visible files.

The direct host AWS read is a credential-bootstrap exception. AOS must obtain
the credential before Ward can expose any guarded operator surface. AWS and SSM
remain deployment details in AOS rather than dependencies in Ward.

An absent or unreadable deployment credential fails the launch before Ward
starts. The operator refreshes the host AWS session or supplies the explicit
environment override. AOS never asks the operator to paste the token.

## Boundaries

* Ward remains provider-neutral and accepts the credential as a launch input.
* AOSguard keeps its independent umbra-owned credential mounts.
* Standalone AOS launches do not gain this Ward broker credential.
* Agent-compose context bundles carry no credential or Ward authority.

## AOS kubeconfig

## AOS kubeconfig projection

AOS can project one operator-selected host kubeconfig into the full compatibility
container when AOS owns the standalone runtime:

```bash
aos \
  --agent codex \
  --role sysadmin \
  --composed \
  --kubeconfig "/operator configs/homelab.yaml"
```

There is no default host path. The operator must select the source explicitly.
AOS resolves the path, requires a readable regular file, and verifies that it is
a single structurally valid Kubernetes `v1` `Config` document before Docker
starts.

## Role boundary

Every standalone role may receive the selected kubeconfig. The explicit
`--kubeconfig` flag is the only gate, so a launch either projects the source it
was handed or fails loudly on a missing, malformed, or irregular file. No role
slug narrows that, `engineer` and `qa` included.

The mount belongs to the standalone AOS runtime. It does not transfer a
permission into Ward, agent-compose, or AOSguard merely because those layers
use the same role slug.

`--kubeconfig` is rejected with `--warded`. Ward owns warded runtime mounts, so
AOS does not pass an operator-local host path across that lifecycle boundary.

## Container projection

AOS passes the selected path to Docker as one bind-mount argument, so host paths
containing spaces remain intact. The file mounts read-only at:

```text
/run/aos/kubeconfig
```

The child process receives:

```text
KUBECONFIG=/run/aos/kubeconfig
```

The mount exposes the selected document and nothing else from the host
Kubernetes configuration directory.

## Binary, credentials, and transport

The full dev-base image ships the `kubectl` client binary. A binary in an
image is not live cluster access. Access exists only when container bring-up
also supplies all of the following:

* an authorized standalone role
* an explicitly selected kubeconfig
* network transport that can reach the kubeconfig's cluster endpoint

Private-network composition remains independent. AOS can attach the container
to `ward-tailnet` and expose `AOS_TAILNET_SOCKS5` alongside the kubeconfig
mount. The kubeconfig or invoked client still has to select the appropriate
proxy or otherwise use a reachable endpoint. Kubeconfig projection neither
starts nor repairs private-network infrastructure.
