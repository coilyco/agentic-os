"""How stale a checkout is, reported as a pair rather than a count.

A walked checkout answers from whatever it last fetched, so a measurement taken
from one is only as current as that fetch. The commit gap alone cannot say so:
`@{u}` is a local remote-tracking ref, and a repository nobody has fetched
reports zero behind while arbitrarily old. See docs/features-agents.md.
"""
from __future__ import annotations

import subprocess
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path


@dataclass(frozen=True)
class Freshness:
    """The commit gap and the age of the ref that gap was measured against."""

    behind: int | None
    fetched_at: datetime | None

    @property
    def unknown(self) -> bool:
        return self.behind is None

    @property
    def trustworthy(self) -> bool:
        """Zero behind, measured against a ref that was actually fetched."""
        return self.behind == 0 and self.fetched_at is not None


def _git(repo_dir: Path, args: list[str]) -> str | None:
    try:
        done = subprocess.run(
            ["git", "-C", str(repo_dir), *args],
            capture_output=True,
            text=True,
            check=True,
        )
    except (subprocess.CalledProcessError, OSError):
        return None
    return done.stdout.strip()


def checkout_freshness(repo_dir: Path) -> Freshness:
    """The gap to the tracked upstream, and when that upstream ref last moved."""
    counted = _git(repo_dir, ["rev-list", "--count", "HEAD..@{u}"])
    behind: int | None = None
    if counted is not None and counted.isdigit():
        behind = int(counted)

    fetched_at = None
    marker = repo_dir / ".git" / "FETCH_HEAD"
    try:
        if marker.is_file():
            fetched_at = datetime.fromtimestamp(marker.stat().st_mtime, tz=timezone.utc)
    except OSError:
        fetched_at = None
    return Freshness(behind=behind, fetched_at=fetched_at)


def freshness_line(name: str, fresh: Freshness, now: datetime) -> str:
    """One row, always carrying both numbers.

    Reporting the gap alone would rebuild the defect this guards: a bare
    `behind:0` cannot be discounted, `behind:0 never fetched` can.
    """
    if fresh.unknown:
        gap = "no upstream"
    else:
        gap = f"{fresh.behind} behind"
    if fresh.fetched_at is None:
        age = "never fetched"
    else:
        hours = int((now - fresh.fetched_at).total_seconds() // 3600)
        age = f"fetched {hours}h ago"
    return f"{name:28} {gap}, {age}"


def stale_enough_to_mention(fresh: Freshness) -> bool:
    """Whether a reader quoting this measurement should be told about it."""
    return not fresh.trustworthy
