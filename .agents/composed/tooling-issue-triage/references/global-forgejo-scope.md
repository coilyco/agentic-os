# Global Forgejo triage scope

Use this scope whenever Kai asks for issue triage, triage distribution, a backlog burn-down, or a global pass without explicitly narrowing the request.

## Inventory

* Source - live Forgejo - enumerate every repository across organization and personal namespaces at the start of the run.
* Included - active repositories - do not substitute a gaming-only, single-organization, on-disk, or cached catalog subset.
* Exemptions - visible and explicit - retain archived repositories, repositories with issues disabled, and inaccessible repositories in the report instead of omitting them.
* Narrowing - Kai-owned decision - use a smaller owner or repository scope only when Kai asks for it.

## Distribution

* Denominator - full fleet - sum the authoritative per-repository open counts from [coverage-and-counts](coverage-and-counts.md).
* Breakdown - fleet, owner, repository - report the fleet rollup first, then preserve owner and repository subtotals so concentration and coverage remain visible.
* Rubric - one global classification - repository context may change an issue's classification, but it never changes whether the repository is accounted for.
* Labels - explicit mapping - preserve repository-specific label vocabularies by naming each mapping or gap instead of silently changing the classification standard.

## Cascade and completion

* Parent state - one fleet ledger - keep the complete inventory, denominator, per-repository N, retrieved count, and exemption state outside the workers.
* Fan-out - one repository per worker - use as many capacity-bounded waves as needed without shrinking the fleet scope.
* Coverage - retrieve before classifying - each worker reaches its exact N before returning a classification.
* Completion - reconcile both levels - every inventory entry ends as `retrieved == N`, zero open issues, or an explicit exemption, and every per-repository N sums to the fleet denominator.
* Drift - final live recount - retrieve issues created during the run before declaring the global triage complete.
