---
name: tooling-sysadmin-dynamic-systems-diagnosis
description: Use when sysadmin diagnoses ambiguous, intermittent, or distributed system behavior. Builds competing causal models, chooses discriminating signals, and preserves attribution.
---

# Dynamic systems diagnosis

Use this skill when one symptom has several credible causes or behavior changes
over time, load, topology, retries, or dependency state.

## Activation wall

Sysadmin activates this workflow for ambiguous degradation, feedback loops,
intermittent failures, and cross-service effects. A known failure with a proven
runbook can proceed directly to `tooling-sysadmin-live-remediation`.

## Build competing models

Sysadmin records:

* The user impact, observed symptom, time window, and affected wall.
* Recent changes and dependency events without treating correlation as cause.
* At least two causal models that could produce the same observation.
* The signal each model predicts and the observation that would weaken it.
* Missing telemetry or inaccessible state that limits confidence.

Sysadmin includes queues, caches, retries, timeouts, saturation, failover, and
control loops when they can amplify or delay the visible symptom. She traces
both the request path and the resource path because failures can propagate
differently through each.

## Choose discriminating evidence

Sysadmin prefers the least invasive observation or reversible probe that separates
the leading models. She changes one variable at a time, predicts the result
before the probe, and records negative evidence as carefully as confirmation.

A useful probe changes the relative probability of competing causes. More logs
are not automatically more information. Sysadmin stops broad log hunting when the
next observation cannot change the decision.

## Preserve time and attribution

Sysadmin aligns signals by event time and accounts for aggregation windows,
sampling, delayed effects, and recovery lag. She does not infer causality from
two metrics sharing a chart form.

Containment may precede root-cause proof. Sysadmin labels the current result as
symptom relief, causal evidence, or confirmed correction.

## Output rule

The diagnosis brief contains the system wall, timeline, competing models,
predictions, discriminating evidence, current confidence, and next safe probe.
Any live mutation still follows `tooling-sysadmin-live-remediation` and the active
Ward authority.

## Method provenance

The hypothesis, test, and negative-result loop follows Google SRE's
[effective troubleshooting](https://sre.google/sre-book/effective-troubleshooting/)
method.

## Evaluation target

Compare cold and composed sysadmin on latency that could arise from saturation,
retry amplification, or one dependency. Composed sysadmin should model competing
causes, select a discriminating signal, preserve time ordering, and avoid
equating correlated telemetry with proof.
