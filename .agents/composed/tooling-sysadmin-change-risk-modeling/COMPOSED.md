---
name: tooling-sysadmin-change-risk-modeling
description: Use before an unfamiliar or high-coupling live change. Models failure propagation, correlated scope, reversibility, detection latency, and controls that bound the experiment.
---

# Change-risk modeling

Use this skill before a live change whose failure mode, coupling, or effective
scope is not obvious from the target resource alone.

## Replace intuition with a model

Sysadmin records the intended effect and maps risk across:

* Direct targets, callers, dependencies, shared control planes, and data paths.
* Tenants, regions, failure domains, credentials, and correlated resources.
* Capacity, security, privacy, integrity, and availability consequences.
* Detection latency, observability gaps, rollback time, and irreversible steps.
* Human coordination, automation, and retries that can multiply the change.

The visible resource count is not the blast radius. A one-line configuration
change can cross every failure domain, while a large rollout may remain
partitioned.

## Name the credible failure story

Sysadmin writes the shortest plausible sequence from change to user harm. She names
the assumption that stops propagation at each wall and the evidence that
the wall actually holds.

Sysadmin treats prior success as evidence about similar conditions, not immunity.
Novel dependencies, changed scale, and shared credentials reduce the value of
precedent.

## Bound the change

For each material risk, sysadmin selects one or more controls:

* A canary, tenant subset, region, shard, or rate limit.
* A dry run or read-only probe that validates preconditions.
* A staged checkpoint with an observable hold period.
* An automatic or manual abort signal tied to user impact.
* A tested rollback that remains available after partial success.

If no control can bound an unacceptable failure story, sysadmin stops and escalates
the risk decision. A numeric score does not substitute for that judgment.

## Output rule

The change brief contains the propagation map, failure stories, affected
domains, reversibility limits, detection and abort signals, rollout partitions,
rollback, residual risk, and decision owner.

Execution and before-and-after verification remain governed by
`tooling-sysadmin-live-remediation`. This skill grants no mutation authority.

## Evaluation target

Compare cold and composed sysadmin before a small configuration change with a
shared dependency and slow failure signal. Composed sysadmin should identify
correlated scope, model the harm sequence, partition the rollout, and reject a
nominally reversible plan whose rollback arrives after damage.
