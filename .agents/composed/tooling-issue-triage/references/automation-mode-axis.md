# Autonomy axis

A second, orthogonal axis beside `priority/*`. Tier answers "how much does this matter?" Autonomy answers "can an agent land it unattended?" They are independent: a backlog holds real, urgent `priority/P1` work that still needs a human, so urgency alone cannot decide what may auto-run. Sorting by tier leaves you re-deciding per issue what is safe to dispatch - the decision fatigue that appears once an agent burns issues faster than you can prioritize. This axis is the eligibility filter that removes that decision.

## The four values

Read the first three as a **ceiling** - the highest agent autonomy the task supports. The gate runs an issue at a surface no higher than its label.

- **`autonomy/headless`** - the agent can perform the work on its own, from open issue to merged change, with no human in the loop. Self-contained code/docs/config work, clear enough to act on now, no pending design decision, no credentials or access the agent lacks, no destructive or externally-visible production step. The auto-burndown queue.
- **`autonomy/live-collab`** - the agent and the human need to work together in realtime. A design choice to make jointly, a destructive step to approve as it happens, or verification only a person can perform. The agent can start and work alongside, but the human has to actually be there.
- **`autonomy/async-consult`** - a human needs to consult on the issue before it can be upgraded to headless. An ambiguous or underspecified ask, a product or strategy call, access the agent does not have, or a mostly-human task. The consult is asynchronous: it is a question waiting in a queue, not an appointment. An agent should not start; at most it preps context.
- **`autonomy/epic`** - the issue holds many units of sub work, and its size makes it meaningfully exclusive with the other three. An epic has no single autonomy ceiling because its children each have their own. It is a container, not a task.

The **`live-collab` versus `async-consult` split** earns its keep on a heavy backlog, and the two names now say why they differ rather than how much autonomy each allows. `live-collab` needs a human **present**; `async-consult` needs a human **answer**. One is scheduled time, the other is queue latency. Collapsing them back into "not auto" loses exactly that.

## A third property: readiness

Autonomy is the ceiling. It does not say whether a correctly-scoped issue can run **now** or is parked on an upstream. That is **readiness**, orthogonal to both. One blocked state earns a name: `blocked-on-dependency` - settled, `autonomy/headless`-eligible work waiting on another repo's issue rather than on a human, so distinct from `autonomy/async-consult`. See [readiness-axis](readiness-axis.md).

## Fail-closed default

An unlabeled issue is treated as `autonomy/async-consult`, and an automated classifier falls back to it whenever it is not confident. Nothing auto-runs headless until a deliberate, confident promotion - automation is opt-in, not opt-out, matching the fleet lockdown posture.

## Assigning the value

Unlike the tier, autonomy cannot come from a percentile cut - it is a per-issue judgment made during the triage pass. For each issue the pass classifies it into one value plus a confidence; only a high-confidence `autonomy/headless` or `autonomy/live-collab` promotes the issue out of human-gated, everything else fails closed.

## The dispatch gate

The enforcing half is a ceiling gate at the shared dispatch chokepoint, and
renaming these labels breaks it silently. See [dispatch-gate](dispatch-gate.md).
