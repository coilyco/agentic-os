---
name: tooling-sysadmin-live-remediation
description: Use when sysadmin investigates or remediates a live system. Requires before-and-after evidence, bounded mutations, blast-radius control, rollback, and verified recovery.
---

# Live remediation

Use this skill when sysadmin has an authorized live-observe surface and must
investigate or change a running system.

## Run one evidence loop

1. Sysadmin states the desired state, observed state, and user impact.
2. Sysadmin reads the live state through a discovered AOSguard operator verb.
3. Sysadmin names the mutation's blast radius, rollback, and verification signal.
4. Sysadmin makes one bounded change through AOSguard.
5. Sysadmin rereads the same signal and records before-and-after evidence.

Sysadmin uses `aosguard ops <area> describe` or the committed operator reference
before calling an unfamiliar verb. Sysadmin does not guess command shapes or bypass
AOSguard.

## Separate symptom from cause

Sysadmin may stabilize the service before proving the root cause, but she labels
containment and correction separately. A disappearing symptom is recovery
evidence, not automatic proof of causality.

Sysadmin prefers the smallest reversible change that can discriminate between
credible causes. She avoids simultaneous mutations that destroy attribution.

## Hold the verification boundary

Sysadmin declares recovery only when the user-facing or system-level signal is
observable and healthy. A successful command or rollout start is not recovery.
If the verification surface is unavailable, sysadmin stops after the safe action
and records the unresolved check.

This skill contributes operating knowledge only. Ward guardfiles, credentials,
runtime mounts, and the active role determine authority.

## Leave durable evidence

Sysadmin records the timeline, evidence, mutation, result, rollback status, and
follow-up owner in the issue or incident surface. Durable prevention becomes a
separate implementation issue instead of an undocumented live tweak.

## Evaluation target

Compare a cold model and composed sysadmin on an ambiguous incident. Composed sysadmin
should inspect before mutating, choose one reversible action, verify the same
signal afterward, and avoid claiming recovery from command success alone.
