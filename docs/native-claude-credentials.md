# Claude credential bridging

A native session home is session-scoped, and Claude Code keys its macOS Keychain
credential to that home. Without a bridge the harness starts logged out on every
launch. This page records why, and what the launcher does about it.

## Why the symlink farm cannot cover it

[The Claude configuration projection](native-harness-config.md) links
`.claude.json` into the session home, so folder trust, the MCP registry, and
onboarding state all carry forward. Credentials do not travel that path. On
macOS the OAuth token lives in the login Keychain, and no file in the projected
home holds it.

Claude Code namespaces that Keychain item by a digest of `CLAUDE_CONFIG_DIR`.
The default config directory keeps the bare service name
`Claude Code-credentials`, and any other directory takes a suffix of the first
four bytes of the SHA-256 of the exact path string.

Every native session mints a fresh id, so `CLAUDE_CONFIG_DIR` differs on every
launch, which yields a service name that has never been written. The harness
reports no error. It simply asks the operator to log in, stores the result under
the session-scoped service, and abandons it when the session directory is
reaped. Repeated launches accumulate stranded Keychain items.

## What the launcher does

The launcher lends the credential in and takes it back.

At session creation, when the harness is Claude and the session has its own
home, the launcher reads the host service and writes the same secret under the
session service. The resolved session service is recorded on the lease.

At the next startup, cleanup sees the finished session, reads whatever the
session service now holds, writes it back to the host service, and deletes the
session item. The harvest happens as soon as a session is observed dead, ahead
of the worktree grace period, because the next launch needs the refreshed value
rather than the one lent out a day earlier. Deleting the session item on harvest
is what keeps orphans from accumulating.

The write-back matters because a refresh token normally rotates when it is used.
A session that never handed its token back would leave the host holding a value
that the provider has already retired.

Both directions are warnings, never launch blockers. A lost token costs one
login. A cleanup that refuses to finish costs every later launch.

## Boundaries and tradeoffs

The bridge is macOS only. Linux keeps credentials in a file inside the config
directory, which the projected home already carries, and Windows credential
storage is not wired up here.

`/usr/bin/security` accepts a secret on argv or from a terminal prompt, and a
launcher cannot drive the prompt. The token therefore crosses on argv for the
duration of one call. macOS restricts another user's argv, so the exposure is a
same-user read during that window. This is a deliberate exception to the
argv-secret rule, and it is confined to this one call.

Two live sessions that each refresh their token will each write back when they
are reaped, and the later harvest wins. The earlier session's rotation is lost,
which costs one login.

A session home that resolves to the host config directory is left alone. Lending
onto the host service would make the later harvest delete the host login.

## Related

* [Claude configuration projection](native-harness-config.md) - the `.claude.json`
  link and folder trust seeding.
* [Native agent workspaces](native-agent-workspaces.md) - session home and lease
  lifecycle.
