# Ward burndown policy

Ward's issue burndown stays deny-by-default until actor-aware admission lands.
Autonomous triage can only run where a repo is explicitly opted in.

## Current containment

- `burndown default=#false` is the fleet default.
- `coilyco-bridge/agentic-os-kai` and `coilyco-bridge/agentic-os-hardware` are the only explicit burndown opt-ins.
- `coilysiren/inbox`, `coilyco-flight-deck/infrastructure`, and `coilyco-bridge/deploy` stay off.
- Public repos stay available for normal issue reporting and human triage, but they do not inherit autonomous execution.

## Rollout order

1. Keep deny-by-default containment in Ward's actor-aware queue filter, with
   the candidate set selected through host-local `director.default-scope`.
2. Add a repo-level external issue-admission setting in Ward.
3. Require the setting to name the approval mode and configured automation actors.
4. Fail closed when the setting is missing or unknown.
5. Re-enable an external tracker only after Ward provenance checks and snapshot tests pass.

## Current inventory

- `coilyco-bridge/agentic-os-kai` - not externally writable from the public Forgejo API, enabled explicitly.
- `coilyco-bridge/agentic-os-hardware` - not externally writable from the public Forgejo API, enabled explicitly.
- `coilysiren/inbox` - externally writable, denied.
- `coilyco-flight-deck/infrastructure` - externally writable, denied.
- `coilyco-bridge/deploy` - not externally writable from the public Forgejo API, denied.
- `coilyco-flight-deck/agentic-os` - externally writable, denied.
- `coilysiren/coilysiren` - externally writable, denied.

## Acceptance trail

- The fleet docs distinguish repository-owner trust from issue-actor trust.
- Every return to autonomous burndown needs the Ward issue provenance checks and snapshot tests to pass.
- The parent program links this inventory and the staged restoration evidence.
