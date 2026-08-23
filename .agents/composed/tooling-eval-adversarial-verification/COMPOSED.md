---
name: tooling-eval-adversarial-verification
description: Use when eval verifies a change or release candidate. Maps requirements to positive, negative, regression, and unobservable evidence before rendering a verdict.
---

# Adversarial verification

Use this skill when eval must decide whether evidence supports a release,
handoff, or implementation claim.

## Build the evidence map first

For every acceptance requirement, eval identifies:

* Positive evidence - the intended path works.
* Negative evidence - forbidden or malformed inputs fail safely.
* Regression evidence - adjacent established behavior still works.
* Unobservable evidence - the current surface cannot prove the claim.

Eval derives cases from requirements, diffs, failure modes, and trust
boundaries. Eval does not treat the engineer's test list as the complete test
surface.

## Attack the assumptions

Eval looks for ambiguous defaults, partial writes, stale state, ordering
dependencies, duplicate inputs, missing inputs, boundary values, and success
messages emitted before durable success. Eval prioritizes cases that could
produce a confident but false result.

Eval distinguishes a product failure from a test-environment limitation. Missing
access is not evidence that behavior passed or failed.

## Respect the evidence boundary

Repository behavior, local tests, and available CI evidence are observable.
Live deployment state is not observable from a sealed eval surface. Eval records
the exact live check an authorized operator must perform and files an
interactive follow-up when the verdict depends on it.

Eval does not modify the implementation while rendering an independent verdict.
The engineer receives reproducible findings and owns fixes.

## Render a traceable verdict

Eval reports pass, pass with non-blocking findings, or fail. Every blocking
finding cites the requirement, observed evidence, minimal reproduction, and
user impact. Eval calls out untested claims separately from failures.

## Evaluation target

Compare a cold model and a composed eval on a plausible happy-path change. The
composed eval should find meaningful negative and regression cases, separate
unobservable state, and avoid both rubber-stamping and speculative failure.
