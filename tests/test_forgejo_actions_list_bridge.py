from __future__ import annotations

import io
import json
import urllib.request

import pytest

from agentic_os import forgejo_actions_list


class _Response(io.BytesIO):
    def __enter__(self):
        return self

    def __exit__(self, *_args):
        self.close()


@pytest.mark.parametrize(
    ("kind", "page", "tail"),
    [
        ("runs", 1, "/actions/runs?page=1&limit=1"),
        ("tasks", 3, "/actions/tasks?page=3&limit=1"),
    ],
)
def test_actions_list_defaults_page_and_honours_override(
    monkeypatch, capsys, kind: str, page: int, tail: str
) -> None:
    requests: list[urllib.request.Request] = []

    def fake_urlopen(request: urllib.request.Request):
        requests.append(request)
        return _Response(b"[]")

    monkeypatch.setattr(forgejo_actions_list.urllib.request, "urlopen", fake_urlopen)
    monkeypatch.setenv("FORGEJO_TOKEN", "token")
    monkeypatch.setenv("FORGEJO_BASE_URL", "https://forgejo.example")

    assert (
        forgejo_actions_list.main(
            [
                kind,
                "coilyco-flight-deck",
                "infrastructure",
                "--limit",
                "1",
                "--page",
                str(page),
            ]
        )
        == 0
    )

    assert json.loads(capsys.readouterr().out) == []
    assert requests[0].full_url.endswith(tail)
    assert requests[0].get_header("Authorization") == "token token"
    assert requests[0].get_header("Accept") == "application/json"


def test_actions_list_requires_the_guard_injected_token(monkeypatch, capsys) -> None:
    monkeypatch.delenv("FORGEJO_TOKEN", raising=False)

    assert (
        forgejo_actions_list.main(
            ["runs", "coilyco-flight-deck", "infrastructure"]
        )
        == 1
    )
    assert "FORGEJO_TOKEN is required" in capsys.readouterr().err
