# Claude settings guardrails

Back to [features-agents.md](features-agents.md).

The fleet guardrails that live in `~/.claude/settings.json`. Both are authored
here and converged by the `claude-hooks` ansible role in `infrastructure`, per
the authoring-vs-rollout rule in [AGENTS.md](../AGENTS.md).

## Fleet permission denies

`scripts/apply-base-claude-settings.py` appends `BASE_DENIED_PERMISSIONS` to
`permissions.deny`. It never replaces the list, so operator-authored denies and
the sibling `allow` / `ask` / `defaultMode` keys survive a converge untouched,
and a second run reports no change.

Two groups:

* **Live-infrastructure CLIs** - `gcloud`, `kubectl`, `helm`, `terraform`,
  `gsutil`, `mongosh`, `mongo`. Each mutates production or a database, so it
  belongs to an operator or a guarded `aosguard ops` verb, not a raw agent
  shell. The deny is what steers an agent to the guarded surface.
* **Harness memory directory** - `Write` and `Edit` against
  `**/.claude/projects/**/memory/**`. This is the enforcement leg of the
  no-auto-memory rule. `autoMemoryEnabled: false` stops the harness from
  writing memory files; it does not stop an agent from authoring them by hand.

`effortLevel` is deliberately not a fleet key. It tunes latency and spend per
host, which makes it operator-local preference under the config-placement axes,
so it stays hand-edited and no writer owns it.

## Issue-ref Stop hook

`scripts/check-issue-refs.sh` runs as a `hooks.Stop` entry. It blocks an agent
turn whose final reply references an issue or PR as a hash-ref (`owner/repo#N`
or a bare `#NN`) instead of a fully-qualified canonical Forgejo URL, which is
ambiguous after the org migration and breaks tooling. Fenced and inline code are
exempt so the convention can be quoted, and a loop guard passes the turn on the
second stop so an unfixable message cannot wedge the session. Entry-time lines
land in `~/.claude/check-issue-refs.log`, which separates "never fired" from
"fired and passed"; `CHECK_ISSUE_REFS_LOG` overrides the path.

The script lived in `agentic-os-kai` until the `settings-shared.json` merge flow
was retired, which left it with no writer. It moved down here so the fleet
writer and the ansible role could own it (agentic-os-kai#847). The role unwires
the old bridge path before wiring this one, so hosts converged before the
retirement get re-pointed rather than left stale.

## Read-only assertion

`agentic-os-kai/scripts/up-to-date.py` asserts both guardrails are present in
the harness step and reads the expected deny rules from
`BASE_DENIED_PERMISSIONS` rather than restating them. It never writes: a failure
there means the host needs convergence.
