# Native harness configuration

What a native launch projects into each harness before it starts.

## Claude configuration projection

A native session home projects host configuration into
[an isolated workspace](native-agent-workspaces.md). Claude Code needs one step
beyond the ordinary symlink farm, because its config file sits outside the
directory the session projects.

## The asymmetry with Codex

Codex reads `config.toml` from inside `$CODEX_HOME`, so the session's `.codex`
symlink carries it whole. Claude Code reads `.claude.json` from
`$CLAUDE_CONFIG_DIR`, while most installs keep that file at the home root, one
level above the projected `.claude` directory. Staging `.claude` alone
therefore leaves the harness pointed at a path that holds nothing.

Without the link, the session starts on an empty config. It loses folder trust,
the entire MCP server registry, and every recorded onboarding and permission
decision, while the harness reports no error. The MCP projection writes the
host file, so an unlinked session also reads a registry nothing updates.

## What the launcher does

The session home links the host config into its `.claude` directory. One
resolver owns the config-location question for the whole CLI, preferring the
`$CLAUDE_CONFIG_DIR` spelling when it exists and falling back to the home root.
Both the MCP projection and the session staging resolve through it, so
projection and consumption cannot drift apart.

A standalone home copies rather than links, keeping its sealed boundary. It
receives its own config inside `.claude` instead of a view of the host file.

## Folder trust

Trust is keyed by absolute project path, and every session mints a fresh
workspace path, so an accepted dialog never carries forward on its own. The
launcher pre-accepts the paths it just created before the harness starts.

Each path is seeded in both its raw and its symlink-resolved spelling, because
macOS resolves `/var` to `/private/var` and the harness records whichever form
it was launched with. Writers resolve the link before an atomic rename, so the
session keeps a symlink to the host file rather than a divergent copy.

Seeding failure is reported and never blocks a launch. Trust is a convenience,
and a session that prompts is still a working session.

## Credentials

The config link carries onboarding and registry state, not the login itself. On
macOS the OAuth token lives in the Keychain under a service name keyed to
`CLAUDE_CONFIG_DIR`, so a session-scoped config directory never finds it. See
[Claude credential bridging](native-claude-credentials.md).

## Native Codex hook trust

Assigned `acompose <role> codex` launches persist trust for the converged native
Git attribution hook. This removes the repeated `/hooks` review after a new or
changed attribution definition while keeping trust scoped to that one hook.

## Trust flow

AOS starts Codex app-server over its local standard-input transport with the
new shadow's `CODEX_HOME`, then uses the supported `hooks/list` method. A hook
qualifies only when all of these properties match:

* the source is that shadow's `$CODEX_HOME/hooks.json`
* the event is `PreToolUse` with the `Bash` matcher
* the handler is an enabled, non-managed command
* the command exactly matches the converged `agent-git-attribution` path

For an untrusted or modified match, AOS writes Codex's reported hook key and
current hash through `config/batchWrite`. This records the shadow-local source
key that the new Codex process will use. The `hooks.state` edit uses an upsert,
so unrelated trust entries remain unchanged. An already trusted definition
needs no write.

Missing Codex and missing attribution hooks are no-ops. App-server failures
produce a launch warning and preserve Codex's normal interactive review path.
AOS never uses `--dangerously-bypass-hook-trust` or edits Codex's private state
directly.

## Role model profiles

A role can pin the model and effort its Claude seat runs at, in
[`harness-launch-profiles.yaml`](../.agents/harness-launch-profiles.yaml) under
`roles.<role>.harnesses.claude` with `model` and `effort`. A role without the
block keeps the harness default. Model is deployment tuning, so it sits beside
the role's default agent, the one registry AOS already owns
(`teable:coilyco-flight-deck/agentic-os#7835`).

An assigned-role launch, native or containerized, inserts `--model` and
`--effort` directly after the harness. A flag the human typed wins, and so do
`ANTHROPIC_MODEL` and `CLAUDE_CODE_EFFORT_LEVEL`, because Claude Code ranks the
effort variable above the flag. A profile the loader rejects refuses the launch
and names the role, so a seat never quietly falls back to a different model.

A goose seat takes `harnesses.goose` with `provider` and `model`, both required,
exported at a native launch as `GOOSE_PROVIDER` and `GOOSE_MODEL`. Exported env
wins, and the provider must be registered in goose's user config (`goose-config`).

Validation has two layers. The loader checks the shape: claude and goose are the
only harnesses, effort is `low` to `max`, and a claude model is an alias the live check can
resolve (`sonnet`, `opus`, `haiku`, `fable`, plus `[1m]`) or a `claude-*` id.
`aos models check` (`just aos-models-check`) then lists the Anthropic API models
with `ANTHROPIC_API_KEY`. It resolves an alias to the newest id in its family,
fails an absent id or an unsupported effort, and warns when a pinned id has a
newer sibling. It refuses a Bedrock, Vertex, or Foundry session, where aliases
resolve to different models. `--offline` runs the loader alone. The API returns
no retirement dates, so only a scheduled run would catch a retirement. The
schedule is off until an API key exists (`teable:coilyco-flight-deck/agentic-os#7838`).

## Scope

This behavior runs only for caller-assigned Codex launches through `acompose`.
Bare native harness launches retain their existing trust behavior. A changed
attribution definition receives its new current hash on the next assigned
launch.
