"""A checkout's staleness, reported as a pair so a bare zero cannot mislead."""
from __future__ import annotations

import subprocess
from datetime import datetime, timedelta, timezone
from pathlib import Path

from agentic_os.freshness import (
    Freshness,
    checkout_freshness,
    freshness_line,
    stale_enough_to_mention,
)

NOW = datetime(2026, 9, 14, tzinfo=timezone.utc)


def _repo(path: Path) -> Path:
    path.mkdir(parents=True, exist_ok=True)
    subprocess.run(["git", "init", "-q", str(path)], check=True)
    return path


# The path bug that produced a uniform "never fetched" column across 23 repos
# reads exactly like a fleet fact, so a known-good repo must report a real age.
def test_a_fetched_repo_reports_a_real_age(tmp_path: Path) -> None:
    repo = _repo(tmp_path / "fetched")
    marker = repo / ".git" / "FETCH_HEAD"
    marker.write_text("", encoding="utf-8")
    fresh = checkout_freshness(repo)
    assert fresh.fetched_at is not None
    assert (datetime.now(timezone.utc) - fresh.fetched_at) < timedelta(minutes=5)


def test_an_unfetched_repo_reports_no_age_rather_than_zero(tmp_path: Path) -> None:
    repo = _repo(tmp_path / "unfetched")
    assert checkout_freshness(repo).fetched_at is None


# No upstream is a third state: it is not zero behind and not a stale ref.
def test_no_upstream_reports_unknown_rather_than_zero(tmp_path: Path) -> None:
    repo = _repo(tmp_path / "detached")
    assert checkout_freshness(repo).behind is None


# The amendment that made the control worth building: a gap measured against a
# ref nobody fetched is a lower bound, so the line always carries both numbers.
def test_the_line_always_carries_both_numbers() -> None:
    stamped = datetime(2026, 9, 13, tzinfo=timezone.utc)
    line = freshness_line("repo", Freshness(behind=0, fetched_at=stamped), NOW)
    assert "0 behind" in line and "24h ago" in line

    never = freshness_line("repo", Freshness(behind=0, fetched_at=None), NOW)
    assert "0 behind" in never and "never fetched" in never


def test_an_unfetched_zero_is_still_worth_mentioning() -> None:
    assert stale_enough_to_mention(Freshness(behind=0, fetched_at=None))


def test_a_fetched_zero_is_not_worth_mentioning() -> None:
    assert not stale_enough_to_mention(Freshness(behind=0, fetched_at=NOW))


def test_any_gap_is_worth_mentioning() -> None:
    assert stale_enough_to_mention(Freshness(behind=1, fetched_at=NOW))
