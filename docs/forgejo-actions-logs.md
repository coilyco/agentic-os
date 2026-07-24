# Forgejo Actions log bridge

`aguard ops actions logs` calls the packaged
`agentic_os.forgejo_actions_logs` module. It fetches the live plaintext job log
for a PR status target and prints it raw to stdout. Ward retains a separate
role-scoped bridge for agent containers.

The companion specverb fetch overlay in
[Aguard's Forgejo spec](../.specgen/aguard/forgejo.kdl) pins the dead
Forgejo API log route from agentic-os#473. PR #529 replaced that route with
the live web UI helper below, and this fetch mirror keeps the dead shape
documented instead of hand-coding another raw HTTP call.

Why the bridge still exists:

- Forgejo 15.0.2 exposes the Actions metadata in swagger, but not this log
  route.
- The module preserves the owner gate and uses Basic auth against the job page
  and its `logCursors` POST, and it does not JSON-render the response body.
- The operator command is independent of the current repository directory.

The visible status target uses the repository run index and job index, for
example `/actions/runs/886/jobs/0`.

Mapping:

- `886` is the run index from the status `target_url`.
- `0` is the job index from the status `target_url`.
- The bridge opens `/coilyco-flight-deck/agentic-os/actions/runs/886/jobs/0/attempt/1`.
- The page exposes `data-run-id` for the internal run id.
- The page exposes the job list, and the entry at job index `0` provides the
  internal job id.
- The bridge then fetches the plaintext log stream with those internal ids.

The resolved log route is:

`GET` and `POST` `/repos/{owner}/{repo}/actions/runs/{run}/jobs/{job}/attempt/{attempt}`

See also:

- [aguard.md](aguard.md)
- [Forgejo Actions rerun bridge](forgejo-actions-rerun.md)
- [Cross-repo tooling and release](FEATURES.md)
