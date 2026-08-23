---
name: tooling-sysadmin-incident-command
description: Use when sysadmin coordinates a multi-party production incident. Separates command, operations, communications, and recordkeeping while maintaining objectives and transfer state.
---

# Incident command

Use this skill when service impact or response complexity requires several
people or workstreams to act as one incident organization.

## Activation boundary

Sysadmin activates incident command when impact is severe, expanding, uncertain, or
too broad for one operator. A bounded investigation with one owner stays in
`tooling-sysadmin-live-remediation`.

## Establish command

The incident commander states the current impact, severity rationale, response
objective, and next decision time. She assigns distinct owners for:

* Command, priorities, safety, and final operational decisions.
* Operations, including investigation and mitigation workstreams.
* Communications to affected people and internal stakeholders.
* Scribing the timeline, evidence, actions, decisions, and owners.

One person may hold several roles in a small incident, but each responsibility
remains explicit. The commander coordinates the system. She does not disappear
into a debugging workstream.

## Run operational periods

Each operational period starts with one objective and ends at a named check-in.
Every workstream reports its hypothesis, action, expected signal, owner, and
deadline. The commander stops duplicated work, conflicting mutations, and
unowned questions.

Sysadmin prioritizes life and safety, data integrity, containment, and service
restoration before root-cause completeness. Nonessential changes pause while
the incident is active.

Communications state known impact, current action, unknowns, and the next update
time. Silence and false precision both spend trust.

## Make decisions recoverable

The scribe records why each material action was chosen, its blast radius,
rollback, and result. The commander names explicit abort conditions for risky
mitigation and keeps one current source of incident truth.

Command transfer includes current impact, objective, active workstreams,
pending decisions, next communication, and unresolved risks. Transfer is
acknowledged by both commanders.

## Close deliberately

The commander ends active response only after recovery evidence is stable and
ownership exists for monitoring, customer follow-up, and prevention work. The
incident record separates recovery, root cause, and follow-up hypotheses.

This skill grants no live-system, messaging, or account authority.

## Method provenance

Role separation and transfer discipline draw from Google SRE's
[incident response](https://sre.google/workbook/incident-response/) guidance.

## Evaluation target

Compare cold and composed sysadmin on a noisy outage with three responders and
conflicting fixes. Composed sysadmin should establish command, separate workstreams,
run a decision cadence, prevent simultaneous mutations, and transfer or close
with explicit state.
