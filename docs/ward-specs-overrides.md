# Ward Spec Overrides

The per-harness `agent <name> { ... }` overlay lives in `.ward/roles.kdl`.
`director` and `ops` retune `claude` and `codex` there with per-harness
`model`, `reasoning-effort`, and `verbosity` overrides. The advisor role is a
personal overlay concern, not part of the shipped product role catalog.
The director's codex override rides a stronger model than the rank-and-file
engineer default (aos#450): a coordination role decides what to dispatch and
whether it can land, so it does not need to be cheap.
