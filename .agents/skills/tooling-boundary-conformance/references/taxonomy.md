# The taxonomy in full

Every entry as the observation, then the check that catches it, with the
case that produced it. `SKILL.md` carries the thesis and the index; this is
the detail it points at, split out when the entry set outgrew the skill's
own size cap rather than because the two halves are different in kind.

## The taxonomy

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
  that did land as one that did not. A denied read reaches the same place by a
  different mechanism: macOS masks EPERM as ENOENT on a privacy-protected
  directory, so a listing that is refused presents as a folder that is empty,
  and only an explicit listing attempt tells the two apart.
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
  redacted and a sibling emission of the same string is not, or generally a
  value kept on two surfaces where only the one in hand was updated. **Check: assert
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
  and reported as uncovered a case already documented here. Watch the success
  direction hardest: a stale failure looks wrong and gets investigated, a stale
  success ships behind a green board.
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
* **The identifier that was predicted rather than read back** - a key composed
  from what the writer expected, instead of read out of the write's own
  response. **Check: cite the identifier the write returned, and where the
  surface returns none, cite a form that fails loudly.** A guessed key is
  well-formed, so nothing errors: it resolves to a real record belonging to
  something else, and the reader lands somewhere plausible and unrelated. Two
  seats did this in one evening on a tracker whose sequence is global, and each
  guess landed on the other's record.
* **The window that cannot show the change** - a readout whose shape holds
  steady no matter what moves underneath it. **Check: assert on a value that
  must move, the tail or a count or a named member, never on the head.** A list
  ordered descending and read with a small limit returns the same rows whether
  the queue behind it is draining or stuck, so watching it is watching the
  window rather than the thing.
* **The detector whose success state is silence** - a matcher written against
  the failure it is looking for, so it goes quiet the moment that failure
  clears. **Check: assert the corrected state positively, and report the
  failure state as a separate value.** Success and never-having-run are then
  distinguishable, which they are not when absence is the only signal. A deploy
  poll grepping for the broken string reported a timeout after ten minutes on a
  deploy that had completed in two, and the tell was in its own output: last
  seen was empty rather than the old value.
* **The failure set reported without a baseline** - a suite with pre-existing
  failures leaves two readings and no third, claim green or guess which
  failures are yours. **Check: run the same suite at the unmodified reference
  and diff the two failure sets by name.** Extract the reference to a scratch
  path rather than stashing, so the working tree is never disturbed. Counts are
  not enough: twenty-two local failures and twenty-four on a clean main looked
  like a regression and were the same set, and the diff is what said so.
