# Phase 1 - Grounded sweep

Walk the working surface and build a verbose list of **buildable** candidates. Do not
deep-read code, read the shape.

**Grounded pass.** For each repo in the run-config repo list, read:

- `README.md` and `AGENTS.md` (top-level), plus `docs/FEATURES.md` if present - the
  inventory of what already ships is the raw material this leg builds on.
- Recent commit subjects (`git log --oneline -50`) - direction and momentum.
- Open issues (the tracker's `--state open` list) - already-wanted work.

Plus Kai's notes: the index, the most recent 14 days of inbox, and any
ideas/tasks files. And, **if present, the outputs of the sibling scouts and the
backlog**:

- The latest `*-capability-scout-*` and `*-displacement-scout-*` notes files - newly
  acquired tools are prime build material, and a planned displacement is a refactor
  waiting to happen.
- The latest `tooling-issue-triage` output - P1/P2 items already carry intent.

**Tag every candidate by kind**, because kind drives the phase-3 multiplier:

- `refactor` - improve or consolidate an existing subsystem.
- `backfill` - author/extend one thing and roll it across many repos.
- `net-new` - did not exist before. Must name the existing tool(s) it composes, or it
  is demoted in phase 3.

For each, write one line: `- {kind}: {name} - builds-on: {existing tool/repo} -
because: {one-sentence rationale tied to a specific repo, issue, or note}`.

Apply the **reserved-surface fence** from run-config here at intake: drop any candidate
that touches a fenced subsystem before it ever reaches scoring. Cheaper to exclude early.

Output: `YYYY-MM-DD-scout-autonomy-1-candidates.md` plus a `.yaml` sibling
(machine-readable for later phases). Bare candidates only - no scores yet.
