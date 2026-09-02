# Claude credentials and settings

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

## Settings guardrails

Back to [features-agents.md](features-agents.md).

The fleet guardrails that live in `~/.claude/settings.json`. Both are authored
here and converged by the `claude-hooks` ansible role in `infrastructure`, per
the authoring-vs-rollout rule in [AGENTS.md](../AGENTS.md).

## Fleet permission rules

`scripts/apply-base-claude-settings.py` appends to `permissions.deny` and
`permissions.allow` and removes only the two `RETIRED_*` lists, so operator
rules and the sibling `ask` / `defaultMode` keys survive, and a rerun no-ops.

Two shut, none open:

* **Live-infrastructure CLIs** - `gcloud`, `kubectl`, `helm`, `terraform`,
  `gsutil`, `mongosh`, `mongo`. Each mutates production or a database, so it
  belongs to an operator or a guarded `aosguard ops` verb, not a raw agent
  shell. The deny is what steers an agent to the guarded surface.
* **Harness memory directory** - `Edit` against
  `**/.claude/projects/**/memory/**`, one rule that binds Write, Edit,
  MultiEdit, and NotebookEdit. `autoMemoryEnabled: false` stops the harness
  writing memory files, and the deny stops an agent authoring one by hand.

`BASE_ALLOWED_PERMISSIONS` is empty. An allow rule must name the tool it widens,
so agentic-os#1165's bare `*` only ever warned at startup and is now retired.

`effortLevel` is deliberately not a fleet key. It tunes latency and spend per
host, which makes it operator-local preference under the config-placement axes,
so it stays hand-edited and no writer owns it.

## Fleet preference

`tui: fullscreen` is the one preference the writer sets beside the guardrails,
because Kai chose the fullscreen renderer as the fleet default rather than a
per-host tuning. A host whose terminal cannot take the alternate screen, such as
iTerm2 under `tmux -CC` or a screen reader, exports `CLAUDE_CODE_NO_FLICKER=0`
in `~/.shellrc.local`: the env var outranks the saved key, so convergence keeps
writing the default and the host keeps ignoring it.

## Read-only assertion

`agentic-os-kai/scripts/up-to-date.py` asserts the remaining guardrails are
present and reads the deny rules from `BASE_DENIED_PERMISSIONS` rather than
restating them.
It never writes, so a failure means the host needs convergence.
