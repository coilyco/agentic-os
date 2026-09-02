# Forgejo Actions runs and logs

Listing runs, re-running one, and reading their logs.

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

## There is no rerun bridge

`aosguard ops actions rerun` and `rerun-failed-jobs` were removed in agentic-os#1428. They 404'd on this Forgejo, and the packaged `forgejo_actions_rerun` module went with them. Re-run a workflow from the Forgejo web UI, or push a new commit.

The fetch overlay in [AOSguard's Forgejo spec](../../../../.umbra/guardfiles/aosguard/forgejo.kdl) still pins the dead API rerun routes from agentic-os#473, so the dead shape stays documented rather than becoming another hand-coded HTTP call.


## Forgejo Actions logs

`aosguard ops actions logs` fetches workflow-run and job logs through Forgejo's
official REST API. It requires Forgejo 16.0 or newer. It does not read HTML,
call web routes, use browser cookies, or synthesize log text.

## Resolved command

```text
aosguard ops actions logs <owner> <repo> <run> [job] [attempt] [--max-bytes N]
```

```bash
aosguard ops actions logs coilyco-flight-deck agentic-os 2766 > run-2766.zip
aosguard ops actions logs coilyco-flight-deck agentic-os 2766 0
aosguard ops actions logs coilyco-flight-deck agentic-os 2766 0 2
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
