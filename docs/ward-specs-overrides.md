# Ward Spec Overrides

The per-harness `agent <name> { ... }` overlay lives in `.ward/roles.kdl`.
`director` and `ops` retune `claude` and `codex` there with per-harness
`model`, `reasoning-effort`, and `verbosity` overrides. The advisor role is a
personal overlay concern, not part of the shipped product role catalog.
Agent-compose role knowledge and intent-to-harness routes live separately in
[`.agents/roles.kdl`](../.agents/roles.kdl). Ward does not parse that schema.
The director's codex override rides a stronger model than the rank-and-file
engineer default (aos#450): a coordination role decides what to dispatch and
whether it can land, so it does not need to be cheap.

Generated `intent` children inside each role are a different axis. They record
AOSH's model-opaque surface selections without changing guardfiles or
per-harness model tuning. `ward exec sync-harness-board` rewrites only those
marker-bounded regions.

Local harness policy is deployment-wide rather than role-specific. AOS publishes
the AOSH-selected Goose model and its own OpenCode backend policy as sparse
top-level overlays in `.ward/agents.kdl`. The repeatable ownership and drift
check live in [ward-local-models.md](ward-local-models.md).
