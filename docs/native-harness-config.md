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
`CLAUDE_CONFIG_DIR`, so every seat shares `~/.claude-seats` and carries its
own load points in as `--add-dir`: [credentials](native-claude-credentials.md).

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

`roles.<role>.harnesses` in [`harness-launch-profiles.yaml`](../.agents/harness-launch-profiles.yaml) pins a seat's model (`teable:coilyco-flight-deck/agentic-os#7835`). `claude` takes `model` and `effort`, inserted as `--model` and `--effort` after the harness. `goose` takes `provider` and `model`, both required, exported as `GOOSE_PROVIDER` and `GOOSE_MODEL`, which a container plan carries as `--env K=V` (`COI-1842`). `codex` takes `model` alone, as a root `-c model=` override. `opencode` takes `provider/model` as `--model` (`teable:coilyco/agentic-os#8439`). A typed flag or exported env (`ANTHROPIC_MODEL`, `CLAUDE_CODE_EFFORT_LEVEL`, the goose pair) wins. A profile the loader rejects refuses the launch.

A pin its source does not list launches on the harness default, with one warning. A pin whose source is unreadable within three seconds launches as pinned and says why (`teable:coilyco/agentic-os#8438`). Claude has no launch-time source, as listing needs an API key and [none reaches a launch](aos-auth.md). Goose asks `/v1/models` on its own `OPENAI_HOST` (Agent Proxy), `openai` provider only, and drops provider and model together. Codex asks `codex app-server` `model/list`. Opencode runs `opencode models <provider>`. A host without the binary launches unchecked.

The loader checks shape: effort is `low` to `max`, and a claude model is an alias (`sonnet`, `opus`, `haiku`, `fable`, plus `[1m]`) or a `claude-*` id. `aos models check` (`just aos-models-check`) checks claude pins with the operational `ANTHROPIC_MODELS_API_KEY`, resolves an alias to its newest id, fails an absent id or unsupported effort, warns on a newer sibling, and refuses Bedrock, Vertex, or Foundry. `--offline` runs the loader alone. Its schedule is off until an API key exists (`teable:coilyco-flight-deck/agentic-os#7838`).

## Claude UI

`aos claude-ui` renders each role's Claude Code theme (`themes/aos-<role>.json`) and settings fragment (`settings.<role>.json`: `custom:` theme, spinner verbs, doctrine tips, subagent status line) from `agent-compose catalog snapshot`. It is byte-identical to the `agent-compose native-ui` renderer it replaces, pinned by fixtures in `aos-cli/testdata/claude-ui/`. The theme math is OKLab: role color on the frame, personality colors on interactions, a lightness-lifted shimmer per pair, and the role color on the nearest of eight fixed subagent slots. Moving it here is part of taking every harness-specific surface out of agent-compose (`teable:coilyco/agent-compose#8199`).

`AOS_LAUNCH_SPEC=1` switches a native launch to spec mode, off by default until a canary passes. aos runs `agent-compose launch --spec-out <file> <role> <harness>`, which composes and projects and then exits without exec. aos then applies the spec's env and the runtime home (`HOME`, `USERPROFILE`, `CODEX_HOME`, `XDG_CONFIG_HOME`, and `CLAUDE_CONFIG_DIR` for claude), and for claude adds `--name <seat>` and `--settings`, rendered as above with the theme installed under the runtime home's `.claude/themes/`. Finally it execs the harness itself. It also narrows the host MCP inventory (`~/.mcporter/mcporter.json`) to the role: untagged servers plus those whose `x-aos.roles` names it, as a `--strict-mcp-config --mcp-config` file for claude, `-c mcp_servers.<name>.enabled=false` overrides for codex, `session --no-profile` plus its builtins and the selected servers for goose, or `OPENCODE_CONFIG_CONTENT` for opencode, as agent-compose's launch does. A tag naming no roster role refuses the launch. Retired role aliases are not resolved in tags. A flag the caller passed wins.

A session staged in a live seat inherits the `AGENT_COMPOSE_LAUNCH` markers agent-compose uses to skip a converged parent. A launch with its own runtime home drops them; on the canonical home it keeps them.

## Scope

This behavior runs only for caller-assigned Codex launches through `acompose`.
Bare native harness launches retain their existing trust behavior. A changed
attribution definition receives its new current hash on the next assigned
launch.
