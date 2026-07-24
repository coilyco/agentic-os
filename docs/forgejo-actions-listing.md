# Forgejo Actions listing

The packaged `agentic_os.forgejo_actions_list` module lists Forgejo Actions runs
or tasks and defaults to `page=1` when the caller omits `--page`. That keeps CI
checks on the first page instead of accidentally pulling the whole history
when `limit` is present. The native `action-run list` wrapper also pins `page=1`
when a caller passes `--limit`, because Forgejo can ignore that limit on the
runs endpoint unless the page is explicit.

Use Aguard for live inspection:

- `aguard ops forgejo action-run list <owner> <repo> --limit N`
- `aguard ops forgejo tasks list <owner> <repo> --limit N`
- `aguard ops actions runs <owner> <repo> [--page 1] [--limit N]`
- `aguard ops actions tasks <owner> <repo> [--page 1] [--limit N]`

Raw API examples should include `page=1` whenever they include `limit`:

- `GET /api/v1/repos/{owner}/{repo}/actions/runs?page=1&limit=1`
- `GET /api/v1/repos/{owner}/{repo}/actions/tasks?page=1&limit=1`

See also:

- [Forgejo Actions log bridge](forgejo-actions-logs.md)
- [Aguard](aguard.md)
