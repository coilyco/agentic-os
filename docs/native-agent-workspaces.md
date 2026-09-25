# Native agent workspaces

The shared shell shadows `claude`, `codex`, `goose`, and `opencode`. A supported
AOS binary first cleans recoverable predecessors and creates an isolated,
fleet-shaped workspace. No timer, daemon, exit hook, or operator command runs.

## Workspace projection

Agent Compose's compiled repository plan drives one linked worktree per checkout.
AOS reads `$AOS_REPOSITORY_PLAN` or `~/.agent-compose/repository-plan.yaml`.
Legacy JSON remains a rollout fallback. No embedded roster exists. Exact
`owner/repository` identities supply writable worktrees from canonical Git objects.

### Role scope

The plan carries a per-role selection beside residency, and an assigned-role
launch links only its own role's selection. `aterm gamedev` and `acompose
gamedev claude` link the game checkouts, a `platform` launch does
not, and neither pays for worktrees it will never open. AOS grows no second
opinion about which repository serves which charter: the plan decided, and
`--role` is how the launch hands AOS that answer.

Residency stays the full set, and everything except projection reads it: the
[ten-minute fleet pass](#ten-minute-fleet-pass) fetches and normalizes every
resident checkout, and [unexpected-clone detection](#unexpected-clones) counts
every resident identity as belonging on disk.

Full residency is the fallback: no role, a role the plan does not name, and a
role whose every selection is missing from disk. Linking nothing drops the
session into the canonical checkout while still reporting itself isolated.

A launch from `$PROJECTS_ROOT` enters:

```text
$TMPDIR/aos/native/<id>/projects/<owner>/<repo>
```

with the owner/repository hierarchy reproduced below `projects`. A repository
subdirectory maps to its session twin. An outside launch keeps its directory
and the full fleet workspace. The collision-checked `<id>` uses the canonical
dictatable shape `ab85`: two lowercase letters then two digits.

Native and standalone AOS use this shadow before agent-compose or Docker runs.
Bare harness commands let the native roster infer a role.
`acompose <role> <harness> [args...]` enters the clean shadow workspace for
every role. A native director has no Ward broker. Dispatch and credentials
need a warded launch. Native AOS clears deprecated model selectors, so every
harness receives the complete selected role composition. User skills stay empty.
Claude auto-updates cannot replace the host launcher, and both harnesses trust
the workspace. Assigned launches apply [Codex hook trust](native-harness-config.md).

## Startup leases

One JSON lease records process identity, directories, repositories, branches,
and worktrees. Exec preserves the PID through the vendor binary, and the next
launch detects a closed terminal. Resolved identity keeps path aliases leased.

Every launch timestamps a dead lease. The next launch reading it dead again
removes each clean worktree whose branch tip is reachable from `origin`, then
deletes its local branch. Dirty, untracked, unpushed, unreadable, and `*-workdir`
state stays, so clean siblings may go alone. The grace holds the session root.

## Daily fleet pass

Once a day at most, startup pulls resident repositories already on disk (a launch
between passes fetches only its own). It fetches `origin`, then:

* A clean, inactive checkout on a remotely recoverable non-main branch switches
  to `main`, then deletes that local branch.
* A clean `main` fast-forwards to `origin/main`.
* An inactive linked worktree is removed with its local branch when clean and
  remotely recoverable.

It never creates merge commits, pushes, deletes remote branches, commits, or clones missing repositories.

## Unexpected clones

Fleet ownership comes from `$AOS_FLEET_ORGS` or its conventional config path.
A direct `$PROJECTS_ROOT/<owner>/<repo>` checkout is unexpected when its origin
belongs to a fleet org but compiled residency omits its exact identity.

An unexpected clone must be inactive on clean `main` at fetched `origin/main`.
It cannot have linked worktrees, Git operations, submodules, ignored or
untracked files, or local-only branch, tag, stash, or reflog commits. Its origin
owner must match its directory owner.

The cache records an exact origin, HEAD, and branch fingerprint. The third
consecutive qualifying fleet pass deletes the clone. Any failed proof or
changed fingerprint resets its count. With the ten-minute pass interval, three
qualifying startups span at least twenty minutes.

## Serialized repositories

The serialized set is three checkouts on a native Windows host:
`coilyco/eco-app`, `coilyco/eco-mods`, and
`coilyco/eco-ops`. Projection links no worktree for them, the fleet pass
neither switches nor deletes their branches, and unexpected-clone detection
never counts them, so a direct checkout under `$PROJECTS_ROOT` survives every
qualifying pass whether or not compiled residency names it.

Unity serializes its project through an editor lock and a single asset
database, and the Eco server state those assets drive is one mutable world. A
second writable tree is not isolation there: both trees resolve the same lock,
and the merge back is a corrupt scene rather than a conflict Git can show. The
exemption therefore removes the isolation instead of tuning it, and the agent
carries the serialization itself.

Scope is the host, not the repository. The same three identities keep ordinary
projection everywhere else, because the lock lives on the Windows tower where
the Unity editor and the server run. `eco-app` holds no Unity assets and earns
the exemption by sharing their release cycle, so one agent covers all three
rather than splitting a rule nobody can recall correctly under pressure.

## Local state

Leases and the pass cache use the platform cache at `agentic-os/native-shadow`. AOS
groups temporary state under platform `aos`: worktrees in `native`, requests in
`compose`, bundles in `bundles`. `AOS_NATIVE_STATE_DIR` and `AOS_NATIVE_SESSIONS_DIR`
override. See [session shadow](native-shadow.md), [shadow home](native-shadow.md),
[narration](native-session-start.md), [shell owner](install.md), and [Claude config](native-harness-config.md).
