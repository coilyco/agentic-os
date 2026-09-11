---
name: writing-system-improvement-vocab
description: Distinguishes self-healing, self-improving, continuous comprehension, and meta-improvement on locus of agency and what gets changed. Triggers - self-healing, meta-improvement, fix the bug vs fix the system, improvement loci, reliability vocab.
---

# System-improvement vocab

Four concepts that look like the same idea but aren't. Distinguish on **locus of agency** (system or human) and **what gets changed** (the fault, the producer, the model, the understanding).

Family resemblance: "improve the producer, not just the artifact." They differ on who acts and what they change.

## The four

### Self-healing - "patch the symptom, keep serving"

**System operational characteristic.** The running system masks faults and continues without human intervention. Retries, circuit breakers, restart loops, failover. The same fault recurs while recovery hides it. The producer is unchanged.

Reach for it when: the cost of brief degradation is high and the cost of recurrence is low. Latency-sensitive prod paths, distributed systems with known failure modes.

### Self-improving systems - "let the system tune itself"

**System operational characteristic.** The running system modifies its own behavior over time to perform better against some objective. RL agents, auto-tuned databases, agentic codegen pipelines, adaptive load balancers. Agency lives in the machine. AI-loaded term in 2026 - use carefully.

Reach for it when: the search space is too large or fast-moving for human tuning, and you have a reliable fitness signal. Caches, schedulers, model routing.

### Continuous comprehension - "stay current with the system you actually have"

**Human practice (system-supported).** Maintain an ongoing, accurate mental model of a system that is changing faster than periodic review can keep up with. Weekly architecture retros, ensemble programming, AI-assisted code-comprehension tools. Output is updated **understanding**, not updated code. (Thoughtworks term, Feb 2026 retreat.)

Reach for it when: code review is no longer a viable learning channel because change rate is too high. Post-AI-codegen environments. New team members in fast-moving codebases.

### Meta-improvement - "fix the opaqueness first, then the bug"

**Human practice (priority heuristic).** When an object-level problem surfaces, ask whether the system that produced it can be improved so the same class of problem becomes cheaper to handle next time. Output is changes to tools, wrappers, docs, skills, AGENTS.md, error messages, structured fields. Agency is the engineer's. The artifact is the surrounding workflow, not the running system.

Reach for it when: a recurring or recurring-shaped class of friction is showing up. Low priority -> meta first. Medium -> alongside. High -> object-level fix now, meta as immediate follow-up. See AGENTS.md "Meta-improvement bias."

## Decision shortcut

* **The fault's blast radius right now** - self-healing.
* **The system's behavior over time, automatically** - self-improving.
* **Your team's understanding of the live system** - continuous comprehension.
* **The workflow that produced the problem** - meta-improvement.

Combine freely. A self-healing fix often surfaces a meta-improvement opportunity ("why was this fault possible at all"). Continuous-comprehension practices often feed self-improving systems with the fitness functions they need.

## Bridge phrases for outside conversations

"Meta-improvement" is central internal vocab. Outside this vocabulary's home
repos, reach for legible bridges:

- "human-side analogue of self-improving systems"
- "the dev-discipline counterpart to continuous comprehension"

Do not rename it. Bridge it.

## Why this skill exists

- [Why this skill exists](references/why-this-skill-exists.md) - the 2026-05-04 origin and how to apply the vocab in live conversation.
