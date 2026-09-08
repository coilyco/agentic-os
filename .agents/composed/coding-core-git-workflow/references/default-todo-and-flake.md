# Default TODO destination and flake discipline

## Default TODO destination

When the human asks to file a todo without naming a destination, file a record in the Teable `issues` table, tagging the repository the work actually touches. `repo: inbox` is the fallback only when no repository is clearly relevant. Confirmed 2026-09-02 and re-verified 2026-09-03: every repository in the fleet reports `has_issues: false` except `coilysiren/coilysiren`, so `POST /issues` returns 404 on Forgejo even under `admin`, `list_issue` returns an empty array, and `open_issues_count` still serves the pre-migration number. That count is a fossil rather than a queue.

Forgejo stays canonical for git history, branches, pull requests, and releases. Only the issue tracker moved. Cite a record as `teable:<owner>/<repo>#<n>`, because a bare `<owner>/<repo>#<n>` still means Forgejo, which is where pull requests and historical issues live. The `closes <forgejo url>` trailer is retired along with the trackers, so closing a record is a separate deliberate step that no commit performs for you. Mechanics, schema, and the two failures that return 200 while doing nothing: `docs/teable-tracker.md` in `agentic-os-kai`.

GitHub issues on external-facing `coilysiren/*` repos remain an inbox for external contributors only, and agents never file there. The old `hasIssuesEnabled` split no longer classifies anything, since the flag is now false fleet-wide for reasons unrelated to whether a repo faces outward.

**Never ask whether to file an issue. Just file it.** If you're about to ask "should I file an issue for X" or offer it as a choice, the answer is always yes - file it and mention in one line what you filed. Issues are post-it notes: cheap, swept up by backlog routines. Asking is pure overhead, and at Kai's pace a deferred "want me to file this?" is something she will miss. Applies to any issue surfaced mid-task.

## Test flake discipline

Every flaky-test sighting in a fleet repo becomes a tracker record tagged to that repo, no exceptions (flaky = failed then passed on re-run with no code change, or non-deterministic across two runs). File immediately, even mid-task: test name, both runs' output, candidate causes. Don't paper over by re-running until green.
