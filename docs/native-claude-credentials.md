# Claude credentials and settings

Claude Code keys its macOS Keychain login to `CLAUDE_CONFIG_DIR`, so every
distinct config directory is a distinct login. This page records how native
seats share one, and what the launcher still seeds for the host.

## Why the Keychain cannot follow a session

Claude Code namespaces the Keychain item by a digest of `CLAUDE_CONFIG_DIR`. The
default directory keeps the bare service name `Claude Code-credentials`, and any
other takes a suffix of the first four bytes of the SHA-256 of the exact path.

A session-scoped config directory is therefore a login nobody has done. Linking
the host `.credentials.json` into it did not bridge that: every seat refreshed
the same snapshot on its own, the first refresh consumed the single-use refresh
token, and every other live seat, plus every later launch, failed to refresh and
asked for a login. The harness deletes the link when the refresh fails, and a
rotation reached the host only when a session was reaped (COI-1099).

## One shared seats directory

Every native Claude seat launches with `CLAUDE_CONFIG_DIR=~/.claude-seats`, one
path string on the host, so every seat reads and writes one Keychain item and
Claude Code's own refresh handling coordinates them. The host's bare item,
plain `claude`, stays separate: the seats pay one login per refresh lifetime
between them, and plain `claude` pays its own.

The directory is a symlink farm over `~/.claude`, built on first use and topped
up each launch with entries the host grew since. Three entries never link:
`CLAUDE.md` and `skills`, which are per seat, and `.credentials.json`, which the
shared Keychain item replaces. `skills` is a real, empty directory, so the role
filter keeps binding. `.claude.json` links to the host config, where folder
trust and the external-import approval are seeded per session path.

The two per-seat load points agent-compose projects into the session home,
`.claude/CLAUDE.md` and `.claude/skills`, are linked into `<session
root>/seat/.claude/` and handed to the harness as `--add-dir=<session root>/seat` with `CLAUDE_CODE_ADDITIONAL_DIRECTORIES_CLAUDE_MD=1`, which is how
Claude Code loads a `CLAUDE.md` from an additional directory. Nothing moves:
the shadow home is staged exactly as before, and a standalone (sealed) home
keeps its own config directory.

agent-compose exports `CLAUDE_CONFIG_DIR` from the runtime home at exec, so aos
hands it the shared path as `AGENT_COMPOSE_CLAUDE_CONFIG_DIR`, which the
agent-compose exec and aos's own spec-mode exec both honor and then drop.

## The host login

The seats directory is seeded once, when it is created, with the host
`~/.claude/.credentials.json` if that file is usable, so the first seat refreshes
the host login instead of prompting. After that the file is the harness's to
migrate into the Keychain and delete, and no launch reseeds it.

The host file itself is still seeded at launch from the bare Keychain item when
it cannot carry a launch, never over a usable one (`#7234`, `#7258`): usable
means a token to present, unlapsed or backed by a live refresh token, and `aos
_native-shadow --credential` calls a stamped file with neither token **hollow**.
Container launches under `--auth` read that file.

## Reaping older sessions

A session staged before the shared directory still holds its own Keychain item.
Reap reads the per-session file or item, writes it to the host file only when it
is worth more (a token beats none, a live refresh token next, `expiresAt` last),
then removes the item, never the bare host service. A session on the shared
directory has neither, so both reads are no-ops.

No secret crosses argv: `find-generic-password` returns the payload on stdout,
`delete-generic-password` carries none, and files are written at `0600`. The
Keychain paths are macOS only; Linux keeps the login in the file.

## Settings guardrails

Back to [features-agents.md](features-agents.md).

The fleet guardrails that live in `~/.claude/settings.json`. Both are authored
here and converged by the `claude-hooks` ansible role in `infrastructure`, per
the authoring-vs-rollout rule in [AGENTS.md](../AGENTS.md).

## Fleet permission rules

`scripts/apply-base-claude-settings.py` appends to `permissions.deny` and
`permissions.allow` and removes only the two `RETIRED_*` lists, so operator
rules and the sibling `ask` / `defaultMode` keys survive, and a rerun no-ops.

One shut, two open:

The live-infrastructure CLI denies (`gcloud`, `kubectl`, `helm`, `terraform`,
`gsutil`, `mongosh`, `mongo`) are **retired**, Kai's call 2026-09-14: a deny
matched the command string, missing `just <verb>` while blocking the direct call. A per-role launch `--settings` deny that brought bare `kubectl` and `helm` back (`teable:coilyco/agentic-os#8282`) was removed too.

* **Harness memory directory** - `Edit` against
  `**/.claude/projects/**/memory/**`, one rule that binds Write, Edit,
  MultiEdit, and NotebookEdit. `autoMemoryEnabled: false` stops the harness
  writing memory files, and the deny stops an agent authoring one by hand.

`BASE_ALLOWED_PERMISSIONS` carries two ssh host rules, Kai's call 2026-09-20.
Both miss a flagged `ssh -o ...`: `teable:coilyco-flight-deck/agentic-os#7992`.

It also carries an exact `describe` rule for `forgejo` and `forgejo-admin`
(COI-2659). Whether they skip the classifier is unverified until a host converges.

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
