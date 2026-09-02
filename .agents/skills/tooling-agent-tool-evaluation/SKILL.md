---
name: tooling-agent-tool-evaluation
description: Evaluate whether an agent tool is understandable and useful across harnesses by running representative tasks and comparing definitions. Triggers - tool evaluation, MCP tool usability, function-calling science, tool schema review, tool redesign.
license: MIT
metadata:
  source-url: https://github.com/openai/openai-cookbook/blob/main/examples/evaluation/use-cases/tools-evaluation.ipynb
---

# Agent tool evaluation

Evaluate the agent's observed path, not only whether the tool can eventually
produce a correct result. Keep tasks and expected outcomes separate from tool
definitions so grading does not teach the agent which calls to make.

## Prepare

1. Snapshot each candidate tool exactly as the harness exposes it. Preserve its
   name, description, input schema, required fields, and result rule.
2. Write representative tasks in a separate artifact. Cover a routine success,
   an invalid or boundary input, and a result that could become oversized.
   Record the expected outcome and correctness rule without naming tool calls.
3. Include a baseline or prior definition when its result could change whether
   the tool is adopted or revised. State why comparison is unnecessary when no
   decision could change.
4. Hold the agent, model, instructions, access, data, retry rules, and run
   count constant across definitions. Start a fresh session for each run when
   the harness permits it.

## Run

1. Give the agent one task and the exact exposed definitions. Do not explain
   the intended call sequence.
2. Use the real tool or a faithful fixture with representative successes,
   errors, latency, and result sizes.
3. Record the final answer, correctness, wall duration, ordered tool calls,
   call count, failures, result size or truncation, and the agent's feedback.
4. Repeat nondeterministic cases. Separate executor or service failures from
   failures caused by the tool's interface.

## Review

* **Outcome** - Grade the final answer against the prewritten correctness rule.
* **Efficiency** - Flag unnecessary duration, calls, and retries. A correct
  answer after avoidable retries is weaker than a first-call correct answer.
* **Definition** - Check that names and descriptions communicate purpose.
  Check parameter meaning, formats, defaults, and required versus optional
  inputs without relying on hidden conventions.
* **Recovery** - Check whether errors identify the bad input and explain a
  valid correction.
* **Result size** - Check whether defaults stay focused and whether filters,
  limits, fields, summaries, or pagination prevent oversized results.
* **Feedback** - Tie each proposed change to trace evidence. Prefer actionable
  edits such as a rename, a unit or format note, a corrected required list, a
  recovery example, or a bounded result default.

Compare raw task results as well as aggregate correctness, duration, call
count, and failure rate. Recommend keeping, revising, replacing, or rejecting
the definition and name the evidence that changes the decision.

## Report

Use flat records that remain comparable across harnesses:

* **Setup** - harness, agent or model, instructions, tool version, data, and runs.
* **Task result** - task ID, correctness, duration, tool-call count, and failures.
* **Tool feedback** - observed friction, trace evidence, and proposed definition change.
* **Comparison** - candidate versus baseline or prior definition on the same tasks.
* **Decision** - recommendation, confidence, remaining uncertainty, and next experiment.

## Fixtures

Load the focused fixtures when testing or adapting the method:

* [Tool definition variants](references/fixture-tool-definitions.yaml)
* [Tasks and expected outcomes](references/fixture-tasks.yaml)
* [Observed runs and comparison](references/fixture-runs.yaml)
