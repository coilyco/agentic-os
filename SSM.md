# AWS SSM Parameter Inventory (agentic-os)

Focused pointer to the SSM parameters `agentic-os` code reads at runtime. The
canonical fleet-wide inventory (with rotation/runbook detail) is the generated
`agentic-os-kai/SSM.md`; this file records only the params this repo's tooling
consumes, next to the code that consumes them. All values are SecureString.
Resolve at runtime via `ward ops aws ssm get-parameter`, never paste an opaque
id or DSN into a tracked file.

## `/sentry-dsn/`

- `/sentry-dsn/tool-failures` - GlitchTip DSN for the shared tool-use
  failure-record feed. Read (cached on first success) by the
  [tool-failure GlitchTip shipper](docs/tool-failures-shipper.md)
  (`ward exec ship-tool-failures`) to emit one Sentry-protocol envelope per
  genuine failure, fingerprint-grouped. Follows the observability skill's
  `/sentry-dsn/<project>` convention. Override the path with
  `TOOL_FAILURES_DSN_SSM_PATH`, or inject a DSN directly (tests, pre-project)
  with `TOOL_FAILURES_DSN`. **Fail-soft**: absent until Kai creates the
  GlitchTip project and populates this param (the one externally-visible step,
  agentic-os#250); the shipper no-ops and the buffer keeps accumulating until
  then.

## `/coilysiren/`

- `/coilysiren/gpg-secret-key` - shared armored GPG secret key imported on demand by `scripts/gpg-ssm` when the configured signing key is not yet local.
- `/coilysiren/gpg-passphrase` - shared GPG signing passphrase fetched on demand by `scripts/gpg-ssm` at sign time.
