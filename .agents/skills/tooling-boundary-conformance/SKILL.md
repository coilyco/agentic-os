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

**A readout fails the same way with no wall in it.** Several entries below are
not controls at all. They are counts, conditions, orderings and propagated
values that render plausibly while structurally decoupled from what they
describe, so there is nothing in them that could have refused. They belong here
because the failure and the remedy keep their shape: a plausible answer nobody
can distinguish from a real one, caught by varying the subject and asserting
the readout follows.

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
* **The indicator with no mechanism** - a count, condition or status renders
  plausibly while the subsystem it describes is switched off or unreachable.
  **Check: disable the mechanism and assert the indicator moves.** Where it
  cannot be disabled, assert it against one case whose true value is known by
  another route. A Forgejo repository carrying `has_issues` false still reports
  `open_issues_count` 76: the counter reads rows the disabled unit no longer
  serves, and nothing in the response disagrees with itself, so a caller
  planning against the count schedules work onto a tracker nobody can open.
* **The value that landed and did not travel** - a change is committed,
  promoted, or marked superseded at the source while live consumers still hold
  the prior one. **Check: read the value back from a consumer rather than from
  the source.** A source-side read confirms the write and says nothing about
  propagation, and the gap is invisible from both ends, because the author sees
  a landed commit and the consumer sees a value with no age on it. Three
  sessions independently reported an MCP server unreachable nine hours after
  its pods recovered, each holding a connection result recorded once at startup
  and never retried. The specification proposing this entry measured its own
  coverage against a copy of this file that predated this file's newest entry,
  and reported as uncovered a case already documented here.
* **The order that renders but does not sort** - a sequence displays plausibly
  while resting on an arbitrary key, such as the creation order of a choice
  list. **Check: assert two elements whose correct relative order is known
  appear in that order**, rather than asserting the list rendered. A plausible
  order is the hard case, because a reversed one gets noticed and an arbitrary
  one gets believed, and a caller whose sort was dropped upstream reads arrival
  order as the answer.
* **The second reader who was never independent** - two readers agree, and the
  agreement is treated as confirmation while both drew from the same upstream at
  the same remove. **Check: name what each reader read, and confirm at least one
  went to the authority.** Two stale copies agree exactly as well as two current
  ones, so agreement measures shared provenance rather than truth, and a
  verification step built this way raises confidence without adding evidence.
  A specification written against a composed copy of this file was confirmed
  against a second composed copy: both carried 8 entries, the authority carried
  12, and the reported gap was wrong in the same direction twice.

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
