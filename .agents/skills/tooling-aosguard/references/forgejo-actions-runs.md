# Forgejo Actions runs and logs (retired)

Everything on this page retired with Forgejo on 2026-11-01 (COI-3075). GitHub is canonical for every repo (COI-3083). The guardfiles and code that still name these surfaces are cleanup under COI-2931.

## Retired surfaces

* **`aosguard ops actions logs`** - the run and job log bridge over Forgejo's REST API, with its byte ranges, 64 MiB bound and typed exit states.
* **`aosguard ops actions-rerun rerun`** - the failed-run rerun that logged in to the Forgejo web UI as the bot.
* **`aosguard ops forgejo action-run`, `action-run-job`, `action-job` and `tasks`** - the direct run, job and task leaves, and the run id, run number, task id and job id split they needed.
* **Forgejo runners and their egress proxy** - `FORGEJO_EGRESS_PROXY` in `scripts/ci-command.sh` served Forgejo runners that no longer exist.

## On GitHub

Reading a failed run and rerunning it are `gh run list`, `gh run view ID --log-failed` and `gh run rerun ID --failed`, each with `--repo coilyco/<repo>`. The verbs are listed under the seat lifecycle in [gh-replacement.md](gh-replacement.md). A GitHub Actions workflow or runner image that replaces a Forgejo one exists only where its file has been read, so none is claimed here.

## History

The run list once pulled a repo's whole history because the wrapper ignored `--limit` without an explicit `page=1`. The pre-commit cache blocks that hid a cold hook install were removed when the image baked `PRE_COMMIT_HOME` (agentic-os#1031). Both are dated records of the old setup.
