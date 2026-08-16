# Forgejo Actions runs

Listing runs and re-running one. Reading their logs is
[forgejo-actions-logs.md](forgejo-actions-logs.md).

## Forgejo Actions listing

The packaged `agentic_os.forgejo_actions_list` module lists Forgejo Actions runs
or tasks and defaults to `page=1` when the caller omits `--page`. That keeps CI
checks on the first page instead of accidentally pulling the whole history
when `limit` is present. The native `action-run list` wrapper also pins `page=1`
when a caller passes `--limit`, because Forgejo can ignore that limit on the
runs endpoint unless the page is explicit.

Use AOSguard for live inspection:

- `aosguard ops forgejo action-run list <owner> <repo> --limit N`
- `aosguard ops forgejo tasks list <owner> <repo> --limit N`
- `aosguard ops actions runs <owner> <repo> [--page 1] [--limit N]`
- `aosguard ops actions tasks <owner> <repo> [--page 1] [--limit N]`

Raw API examples should include `page=1` whenever they include `limit`:

- `GET /api/v1/repos/{owner}/{repo}/actions/runs?page=1&limit=1`
- `GET /api/v1/repos/{owner}/{repo}/actions/tasks?page=1&limit=1`

See also:

- [Forgejo Actions log bridge](forgejo-actions-logs.md)
- [AOSguard](aosguard.md)

## Forgejo Actions rerun bridge

`aosguard ops actions rerun` and `rerun-failed-jobs` call the packaged
`agentic_os.forgejo_actions_rerun` module. The call targets a known run, then
falls back to dispatching that run's workflow file for the same ref when
Forgejo does not expose a usable rerun control.

The companion specverb fetch overlay in
[AOSguard's Forgejo spec](../.specgen/guardfiles/aosguard/forgejo.kdl) pins the dead
Forgejo API rerun routes from agentic-os#473. PR #529 replaced those routes
with the live web UI helper below, and this fetch mirror keeps the dead shape
documented instead of hand-coding another raw HTTP call.

Why the bridge exists:

- Forgejo exposes the rerun controls inconsistently on this deployment, so the
  bridge keeps the bot token inside AOSguard and the run id as the authority
  boundary, then dispatches the workflow file when rerun controls are absent.
- `actions rerun` tries the run page's `/rerun` control first, then falls back
  to workflow dispatch for the same ref.
- `actions rerun-failed-jobs` tries the failed-job rerun controls when
  available, then falls back to workflow dispatch for the same ref.

The resolved routes are:

- `POST /{owner}/{repo}/actions/runs/{run_id}/rerun`
- `POST /{owner}/{repo}/actions/runs/{run_id}/jobs/{job_index}/rerun`
- `POST /api/v1/repos/{owner}/{repo}/actions/workflows/{workflowfilename}/dispatches`

See also:

- [aosguard.md](aosguard.md)
- [Forgejo Actions log bridge](forgejo-actions-logs.md)
- [Cross-repo tooling and release](FEATURES.md)
