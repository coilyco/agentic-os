# Phase 2 - Projection

After the grounded pass, project 10-20 pieces of software that **do not exist yet** but
would be useful to build. This is the speculative half, kept separate so it does not
drown the grounded signal.

**Composition is required, not optional.** This is the line that separates this leg from
generic idea generation. Every projected entry must name **at least one tool the
Kai already maintains** that it builds on. The framing is "X already exists, so Y
becomes cheap to build and useful to people who already run X." An entry that does not
compose an existing tool is greenfield, and greenfield is not what this leg is for -
either attach it to a real tool or drop it.

For each projected entry write:

- `net-new: {name} - builds-on: {one or more existing tools} - because: {who it helps
  and why the composition makes it cheap} - novelty: {what does not exist today}`

**Bias the projection toward Kai's stated audience.** The goal is software
useful to "people like Kai" - others running a similar toolset. So favor
entries that generalize past Kai's own setup over one-off personal conveniences.

**Two projection prompts that tend to surface the best entries:**

1. **Composition gaps** - two tools Kai runs that nobody has wired together yet.
   The connective tissue is usually small and high-value.
2. **Backfill-as-product** - a discipline Kai applies by hand across repos that
   could become an authored, rolled-out tool. These cross into phase 1's `backfill` kind
   and tend to win phase 3, so surface them aggressively.

Output: append to `YYYY-MM-DD-scout-autonomy-2-projection.md` (+ `.yaml`). Merge with the
phase-1 grounded candidates into one pool for scoring.
