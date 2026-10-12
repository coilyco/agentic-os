# Auto-mode denials and dispatch

What the harness's auto mode refused during the 2026-10-09 cycle 15 launch, and the dispatch rules that follow. Sources: the Claude Code permission-modes, errors and auto-mode-config pages read that day, plus the denied calls in COI-2659.

## Who can authorize a gated step

The classifier reads typed user messages, non-read tool calls and CLAUDE.md. The docs say tool results are stripped from its requests. Two findings followed in the launch:

* **A peer relay is not the human's approval.** A brief saying "the human approved" is a user-turn message stamped `[from <role>]`. The relaunched sysadmin seat was refused twice with that brief in hand, and both refusals cleared when the human typed an approval naming the deploy in that seat's own window.
* **An `ask_choice` answer is a tool result.** The human answered yes to a wipe and the next `rm -rf` was still refused. Inference from the docs plus that one run, not a tested rule.

## Dispatch rules

* For a gated step (production deploy, irreversible delete, merge into the deploy repo), send the tracker ref and the context, and say the human types the approval in the target window. Do not present a relayed approval as the authority.
* Never tell a seat to do the steps its own harness just refused, even when the seat is about to be relaunched. The relaunched seat still needs the human's typed approval, in its own window, for any step the old seat was refused.
* Hand an artifact over a pushed branch or a tracker comment, not a path in another session's scratch directory.

## Shell rules

* Move a removal into the snapshot directory instead of deleting it, because a move can be put back.
* Put multi-step shell logic in a script file, because a file can be read and reviewed before it runs.

## When a commit, push or edit is denied

All three launch-night cases (a launcher pin, a connector grant, a container first-run edit) widened agent permissions, so the denial was a request for the human's eyes. The risk after it is the work, which sits uncommitted in a temp worktree that a purge or a tidy seat can destroy. In the same turn:

* **Keep the worktree.** Never reset, checkout, clean or delete it.
* **Make the work durable off the machine.** Attach `git diff --binary` (staged and unstaged) to the tracker record as a file through the Linear attachment upload. Name the branch, the worktree path and the base commit in a comment.
* **Hand the human one command.** A `!`-prefixed line, run from that worktree, that commits, pushes and opens the pull request, leaving the merge to the lane. Say in one line what the change grants.
* **Do not retry the denied action in another shape.** A rephrase that passes is the pattern the denial exists to catch.

A blanket `Bash(git commit:*)` or `Bash(git push:*)` allow rule is not part of this rule. It would land permission-widening changes without the human seeing them, and adding one is the human's call.

## When every call returns "a safety check separate from auto mode"

Read and Edit still work and every Bash and MCP call is refused. Retrying changes nothing. The harness docs give the remedy: the human switches that window out of auto mode and approves at the prompt, or starts a fresh conversation. Try the mode switch first, since it keeps the session and its shadow. These denials do not count toward auto mode's pause thresholds, so it never falls back by itself. The cause of the launch-night instance is unidentified. The first refused call embedded a base64 blob, which is a hypothesis only.
