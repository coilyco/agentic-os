# Native session start

What a native session does on the way up, and which branch it lands on.

## Native default branch ownership

Why the canonical checkout owns `main`, and the three guards that keep it that
way. Companion to [native agent workspaces](native-agent-workspaces.md).

## A worktree on main takes it from everyone

Git allows one checkout of a branch per repository, so a linked worktree
holding `main` takes it from the canonical checkout, which then cannot switch
back. The fleet pass that would have returned it fails the same way rather than
repairing it, so the checkout stays parked on a stale branch indefinitely.

Squatting is worse than a lock, because branch configuration is
repository-global. A worktree sitting on `main` that pushes with an
upstream-setting push rewrites `branch.main.merge` for every worktree including
the operator's. With `pull.rebase` set, their next `git pull` rebases `main`
onto a session branch and reports divergence against a ref nobody chose. That
failure is silent and reads like their own commits went wrong.

Startup therefore detaches any non-canonical worktree found on `main`, at the
same commit so the working tree is untouched, and resets `branch.main.merge`
when it points elsewhere. Both are backstops. An agent stays on its session
branch and uses a detached HEAD when it needs main's content.

## Absence is not uncertainty

The live-worktree set fails closed: when a path cannot be identified, every path
reads as live and the pass does nothing. A purged temporary root used to trip
that, because a lease naming a worktree that no longer exists made
`EvalSymlinks` fail. One purged session disabled the whole pass indefinitely.

Absence now answers "not live". Only a path unreadable for another reason still
fails closed.

## Branch reaping

The pass deletes a local branch when `rev-list <branch> --not --remotes=origin`
is empty, so every commit on it already exists on the remote. That is stricter
than `git branch --merged`, which would delete a branch whose commits reached
main but were never pushed anywhere.

Checked-out branches, worktree-held branches, and the `aos/` namespace are
skipped. `aos/` is session bookkeeping released by the lease path, and
session-ID uniqueness reads it to tell whether an ID is taken, so reaping it
would hand out an ID another session still owns.

## Native startup narration

Native startup runs several seconds of local Git and remote fetch work before
the harness takes the terminal. It announces each phase on stderr before that
phase runs and reports the phase's elapsed time when it ends, so the wait is
attributable rather than silent. Owner:
[native agent workspaces](native-agent-workspaces.md).

## What a launch prints

```text
aos: launch   native claude startup
aos: start    converge environment
aos: done     converge environment 0.18s (0 catalogues, 12 MCP servers)
aos: start    reclaim finished sessions
aos: done     reclaim finished sessions 0.31s (4 live worktrees)
aos: start    fleet pass over 19 repositories
aos: fetch    1/19 coilyco-flight-deck/agentic-os
aos: fetch    2/19 coilyco-flight-deck/ward
aos: done     fleet pass over 19 repositories 14.8s (slowest coilyco-flight-deck/infrastructure 2.10s)
aos: start    link 19 session worktrees
aos: worktree 1/19 coilyco-flight-deck/agentic-os
aos: done     link 19 session worktrees 1.10s (19 linked)
aos: ready    native startup 16.2s (fleet pass 14.8s, link 19 session worktrees 1.10s)
aos: exec     agent-compose
```

The per-repository `fetch` and `worktree` lines print before their command
runs, so a stalled remote names the checkout it is stuck on. Each looping
phase closes by naming its slowest item, which turns a long phase into one
attributable repository.

## Reading the closing lines

`ready` totals the run and ranks the phases that took at least a tenth of a
second. Instant phases stay out of that list, so the line names the cost
instead of restating the pipeline. `skip` explains a phase that did not need
to run, which is how a fast startup accounts for itself: the ten-minute fleet
pass skips most launches.

`exec` marks the boundary. Waiting after that line belongs to Agent Compose or
the harness, not to AOS.

## Waiting on the startup lock

Startup cleanup is serialized by a lock directory under the native state root.
The lock records the PID and process-start identity of the launch holding it,
so a launch interrupted mid-cleanup is reclaimed by the next launch at once
rather than blocking it. Reclaiming prints `wait  reclaiming startup lock
abandoned by pid N`.

A genuinely live holder is never stolen from. The waiting launch reports the
holder's PID every five seconds and gives up after two minutes with an error
naming that PID and the lock path, so the terminal always says who to wait for.

## Volume

`AOS_NATIVE_PROGRESS` selects how much reaches stderr.

* `steps` - the default. Every phase, every loop item, and the total.
* `summary` - the `ready` total alone.
* `debug` - adds the launch command and internal notes.
* `off` - restores silence.

Warnings and errors ignore the setting and always print.
