"""Tests for the tiered dev-base publish workflow surface."""
from __future__ import annotations

from pathlib import Path


ROOT = Path(__file__).resolve().parent.parent
PROMOTE = ROOT / ".forgejo" / "workflows" / "promote.yml"
RELEASE = ROOT / ".forgejo" / "workflows" / "release.yml"


def _assert_alert_steps_are_non_blocking(text: str) -> None:
    lines = text.splitlines()
    alert_indexes = [
        i for i, line in enumerate(lines) if line.strip() == "- name: Alert Telegram on main failure"
    ]
    assert alert_indexes
    for idx in alert_indexes:
        window = lines[idx : idx + 8]
        assert any("continue-on-error: true" in line for line in window)


def test_dev_base_publish_workflows_support_tier_reruns_and_non_blocking_alerts() -> None:
    promote = PROMOTE.read_text(encoding="utf-8")
    release = RELEASE.read_text(encoding="utf-8")

    for needle in (
        "sha:",
        "tier:",
        "tag:",
        "continue-on-error: true",
    ):
        assert needle in promote

    for needle in (
        "sha:",
        "tier:",
        "tag:",
        "source-tag:",
        "tag_name:",
        "release_sha:",
        "continue-on-error: true",
    ):
        assert needle in release

    assert "needs: [plan-release, retag-core]" in release
    assert "needs: [plan-release, retag-ops, retag-lang-node]" in release
    assert "needs: [plan-release, retag-agent, retag-lang-go, retag-lang-dotnet]" in release
    _assert_alert_steps_are_non_blocking(promote)
    _assert_alert_steps_are_non_blocking(release)
