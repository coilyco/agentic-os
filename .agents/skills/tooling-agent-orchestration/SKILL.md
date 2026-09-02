---
name: tooling-agent-orchestration
description: Multi-agent orchestration patterns Ward and o2r formalized, kept past their retirement, and how to express each on the harness surface that survives. Use when coordinating several agents, fanning out work, handing off, dispatching background or scheduled work, or designing a coordination protocol. Triggers - orchestration, multi-agent, fan out, subagent, coordinate agents, handoff, dispatch, reservation, agent channel, background task, cron, cross-session.
---

# Agent orchestration

Two subsystems in this estate formalized how autonomous agents coordinate, and both are leaving service.

* **o2r** (`otel-a2a-relay`) is archived. It carried the wire: sessions, handoff, liveness, and agent activity as OTel spans.
* **Ward** is being archived. [agentic-os#1299](https://forgejo.coilysiren.me/coilyco-flight-deck/agentic-os/issues/1299) cuts its runtime from AOS CI and the dev-base image, and Kai superseded that issue's freeze-rather-than-archive posture on 2026-08-27. It governed unattended runs: dispatch, reservation, lifecycle, recovery, and landing evidence.

What they learned does not leave with them. This skill is the inventory.

## The rule this whole skill turns on

**Patterns are durable. Mechanisms are not.** Ward's reservation logic stops enforcing anything once its runtime leaves the hot paths, and the reason it existed does not stop: two dispatchers racing the same work will still collide. A harness tool that looks like a Ward verb is not the Ward verb, because the enforcement underneath it is missing. Read a pattern for the failure it prevents, then ask what enforces it here.

## The patterns

* [Ward patterns](references/ward-patterns.md) - admission and identity, state and recovery, authority, evidence, serialized mutation. What a governed unattended run has to get right.
* [o2r patterns](references/o2r-patterns.md) - sessions and identity, the channel coordination protocol, trust and admission, activity as traces. What agents on different hosts have to agree on.
* [Harness surface](references/harness-surface.md) - the orchestration tools available now, and which pattern each one can and cannot carry.
* [What does not transfer](references/does-not-transfer.md) - the guarantees that stop being enforced once these two leave service. Read this before assuming coverage.

## Four rules that bind every use of the current surface

* **Nothing here is durable.** `CronCreate` writes nothing to disk and dies with the session. Recurring jobs expire after seven days. `Workflow` resume is same-session only. Ward's dispatch was durable, with issue-backed reservations and restart reconciliation, and no harness tool replaces that.
* **Messaging carries no authority.** Asking a peer to do what your own session denied is cross-session permission laundering and the `SendMessage` rule prohibits it. This is the wall model restated: a seat that defers a wall does not acquire it by asking a seat that holds it.
* **It is one harness.** This surface is Claude Code. The inventory runs seats on codex, openhands, goose, holmesgpt, plandex, hermes, anythingllm, mixpost, penpot, and discord. Anything built on it is unavailable to those seats by construction.
* **Checkpoint discipline does not relax inside a subagent.** Work a subagent produced is work product. A fan-out whose findings live only in a transcript has lost them.

## Provenance

Read from `coilyco-flight-deck/ward` at `040f159` and `coilyco-flight-deck/otel-a2a-relay` at `8b96ed1`, both public. Where a pattern below disagrees with one of those repositories, the repo was right and this file has drifted.

**What survives Ward's archival.** `.ward/ward.yaml` still carries catalog metadata, and the `ward:` AGENTS.md frontmatter key still selects a landing lane in every repo that declares one. Both are read by the catalog hooks and by composition rather than by any Ward process, so archiving the repo retires neither. What ends is the runtime, and with it the enforcement behind every pattern here.

## See also

* [AGENTS.md](../../../AGENTS.md) - the composed operating base, including command delivery.
* [tooling-agent-workflows](../tooling-agent-workflows/SKILL.md) - documenting an agent-facing CLI, a different subject.
