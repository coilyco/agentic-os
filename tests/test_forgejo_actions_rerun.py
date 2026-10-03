from __future__ import annotations

import dataclasses
import http.cookiejar
import io
import json
import urllib.error
import urllib.request
from email.message import Message
from typing import Any

import pytest

from agentic_os import forgejo_actions_rerun as rerun
from agentic_os.forgejo_actions_logs import ForgejoAPI, RepositoryTarget

REPOSITORY = RepositoryTarget("coilyco", "website")
BASE = "https://forgejo.example"
PASSWORD = "hunter2-fixture"
RUN = {"id": 50971, "index_in_repo": 1831, "status": "cancelled", "event": "pull_request"}
JOBS = [
    {"id": 76195, "name": "lint", "attempt": 1, "status": "success", "task_id": 1},
    {"id": 76196, "name": "test-e2e", "attempt": 1, "status": "cancelled", "task_id": 2},
]


class FakeResponse:
    def __init__(self, body: bytes):
        self._body = io.BytesIO(body)
        self.headers = Message()

    def __enter__(self) -> FakeResponse:
        return self

    def __exit__(self, *args: object) -> None:
        return None

    def read(self, size: int = -1) -> bytes:
        return self._body.read(size)

    def close(self) -> None:
        return None


def _http_error(url: str, code: int, body: bytes = b"", **headers: str):
    message = Message()
    for key, value in headers.items():
        message[key] = value
    return urllib.error.HTTPError(url, code, "fixture", message, io.BytesIO(body))


def _serve_api(monkeypatch: pytest.MonkeyPatch, run: dict[str, Any]) -> ForgejoAPI:
    """Answer the three metadata reads the planner makes, by path."""

    def urlopen(request: urllib.request.Request, **_: object) -> FakeResponse:
        path = request.full_url.split("?")[0]
        if path.endswith("/jobs"):
            return FakeResponse(json.dumps(JOBS).encode())
        if path.endswith("/actions/runs"):
            return FakeResponse(json.dumps({"workflow_runs": [run]}).encode())
        return FakeResponse(json.dumps(run).encode())

    monkeypatch.setattr(urllib.request, "urlopen", urlopen)
    return ForgejoAPI(base_url=BASE, token="token")


@dataclasses.dataclass
class Script:
    """What the fake web opener does, and what it saw."""

    login: str = "redirect"
    rerun_status: int = 200
    rerun_body: bytes = b'{"redirect": "https://forgejo.example/coilyco/website/actions/runs/1831/attempt/2"}'
    requests: list[urllib.request.Request] = dataclasses.field(default_factory=list)


class FakeOpener:
    def __init__(self, script: Script, jar: http.cookiejar.CookieJar):
        self.script = script
        self.jar = jar

    def open(self, request: urllib.request.Request, **_: object) -> FakeResponse:
        self.script.requests.append(request)
        if request.full_url.endswith("/user/login"):
            return self._login(request)
        if request.full_url.endswith("/rerun"):
            if self.script.rerun_status != 200:
                raise _http_error(
                    request.full_url, self.script.rerun_status, self.script.rerun_body
                )
            return FakeResponse(self.script.rerun_body)
        return FakeResponse(b"")

    def _login(self, request: urllib.request.Request) -> FakeResponse:
        if self.script.login == "rejected":
            return FakeResponse(b"<form>wrong password</form>")
        if self.script.login == "two_factor":
            raise _http_error(request.full_url, 303, Location="/user/two_factor")
        self.jar.set_cookie(
            http.cookiejar.Cookie(
                0, "session", "v", None, False, "forgejo.example", True, False,
                "/", True, True, None, True, None, None, {},
            )
        )
        raise _http_error(request.full_url, 303, Location="/")


@pytest.fixture
def script(monkeypatch: pytest.MonkeyPatch) -> Script:
    scripted = Script()

    def build_opener(*handlers: Any) -> FakeOpener:
        jar = next(h.cookiejar for h in handlers if hasattr(h, "cookiejar"))
        return FakeOpener(scripted, jar)

    monkeypatch.setattr(urllib.request, "build_opener", build_opener)
    return scripted


def _session() -> rerun.WebSession:
    return rerun.WebSession(base_url=BASE, user="bot", password=PASSWORD)


@pytest.mark.parametrize("status", ["failure", "cancelled"])
def test_a_failed_or_cancelled_pull_request_run_is_rerunnable(
    monkeypatch: pytest.MonkeyPatch, status: str
) -> None:
    api = _serve_api(monkeypatch, {**RUN, "status": status})

    rerun.check_rerunnable(api, REPOSITORY, 50971, status)


@pytest.mark.parametrize("event", ["push", "schedule", "workflow_dispatch", None])
def test_a_run_not_triggered_by_a_pull_request_is_refused(
    monkeypatch: pytest.MonkeyPatch, event: str | None
) -> None:
    api = _serve_api(monkeypatch, {**RUN, "status": "failure", "event": event})

    with pytest.raises(rerun.ForgejoRerunError) as caught:
        rerun.check_rerunnable(api, REPOSITORY, 50971, "failure")

    assert caught.value.kind == "not_rerunnable"
    assert repr(event) in str(caught.value)


@pytest.mark.parametrize("status", ["success", "running", "waiting", "skipped"])
def test_a_pull_request_run_that_did_not_fail_is_refused(
    monkeypatch: pytest.MonkeyPatch, status: str
) -> None:
    api = _serve_api(monkeypatch, {**RUN, "status": status})

    with pytest.raises(rerun.ForgejoRerunError) as caught:
        rerun.check_rerunnable(api, REPOSITORY, 50971, status)

    assert caught.value.kind == "not_rerunnable"


