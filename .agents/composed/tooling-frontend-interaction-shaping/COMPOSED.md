---
name: tooling-frontend-interaction-shaping
description: Use when frontend turns a chosen product direction into an interaction specification. Defines the shortest flow, meaningful states, recovery, hierarchy, and handoff evidence.
---

# Interaction shaping

Use this skill when frontend must turn a selected product direction into
observable behavior before visual polish or implementation begins.

## Activation boundary

Frontend activates this workflow after the user problem and intended outcome
are understood. Open-ended concept generation stays in
`tooling-tpm-product-brainstorming`. CSS, component implementation, and baseline
accessibility knowledge stay with the engineering workflow.

## Shape the experience

1. Frontend names the actor, situation, goal, and current obstacle.
2. Frontend draws the shortest successful path from entry to durable outcome.
3. Frontend enumerates meaningful loading, empty, partial, error, permission,
   success, and recovery states.
4. Frontend assigns one dominant action and clear information hierarchy to
   each state.
5. Frontend identifies the risky interaction assumption and the cheapest
   prototype or observation that can test it.

Frontend specifies state transitions, not only screens. Every state names its
entry condition, available action, system response, and escape or recovery
path.

## Produce a behavior-first handoff

The interaction specification contains the experience promise, flow, state
inventory, interaction rules, content intent, and unresolved questions.
Frontend writes acceptance statements in observable terms so engineering can
verify behavior without guessing at the design rationale.

Frontend uses visual treatment to communicate priority, grouping, feedback,
and affordance. Decoration without a behavioral purpose does not enter the
handoff.

This skill grants no authority to approve scope or modify production.

## Evaluation target

Compare a cold model and composed frontend on the same rough feature concept.
The composed frontend should expose missing states and recovery paths, reduce
the happy path, identify a testable interaction risk, and hand engineering a
behavior specification rather than a mood board.
