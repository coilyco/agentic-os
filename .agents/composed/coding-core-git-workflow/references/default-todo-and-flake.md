# Default TODO destination and flake discipline

## Default TODO destination

When the human asks to file a todo without naming a destination, file a record in Linear (team `COI`), with the repository the work actually touches as its `repo` label. `inbox` is the fallback only when no repository is clearly relevant. The tracker moved off Teable at the epic 51 cutover, and Teable is frozen. Forgejo was never the destination: its issue trackers were off fleet-wide (confirmed 2026-09-02 and re-verified 2026-09-03), and Forgejo itself was deleted on 2026-11-01 (COI-3075).

GitHub is canonical for git history, branches, pull requests, and releases, for every repo. Cite a record as `COI-<n>`. A `teable:<owner>/<repo>#<n>` ref names a frozen pre-cutover record, and a bare `<owner>/<repo>#<n>` is an archived Forgejo ref kept in the COI-3075 dump (`s3://kai-game-backups/backups/coi3075-forgejo-final/`), never rewritten one by one. The `closes <forgejo url>` trailer is retired along with the trackers, so closing a record is a separate deliberate step that no commit performs for you. Mechanics, field mapping, and resolving an old `teable:` ref: `docs/linear-tracker.md` in `agentic-os-kai`.

GitHub issues on external-facing `coilysiren/*` repos remain an inbox for external contributors only, and agents never file there. The old `hasIssuesEnabled` split no longer classifies anything, since the flag is now false fleet-wide for reasons unrelated to whether a repo faces outward.

**Never ask whether to file an issue. Just file it.** If you're about to ask "should I file an issue for X" or offer it as a choice, the answer is always yes - file it and mention in one line what you filed. Issues are post-it notes: cheap, swept up by backlog routines. Asking is pure overhead, and at Kai's pace a deferred "want me to file this?" is something she will miss. Applies to any issue surfaced mid-task.

## Test flake discipline

Every flaky-test sighting in a fleet repo becomes a tracker record tagged to that repo, no exceptions (flaky = failed then passed on re-run with no code change, or non-deterministic across two runs). File immediately, even mid-task: test name, both runs' output, candidate causes. Don't paper over by re-running until green.