def test_the_plan_names_the_run_number_and_resolves_a_job_by_name(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    api = _serve_api(monkeypatch, RUN)

    plan = rerun.plan_rerun(api, REPOSITORY, "1831", "test-e2e")

    assert plan == rerun.RerunPlan(1831, "cancelled", 1, "test-e2e")


def test_the_plan_without_a_job_reruns_the_whole_run(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    api = _serve_api(monkeypatch, RUN)

    plan = rerun.plan_rerun(api, REPOSITORY, "1831", None)

    assert plan == rerun.RerunPlan(1831, "cancelled", None, None)


def test_login_posts_the_form_and_needs_no_csrf_token(script: Script) -> None:
    _session()

    login = script.requests[0]
    assert login.full_url == f"{BASE}/user/login"
    assert login.data == f"user_name=bot&password={PASSWORD}".encode()


def test_a_rejected_login_is_a_typed_failure_that_never_echoes_the_password(
    script: Script,
) -> None:
    script.login = "rejected"

    with pytest.raises(rerun.ForgejoRerunError) as caught:
        _session()

    assert caught.value.kind == "login_failure"
    assert PASSWORD not in str(caught.value)


def test_a_bot_that_asks_for_a_second_factor_is_named(script: Script) -> None:
    script.login = "two_factor"

    with pytest.raises(rerun.ForgejoRerunError, match="second factor"):
        _session()


def test_a_whole_run_rerun_posts_the_run_route_and_returns_the_new_url(
    script: Script,
) -> None:
    session = _session()

    url = session.rerun(REPOSITORY, rerun.RerunPlan(1831, "cancelled", None, None))

    post = script.requests[-1]
    assert post.get_method() == "POST"
    assert post.full_url == f"{BASE}/coilyco/website/actions/runs/1831/rerun"
    assert url.endswith("/attempt/2")


def test_a_single_job_rerun_posts_the_job_route(script: Script) -> None:
    session = _session()

    session.rerun(REPOSITORY, rerun.RerunPlan(1831, "cancelled", 1, "test-e2e"))

    assert script.requests[-1].full_url.endswith("/actions/runs/1831/jobs/1/rerun")


@pytest.mark.parametrize(
    ("status", "body", "kind"),
    [
        (400, b'{"errorMessage": "workflow is still running"}', "rerun_refused"),
        (403, b"", "authorization_failure"),
        (404, b"", "missing_run"),
        (500, b"", "api_failure"),
    ],
)
def test_forgejo_refusals_map_to_typed_errors(
    script: Script, status: int, body: bytes, kind: str
) -> None:
    session = _session()
    script.rerun_status = status
    script.rerun_body = body

    with pytest.raises(rerun.ForgejoRerunError) as caught:
        session.rerun(REPOSITORY, rerun.RerunPlan(1831, "cancelled", None, None))

    assert caught.value.kind == kind
    if status == 400:
        assert "still running" in str(caught.value)


def test_dry_run_checks_the_run_and_never_logs_in(
    monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture[str]
) -> None:
    monkeypatch.setenv("FORGEJO_TOKEN", "token")
    monkeypatch.setenv("FORGEJO_BASE_URL", BASE)
    monkeypatch.delenv("FORGEJO_WEB_PASSWORD", raising=False)
    _serve_api(monkeypatch, RUN)

    def no_login(**_: object) -> None:
        raise AssertionError("a dry run must not log in")

    monkeypatch.setattr(rerun, "WebSession", no_login)

    code = rerun.main(["coilyco", "website", "1831", "--job", "test-e2e", "--dry-run"])

    assert code == 0
    assert "would rerun job 'test-e2e' of run 1831" in capsys.readouterr().out


def test_a_refused_run_exits_before_the_password_is_used(
    monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture[str]
) -> None:
    monkeypatch.setenv("FORGEJO_TOKEN", "token")
    monkeypatch.setenv("FORGEJO_BASE_URL", BASE)
    monkeypatch.setenv("FORGEJO_WEB_PASSWORD", PASSWORD)
    _serve_api(monkeypatch, {**RUN, "event": "push"})

    def no_login(**_: object) -> None:
        raise AssertionError("a refused run must not reach the login")

    monkeypatch.setattr(rerun, "WebSession", no_login)

    code = rerun.main(["coilyco", "website", "1831"])

    assert code == rerun.EXIT_CODES["not_rerunnable"]
    err = capsys.readouterr().err
    assert "not_rerunnable" in err
    assert PASSWORD not in err


def test_a_run_that_does_not_exist_reports_the_resolver_error(
    monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture[str]
) -> None:
    monkeypatch.setenv("FORGEJO_TOKEN", "token")
    monkeypatch.setenv("FORGEJO_BASE_URL", BASE)
    monkeypatch.setenv("FORGEJO_WEB_PASSWORD", PASSWORD)
    monkeypatch.setattr(
        urllib.request,
        "urlopen",
        lambda *_, **__: FakeResponse(json.dumps({"workflow_runs": []}).encode()),
    )

    code = rerun.main(["coilyco", "website", "9999"])

    assert code == 66
    assert "missing_run" in capsys.readouterr().err


def test_main_requires_the_credentials(
    monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture[str]
) -> None:
    monkeypatch.delenv("FORGEJO_TOKEN", raising=False)
    monkeypatch.delenv("FORGEJO_WEB_PASSWORD", raising=False)

    assert rerun.main(["coilyco", "website", "1831"]) == 77
    assert "authorization_failure" in capsys.readouterr().err
