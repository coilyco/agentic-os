---
name: coding-kubernetes
description: Kubernetes umbrella skill. K3s on kai-server is the homelab cluster. kubectl via AOSguard. Helm for packaged apps. Plain manifests over Kustomize.
---

# coding-kubernetes

Umbrella for any Kubernetes work.

## Triggers

kubernetes, k8s, k3s, kubectl, helm, manifest, deploy, statefulset, daemonset, configmap, secret, ingress, namespace, pod, service, pv, pvc, externalsecrets, cert-manager.

## Defaults

- **Cluster**: two K3s clusters, `kai-server` (homelab) and `ser8` (most Actions runners). Each single-node, Tailscale-fronted.
- **kubectl**: route guarded operator work through `aosguard ops kubectl`, which requires `--context kai-server` or `--context ser8` on every call and fails closed without one. Enumerate the live surface with `aosguard ops kubectl describe` or `--help`.
- **Packaging**: Helm for upstream apps with charts. Plain YAML manifests for Kai's own services. Kustomize is fine when it earns its complexity, not by default.
- **Secrets**: ExternalSecrets operator + AWS SSM. No raw `Secret` resources committed to git, ever.
- **Ingress**: Traefik (k3s default).
- **Cert**: cert-manager + Route53 DNS-01 via `/coilysiren/route53/zone-id` (see `SSM.md` in agentic-os-kai).

## Conventions

- Manifests in [`coilyco-bridge/infrastructure`](https://github.com/coilyco-bridge/infrastructure). Apply via the repo's deploy scripts, not ad-hoc `kubectl apply`.
- Namespaces match the service name. One service, one namespace, when reasonable.
- Resource limits set explicitly (cluster is small, OOM evictions are real - see `ops-investigation-k3s-pod-eviction` (in agentic-os-kai)).

## Investigation playbooks

- Pod evictions → `ops-investigation-k3s-pod-eviction` (in agentic-os-kai).
- Cluster upgrades → `ops-investigation-k3s-upgrade-homelab` (in agentic-os-kai).

## When this skill is active

Editing manifests, debugging cluster state, designing a new k8s service. Inherit Kai's homelab posture before generic Kubernetes guidance.
