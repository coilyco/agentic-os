---
name: tooling-boundary-conformance
description: Audit whether a declared boundary actually binds, using the failure taxonomy observed across the coilysiren/inbox#426 sweep. Every entry is a way a control passes silently instead of refusing, and each carries the check that catches it. Triggers - does this boundary bind, silent pass, negative control, filter dropped, conformance check, boundary audit, declared but not enforced.
---

# Does the boundary bind, or does it pass quietly

A declared boundary has three possible behaviours and only two are acceptable.
It **binds**, it **refuses**, or it **passes silently** and reports success.
The third is the defect this taxonomy is about, and it is one defect class
rather than a list of unrelated bugs.

Silent passing is worse than the hazard it fails to stop. A control that errors
gets fixed. A control that returns a plausible answer nobody can distinguish
from a real one gets believed, and the wrong number travels.

## The taxonomy

Each entry is stated as the observation, then the check that catches it.

* **The filter that could not be applied** - a query the surface cannot honour
  returns unfiltered data with a success status. **Check: a negative control.**
  Supply a value that cannot match and assert the result differs from the
  unfiltered one. This is the highest-yield check here: it caught a dropped
  argument, an unresolvable label filter, and a partial view reported as an
  absence, in three unrelated systems.
* **The type that was quietly narrowed** - a value crosses the boundary and
  arrives as a type nothing declared. **Check: round-trip each declared type**
  and compare the wire form, including the union case where a schema declares
  more than one.
* **The constraint that cannot be expressed** - the language has no way to say
  the thing, so intent is narrowed at authoring time with no diagnostic.
  **Check: for each input kind the surface cannot carry, assert an
  authoring-time refusal exists** that names the limit.
* **The control that runs after the hazard** - a check evaluated on the
  response cannot prevent the write. **Check: assert the refusal happens with
  no side effect**, by counting upstream calls rather than reading an exit code.
* **The field that is parsed and never read** - a declared constraint that no
  code enforces. **Check: for every declared constraint, assert that violating
  it fails.** A constraint with no failing test is decoration.
* **The guarded copy and its unguarded twin** - one emission of a string is
  redacted and a sibling emission of the same string is not. **Check: assert
  every surface carrying that value is covered**, not the one the incident
  named.
* **The authority that cannot be withheld** - a charter names a limit with no
  mechanism, or a gate offers one exit. **Check: assert each declared boundary
  has an owning side and a deferring side**, and each gate has every honest
  outcome its decision can produce.
* **The rule announced while being broken** - a refusal that states the
  boundary and violates it in the same reply. **Check: the value, not the
  phrasing.** Match the identifier by value, and bound the reply length,
  because the leak lives inside the explanation.

## How to use it

Read the boundary's own declaration first, then pick the entries that could
apply and write the check before deciding whether it binds. A boundary you
reasoned about and did not probe is unmeasured, not passing.

Two rules that fall out of the whole set:

**A surface is stricter than the thing it guards.** Where a permissive default
exists, the guard inverts it. The permissive default is what let most of these
through.

**Absence established one way is not absence.** One search modality answers
about that modality only, and a single empty query is not a negative result.
