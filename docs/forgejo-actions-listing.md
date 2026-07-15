# Forgejo Actions listing

The `.ward/forgejo-actions-list.sh` bridge lists Forgejo Actions runs or tasks
and defaults to `page=1` when the caller omits `--page`. That keeps CI checks on
the first page instead of accidentally pulling the whole history when `limit`
is present. The native `action-run list` wrapper also pins `page=1` when a caller
passes `--limit`, because Forgejo can ignore that limit on the runs endpoint
unless the page is explicit.

Use the safe ward surface for live inspection:

- `ward ops forgejo action-run list <owner> <repo> --limit N`
- `ward ops forgejo tasks list <owner> <repo> --limit N`
- `ward ops forgejo actions runs <owner> <repo> [--page 1] [--limit N]` when the role-scoped readactions bridge is mounted
- `ward ops forgejo actions tasks <owner> <repo> [--page 1] [--limit N]` when the role-scoped readactions bridge is mounted

Raw API examples should include `page=1` whenever they include `limit`:

- `GET /api/v1/repos/{owner}/{repo}/actions/runs?page=1&limit=1`
- `GET /api/v1/repos/{owner}/{repo}/actions/tasks?page=1&limit=1`

See also:

- [Forgejo Actions log bridge](forgejo-actions-logs.md)
- [Ward spec bundle](ward-specs.md)
