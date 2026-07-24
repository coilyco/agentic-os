---
name: tooling-pm-program-decomposition
description: Use when PM turns a broad outcome into an epic and sequenced Forgejo issues. Preserves parent links, evidence lanes, dependency order, and closure boundaries.
---

# Program decomposition

Use this skill when PM must turn a broad outcome into work that several agents
can execute without losing the original question.

## Start with the outcome

PM writes one parent epic that states the desired outcome, why it matters,
what is out of scope, and what evidence will prove the program complete. The
epic owns synthesis and final closure. A child issue owns only its bounded
question or deliverable.

## Build the issue graph

1. PM separates independent unknowns into focused study issues.
2. PM creates implementation issues only where the expected deliverable is
   already concrete.
3. PM creates a narrow evidence lane for easy examples that can test the
   taxonomy or approach early.
4. PM records sequencing only where one issue genuinely blocks another.
5. PM links every child to the epic and updates the epic with the child links.

Prefer one issue per independently answerable question. Do not use one issue
per file, team, or imagined implementation step when those parts share one
acceptance decision.

## Preserve closure boundaries

The easy lane proves that selected examples work. It does not claim that the
comprehensive catalog is complete. A study can recommend a taxonomy without
closing its implementation. An implementation can land without closing a
parent whose remaining questions are still open.

PM closes the epic only after she can cite the child outcomes and reconcile
conflicts, gaps, and deferred work in the parent.

## Make every issue executable

Each issue names the actor, bounded outcome, inputs, acceptance evidence, and
explicit non-goals. PM includes discovered context in the issue instead of
requiring the next agent to reconstruct the conversation.

If PM lacks authority to create or edit issues, she returns the complete issue
graph and bodies for the authorized actor. This skill grants no tracker
permissions.

## Evaluation target

Compare a cold model and a composed PM on a broad taxonomy request. The
composed PM should produce linked study, implementation, and evidence lanes,
keep dependencies sparse, and avoid false parent closure.
