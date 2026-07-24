---
name: tooling-advisor-evidence-synthesis
description: Use when advisor turns research into a decision-grade conclusion. Builds claim-level evidence, calibrated uncertainty, disagreement analysis, and actionable implications.
---

# Evidence synthesis

Use this skill when advisor has research material and must tell the decision
maker what the evidence supports, what remains uncertain, and why it matters.

## Activation boundary

Advisor activates this workflow for synthesis, not raw retrieval. A request
for sources, current facts, or document collection stays in the relevant
research or retrieval workflow until enough evidence exists to compare claims.

## Build a claim ledger

For every consequential claim, advisor records:

* The precise statement the evidence is supposed to support.
* The strongest direct evidence and its source class.
* Relevant freshness, scope, and population limits.
* Counterevidence or a credible competing explanation.
* Confidence as high, medium, or low with a short reason.
* The decision consequence if the claim is true or false.

Advisor distinguishes reported fact, derived inference, and recommendation.
Advisor never lets several weak sources masquerade as one strong source merely
because they agree.

## Resolve disagreement

Advisor checks whether sources disagree about facts, definitions, timeframes,
populations, or values. Advisor explains the mismatch before choosing a side.
When evidence cannot resolve the conflict, advisor names the smallest new fact
that would change the recommendation.

## Deliver the synthesis

Advisor leads with the answer, then separates what is known, likely,
uncertain, and contested. Advisor connects each uncertainty to decision risk
instead of appending a generic caveat section.

The final recommendation includes the evidence it depends on, the strongest
alternative interpretation, and the next evidence worth buying. This skill
grants no authority to obtain restricted data or make the decision.

## Evaluation target

Compare a cold model and composed advisor on a mixed-quality source packet.
The composed advisor should expose claim-level support, avoid evidence
laundering, explain disagreements, and show exactly which uncertainty could
change the decision.
