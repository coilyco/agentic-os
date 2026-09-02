# Native session start

What a native session does on the way up, and which branch it lands on.

## A worktree on main takes it from everyone

Why the canonical checkout owns `main`, and the guards that keep it that way. Companion to [native agent
workspaces](native-agent-workspaces.md). Git allows one checkout of a branch per repository, so a linked worktree holding `main`
takes it from the canonical checkout, which then cannot switch back. The fleet pass that would have returned it fails the same way
rather than repairing it, so the checkout stays parked on a stale branch indefinitely.

Squatting is worse than a lock, because branch configuration is repository-global. A worktree on `main` pushing with an
upstream-setting push rewrites `branch.main.merge` for every worktree including the operator's, and with `pull.rebase` set their
next `git pull` rebases `main` onto a session branch and reports divergence against a ref nobody chose, silently, reading like their
own commits went wrong.

Startup therefore detaches any non-canonical worktree found on `main` at the same commit, leaving the working tree untouched, and
resets `branch.main.merge` when it points elsewhere. Both are backstops: an agent stays on its session branch and detaches when it
needs main's content.

## Absence is not uncertainty

The live-worktree set fails closed: when a path cannot be identified, every path reads as live and the pass does nothing. A purged
temporary root used to trip that, because a lease naming a worktree that no longer exists made `EvalSymlinks` fail, so one purged
session disabled the whole pass indefinitely. Absence now answers "not live", and only a path unreadable for another reason still
fails closed.

The repository plan works the same way. An absent plan fell back to a seed expecting almost nothing, so every clean fleet checkout
read as unexpected and sat three sweeps from deletion. Cleanup now runs only from a plan-backed expectation, counters included,
since an advancing counter still deletes on the fourth scan (agentic-os#903).

## Sealed plan provenance

Agent Compose seals each policy source's identity, revision, and policy SHA-256 into the plan. AOS decoded those and decided nothing
from them, so a plan compiled from policy that had since moved read exactly like one compiled a minute ago. Startup now fetches each
source and verifies the seal, and a stale digest is a refresh trigger rather than a wall (agentic-os#1215).

The digest triggers and the revision only reports. A repository holding policy also takes ordinary commits, and on a measured day
its revision moved eight times while the policy never moved. A gate that fires on every commit gets routed around.

Two commits are checked because two are read. The seal is of the working-tree file, and session worktrees are cut from
`origin/main`, so that is where a session's own policy comes from. Checking the base keeps session policy no newer than the
repository selection, without stranding sessions on the sealed commit's code.

The retry budget is exactly one, and it runs in silence: the converge report is captured for the error rather than forwarded. A
regeneration that is unavailable, fails, or still mismatches leaves the loaded plan in place and launches anyway, and only a refresh
that could not run says so, in one line. Stopping was the wrong half of this gate, and it owned the only automatic refresh, so
deleting it outright would trade a wedge for a plan that silently goes stale. It takes `HOME` from the plan it read, since a shadow
composing into its own `HOME` leaves the canonical plan stale. The fetch is best-effort, so a host off the network still launches.

`repository-plan.json` seals nothing to verify: it warns until 2026-10-01 and then goes. An unverified plan is never authoritative,
so cleanup stays off for it as for an absent plan.

## Branch reaping

The pass deletes a local branch when `rev-list <branch> --not --remotes=origin` is empty, so every commit already exists on the
remote, which is stricter than `git branch --merged`. Checked-out, worktree-held, and `aos/` branches are skipped: `aos/` is session
bookkeeping the lease path releases, and session-ID uniqueness reads it, so reaping would hand out a live ID. That test also spares
a dead session's branch whose worktree is gone and whose commits are local-only, and startup names those once (agentic-os#1084).

Neither reading sees a branch no lease recorded, made by hand or outliving its lease: one checkout held 78 branches with 13 reported
(agentic-os#1286). Startup now names a branch no worktree or lease holds, with no `origin` counterpart, carrying a commit `git
cherry origin/main` marks `+`. Patch-id not reachability: the merge style is squash, so the test above calls every landed branch
unpushed.

## Resident checkout drift

Normalization returns a resident checkout to `main` only when it is clean and already on the remote, and said nothing otherwise: six
of twelve sat off `main` unnoticed, one changing what a composed artifact recorded as live deployment state. The pass names each
with its branch and whether it is dirty, unpushed, or how far behind origin, that last because one untracked file stops
normalization and a resident `deploy` then sat 421 commits back, otherwise clean on `main`.

## Native startup narration

Native startup runs several seconds of local Git and remote fetch work before the harness takes the terminal. It announces each
phase on stderr so the wait is attributable rather than silent. The narration has two forms and the stream decides which: a terminal
gets one row that rewrites itself, anything else gets a line per phase, which is the form a log is read from later.

On a terminal, the row is redrawn about ten times a second, so a slow phase still visibly moves and a frozen frame means a stuck
launch:

```text
aos ⠹ fleet pass over 19 repositories // 7/19 coilyco-bridge/infrastructure 6.20s
```

Off a terminal, each phase prints `start` before it runs and `done` with its elapsed time after. Per-repository `fetch` and
`worktree` lines name the checkout the command is on, and a looping phase closes by naming its slowest item:

```text
aos: start    fleet pass over 19 repositories
aos: fetch    1/19 coilyco-flight-deck/agentic-os
aos: done     fleet pass over 19 repositories 14.8s (slowest infrastructure 2.10s)
```

Either way the run closes on one `ready` line carrying the total, the session, and the phases that took at least a tenth of a
second. A terminal erases the row first, and a warning written straight to stderr erases it too, landing on its own clean line with
the row restored underneath.

```text
aos: ready    native startup 16.2s // 19 worktrees // /tmp/aos/native/ab85/projects // slowest fleet pass 14.8s
```

`skip` explains a phase that did not need to run. `exec` marks the boundary off a terminal, and waiting after it belongs to Agent
Compose or the harness.

`AOS_NATIVE_PROGRESS` selects the volume: `steps` is the default above, `summary` keeps the `ready` total alone, `debug` adds the
launch command and internal notes and never collapses to one row, and `off` restores silence. Warnings and errors print at every
level.

## Waiting on the startup lock

Startup cleanup is serialized by a lock directory under the native state root, recording the PID and process-start identity of its
holder, so a launch interrupted mid-cleanup is reclaimed at once rather than blocking the next one, printing `wait  reclaiming
startup lock abandoned by pid N`. A live holder is never stolen from: the waiting launch reports its PID every five seconds and
gives up after two minutes naming that PID and the lock path.

## Terminal titles

An assigned native Codex launch projects Agent Compose's canonical annotation and short session ID into its interactive terminal
title, keeping concurrent sessions recognizable by seat, role, and session. `aterm` keeps its static title for new kitty windows.
