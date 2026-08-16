# Native session shadow

The per-session checkout a native launch runs in, and the home beside it.

## Native session shadow

A native AOS launch runs the agent inside a per-session shadow instead of the
canonical checkout. This page holds the mechanics behind the AGENTS.md rules
that reference it. Workspace projection itself is in
[native agent workspaces](native-agent-workspaces.md).

## Session markers

`AOS_NATIVE_SESSION` carries the session id and `AOS_NATIVE_SESSION_PROJECTS`
carries the session `projects` root. Both are exported only by a launch that
links at least one worktree. A launch linking none leaves both unset, because it
stays in the canonical checkout and no isolation claim would be true there. An
agent reads the pair as the authoritative answer to whether it is isolated,
rather than pattern-matching its own working directory against a path.

`AOS_REPO_ROOT` names the canonical checkout and is unrelated. It is set by the
shell base and the container entrypoint, so it stays populated inside a shadow
and points away from it.

## Durability asymmetry

Session worktrees are linked from the canonical repository, so they share its
Git object store. A commit is durable the moment it is written, because the
objects land in the canonical repository rather than in the session directory.
Branch refs likewise outlive the session.

The working tree has no such protection. It lives under the platform temporary
root, which the operating system purges on its own schedule. macOS empties
`DARWIN_USER_TEMP_DIR` through `com.apple.bsd.dirhelper`, deleting files by
access time and leaving the directory skeleton behind, which surfaces in Git as
a worktree that is `prunable` with a missing gitdir.

Committed work therefore survives a purged shadow and uncommitted work has no
second copy. That asymmetry is the mechanism behind the remote-checkpoint rule,
which is why the rule is stated as a consequence rather than an edict.

## Hand-made worktrees

An agent working without a shadow still needs isolation from a checkout holding
foreign work. Those worktrees belong under the platform cache at
`agentic-os/agent-worktrees`, not in the temporary directory. A hand-made
worktree carries no session lease, so nothing sweeps or recreates it, and a
purge there destroys the only copy of anything uncommitted.

A shadow stays on its session branch either way. Why, and the guards that
enforce it: [default branch ownership](native-session-start.md).

## Native shadow home

An assigned-role launch stages a shadow `HOME` under the session root. Every
host entry becomes a symlink back to the real home, so credentials, caches, and
tool config keep working.

## Staged config directories

`.agents`, `.claude`, `.codex`, and `.config` (plus its `goose` and `opencode`
children) are staged rather than linked whole: each becomes a real directory
whose entries are symlinks. Agent-compose projects the composed role into this
home at the harness global load points, and a projection resolves symlinks, so
a directory linked whole would write the host's copy instead of the session's.

Inside those directories the projected load points are reserved, which means no
host copy is linked over them: `.claude/CLAUDE.md`, `.codex/AGENTS.md`,
`.config/goose/.goosehints`, `.config/opencode/AGENTS.md`, and the `skills`
directories. Projection refuses foreign files, so a leftover host link there
would fail the launch rather than be replaced. Reserving them is also what makes
role scoping real: the host instructions file carries every role, and leaving it
in place put all of them in context beside the one selected role.

Staging changes where new writes land. An entry that already exists still
resolves to the host through its symlink, but a file the session creates under a
staged directory stays in the session. `~/.config` is the one worth knowing,
since tools write there without being asked.

## The projects hole

`$PROJECTS_ROOT` is the one host entry the shadow home leaves out.

The session already owns writable worktrees under `<session>/projects`, one per
resident checkout, each on its own `aos/<harness>/<id>` branch. Linking the host
projects root into the shadow home would give the canonical checkouts a second
name inside the session. That name resolves past the worktrees to the shared
trees on `main`, and it looks exactly as natural as the real one.

An agent that reaches for `~/projects/<owner>/<repo>` out of habit lands in the
shared checkout, edits it, and commits there. Two sessions doing that at once
collide over one index, and each can absorb the other's in-flight files into its
commit. Both stay entirely inside their own isolation the whole time.

Leaving the entry absent turns that into an immediate "no such file or
directory" instead of a silent wrong answer. The failure names itself at the
first command rather than at the first collision.

## Working in a session

Use the launch directory and relative paths. `pwd` at startup is the session
tree, and `git rev-parse --git-dir` resolves to a `worktrees/` entry under the
canonical repository, which is how a session checkout is recognized.

Reaching the canonical tree on purpose is still possible through its absolute
`/Users/<user>/projects` path. That is deliberate: the guard removes the
accident, not the capability, and the explicit path reads as a decision in a
transcript.

## Scope

The rule binds `stageNativeRoleHome`, which serves assigned-role and workspace
-root launches. `stageStandaloneRoleHome` copies a narrow allowlist instead of
mirroring the home, so it never had the entry. A `PROJECTS_ROOT` override is
matched by inode identity, not by the literal name `projects`.
