#!/usr/bin/env python3
"""Knowledge-base freshness probe: catch silently-rotted asserted facts.

Code drift is caught loud by CI. Knowledge rot is silent - a hand-written
fact (a verb name, a model id, a price) goes stale without any commit, and a
cold agent reads it confidently wrong. This probe is the detection layer for
that rot (agentic-os#262, action item 1).

It works off **provenance markers** a fact carries inline. A graded fact
declares, in a machine-readable HTML comment next to it:

    <!-- freshness: as-of=2026-06-24 decay-class=derived half-life=fast
         source="aguard ops forgejo describe" -->

- **as-of** (required) - the date the fact was last verified against its source.
- **half-life** (required for grading) - how fast the world rewrites it:
  `fast` (verbs, SDK APIs, model ids, pricing, ToS) gets a short horizon;
  `slow` (doctrine, voice, taste) a long one; `none` opts out of staleness
  while staying classified.
- **decay-class** (optional) - how the fact is stored: `asserted` (hand-written,
  highest decay), `pointer` (states where to fetch it fresh), `derived`
  (rendered from a ground-truth source, cannot drift past it).
- **source** (optional) - where the fact is re-verified from.

The probe walks tracked markdown, parses every marker, and grades each by age
against its half-life horizon. `--check` exits non-zero when any fast-decay
fact is past its horizon - the loud trigger a scheduled CI job fails on, the
thing code has and knowledge did not. `--lint` checks markers are well-formed
(a change-time concern) without grading age. Plain invocation prints a report.

Horizons are configurable in `[tool.agentic-os.freshness]` (fast_days,
slow_days, excludes). The probe is hermetic - no network, no binary - so it
runs anywhere; the heavier cold-agent assert-then-verify probe is a follow-up.
"""
from __future__ import annotations

import argparse
import re
import shlex
import sys
from dataclasses import dataclass, field
from datetime import date
from pathlib import Path
from typing import Iterable

from agentic_os.config import (
    REPO_ROOT,
    get_int_option,
    is_enabled,
    is_excluded,
    load_excludes,
)

HOOK_ID = "freshness"

DEFAULT_FAST_DAYS = 30
DEFAULT_SLOW_DAYS = 365

HALF_LIVES = ("fast", "slow", "none")
DECAY_CLASSES = ("asserted", "pointer", "derived")

# A provenance marker, tolerant of internal newlines so an editor can wrap it.
MARKER_RE = re.compile(r"<!--\s*freshness:\s*(.*?)\s*-->", re.DOTALL)
INLINE_CODE_RE = re.compile(r"`[^`\n]+`")


def strip_fenced_code(text: str) -> str:
    """Blank out fenced and inline code, preserving line count.

    A marker shown inside a ``` fence or an inline `code` span is documentation
    of the format (FEATURES.md, the program doc), not a live provenance claim,
    so the probe must not grade it. Fenced lines and inline spans are blanked
    without removing newlines, so reported line numbers still point at source.
    """
    out: list[str] = []
    in_fence = False
    for line in text.splitlines():
        if line.lstrip().startswith("```"):
            in_fence = not in_fence
            out.append("")
            continue
        out.append("" if in_fence else INLINE_CODE_RE.sub("", line))
    return "\n".join(out)


@dataclass
class Marker:
    """One parsed provenance marker, with any well-formedness problems."""

    path: Path
    line: int
    as_of: date | None = None
    decay_class: str | None = None
    half_life: str | None = None
    source: str | None = None
    raw: str = ""
    errors: list[str] = field(default_factory=list)

    @property
    def ok(self) -> bool:
        return not self.errors


def _parse_fields(body: str) -> dict[str, str]:
    """Split a marker body into key=value tokens, honouring quoted values."""
    out: dict[str, str] = {}
    for tok in shlex.split(body):
        if "=" not in tok:
            continue
        key, _, value = tok.partition("=")
        out[key.strip()] = value.strip()
    return out


def parse_markers(text: str, path: Path) -> list[Marker]:
    """Parse every freshness marker in `text`, validating each one's shape."""
    markers: list[Marker] = []
    for match in MARKER_RE.finditer(text):
        body = match.group(1)
        line = text.count("\n", 0, match.start()) + 1
        marker = Marker(path=path, line=line, raw=match.group(0))
        fields = _parse_fields(body)

        raw_as_of = fields.get("as-of")
        if raw_as_of is None:
            marker.errors.append("missing required `as-of`")
        else:
            try:
                marker.as_of = date.fromisoformat(raw_as_of)
            except ValueError:
                marker.errors.append(f"`as-of={raw_as_of}` is not an ISO date (YYYY-MM-DD)")

        half = fields.get("half-life")
        if half is None:
            marker.errors.append("missing required `half-life` (fast|slow|none)")
        elif half not in HALF_LIVES:
            marker.errors.append(f"`half-life={half}` not one of {'|'.join(HALF_LIVES)}")
        else:
            marker.half_life = half

        decay = fields.get("decay-class")
        if decay is not None:
            if decay not in DECAY_CLASSES:
                marker.errors.append(
                    f"`decay-class={decay}` not one of {'|'.join(DECAY_CLASSES)}"
                )
            else:
                marker.decay_class = decay

        marker.source = fields.get("source")
        markers.append(marker)
    return markers


