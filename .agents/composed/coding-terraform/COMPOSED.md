---
name: coding-terraform
description: Terraform umbrella skill for AWS infra. Backend state in S3 with DynamoDB lock, region us-east-1. Pinned provider versions, plan-before-apply, for_each over count.
---

# coding-terraform

Umbrella for any Terraform work.

## Defaults

- **Tool**: Terraform (not OpenTofu, unless a project pins it).
- **State backend**: S3 bucket + DynamoDB lock table, both in `us-east-1`. Backend config goes in `backend.tf`, never inline literal credentials.
- **Provider versions**: pinned in `versions.tf` with a `~>` constraint, not floating.
- **Module form**: reusable modules keep variables in `variables.tf`, outputs in `outputs.tf`, a README per module, and pin provider versions. One stack per concern, never a mega-module.
- **Multi-cloud**: Kai's primary is AWS. Azure / GCP / OCI are valid targets when a module needs to be portable.

## Style

- One stack per concern (network, app, observability), not a single mega-module.
- Variables in `variables.tf`, outputs in `outputs.tf`. Don't sprinkle.
- `for_each` over `count` for resource collections (count breaks on reorder).
- `moved {}` blocks before refactoring resource addresses.
- Avoid `terraform import` in scripts - reach for `terraform state mv` or `moved` blocks first.

## Plan-and-apply discipline

- `terraform plan` is read-only and free, run it often.
- `terraform apply` only after the plan output is reviewed by the human. No `--auto-approve` in interactive sessions.
- For destroys, double-confirm. State drift is recoverable, deleted resources often are not.

## When this skill is active

Editing `.tf` files, designing infrastructure, refactoring modules. Inherit conventions before reaching for general Terraform guidance.

## Triggers

terraform, tf, tfstate, tfvars, hcl, opentofu, .tf, terragrunt, provider, backend, module, state, plan, apply, destroy, import.

## See also

- `coding-shape-iac` - the IaC umbrella. Terraform is the API-hitting leg of the Ansible / Terraform / Kubernetes trifecta.
- `coding-aws` - the cloud Terraform mostly targets.
