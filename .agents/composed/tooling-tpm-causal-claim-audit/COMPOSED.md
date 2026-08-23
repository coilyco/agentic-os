---
name: tooling-tpm-causal-claim-audit
description: Use when tpm evaluates whether evidence supports a causal claim. Forces an estimand, causal graph, identification assumptions, refutation, and calibrated causal language.
---

# Causal claim audit

Use this skill when a recommendation depends on whether an exposure,
intervention, or event causes an outcome rather than merely predicts it.

## Activation boundary

TPM activates this workflow after enough evidence exists to state a causal
question. Ordinary evidence synthesis remains sufficient for descriptive,
predictive, and value judgments. Estimator implementation stays with the
relevant research or coding workflow.

## Define the causal claim

1. TPM names the intervention or exposure, outcome, population, timeframe,
   and comparison that define the estimand.
2. TPM draws the smallest useful causal graph containing the exposure,
   outcome, plausible common causes, mediators, colliders, and selection
   mechanisms.
3. TPM marks which variables are observed, which are latent, and which
   occur before or after the exposure.
4. TPM names the intervention that the causal language implies. A vague
   change such as "more engagement" is not yet a causal treatment.

The graph records assumptions. It does not prove that the arrows are true.

## Hold the identification gate

TPM evaluates the design before the estimate and classifies the evidence as
randomized, quasi-experimental, longitudinal observational, or cross-sectional.

Relevant assumptions may include exchangeability, positivity, consistency,
temporal ordering, no interference, parallel trends, an exclusion restriction,
or absence of manipulation near a cutoff. TPM uses only assumptions the
design actually needs.

If credible alternative graphs produce different effects from the same
observations, tpm reports that the causal effect is not identified. A
precise estimate and a small p-value do not repair an unidentified design.

## Try to refute the result

TPM seeks evidence that could break the favored interpretation:

* Negative controls, placebo outcomes, or placebo treatment dates.
* Sensitivity to an unmeasured common cause or plausible measurement error.
* Alternative graph structures, samples, specifications, and time windows.
* Selection effects, attrition, spillovers, and post-treatment adjustment.
* Temporal and mechanism evidence that discriminates among surviving causes.

TPM distinguishes a failed refutation attempt from proof. Several methods
that share the same identifying assumption do not constitute independent
triangulation.

## Output contract

TPM reports the estimand, causal graph, identifying assumptions, design
strength, refutation results, surviving alternatives, and the smallest new
observation or experiment that could change the judgment.

The conclusion labels the claim as identified, plausibly identified, or not
identified. TPM uses causal language only when the design and robustness
evidence support it. Otherwise she reports an association or prediction.

## Method provenance

The separation of model, identification, estimation, and refutation follows the [DoWhy causal-inference workflow](https://www.pywhy.org/dowhy/).

## Evaluation target

Compare a cold model and composed tpm on an observational study with one
hidden common cause, one collider, and an impressive estimate. The composed
TPM should define the estimand, expose the invalid adjustment, withhold the
causal claim, and propose a decisive refutation or design improvement.
