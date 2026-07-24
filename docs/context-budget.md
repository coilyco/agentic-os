# Context-budget report

`check-context-budget` reports the eager startup context each agent harness
loads at session start, per harness, against a per-harness token budget, with
per-source attribution. It is an on-demand tool (the `ward context-budget`
verb), not a pre-commit hook, so it carries no weight in the universal commit
path and is free to grow heavier measurement later.

The [fixed Goose snapshot mode](context-budget-goose.md) measures one
representative open-source lane before and after the ordinary-skill refactor.

## What it measures

It sums everything a harness ingests at session start across three axes, each
with a different growth lever, then reports the total against a per-harness
budget with a fill bar and an `OVER by N` flag:

- **doc** - the composed AGENTS.md/CLAUDE.md load point. It reuses the composer's
  own resolution, so the bytes are exactly what the load point holds, with a
  per-source breakdown. Lever: edit the AGENTS.md sources. The `agent-compose-size`
  hook caps these sources at commit time; this measures the composed result.
- **skills** - every mounted skill's SKILL.md **frontmatter** (name +
  description) is eager so the model knows the skill exists; bodies load lazily
  on invoke. With a large skill surface this is routinely the **biggest** axis,
  larger than the composed doc. Lever: prune the skill set.
- **mcp** - MCP tool schemas. The shared mcporter inventory is projected
  into each supported native registry. Harness schema discovery stays deferred,
  and `mcporter call` remains the CLI fallback, so the eager figure is ~0. The
  report shows a server-count note rather than a token sum.

When no `agent-compose.yaml` is present (agent-compose is opt-in), the doc axis
falls back to measuring the installed load-point files directly. These three axes
are the **proactive** tier (eager prompt bytes); the `immediate_walk` /
`peripheral_walk` primitives and `--immediate` / `--peripheral` flags measure the
reachable working-dir/reference tiers - see [context-tiers.md](context-tiers.md).

### Skill scope follows the CWD

`mount-skills.sh` exposes plugin skills plus the skills scoped to the
CWD. Relative roots expand across the workspace and resolved paths dedupe, so
the report is a workspace-union worst case. A single-repo session sees fewer.
Defaults come from `DEFAULT_SKILL_ROOTS`; `skill_roots:` overrides them.

## Why per-harness budgets differ

The budgets are not one number. They encode three distinct failure modes:

- **claude** - attention dilution. Its composed slice alone gets the private
  overlay, so it is the heaviest, and a bloated baseline crowds the task and
  makes the model miss the obvious. The budget is a forcing function: doc growth
  is zero-sum against it.
- **codex** - its native MCP schemas are deferred, so the eager MCP surface is
  ~0 and the budget only bounds the composed doc. Sweep blow-out is runtime
  accumulation a static report cannot govern.
- **opencode** - a small local qwen model, curated hardest, so its budget is
  tightest.

## Token counting

v1 uses a deterministic chars/4 proxy. tiktoken ships no qwen encoding and the
qwen BPE needs its vocab assets, so v1 stays hermetic behind a single
`count_tokens` function. The proxy is ~10% off absolute but consistent across
harnesses, which a zero-sum comparison needs. The swap is one function.

## Budgets and flags

Budgets are global (host-wide, not repo-scoped): module defaults, overridable by
a `budgets:` mapping in `agent-compose.yaml` or `--<harness>-budget` flags.
`--check` exits non-zero when any harness is over budget, for CI. `--mcporter`
points at the shared inventory projected into each native harness registry.

## See also

- [features-agents-sessions.md](features-agents-sessions.md) - agent-compose, the composer this measures.
- [role-composed-skills.md](role-composed-skills.md) - role-gated skills selected into the Goose bundle.
- [.ward/ward.yaml](../.ward/ward.yaml) - allowlisted commands.
