# Forgejo Actions rerun bridge

`aguard ops actions rerun` and `rerun-failed-jobs` call the packaged
`agentic_os.forgejo_actions_rerun` module. The call targets a known run, then
falls back to dispatching that run's workflow file for the same ref when
Forgejo does not expose a usable rerun control.

The companion specverb fetch overlay in
[Aguard's Forgejo spec](../.specgen/aguard/forgejo.kdl) pins the dead
Forgejo API rerun routes from agentic-os#473. PR #529 replaced those routes
with the live web UI helper below, and this fetch mirror keeps the dead shape
documented instead of hand-coding another raw HTTP call.

Why the bridge exists:

- Forgejo exposes the rerun controls inconsistently on this deployment, so the
  bridge keeps the bot token inside Aguard and the run id as the authority
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

- [aguard.md](aguard.md)
- [Forgejo Actions log bridge](forgejo-actions-logs.md)
- [Cross-repo tooling and release](FEATURES.md)
