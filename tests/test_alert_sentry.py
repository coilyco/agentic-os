"""The run link the Sentry alert carries must open the failing GitHub run."""
from __future__ import annotations

import importlib.util
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent


def _load():
    spec = importlib.util.spec_from_file_location("alert_sentry", ROOT / "scripts" / "alert_sentry.py")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def test_run_url_uses_the_run_id_on_github(monkeypatch) -> None:
    module = _load()
    for name in ("RUN_URL", "FORGE_URL", "REPO"):
        monkeypatch.delenv(name, raising=False)
    monkeypatch.setenv("GITHUB_REPOSITORY", "coilyco/eco-ops")
    monkeypatch.setenv("GITHUB_RUN_ID", "41280565920")
    monkeypatch.setenv("GITHUB_RUN_NUMBER", "7")

    assert module.run_url() == "https://github.com/coilyco/eco-ops/actions/runs/41280565920"


def test_an_explicit_run_url_wins(monkeypatch) -> None:
    module = _load()
    monkeypatch.setenv("RUN_URL", "https://example.test/run/1")

    assert module.run_url() == "https://example.test/run/1"


def test_run_url_is_a_visible_gap_without_a_run(monkeypatch) -> None:
    module = _load()
    for name in ("RUN_URL", "GITHUB_RUN_ID"):
        monkeypatch.delenv(name, raising=False)
    monkeypatch.setenv("GITHUB_REPOSITORY", "coilyco/eco-ops")

    assert module.run_url() == "?"
