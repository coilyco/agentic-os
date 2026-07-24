# AWS SSM Parameter Inventory (agentic-os)

Focused pointer to the SSM parameters `agentic-os` code reads at runtime. The
canonical fleet-wide inventory (with rotation/runbook detail) is the generated
`agentic-os-kai/SSM.md`; this file records only the params this repo's tooling
consumes, next to the code that consumes them. All values are SecureString.
Resolve at runtime via `aguard ops aws ssm get-parameter`, never paste an opaque
id or DSN into a tracked file.

## `/coilysiren/`

- `/coilysiren/gpg-secret-key` - shared armored GPG secret key imported on demand by `scripts/gpg-ssm` when the configured signing key is not yet local.
- `/coilysiren/gpg-passphrase` - shared GPG signing passphrase fetched on demand by `scripts/gpg-ssm` at sign time.

## `/forgejo/`

- `/forgejo/coilyco-ops/api-token` - Forgejo token used by `scripts/git-credential-forgejo-ssm.sh` for HTTPS git authentication.
