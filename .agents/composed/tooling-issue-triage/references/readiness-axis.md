# Readiness axis: `blocked-on-dependency`

A third property orthogonal to both tier (urgency) and [autonomy](automation-mode-axis.md) (the ceiling). Autonomy answers "can an agent land it unattended?" Readiness answers a separate question mode keeps swallowing: can the issue run **now**, or is it correctly scoped but waiting on an upstream that has not landed? Most issues are ready and need no readiness label. One blocked state earns a name.

## The state

- **`blocked-on-dependency`** - the work is correctly scoped **and** intrinsically `autonomy/headless`-eligible, with no pending design call and no human decision. It is simply not runnable yet because its resolver is **another repo's issue**, not a person. It carries a **blocker pointer** - the upstream issue whose close unblocks it. The pointer lives in the issue body as `<!-- ward-blocked-on: owner/repo#123 -->`. That body marker is the source of truth. Labels alone cannot carry the upstream ref.

## Why it is not `autonomy/async-consult`

The trap is collapsing this into `autonomy/async-consult`. They look alike (both "do not dispatch now") but resolve on opposite signals:

- An **`autonomy/async-consult`** issue waits for a **human** - a decision, design, or access an agent lacks. It should not be dispatched at all, and a person has to come back and re-decide before it ever moves.
- A **`blocked-on-dependency`** issue waits for a **machine event** - its blocker issue closing. There is nothing to decide. The moment the upstream issue closes it should **auto-resume into the `autonomy/headless` queue** with no human re-triage.

An `autonomy/async-consult` issue waits for a person. A blocked-headless issue should wake on upstream resolution. Reading the second as the first throws away the signal that matters.

## Rides alongside a mode, does not replace one

Because readiness is orthogonal to autonomy, `blocked-on-dependency` does **not** replace an autonomy label - it rides alongside one. The common case is `autonomy/headless` + `blocked-on-dependency`: settled headless work, parked on an upstream, due to wake on its own.

The triggering case is a ward issue blocked only on umbra exporting `ParseIssueRef`/`IssueRef`. Its `ward agent headless` pre-flight NO-GO'd it as a cross-repo coordination **fork** - reading a defer-and-wake as a needs-a-human async-consult, exactly the misread this state removes.

If the blocker pointer is missing or ambiguous, the issue fails closed. The classifier keeps the ordinary autonomy and tier labels and does not claim wake behavior.

## What is named here vs built later

This doc names the state and fixes its semantics. The enforcing half - teaching the `ward agent headless` pre-flight to detect it, plus the auto-resume **wake mechanism** that re-enters a dependent issue into the dispatch queue when its blocker issue closes - is the cross-repo build tracked in the dependency work.
