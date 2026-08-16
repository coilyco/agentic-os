# AOS credential brokering

How a standalone AOS session gets a model credential, per harness.

## Standalone Claude authentication

---
doc_goal: Define fail-closed Claude authentication for standalone AOS launches.
---

Standalone AOS launches default to `--auth=true`. When Claude is selected, AOS
requires supported environment, file-backed, or macOS Keychain credentials
before it starts Docker, mirroring [aos-auth.md](aos-auth.md).

## Host discovery

`ANTHROPIC_API_KEY`, `ANTHROPIC_AUTH_TOKEN`, and `CLAUDE_CODE_OAUTH_TOKEN`
cross the container boundary by name when present, so nothing is projected.

Otherwise AOS resolves `~/.claude/.credentials.json`. The file must be readable
and regular. AOS reports unreadable and non-file credentials without printing
credential values or file contents.

When that file is absent on macOS, AOS reads Claude Code's Keychain record and
writes it to a private temporary file mounted read only. Claude Code namespaces
the record by a digest of `CLAUDE_CONFIG_DIR`, so AOS resolves the service the
same way the harness does: the default directory keeps the bare
`Claude Code-credentials` service and any other directory takes the digest
suffix. See [native-claude-credentials.md](native-claude-credentials.md).

The container copies the mounted file to `~/.claude/.credentials.json`, the
path Claude Code reads.

## Fail closed

An absent credentials file is not an absence of credentials, because macOS
keeps the login in the Keychain. AOS previously treated a missing file as
"nothing to stage" and started the container anyway, which composed the role,
projected it, and only then reported `Not logged in`.

Discovery now fails before Docker starts and names the three ways forward: run
`claude /login` on the host, export `ANTHROPIC_API_KEY`, or pass `--auth=false`
for unauthenticated commands.

A dry run starts no container, so it reports the same message on stderr and
still renders the plan, matching
[aos-auth.md](aos-auth.md#container-staging).

## See also

* [aos-auth.md](aos-auth.md) - the Codex credential path this mirrors.
* [native-claude-credentials.md](native-claude-credentials.md) - the native lend-and-harvest bridge.

## Standalone Codex authentication

---
doc_goal: Define fail-closed Codex authentication for standalone AOS launches.
---

Standalone AOS launches default to `--auth=true`. When Codex is selected, AOS
requires supported environment, file-backed, or macOS Keychain credentials
before it starts Docker.

## Host discovery

`CODEX_API_KEY`, `CODEX_ACCESS_TOKEN`, and the compatibility `OPENAI_API_KEY`
cross the container boundary by name when present. Otherwise, AOS resolves
`auth.json` under `CODEX_HOME`. When `CODEX_HOME` is unset, it uses
`~/.codex/auth.json`, matching Codex's default. The file must be readable,
regular, valid JSON, and contain file-backed API-key or token data. AOS reports
missing, unreadable, non-file, and unsupported credentials without printing
credential values or file contents.

When the file is absent on macOS, AOS reads Codex's direct `Codex Auth`
Keychain record for the resolved `CODEX_HOME`. It writes the returned auth JSON
to a private `0700` temporary directory as a `0600` file, mounts that file
read-only, and removes the directory after the Docker command or dry run ends.
The Keychain value is never printed or placed in Docker arguments or environment
variables.

Codex's encrypted secrets backend stores `secrets/codex_auth.age` on disk and
only its passphrase in the keyring. Standalone AOS does not reimplement that
cryptographic format. It reports the backend as unsupported before Docker
starts. Direct keyring projection is currently macOS-only. Other platforms
require environment or file-backed auth.

## Container staging

The selected host auth projection mounts at `/run/aos/auth/codex.json`
read-only. Container bootstrap copies it into the tmpfs-backed
`$CODEX_HOME/auth.json` with mode `0600`, then hands the ephemeral HOME to
Codex. The host HOME and credential store never mount into the container.

Discovery and validation finish before AOS builds or executes the Docker launch.
Dry-run output contains only the auth path and mount shape, never auth contents.

A dry run starts no container, so it treats unusable credentials as a
diagnostic rather than a wall. AOS reports the same message on stderr and still
renders the plan, which then carries no auth mount because none was staged.
Only a real launch fails closed. A caller that wants neither the projection nor
the report passes `--auth=false`.

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
* [aos-issue-flow.md](aos-issue-flow.md) - bounded Codex check-in.
* [test-harness-composed-roles.md](test-harness-composed-roles.md) - role probes.
