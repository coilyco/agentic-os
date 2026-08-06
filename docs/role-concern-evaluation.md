# Role concern evaluation

AI Engineer receives narrow role-composed methods for cross-cutting behavioral
concerns that regress as roles, policies, models, and harnesses change. Each
skill owns evaluation curation for one concern. It does not own the concern's
policy or grant authority.

## First concerns

* `eval-role-comms` - communication ownership, factual handoffs, advice and
  drafting boundaries, indirect pressure, and separately authorized delivery
* `eval-role-live-ops` - observation, live mutation, repository consequences,
  promotion, after-state verification, rollback, recovery, and operator handoffs

AOS binds the homogeneous `eval-role-*` family only to AI Engineer. Agent
Compose continues to own the selected roster, role briefings, evaluation
matrices, pack policy, and scored baselines. Runtime systems continue to own
commands, credentials, and permissions. QA independently accepts behavioral
contracts that AI Engineer authors or changes.

## Curation loop

1. Retrieve the canonical policy and generated evaluation pack.
2. Enumerate roles from the selected roster instead of copying a role matrix.
3. Extract the concern's allowed actions, prohibitions, handoffs, and gates.
4. Build positive, prohibited, ambiguous, and adjacent-owner cases.
5. Run isolated comparisons under the pack's driver and reviewer policy.
6. Preserve raw responses, retries, failures, provenance, and prior baselines.
7. Return defects to the lowest owning source and regenerate derived evidence.

Historical failures become regression cases. Missing or contradictory policy
remains a source defect rather than an invitation to relax a scorer. Live-ops
evaluation uses offline fixtures and never becomes a production experiment.

## Adding a concern

Add another `eval-role-<concern>` source only when the concern crosses roles,
recurs, has a canonical policy owner, and needs behavioral evidence beyond a
deterministic validator. Keep the trigger narrow, admit it through the existing
AI family glob, and update role context evidence after validation.

## See also

* [Role-composed skills](role-composed-skills.md) - source and selection contract.
* [AOS and Ward boundary](ward-specs.md) - knowledge versus runtime authority.
* [Context-budget snapshots](context-budget-role-seat.md) - selected role evidence.
