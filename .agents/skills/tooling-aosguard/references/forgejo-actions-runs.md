# Forgejo Actions runs and logs

Listing runs, reading their logs, and rerunning a failed one.

## Forgejo Actions listing

The native `action-run list` wrapper defaults to `page=1` whenever a caller
passes `--limit`. Forgejo can ignore that limit on the runs endpoint
unless the page is explicit, pulling the whole history.

Use AOSguard for live inspection:

- `aosguard ops forgejo action-run list <owner> <repo> --limit N`
- `aosguard ops forgejo tasks list <owner> <repo> --limit N`
- `aosguard ops actions tasks <owner> <repo> [--page 1] [--limit N]`

Raw API examples include `page=1` whenever they include `limit`, on both
`/actions/runs` and `/actions/tasks`.

## Rerunning a run

Forgejo 16.0.2 serves no API route to rerun a run. Its swagger has `cancel` and nothing for rerun, and `POST .../actions/runs/{run}/rerun` and `.../rerun-failed-jobs` return 404. So the rerun goes through the web UI route, the one the Re-run button posts to:

```text
aosguard ops actions-rerun rerun <owner> <repo> <run> [--job <job>] [--dry-run]
```

The verb logs in as the bot with a password aosguard injects from SSM, then posts `.../actions/runs/{run}/rerun`, or `.../jobs/{index}/rerun` with `--job`. Forgejo 16 guards POSTs with Go's cross-origin check and not a token, so a login and a session cookie are all it needs. It reads the run through the API first and refuses anything but a failed or cancelled `pull_request` run, because a push or schedule run can be a release or a deploy. `--dry-run` stops after that check and before the login.

The guard cannot tell whose pull request a run belongs to, since every pull request is opened by the one bot. A seat reruns its own PR by convention. Pushing an empty commit to the PR branch also starts a new cycle.

## Which id is which

One run carries four numbers, and the verbs take different ones:

- **run id** - `id` in `action-run list`. Taken by `action-run get`, `action-run cancel` and `action-run-job list <run_id>`.
- **run number** - `index_in_repo` in `action-run list`, `run_number` in `tasks list`, and the number in the run's `html_url`. Taken by `aosguard ops actions logs`, and by no `ops forgejo` verb.
- **task id** - `id` in `tasks list`, `task_id` in `action-run-job list`. No verb takes it.
- **job id** - `id` in `action-run-job list`. Taken by `action-job logs <job_id>`.

To read a failed run, pass its run number to `aosguard ops actions logs` and skip the ids. To drive the `ops forgejo` verbs, match `run_number` in `tasks list` to `index_in_repo` in `action-run list` and read that row's `id`.


## Forgejo Actions logs

`aosguard ops actions logs` fetches workflow-run and job logs through Forgejo's
official REST API. It requires Forgejo 16.0 or newer. It does not read HTML,
call web routes, use browser cookies, or synthesize log text.

## Resolved command

```text
aosguard ops actions logs <owner> <repo> <run> [job] [attempt] [--max-bytes N]
```

```bash
aosguard ops actions logs coilyco agentic-os 2766 > run-2766.zip
aosguard ops actions logs coilyco agentic-os 2766 0
aosguard ops actions logs coilyco agentic-os 2766 0 2
```

The examples select a whole run, visible job index 0, and its attempt 2.
Supported identifiers:

* Repository - separate `<owner> <repo>` names, never a database ID.
* Run - the visible number from `/actions/runs/2766`, or `id:<n>`.
* Job - zero-based visible index, exact name, `name:<exact-name>`, or `id:<n>`,
  where `name:` disambiguates a numeric name.
* Attempt - positive 1-based number. Omit it for Forgejo's latest attempt.

The resolver filters runs by visible number, reads that run's job list, then
calls the log endpoint, so callers need no internal run, job, or task IDs.

## Bytes and bounds

Single-job output is the exact body from
`GET /repos/{owner}/{repo}/actions/jobs/{job_id}/logs`, requested as successive
byte ranges without decoding, so empty and non-UTF-8 logs remain exact.
Whole-run output is the exact ZIP from
`GET /repos/{owner}/{repo}/actions/runs/{run_id}/logs`, where Forgejo may add
`.MISSING` markers for jobs that have not started or whose logs expired. Both
buffer at most 64 MiB by default and leave stdout empty when that bound is
exceeded, `--max-bytes` selects another, and diagnostics go to stderr.

## Typed states

* Running log with bytes - exit 0, `running_log` warning. Completed - exit 0.
* Running log not ready - `running_log`, exit 75.
* Expired completed job log - `expired_log`, exit 69.
* Completed job that never executed - `log_unavailable`, exit 69.
* Missing run, job, or attempt - `missing_*`, exit 66.
* Authorization failure - `authorization_failure`, exit 77.
* Output above the bound - `too_large`, exit 65.

Whole-run ZIPs remain successful with `.MISSING` entries: AOS reads only those
small markers and warns without changing the archive bytes.

## Runner egress

Egress has no direct route out, so `scripts/ci-command.sh` exports
`FORGEJO_EGRESS_PROXY` and execs the command, a no-op when the variable is unset.
Forgejo stays in `NO_PROXY` or the checkout deadlocks on its own ingress. Every
path to `scripts/ci/repo-test-gate.sh` crosses it, since that gate runs
`pre-commit run --all-files` and a cold hook install fetches github.com. The four
`~/.cache/pre-commit` blocks that hid this are gone: the image bakes
`PRE_COMMIT_HOME=/opt/pre-commit`, so they saved an empty directory while reading
as protection (agentic-os#1031).

## Direct guarded leaves

```text
aosguard ops forgejo action-run-job list <owner> <repo> <internal-run-id>
aosguard ops forgejo action-job logs <owner> <repo> <internal-job-id> [--attempt N]
aosguard ops forgejo action-run logs <owner> <repo> <internal-run-id>
```

These expose the official API directly. Use the resolved command for an Actions
URL or job name. Both `logs` leaves return bytes exactly since the lock moved to umbra v0.142.0 (umbra#291).
