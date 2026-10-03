"""Rerun a failed pull-request Actions run through Forgejo's web UI route.

Forgejo 16 serves no REST route to rerun a run, so this logs in as the bot and
posts to the web route the UI button uses. Every guard runs on the API token
first, and the password is only used once a run has passed them. See
tooling-aosguard references/forgejo-actions-runs.md.
"""

from __future__ import annotations

import argparse
import dataclasses
import http.cookiejar
import json
import os
import sys
import urllib.error
import urllib.parse
import urllib.request
from typing import Any

from agentic_os import shared_ssl_context
from agentic_os.forgejo_actions_logs import (
    DEFAULT_BASE_URL,
    ForgejoActionsLogError,
    ForgejoAPI,
    RepositoryTarget,
    list_jobs,
    resolve_job,
    resolve_run,
)

DEFAULT_WEB_USER = "coilyco-ops"
GUARDED_EVENT = "pull_request"
RERUNNABLE_STATUSES = frozenset({"failure", "cancelled"})
MAX_BODY_BYTES = 64 * 1024
TIMEOUT_SECONDS = 60

EXIT_CODES = {
    "not_rerunnable": 65,
    "login_failure": 77,
    "authorization_failure": 77,
    "missing_run": 66,
    "rerun_refused": 69,
    "api_failure": 69,
    "api_contract": 70,
}


class ForgejoRerunError(RuntimeError):
    """A typed failure that the command renders without touching stdout."""

    def __init__(self, kind: str, message: str):
        super().__init__(message)
        self.kind = kind
        self.exit_code = EXIT_CODES[kind]


@dataclasses.dataclass(frozen=True)
class RerunPlan:
    run_number: int
    status: str
    job_index: int | None
    job_name: str | None


class _NoRedirect(urllib.request.HTTPRedirectHandler):
    """Surface a redirect as an HTTPError, because a login answers with one."""

    def redirect_request(self, *_args: Any, **_kwargs: Any) -> None:
        return None


def check_rerunnable(
    api: ForgejoAPI, repository: RepositoryTarget, run_id: int, status: str
) -> None:
    """Refuse anything but a failed or cancelled pull-request run.

    A push or schedule run can be a release or a deploy, and Forgejo's own
    check only rejects a run that is still going.
    """
    payload = api.get_json(
        f"{repository.api_path()}/runs/{run_id}", missing_kind="missing_run"
    )
    event = payload.get("event") if isinstance(payload, dict) else None
    if event != GUARDED_EVENT:
        raise ForgejoRerunError(
            "not_rerunnable",
            f"the run was triggered by {event!r}, and only {GUARDED_EVENT} runs can be rerun here.",
        )
    if status not in RERUNNABLE_STATUSES:
        raise ForgejoRerunError(
            "not_rerunnable",
            f"the run has status {status!r}, and only a failed or cancelled run can be rerun.",
        )


def plan_rerun(
    api: ForgejoAPI,
    repository: RepositoryTarget,
    run_identifier: str,
    job_identifier: str | None,
) -> RerunPlan:
    run = resolve_run(api, repository, run_identifier)
    check_rerunnable(api, repository, run.id, run.status)
    if job_identifier is None:
        return RerunPlan(run.index, run.status, None, None)
    job = resolve_job(list_jobs(api, repository, run), job_identifier)
    return RerunPlan(run.index, run.status, job.index, job.name)


