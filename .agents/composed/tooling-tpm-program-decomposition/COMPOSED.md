---
name: tooling-tpm-program-decomposition
description: Use when director turns a broad outcome into an epic and sequenced Forgejo issues. Preserves parent links, evidence lanes, dependency order, and closure boundaries.
---

# Program decomposition

Use this skill when director must turn a broad outcome into work that several agents
can execute without losing the original question.

## Start with the outcome

TPM writes one parent epic that states the desired outcome, why it matters,
what is out of scope, and what evidence will prove the program complete. The
epic owns synthesis and final closure. A child issue owns only its bounded
question or deliverable.

## Build the issue graph

1. TPM separates independent unknowns into focused study issues.
2. TPM creates implementation issues only where the expected deliverable is
   already concrete.
3. TPM creates a narrow evidence lane for easy examples that can test the
   taxonomy or approach early.
4. TPM records sequencing only where one issue genuinely blocks another.
5. TPM links every child to the epic and updates the epic with the child links.

Prefer one issue per independently answerable question. Do not use one issue
per file, team, or imagined implementation step when those parts share one
acceptance decision.

## Preserve closure boundaries

The easy lane proves that chosen examples work. It does not claim that the
comprehensive catalog is complete. A study can recommend a taxonomy without
closing its implementation. An implementation can land without closing a
parent whose remaining questions are still open.

TPM closes the epic only after she can cite the child outcomes and reconcile
conflicts, gaps, and deferred work in the parent.

## Make every issue executable

Each issue names the actor, bounded outcome, inputs, acceptance evidence, and
explicit non-goals. TPM includes discovered context in the issue instead of
requiring the next agent to reconstruct the conversation.

If director lacks authority to create or edit issues, she returns the complete issue
graph and bodies for the authorized actor. This skill grants no tracker
permissions.

## Evaluation target

Compare a cold model and a composed director on a broad taxonomy request. The
composed director should produce linked study, implementation, and evidence lanes,
keep dependencies sparse, and avoid false parent closure.