def horizon_days(half_life: str | None, fast_days: int, slow_days: int) -> int | None:
    """Days a fact of this half-life may age before it is stale. None = never."""
    if half_life == "fast":
        return fast_days
    if half_life == "slow":
        return slow_days
    return None  # `none` or ungraded never goes stale on age alone


def age_days(marker: Marker, today: date) -> int | None:
    if marker.as_of is None:
        return None
    return (today - marker.as_of).days


def is_stale(marker: Marker, today: date, fast_days: int, slow_days: int) -> bool:
    """True when a well-formed, graded marker has aged past its horizon."""
    if not marker.ok:
        return False
    horizon = horizon_days(marker.half_life, fast_days, slow_days)
    age = age_days(marker, today)
    if horizon is None or age is None:
        return False
    return age > horizon


def iter_markdown_files(repo_root: Path) -> Iterable[Path]:
    excludes = load_excludes(HOOK_ID, repo_root)
    for path in sorted(repo_root.rglob("*.md")):
        rel = path.relative_to(repo_root)
        if is_excluded(rel, excludes):
            continue
        yield path


def scan(repo_root: Path) -> list[Marker]:
    """Every provenance marker across the repo's tracked markdown."""
    markers: list[Marker] = []
    for path in iter_markdown_files(repo_root):
        try:
            text = path.read_text(encoding="utf-8")
        except (OSError, UnicodeDecodeError):
            continue
        markers.extend(parse_markers(strip_fenced_code(text), path.relative_to(repo_root)))
    return markers


def _status(marker: Marker, today: date, fast_days: int, slow_days: int) -> str:
    if not marker.ok:
        return "MALFORMED"
    if is_stale(marker, today, fast_days, slow_days):
        return "STALE"
    horizon = horizon_days(marker.half_life, fast_days, slow_days)
    return "fresh" if horizon is not None else "graded"


def run(
    repo_root: Path,
    today: date,
    fast_days: int,
    slow_days: int,
    check: bool,
    lint: bool,
) -> int:
    markers = scan(repo_root)
    malformed = [m for m in markers if not m.ok]
    stale = [m for m in markers if is_stale(m, today, fast_days, slow_days)]

    if lint:
        if malformed:
            print(f"freshness: {len(malformed)} malformed provenance marker(s):", file=sys.stderr)
            for m in malformed:
                print(f"  {m.path}:{m.line}: {'; '.join(m.errors)}", file=sys.stderr)
            return 1
        print(f"freshness: {len(markers)} provenance marker(s), all well-formed.")
        return 0

    # Report mode (and the table the --check run prints before its verdict).
    if not markers:
        print("freshness: no provenance markers found.")
    else:
        print(f"freshness: {len(markers)} provenance marker(s) (horizons: fast={fast_days}d slow={slow_days}d)\n")
        for m in sorted(markers, key=lambda m: str(m.path)):
            status = _status(m, today, fast_days, slow_days)
            age = age_days(m, today)
            age_str = f"{age}d" if age is not None else "?"
            half = m.half_life or "?"
            print(f"  [{status:>9}] {m.path}:{m.line}  half-life={half} age={age_str}")

    if malformed:
        print(f"\nfreshness: {len(malformed)} malformed marker(s) (run --lint for detail).", file=sys.stderr)
    if stale:
        print(f"\nfreshness: {len(stale)} fact(s) past their half-life horizon - re-verify against source:", file=sys.stderr)
        for m in stale:
            src = f" source={m.source}" if m.source else ""
            print(f"  {m.path}:{m.line}: as-of {m.as_of} ({age_days(m, today)}d, half-life={m.half_life}){src}", file=sys.stderr)

    if check:
        return 1 if (stale or malformed) else 0
    return 0


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        prog="freshness",
        description="Grade knowledge-base provenance markers by half-life and flag rot.",
    )
    parser.add_argument(
        "--check",
        action="store_true",
        help="exit 1 if any fact is stale or malformed (for scheduled CI)",
    )
    parser.add_argument(
        "--lint",
        action="store_true",
        help="exit 1 if any marker is malformed, without grading age (change-time check)",
    )
    parser.add_argument(
        "--today",
        type=date.fromisoformat,
        default=None,
        help="override today's date (YYYY-MM-DD), for deterministic runs/tests",
    )
    parser.add_argument("--fast-days", type=int, default=None, help="override the fast half-life horizon")
    parser.add_argument("--slow-days", type=int, default=None, help="override the slow half-life horizon")
    parser.add_argument(
        "--repo-root", type=Path, default=None, help="repo root to scan (default: cwd)"
    )
    args = parser.parse_args(argv[1:] if argv is not None else None)

    repo_root = args.repo_root or REPO_ROOT
    if not is_enabled(HOOK_ID, repo_root):
        print("freshness: disabled via [tool.agentic-os.freshness] enabled=false.")
        return 0

    fast_days = args.fast_days if args.fast_days is not None else get_int_option(
        HOOK_ID, "fast_days", DEFAULT_FAST_DAYS, repo_root
    )
    slow_days = args.slow_days if args.slow_days is not None else get_int_option(
        HOOK_ID, "slow_days", DEFAULT_SLOW_DAYS, repo_root
    )
    today = args.today or date.today()

    return run(repo_root, today, fast_days, slow_days, check=args.check, lint=args.lint)


if __name__ == "__main__":
    sys.exit(main(sys.argv))
