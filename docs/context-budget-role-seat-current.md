# Current role-class context inventory

The 12 canonical roles are measured in every compatible AOS native layout.
Claude and Codex use the frontier catalog. Goose and OpenCode use the
low-context catalog. CEO is frontier-only. Every entry uses the deterministic
characters-divided-by-four token proxy. The role reports and available
frontier-to-low-context diffs are generated from those snapshots with
`ward exec gen-context-budget-role-reports`.

## Roles

* [Director](context-budget-role-director-current.md) - frontier and low-context.
* [Engineer](context-budget-role-engineer-current.md) - frontier and low-context.
* [QA](context-budget-role-qa-current.md) - frontier and low-context.
* [Advisor](context-budget-role-advisor-current.md) - frontier and low-context.
* [Ops](context-budget-role-ops-current.md) - frontier and low-context.
* [PM](context-budget-role-pm-current.md) - frontier and low-context.
* [Designer](context-budget-role-designer-current.md) - frontier and low-context.
* [Social](context-budget-role-social-current.md) - frontier and low-context.
* [Community](context-budget-role-community-current.md) - frontier and
  low-context.
* [Sales](context-budget-role-sales-current.md) - frontier and low-context.
* [Customer success](context-budget-role-customer-success-current.md) -
  frontier and low-context.
* [CEO](context-budget-role-ceo-current.md) - frontier only.
* [Technical writer](context-budget-role-technical-writer-current.md) -
  frontier and low-context.

Goose and OpenCode have equal token totals for roles that support both because
each receives the same catalog and AGENTS cascade. Their projection paths and
payload hashes remain distinct.

## Interpretation

The AGENTS cascade is the dominant eager cost in every seat. Low-context
selection substantially cuts lazy retrieval and trims eager routing metadata,
but the inherited AGENTS surface keeps every OSS role above the legacy
5,000-token generic harness budget. Role-composed metadata ranges from a small
advisor set through the broad director catalog. Lazy totals describe available
retrieval, not startup prompt load.

## See also

* [Role-seat snapshot contract](context-budget-role-seat.md) - capture,
  comparison, and failure rules.
* [Context-budget report](context-budget.md) - component definitions and token
  proxy.
