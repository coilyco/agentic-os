"""A red main run opens one Linear record per workflow and comments on it after."""
from __future__ import annotations

import importlib.util
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent


def _load():
    spec = importlib.util.spec_from_file_location("file_main_red", ROOT / "scripts" / "file_main_red.py")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class Linear:
    """A stand-in for the GraphQL call, recording each request and answering by shape."""

    def __init__(self, open_issue: bool) -> None:
        self.open_issue = open_issue
        self.requests: list[tuple[str, dict]] = []

    def __call__(self, _key: str, query: str, variables: dict) -> dict:
        self.requests.append((query, variables))
        if "issues(first" in query:
            nodes = [{"id": "u1", "identifier": "COI-1"}] if self.open_issue else []
            return {"issues": {"nodes": nodes}}
        if "teams(" in query:
            labels = [{"id": "l1", "name": "agentic-os"}, {"id": "l2", "name": "unrelated"}]
            return {"teams": {"nodes": [{"id": "t1", "labels": {"nodes": labels}}]}}
        if "commentCreate" in query:
            return {"commentCreate": {"success": True}}
        return {"issueCreate": {"issue": {"identifier": "COI-2"}}}


def test_a_first_red_run_opens_a_record_with_its_labels(monkeypatch) -> None:
    module = _load()
    linear = Linear(open_issue=False)
    monkeypatch.setattr(module, "call", linear)

    assert module.file_failure("key", "t", "body") == "COI-2"

    created = linear.requests[-1][1]["input"]
    assert created["labelIds"] == ["l1"]
    assert created["teamId"] == "t1"
    assert created["priority"] == 2


def test_a_second_red_run_comments_instead_of_opening_another(monkeypatch) -> None:
    module = _load()
    linear = Linear(open_issue=True)
    monkeypatch.setattr(module, "call", linear)

    assert module.file_failure("key", "t", "body") == "COI-1"

    assert not any("issueCreate" in query for query, _ in linear.requests)
    assert linear.requests[-1][1] == {"issue": "u1", "body": "body"}


def test_the_record_names_the_run_and_the_commit() -> None:
    module = _load()
    env = {
        "GITHUB_REPOSITORY": "coilyco/agentic-os",
        "GITHUB_WORKFLOW": "aterm-client",
        "GITHUB_SHA": "a73f4611deadbeef",
        "GITHUB_RUN_ID": "42928757239",
        "COMMIT_SUBJECT": "fix(aterm-client): a thing (#1)\n\nbody line",
    }

    title, body = module.describe(env)

    assert title == "coilyco/agentic-os main: aterm-client is red"
    assert "https://github.com/coilyco/agentic-os/actions/runs/42928757239" in body
    assert "`a73f4611` fix(aterm-client): a thing (#1)" in body
    assert "body line" not in body


def test_without_a_key_the_step_warns_and_files_nothing(monkeypatch, capsys) -> None:
    module = _load()
    linear = Linear(open_issue=False)
    monkeypatch.setattr(module, "call", linear)
    monkeypatch.delenv("LINEAR_ALERT_KEY", raising=False)

    assert module.main() == 0

    assert linear.requests == []
    assert "::warning::LINEAR_ALERT_KEY is not set" in capsys.readouterr().out
