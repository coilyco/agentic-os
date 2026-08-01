# Priority-pool resolution

Separate ranking scope from execution ownership. By default, one repository is
one priority pool. Before ranking, check whether the owning organization has an
explicit portfolio-triage declaration in its organization-profile repository.
When it does, retrieve every open issue from every active repository in that
declared pool, score them together, and apply the percentile cuts once across
the aggregate.

Execution still resolves to the repository that owns the affected artifact.
Keep implementation issues, labels, readiness, dependencies, and milestones in
that repository. An organization-level issue may own synthesis and final
closure, but it does not duplicate executable work merely to simulate a
cross-repository milestone.
