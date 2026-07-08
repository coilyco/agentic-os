---
name: tooling-code-review
description: Run a one-shot adversarial code review over a diff. Read the issue contract, inspect the live tree, compare against the intended baseline implementation, and return one fenced JSON verdict. Triggers - code review, review a diff, refute by default, baseline implementation, one-shot review.
---

# Code Review

Use this skill when you must judge a diff, not edit it.

## Why

A good review checks the change against the intended implementation, not just the patch text.

## How to apply

- Read the issue contract first.
- Inspect the live filesystem state and the tests.
- Compare the diff against the baseline implementation the issue expects.
- Assume the diff is wrong until you can prove otherwise.
- Do not edit files, negotiate, or iterate.
- Return exactly one fenced JSON block with `verdict`, `reason`, and `confidence`.
