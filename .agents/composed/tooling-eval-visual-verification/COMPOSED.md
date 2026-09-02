---
name: tooling-eval-visual-verification
description: Use when science verifies visual behavior or investigates screenshot diffs. Stabilizes capture, separates pixel change from product defect, and governs baseline evidence.
---

# Visual verification

Use this skill when layout, styling, responsive behavior, or rendered state is
part of the product rule and a screenshot can provide relevant evidence.

## Activation wall

Science activates this workflow for visual regressions and state or viewport
comparisons. Functional behavior, accessibility conformance, and design
judgment remain separate evidence lanes.

## Stabilize the observation

Science records the browser and version, viewport, device scale, theme, locale,
fonts, data state, clock, animation state, and capture wall. She fixes or
masks nondeterministic regions only with a stated reason.

A stable screenshot of the wrong state is not a useful baseline. Science reaches
the state through observable product behavior and records the setup.

## Inspect the evidence triad

Science preserves the reference, current render, and diff. She uses:

* Playwright or the available browser surface to reproduce and capture state.
* `tooling-image-zoom` to inspect ambiguous regions without losing context.
* `tooling-imagemagick` for metadata, montage, and pixel comparison.

Science reads the complete images before crops or metrics. A nonzero pixel diff is
a change signal, not automatically a defect. A low diff can still hide a
critical missing control or clipped label.

## Test the visual rule

Science checks the smallest matrix that covers the risk:

* Critical states including loading, empty, error, success, and destructive
  confirmation where applicable.
* Supported viewport boundaries and content lengths likely to reflow.
* Focus, hover, selection, disabled, and validation states when visually
  meaningful.
* Overlays, stacking, clipping, overflow, and content that can obscure action.

She classifies a finding as intended change, regression, baseline defect,
capture instability, or unresolved ambiguity. Severity follows user impact
and recoverability, not changed-pixel count.

## Govern baselines

Science never updates a baseline merely to make a check green. An authorized review
must identify the intended change and approve the new reference. Broad churn
triggers root-cause analysis before mass acceptance.

## Output rule

The report contains capture conditions, state matrix, evidence triads,
classification, user impact, confidence, unstable regions, and baseline
decision. This skill grants no baseline approval or production authority.

## Method provenance

Capture and baseline mechanics align with Playwright's
[visual comparisons](https://playwright.dev/docs/test-snapshots).

## Evaluation target

Compare cold and composed science on a responsive page with font drift, a clipped
primary action, and an intentional color change. Composed science should stabilize
capture, distinguish all three causes, use crops and metrics as evidence, and
refuse an unreviewed baseline update.
