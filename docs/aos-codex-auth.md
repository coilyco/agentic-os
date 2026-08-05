---
doc_goal: Define fail-closed Codex authentication for standalone AOS launches.
---
# Standalone Codex authentication

Standalone AOS launches default to `--auth=true`. When Codex is selected, AOS
requires supported environment or file-backed credentials before it starts Docker.

## Host discovery

`CODEX_API_KEY`, `CODEX_ACCESS_TOKEN`, and the compatibility `OPENAI_API_KEY`
cross the container boundary by name when present. Otherwise, AOS resolves
`auth.json` under `CODEX_HOME`. When `CODEX_HOME` is unset, it uses
`~/.codex/auth.json`, matching Codex's default. The file must be readable,
regular, valid JSON, and contain file-backed API-key or token data. AOS reports
missing, unreadable, non-file, and unsupported credentials without printing
credential values or file contents.

Codex can store credentials in an operating-system keyring, but AOS cannot
project that store into an ephemeral container. A keyring-only login therefore
needs a file-backed Codex login before an authenticated standalone launch.

## Container staging

The host auth file mounts at `/run/aos/auth/codex.json` read-only. Container
bootstrap copies it into the tmpfs-backed `$CODEX_HOME/auth.json` with mode
`0600`, then hands the ephemeral HOME to Codex. The host HOME and credential
store never mount into the container.

Discovery and validation finish before AOS builds or executes the Docker launch.
Dry-run output contains only the auth path and mount shape, never auth contents.

## Startup and inference proofs

`--auth=false` remains available for deliberate unauthenticated commands and
omits authentication environment variables as well as file staging. The
`aos-standalone-composition-smoke` Ward verb uses it with `codex --version`, so
that smoke proves composition and harness startup only.

`ward exec aos-role-question -- cloud design` uses the default authenticated
path and asks Codex a public-safe question. A successful response proves the
separate authenticated inference boundary.

## See also

* [aos-cli.md](aos-cli.md) - standalone and Ward-governed launch contract.
* [aos-acompose-checkin.md](aos-acompose-checkin.md) - bounded Codex check-in.
* [test-harness-composed-roles.md](test-harness-composed-roles.md) - role probes.
