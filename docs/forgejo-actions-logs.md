# Forgejo Actions logs

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
* Job - zero-based visible index, exact name, `name:<exact-name>`, or `id:<n>`.
  The `name:` prefix disambiguates a numeric name.
* Attempt - positive 1-based number. Omit it for Forgejo's latest attempt.

The resolver filters runs by visible number, reads that run's job list, and then
calls the log endpoint. Callers do not need internal run, job, or task IDs.

## Bytes and bounds

Single-job output is the exact body from
`GET /repos/{owner}/{repo}/actions/jobs/{job_id}/logs`. AOS requests successive
byte ranges without decoding them, so empty and non-UTF-8 logs remain exact.

Whole-run output is the exact ZIP from
`GET /repos/{owner}/{repo}/actions/runs/{run_id}/logs`. Forgejo may add
`.MISSING` markers for jobs that have not started or whose logs expired.

Both forms buffer at most 64 MiB by default and leave stdout empty when that
bound is exceeded. `--max-bytes` selects another positive bound. Successful
stdout is raw log or ZIP data. Diagnostics and warnings go to stderr.

## Typed states

* Running log with bytes - exit 0, exact snapshot, `running_log` warning.
* Completed log - exit 0 with exact bytes.
* Running log not ready - `running_log`, exit 75.
* Expired completed job log - `expired_log`, exit 69.
* Completed job that never executed - `log_unavailable`, exit 69.
* Missing run, job, or attempt - `missing_*`, exit 66.
* Authorization failure - `authorization_failure`, exit 77.
* Output above the bound - `too_large`, exit 65.

Whole-run ZIPs remain successful with `.MISSING` entries. AOS reads only those
small markers and warns without changing the archive bytes.

## Direct guarded leaves

```text
aosguard ops forgejo action-run-job list <owner> <repo> <internal-run-id>
aosguard ops forgejo action-job logs <owner> <repo> <internal-job-id> [--attempt N]
aosguard ops forgejo action-run logs <owner> <repo> <internal-run-id>
```

These expose the official API directly. Use the resolved command for an Actions
URL or job name. Both `logs` leaves return bytes exactly since the lock moved to umbra v0.142.0 (umbra#291).

## Prior bridge

The HTML log-cursor implementation and fixtures are removed. The shared web
helper remains only for reruns. `agentic-os#415` is closed and superseded.

## See also

* [aosguard](aosguard.md)
* [Forgejo Actions rerun bridge](forgejo-actions-runs.md)
* [Forgejo Actions list bridge](forgejo-actions-runs.md)
