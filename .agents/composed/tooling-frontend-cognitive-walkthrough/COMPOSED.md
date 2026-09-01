---
name: tooling-frontend-cognitive-walkthrough
description: Use when frontend evaluates whether a first-time or infrequent user can complete a concrete task. Walks intent, action visibility, goal mapping, and feedback step by step.
---

# Cognitive walkthrough

Use this skill to evaluate the learnability of one concrete task from the
perspective of a first-time or infrequent user.

## Activation wall

Frontend activates this workflow when a flow, prototype, screenshot sequence,
or working interface is concrete enough to inspect action by action.
Interaction generation stays in `tooling-frontend-interaction-shaping`.
Visual critique, accessibility conformance, and empirical usability testing are
different methods.

## Frame the walkthrough

Frontend records:

* The task, starting state, success state, and correct action sequence.
* The target user's goals, relevant prior knowledge, and likely vocabulary.
* Information the interface actually exposes at each step.
* The critical path to inspect when several valid paths exist.

Frontend does not invent preferences, emotions, or domain knowledge for the
user. Uncertain assumptions remain visible in the final report.

## Inspect every action

At each step, frontend answers four questions with interface evidence:

1. Will the user form the right subgoal at this point?
2. Will the user notice that the correct action is available?
3. Will the user connect that action with the intended effect?
4. After acting, will the user interpret the feedback as progress?

Frontend records pass, fail, or uncertain for each question. A plausible story
is not enough. The rationale points to a label, control, hierarchy cue, system
response, or missing signal that supports the judgment.

## Diagnose the earliest breakdown

Frontend classifies each failure as goal formation, discoverability, mapping,
feedback, recovery, or missing prerequisite knowledge. She fixes the earliest
cause in the sequence because later confusion may be a consequence instead of
a separate defect.

Severity reflects task impact and recoverability:

* Blocker - the user cannot complete the task without outside knowledge.
* High - the obvious action causes failure, loss, or a costly detour.
* Medium - the user can recover through exploration or visible help.
* Low - hesitation or interpretation cost does not threaten completion.

Frontend proposes the smallest interaction change that repairs the failed
question. The walkthrough does not justify a wholesale redesign by itself.

## Output rule

Frontend delivers the task brief, knowledge assumptions, stepwise findings,
earliest breakdowns, severity, evidence, targeted repair, and confidence.
Predictions that require real user behavior become explicit usability-test
hypotheses.

## Method provenance

The four probes follow Wharton, Rieman, Lewis, and Polson's
[cognitive walkthrough method](https://www.colorado.edu/ics/sites/default/files/attached-files/93-07.pdf).

## Evaluation target

Compare a cold model and composed frontend on a technically complete flow with
an obscure action label, a visually quiet control, and ambiguous success
feedback. The composed frontend should locate the exact cognitive breakdown at
each step, avoid invented user claims, and recommend bounded repairs.
