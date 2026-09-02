---
name: coding-ansible
description: Ansible umbrella - the host-convergence leg of the Ansible / Terraform / Kubernetes trifecta. If it is not Kubernetes, it is Ansible for ad-hoc infra. Collection canon, the role-vs-collection-vs-builtin decision rule, and Galaxy registry notes.
---

# coding-ansible

Umbrella for any Ansible work. Ansible is the **host-convergence leg** of the trifecta (see `coding-shape-iac`): it converges OS and service state that is not API-shaped. Terraform hits APIs, Kubernetes orchestrates containers, Ansible does the rest.

## The migration wall

Kai set up Ansible across the home-lab fleet and is migrating ad-hoc infrastructure-repo work onto it, one piece at a time. The rule:

**If it is not Kubernetes, it is Ansible.** Imperative shell-out scripts, hand-rolled `ssh` loops, and one-off provisioning in the infrastructure repo become playbooks. Containerized workloads stay in Kubernetes. API-provisioned resources stay in Terraform.

## The decision rule: builtin vs collection vs role

Reach in this order, stop at the first that fits:

1. **`ansible.builtin`** - ships with core. The Unix primitives: `package`/`apt`/`dnf`, `copy`, `template`, `file`, `service`/`systemd`, `user`, `lineinfile`, `command`/`shell`. Do not go to Galaxy for these.
2. **`ansible.posix`** - mount, sysctl, firewalld, authorized_key, selinux. Core-adjacent, no Galaxy hunt needed.
3. **A canonical collection** - a vendor- or community-owned pack of modules for a specific system. See [the collection canon](references/collection-canon.md).
4. **A role** - a packaged unit that installs and configures one service end to end. Best when a mature role already does exactly what you need (the geerlingguy family covers most home-lab services). Prefer a collection's modules when you need composability, a role when you want a batteries-included install.

## Collection canon and the registry

The collections worth standardizing on (by tech), the known ecosystem gaps (Vector, OpenTelemetry), and how to query the open-source Galaxy registry all live in [the collection canon reference](references/collection-canon.md).

## Defaults

- **Idempotence is the rule.** Every play is safe to re-run. Use `--check --diff` before applying for real.
- **Inventory and secrets** - inventory in the infra repo, secrets via SSM lookup or vault, never plaintext in the tree. Per configs-in-SSM.
- **Roles over loose tasks** for anything reused. `roles/` per service, `group_vars` for config.
- **Pin collection versions** in `requirements.yml`, install with `ansible-galaxy collection install -r`.

## Triggers

ansible, playbook, ansible-galaxy, galaxy, collection, ansible role, requirements.yml, inventory, group_vars, ansible.builtin, ansible.posix, geerlingguy, idempotent, converge host, ansible-vault.

## See also

- `coding-shape-iac` - the IaC umbrella and the trifecta framing.
- `coding-kubernetes` - the other half of the wall (containerized workloads stay here).
- `coding-terraform` - the API-hitting leg.
