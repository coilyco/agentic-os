# Consult queue artifact

The depth surface, and the overflow for AskUserQuestion. Use it when a fork
needs more context than four option labels can hold, or when more than four are
ready at once, or when the human should answer asynchronously rather than in the
live turn.

Load the `artifact-design` skill before writing the page, and
`artifact-capabilities` before declaring the `artifact` capability on it.

## What the page must carry

- **The read, before the questions.** Open with what the pass found about the queue itself: how many issues were examined, how many carried a real question, and what the rest turned out to be. This is usually the most valuable section on the page, because it is where the queue gets shorter.
- **One row per decision, not per issue.** Two issues blocked on the same call are one row. One issue with three open unknowns is three rows.
- **A source link per row**, plus a kind (fork, go / no-go, yours alone) so the human can see what type of call they are making before they read it.
- **Why this is being asked**, collapsible, carrying the compressed context: what the issue decided already, what it explicitly left open, and any tension with a decision made elsewhere. Name the tension rather than smoothing it.
- **A blocked marker** on any row where an agent verification is still outstanding, saying what would settle it. Asking a question whose answer might be moot is a wasted round.
- **Options with the recommendation first and marked**, each carrying an explicit **"I expect:"** clause. Not a description of the option, a statement of what you predict happens if it is taken, and what would reverse it. That clause is the whole value of the row: it is falsifiable, and it is what lets the human disagree with the reasoning rather than just the choice.
- **A free-text field per row**, prompted with something specific ("the exact handle, if it is not just the existing one"), because the most useful answers are usually not one of the options.
- **A reopen affordance** on answered rows, so a returning reader can change a call without losing the trail.

## State

This page persists its own answers, which is what separates it from a board.

Keep the state in an embedded `<script type="application/json">` block. On save,
clone the document, empty the render mount, replace the state block, and publish
the rebuilt document through the `artifact` capability. **Rebuild from source
plus state, never from the live DOM**, or typed input gets serialized back as
markup.

Handle three outcomes explicitly:

- **No capability.** A read-only view cannot save. Say so in the rail rather than failing silently on submit.
- **Conflict.** Someone saved first. Reload to their version rather than overwriting it.
- **Any other failure.** Render the answers as a copyable JSON block so the round is not lost.

## The page is not the record

Answers on this page are the deliverable, and they go back onto their source
issues as comments in the same turn they arrive. Record the decision, **every
rejected alternative with its reason**, the acceptance criteria implied, and
whatever is still unresolved with its owner. An answer living only in the page
or only in chat is gone.

Then relabel. An issue whose blocking decision has landed is no longer
`autonomy/async-consult`. Say which condition was discharged. See
[askuserquestion-flow](askuserquestion-flow.md).
