# Triage board artifact

The breadth surface. One page carrying every issue in the resolved pool, with
the reading already done and a disposition already recommended, so the human
spends attention only on the rows where the recommendation is wrong.

**This replaces reporting a triage pass in prose.** A prose summary of forty
issues is unreadable and unactionable, and it makes the human re-derive the
per-issue call the pass already made.

Load the `artifact-design` skill before writing the page. Load
`artifact-capabilities` before declaring any capability on it.

## What the page must carry

- **A headline that states the finding, not the count.** "42 open issues, and only three of them are untouched" is a finding. "Triage results" is a label. The headline is the one sentence the human reads if they read nothing else.
- **The shape, above the rows.** Filed against closed per day, as a chart. A filing spike is usually a burst rather than a leak, and which one it is changes what the human does. Say plainly which series is undercounted and why.
- **A provenance stamp.** When the read happened, which API, which commit the local checkout was on and how far behind origin. Every state claim on the page is only as good as that line.
- **Clusters before rows.** Group by workstream or by filing origin, with a short note per cluster naming its root. Duplication concentrates by origin, so the cluster note is where a fold gets proposed.
- **One row per issue**, carrying four things and no more: the identifier as a link to the tracker, the gist in a sentence or two, **the evidence actually read** (a file path and line, an mtime, a command and its output, a child-issue list), and the recommended disposition preselected.
- **A closed disposition set.** Four lanes is the working number. Take next, later, close it, talk it through. More than that and the human is doing taxonomy instead of triage.
- **A running tally and a progress bar.** The human needs to see how much of the board is still undecided without counting.
- **A hand-back block.** A button that renders the marked dispositions as plain text grouped by lane, including an explicit UNDECIDED list. That block is what comes back into the session.
- **A footer that separates observation from inference.** Recommended lanes are a reading, not a measurement. Say so on the page.

## State

Per-viewer `localStorage` is the right default for a board. The marks are one
person's working state on one pass, they do not need to survive for anyone else,
and the deliverable is the hand-back block rather than the page.

Use the `artifact` capability instead when the answers must survive for the
agent to read back without the human copying anything. That is the consult-queue
shape, and the trade is described in
[consult-queue-artifact](consult-queue-artifact.md).

## What kills a board

- **A row with no evidence line.** Without it the human cannot tell a read from a guess, so they re-read the issue, and the board has cost more than it saved.
- **No recommendation.** An unrecommended row is a question, and a board of forty questions is worse than no board.
- **Rows the pass did not actually read.** Coverage is the gate. See [coverage-and-counts](coverage-and-counts.md).
- **A board covering the whole backlog.** Triage the delta since the last pass. A full sweep is a periodic event with its own scope, not the ordinary loop.
