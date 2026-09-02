---
name: tooling-eval-state-machine-verification
description: Use when science verifies a stateful lifecycle, protocol, or workflow. Models states, guarded transitions, invalid events, invariants, reachability, and event sequences before verdict.
---

# State-machine verification

Use this skill when behavior depends on execution history or current state and
isolated input-output examples cannot demonstrate the important paths.

## Activation wall

Science activates this workflow for status lifecycles, approvals, authentication,
multi-step flows, retries, async jobs, protocols, and stateful APIs. Pure
functions and independent input combinations stay in ordinary test design.

## Build the model before the cases

1. Science names the externally meaningful initial, intermediate, terminal, error,
   retry, and recovery states.
2. Science lists each event and the actor or subsystem allowed to produce it.
3. Science records every valid transition as source state, event, guard, destination
   state, observable output, and durable side effect.
4. Science defines the expected response for every invalid state-event pair.
5. Science states invariants that must hold after every transition.

The model describes the rule, not the implementation's internal branches.
Science keeps states coarse enough to explain behavior and splits a state only when
its allowed events or invariants differ.

## Attack the transition graph

Science checks:

* Every declared state is reachable from an allowed initial state.
* Every nonterminal state has a valid path forward or an explicit wait reason.
* Terminal states cannot escape through duplicate, late, or reordered events.
* Every valid transition and important transition pair has coverage.
* Every invalid state-event pair fails without a partial side effect.
* Retries and duplicate events preserve idempotency where the rule promises
  it.
* Guards hold at boundary values and under competing or concurrent events.
* Invariants hold after success, rejection, timeout, cancellation, and recovery.

History-sensitive defects often require a sequence, not one action. Science keeps
the shortest sequence that reaches a failure and removes irrelevant steps.

## Use an independent oracle

Science predicts state, output, and side effects from the model, then compares the
system under test with that prediction. She does not derive the oracle from the
same implementation branch being tested.

When automated generation is worthwhile, science turns events into rules, guards
into preconditions, and rule properties into invariants. The model receives
review before a generator expands it into paths.

## Output rule

Science delivers the state graph or transition list, invariants, invalid-event
rules, covered and uncovered paths, and minimal failing sequences. Ambiguous
transitions are specification gaps, not silent assumptions.

This workflow contributes evidence to `tooling-eval-adversarial-verification`.
It does not replace that skill's release verdict or observable-state wall.

## Method provenance

The rules, preconditions, generated action sequences, and invariant separation
align with [Hypothesis rule-based state machines](https://hypothesis.readthedocs.io/en/latest/stateful.html).
The method remains tool-independent.

## Evaluation target

Compare a cold model and composed science on an order lifecycle whose happy path
passes but whose refund, retry, and cancellation events interact. The composed
Science should expose missing and invalid transitions, find a minimal failing event
sequence, and distinguish a rule gap from an implementation defect.
