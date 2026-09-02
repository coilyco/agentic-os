---
name: coding-shape-iac
description: Category umbrella for infrastructure-as-code. The Ansible / Terraform / Kubernetes trifecta is Kai's full-coverage model - Terraform hits APIs, Ansible converges hosts, Kubernetes orchestrates containers.
---

# coding-shape-iac

Umbrella for any infrastructure-as-code work. Cross-cuts every provisioning and configuration tool. The job of this skill is to route to the right tool by the form of the work, not to relitigate the choice each time.

## The trifecta: full coverage in three legs

Ansible, Terraform, and Kubernetes together cover Kai's whole infrastructure surface. They are not competitors. They split the work by what they act on:

- **Terraform - hits APIs.** Declarative provisioning of anything that lives behind a provider API. AWS (the bulk), Grafana (dashboards, datasources, alerting), Tailscale (ACLs, devices, keys), Cloudflare, and any other service with a provider. If the resource is created by calling someone's API, Terraform is the leg. See `coding-terraform`.
- **Ansible - converges hosts.** Imperative-leaning convergence of OS and service state that is not API-shaped: package installs, config files, daemons, users, the home-lab fleet. The rule from the migration: **if it is not Kubernetes, it is Ansible** for ad-hoc infra in the infrastructure repo. See `coding-ansible`.
- **Kubernetes - orchestrates containers.** Declarative scheduling and lifecycle of containerized workloads. The portable compute layer. See `coding-kubernetes`.

Pick by substrate: API resource -> Terraform, host or service state -> Ansible, containerized workload -> Kubernetes. Most real systems use all three, each on the leg it fits.

## Cross-cutting rules

- **State and secrets stay out of the tree.** Terraform state in S3 + DynamoDB lock, secrets in SSM / native param stores, never literal credentials in a `.tf` / playbook / manifest. Per the configs-in-SSM rule.
- **Plan before apply.** `terraform plan`, `--check` / `--diff` in Ansible, `kubectl diff` / server-side dry-run. Review the diff before mutating anything. Double-confirm destroys.
- **One concern per stack / play / manifest set.** Network, app, observability stay separable. No mega-modules.
- **Pin versions.** Provider constraints, collection versions, image tags and chart versions. No floating.
- **Idempotence is the rule.** Re-running converges to the same state. A tool invocation that is not safe to repeat is a bug.

## Triggers

iac, infrastructure as code, provisioning, configuration management, terraform, ansible, kubernetes, k8s, pulumi, cloudformation, cfn, playbook, manifest, helm, provider, state, converge, declarative infra.

## See also

- `coding-shape-cloud` - the cloud-provider umbrella IaC mostly targets.
- `agentic-os-kai/SSM.md` - where IaC pulls its opaque config and secrets from.
