---
name: tooling-boundary-conformance
description: Audit whether a declared boundary actually binds, using the failure taxonomy observed across the coilysiren/inbox#426 sweep. Every entry is a way a control passes silently instead of refusing, and each carries the check that catches it. Triggers - does this boundary bind, silent pass, negative control, filter dropped, conformance check, boundary audit, declared but not enforced.
---

# Does the wall bind, or does it pass quietly

A declared wall has three possible behaviours and only two are acceptable.
It **binds**, it **refuses**, or it **passes silently** and reports success.
The third is the defect this taxonomy is about, and it is one defect class
instead of a list of unrelated bugs.

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
  absence, in three unrelated systems. Assert the result is the data you asked
  for, not merely that it changed - an empty result also differs from the
  unfiltered one, which is the next entry.
* **The narrowing that returned nothing** - a projection or column selection the
  surface cannot honour returns success with every field emptied, rather than
  the requested subset or an error. **Check: assert the named fields are present
  and populated.** Absence reads as data, so a caller concludes the field is
  unset rather than unreturned, and a read-back written this way reports a write
  that did land as one that did not.
* **The write that reported success and did not land** - a mutation returns 2xx,
  validates nothing and applies nothing. **Check: read the object back through a
  different call and assert the new value.** The write's own response is not
  evidence about the write, and a schema that accepts a property it silently
  discards produces a stored object nobody asked for.
* **The check that takes its expectation from its subject** - a check derives
  its baseline from the artifact under test, so it compares the subject against
  itself and cannot fail. **Check: name where the expectation comes from and
  assert it is independent** - re-fetch from the authority, or record the
  expected value before the subject can change it. A version read at comparison
  time always equals itself; the one read at load time does not. Recording at
  load time closes the tautology and not the staleness: a copy loaded before a
  rotation and checked long after still reads as current, matches, and calls a
  dead credential valid. The check tells revoked from stale only inside the
  window between load and rotation, and says nothing outside it.
* **The type that was quietly narrowed** - a value crosses the wall and
  arrives as a type nothing declared. **Check: round-trip each declared type**
  and compare the wire form, including the union case where a schema declares
  more than one.
* **The constraint that cannot be expressed** - the language has no way to say
  the thing, so intent is narrowed at authoring time with no diagnostic.
  **Check: for each input kind the surface cannot carry, assert an
  authoring-time refusal exists** that names the limit.
* **The control that runs after the hazard** - a check evaluated on the
  response cannot prevent the write. **Check: assert the refusal happens with
  no side effect**, by counting upstream calls instead of reading an exit code.
* **The field that is parsed and never read** - a declared constraint that no
  code enforces. **Check: for every declared constraint, assert that violating
  it fails.** A constraint with no failing test is decoration.
* **The guarded copy and its unguarded twin** - one emission of a string is
  redacted and a sibling emission of the same string is not. **Check: assert
  every surface carrying that value is covered**, not the one the incident
  named.
* **The authority that cannot be withheld** - a charter names a limit with no
  mechanism, or a gate offers one exit. **Check: assert each declared wall
  has an owning side and a deferring side**, and each gate has every honest
  outcome its decision can produce.
* **The rule announced while being broken** - a refusal that states the
  wall and violates it in the same reply. **Check: the value, not the
  phrasing.** Match the identifier by value, and bound the reply length,
  because the leak lives inside the explanation.
* **The check that verifies a neighbouring layer** - a status is a true
  statement about one thing, and the reader treats it as covering a different
  thing sitting next to it that it does not reach. **Check: name exactly what
  the status is a property of, and verify that thing directly** rather than
  something derived from it or supplied to it. A Kubernetes `SecretSynced`
  condition is true about the Secret and says nothing about a process that
  read a value from it at start: an env var injected via `secretKeyRef` is
  fixed at container launch and never re-read, so a correct sync plus an
  already-running pod produces a stale credential with two healthy indicators
  and no third that disagrees. A one-line shell check piping through `tail -1`
  before grepping for an error string caught the line the error pushed the
  real message onto rather than the message itself, and printed the negated
  answer with no exit code to disagree. Both pass their own review, because
  the layer actually checked was correct and the layer the question was about
  was the one next to it.

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