class WebSession:
    """A cookie-jar session for the bot, logged in on construction."""

    def __init__(self, *, base_url: str, user: str, password: str):
        self.base_url = base_url.rstrip("/")
        self.jar = http.cookiejar.CookieJar()
        self.opener = urllib.request.build_opener(
            urllib.request.HTTPCookieProcessor(self.jar),
            urllib.request.HTTPSHandler(context=shared_ssl_context()),
            _NoRedirect,
        )
        self._login(user, password)

    def _open(self, request: urllib.request.Request) -> Any:
        return self.opener.open(request, timeout=TIMEOUT_SECONDS)

    def _login(self, user: str, password: str) -> None:
        # Forgejo 16 guards POSTs with Go's CrossOriginProtection, which only
        # refuses browser cross-site headers, so no CSRF token is fetched.
        form = urllib.parse.urlencode({"user_name": user, "password": password})
        request = urllib.request.Request(
            f"{self.base_url}/user/login",
            data=form.encode("utf-8"),
            headers={"Content-Type": "application/x-www-form-urlencoded"},
        )
        try:
            self._open(request).close()
        except urllib.error.HTTPError as exc:
            location = exc.headers.get("Location", "")
            exc.close()
            if exc.code in (302, 303) and "two_factor" in location:
                raise ForgejoRerunError(
                    "login_failure", "the bot account asks for a second factor."
                ) from exc
            if exc.code in (302, 303) and "/user/login" not in location and len(self.jar):
                return
            raise ForgejoRerunError(
                "login_failure", f"Forgejo answered the login with HTTP {exc.code}."
            ) from exc
        except OSError as exc:
            raise ForgejoRerunError(
                "api_failure", f"Forgejo login request failed: {exc}."
            ) from exc
        raise ForgejoRerunError("login_failure", "Forgejo rejected the bot login.")

    def rerun(self, repository: RepositoryTarget, plan: RerunPlan) -> str:
        owner = urllib.parse.quote(repository.owner, safe="")
        repo = urllib.parse.quote(repository.repo, safe="")
        path = f"/{owner}/{repo}/actions/runs/{plan.run_number}"
        if plan.job_index is not None:
            path += f"/jobs/{plan.job_index}"
        request = urllib.request.Request(
            f"{self.base_url}{path}/rerun",
            data=b"",
            method="POST",
            headers={"Accept": "application/json"},
        )
        try:
            with self._open(request) as response:
                body = response.read(MAX_BODY_BYTES)
        except urllib.error.HTTPError as exc:
            raise _rerun_error(exc) from exc
        except OSError as exc:
            raise ForgejoRerunError(
                "api_failure", f"Forgejo rerun request failed: {exc}."
            ) from exc
        try:
            redirect = json.loads(body).get("redirect")
        except (UnicodeDecodeError, json.JSONDecodeError, AttributeError) as exc:
            raise ForgejoRerunError(
                "api_contract", "Forgejo returned an unreadable rerun response."
            ) from exc
        if not isinstance(redirect, str) or not redirect:
            raise ForgejoRerunError(
                "api_contract", "Forgejo accepted the rerun but returned no run URL."
            )
        return redirect

    def close(self) -> None:
        """Best-effort logout, so a session does not outlive the command."""
        try:
            request = urllib.request.Request(
                f"{self.base_url}/user/logout", data=b"", method="POST"
            )
            self._open(request).close()
        except (urllib.error.URLError, OSError):
            pass


def _rerun_error(exc: urllib.error.HTTPError) -> ForgejoRerunError:
    code = exc.code
    detail = ""
    if code == 400:
        try:
            detail = str(json.loads(exc.read(MAX_BODY_BYTES)).get("errorMessage", ""))
        except (UnicodeDecodeError, json.JSONDecodeError, AttributeError):
            detail = ""
    exc.close()
    if code in (302, 303, 401, 403):
        return ForgejoRerunError(
            "authorization_failure",
            f"Forgejo refused the rerun with HTTP {code}, so the bot lacks Actions write on this repository.",
        )
    if code == 404:
        return ForgejoRerunError("missing_run", "Forgejo has no such run or job to rerun.")
    if code == 400:
        return ForgejoRerunError(
            "rerun_refused", f"Forgejo refused the rerun: {detail or 'HTTP 400'}."
        )
    return ForgejoRerunError("api_failure", f"Forgejo returned HTTP {code} for the rerun.")


def _parse_args(argv: list[str] | None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        prog="actions-rerun rerun",
        description="Rerun a failed or cancelled pull-request Actions run.",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""Reruns the whole run, or one job with --job. Only a pull_request
run with status failure or cancelled is accepted, so a push, schedule, or
release run is refused. --dry-run checks the run and prints the plan without
logging in. stdout carries the new run URL, and diagnostics go to stderr.""",
    )
    parser.add_argument("owner", help="repository owner")
    parser.add_argument("repo", help="repository name")
    parser.add_argument("run", help="visible run number or id:<n>")
    parser.add_argument(
        "--job", help="rerun one job: visible index, exact name, name:<n>, or id:<n>"
    )
    parser.add_argument(
        "--dry-run", action="store_true", help="check the run and stop before login"
    )
    return parser.parse_args(argv)


def _fail(kind: str, message: str, code: int) -> int:
    print(f"forgejo-actions-rerun-error: {kind}: {message}", file=sys.stderr)
    return code


def main(argv: list[str] | None = None) -> int:
    args = _parse_args(argv)
    token = os.environ.get("FORGEJO_TOKEN")
    password = os.environ.get("FORGEJO_WEB_PASSWORD")
    if not token or not (password or args.dry_run):
        return _fail(
            "authorization_failure",
            "FORGEJO_TOKEN and FORGEJO_WEB_PASSWORD are required",
            EXIT_CODES["authorization_failure"],
        )

    base_url = os.environ.get("FORGEJO_BASE_URL", DEFAULT_BASE_URL)
    repository = RepositoryTarget(args.owner, args.repo)
    api = ForgejoAPI(base_url=base_url, token=token)
    try:
        plan = plan_rerun(api, repository, args.run, args.job)
        target = f"job {plan.job_name!r}" if plan.job_name else "every job"
        if args.dry_run:
            print(f"would rerun {target} of run {plan.run_number} ({plan.status})")
            return 0
        session = WebSession(
            base_url=base_url,
            user=os.environ.get("FORGEJO_WEB_USER", DEFAULT_WEB_USER),
            password=password or "",
        )
        try:
            url = session.rerun(repository, plan)
        finally:
            session.close()
    except (ForgejoRerunError, ForgejoActionsLogError) as exc:
        return _fail(exc.kind, str(exc), exc.exit_code)

    print(f"rerun requested for {target} of run {plan.run_number}: {url}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
