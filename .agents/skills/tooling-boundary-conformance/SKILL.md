---
name: tooling-boundary-conformance
description: Audit whether a declared boundary actually binds, using the failure taxonomy observed across the coilysiren/inbox#426 sweep and later sessions. Every entry is a way a control passes silently instead of refusing, and each carries the check that catches it. Triggers - does this boundary bind, silent pass, negative control, filter dropped, conformance check, boundary audit, declared but not enforced.
---

# Does the wall bind, or does it pass quietly

A declared wall has three possible behaviours and only two are acceptable.
It **binds**, it **refuses**, or it **passes silently** and reports success.
The third is the defect this taxonomy is about, and it is one defect class
instead of a list of unrelated bugs.

Silent passing is worse than the hazard it fails to stop. A control that errors
gets fixed. A control that returns a plausible answer nobody can distinguish
from a real one gets believed, and the wrong number travels.

**A readout fails the same way with no wall in it.** Several entries below are
not controls at all. They are counts, conditions, orderings and propagated
values that render plausibly while structurally decoupled from what they
describe, so there is nothing in them that could have refused. They belong here
because the failure and the remedy keep their shape: a plausible answer nobody
can distinguish from a real one, caught by varying the subject and asserting
the readout follows.

## The taxonomy

Twenty ways a control passes quietly, each with the check that catches it.
The observation, the check and the case that produced it are in
[references/taxonomy.md](references/taxonomy.md). Read it before picking
entries: the titles are mnemonics and the check is the load-bearing half.

* **The filter that could not be applied**
* **The narrowing that returned nothing**
* **The write that reported success and did not land**
* **The check that takes its expectation from its subject**
* **The type that was quietly narrowed**
* **The constraint that cannot be expressed**
* **The control that runs after the hazard**
* **The field that is parsed and never read**
* **The guarded copy and its unguarded twin**
* **The authority that cannot be withheld**
* **The rule announced while being broken**
* **The check that verifies a neighbouring layer**
* **The indicator with no mechanism**
* **The value that landed and did not travel**
* **The order that renders but does not sort**
* **The second reader who was never independent**
* **The identifier that was predicted rather than read back**
* **The window that cannot show the change**
* **The detector whose success state is silence**
* **The failure set reported without a baseline**

## How to use it

Read the wall's own declaration first, then pick the entries that could
apply and write the check before deciding whether it binds. A wall you
reasoned about and did not probe is unmeasured, not passing.

Two rules that fall out of the whole set:

**A surface is stricter than the thing it guards.** Where a permissive default
exists, the guard inverts it. The permissive default is what let most of these
through.

**Absence established one way is not absence.** One search modality answers
about that modality only, and a single empty query is not a negative result.
