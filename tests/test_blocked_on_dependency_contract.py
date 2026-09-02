"""Tests for the blocked-on-dependency contract docs."""
from __future__ import annotations

from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
AUTOMATION_MODE = (
    ROOT
    / ".agents"
    / "composed"
    / "tooling-issue-triage"
    / "references"
    / "automation-mode-axis.md"
)
READINESS_AXIS = (
    ROOT
    / ".agents"
    / "composed"
    / "tooling-issue-triage"
    / "references"
    / "readiness-axis.md"
)


def _text(path: Path) -> str:
    return path.read_text(encoding="utf-8")


def test_blocker_pointer_marker_is_pinned_to_issue_body_comment() -> None:
    body = _text(READINESS_AXIS)
    assert "<!-- ward-blocked-on: owner/repo#123 -->" in body
    assert "The pointer lives in the issue body" in body
    assert "source of truth" in body


def test_automation_axis_points_readers_at_readiness_contract() -> None:
    body = _text(AUTOMATION_MODE)
    assert "See [readiness-axis](readiness-axis.md)" in body


def test_blocked_on_dependency_wakes_only_on_issue_close() -> None:
    body = _text(READINESS_AXIS)
    assert "blocker issue closes" in body
    assert "auto-resume into the `autonomy/headless` queue" in body
    assert "PR merge" not in body
    assert "release tag" not in body


def test_missing_or_ambiguous_blocker_ref_fails_closed() -> None:
    readiness = _text(READINESS_AXIS)
    assert "If the blocker pointer is missing or ambiguous, the issue fails closed" in readiness
    assert "does not claim wake behavior" in readiness
