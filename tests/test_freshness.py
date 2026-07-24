"""Tests for agentic_os.freshness: the knowledge-base provenance probe."""
from __future__ import annotations

from datetime import date
from pathlib import Path

import pytest

from agentic_os import config, freshness


def _write(root: Path, rel: str, body: str) -> None:
    p = root / rel
    p.parent.mkdir(parents=True, exist_ok=True)
    p.write_text(body, encoding="utf-8")


TODAY = date(2026, 6, 25)


def _run(monkeypatch: pytest.MonkeyPatch, root: Path, *args: str, today: date = TODAY) -> int:
    monkeypatch.setattr(freshness, "REPO_ROOT", root)
    monkeypatch.setattr(config, "REPO_ROOT", root)
    return freshness.main(["freshness", "--repo-root", str(root), "--today", today.isoformat(), *args])


# --- parsing ---------------------------------------------------------------


def test_parse_well_formed_marker() -> None:
    text = '<!-- freshness: as-of=2026-06-24 decay-class=derived half-life=fast source="aguard ops forgejo describe" -->'
    markers = freshness.parse_markers(text, Path("docs/x.md"))
    assert len(markers) == 1
    m = markers[0]
    assert m.ok
    assert m.as_of == date(2026, 6, 24)
    assert m.decay_class == "derived"
    assert m.half_life == "fast"
    assert m.source == "aguard ops forgejo describe"


def test_marker_may_wrap_across_lines() -> None:
    text = "<!-- freshness: as-of=2026-06-24\n     half-life=slow -->"
    markers = freshness.parse_markers(text, Path("docs/x.md"))
    assert len(markers) == 1 and markers[0].ok
    assert markers[0].half_life == "slow"


def test_line_number_is_reported() -> None:
    text = "line1\nline2\n<!-- freshness: as-of=2026-06-24 half-life=fast -->\n"
    markers = freshness.parse_markers(text, Path("docs/x.md"))
    assert markers[0].line == 3


def test_missing_required_fields_are_errors() -> None:
    markers = freshness.parse_markers("<!-- freshness: decay-class=asserted -->", Path("x.md"))
    errs = markers[0].errors
    assert any("as-of" in e for e in errs)
    assert any("half-life" in e for e in errs)
    assert not markers[0].ok


def test_bad_date_and_bad_enums_are_errors() -> None:
    text = "<!-- freshness: as-of=yesterday half-life=quick decay-class=guessed -->"
    m = freshness.parse_markers(text, Path("x.md"))[0]
    assert any("ISO date" in e for e in m.errors)
    assert any("half-life=quick" in e for e in m.errors)
    assert any("decay-class=guessed" in e for e in m.errors)


# --- horizon / staleness ---------------------------------------------------


def test_horizon_by_half_life() -> None:
    assert freshness.horizon_days("fast", 30, 365) == 30
    assert freshness.horizon_days("slow", 30, 365) == 365
    assert freshness.horizon_days("none", 30, 365) is None
    assert freshness.horizon_days(None, 30, 365) is None


def _marker(as_of: date, half: str) -> freshness.Marker:
    return freshness.Marker(path=Path("x.md"), line=1, as_of=as_of, half_life=half)


def test_fresh_fast_fact_is_not_stale() -> None:
    m = _marker(date(2026, 6, 20), "fast")  # 5 days old, 30d horizon
    assert not freshness.is_stale(m, TODAY, 30, 365)


def test_aged_fast_fact_is_stale() -> None:
    m = _marker(date(2026, 5, 1), "fast")  # 55 days old, 30d horizon
    assert freshness.is_stale(m, TODAY, 30, 365)


def test_slow_fact_tolerates_the_same_age() -> None:
    m = _marker(date(2026, 5, 1), "slow")  # 55 days old, 365d horizon
    assert not freshness.is_stale(m, TODAY, 30, 365)


def test_none_half_life_never_stale() -> None:
    m = _marker(date(2020, 1, 1), "none")
    assert not freshness.is_stale(m, TODAY, 30, 365)


def test_malformed_marker_is_not_graded_for_staleness() -> None:
    m = freshness.Marker(path=Path("x.md"), line=1, errors=["missing as-of"])
    assert not freshness.is_stale(m, TODAY, 30, 365)


# --- CLI -------------------------------------------------------------------


def test_check_passes_on_fresh_repo(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    _write(tmp_path, "docs/a.md", "<!-- freshness: as-of=2026-06-24 half-life=fast -->\n")
    assert _run(monkeypatch, tmp_path, "--check") == 0


def test_check_fails_on_stale_fact(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture[str]
) -> None:
    _write(tmp_path, "docs/a.md", "<!-- freshness: as-of=2026-01-01 half-life=fast -->\n")
    assert _run(monkeypatch, tmp_path, "--check") == 1
    assert "past their half-life horizon" in capsys.readouterr().err


def test_check_fails_on_malformed_marker(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    _write(tmp_path, "docs/a.md", "<!-- freshness: half-life=fast -->\n")
    assert _run(monkeypatch, tmp_path, "--check") == 1


def test_lint_passes_when_well_formed_but_stale(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    # lint grades shape, not age: an old-but-valid marker passes --lint.
    _write(tmp_path, "docs/a.md", "<!-- freshness: as-of=2026-01-01 half-life=fast -->\n")
    assert _run(monkeypatch, tmp_path, "--lint") == 0


def test_lint_fails_on_malformed(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture[str]
) -> None:
    _write(tmp_path, "docs/a.md", "<!-- freshness: as-of=nope half-life=fast -->\n")
    assert _run(monkeypatch, tmp_path, "--lint") == 1
    assert "malformed" in capsys.readouterr().err


def test_excludes_drop_a_path(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    _write(tmp_path, "docs/a.md", "<!-- freshness: as-of=2026-01-01 half-life=fast -->\n")
    _write(
        tmp_path,
        "pyproject.toml",
        '[tool.agentic-os.freshness]\nexcludes = ["docs/a.md"]\n',
    )
    assert _run(monkeypatch, tmp_path, "--check") == 0


def test_disabled_short_circuits(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    _write(tmp_path, "docs/a.md", "<!-- freshness: as-of=2026-01-01 half-life=fast -->\n")
    _write(tmp_path, "pyproject.toml", "[tool.agentic-os.freshness]\nenabled = false\n")
    assert _run(monkeypatch, tmp_path, "--check") == 0


def test_config_horizon_override(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    # A 5-day fast horizon makes a 10-day-old fast fact stale.
    _write(tmp_path, "docs/a.md", "<!-- freshness: as-of=2026-06-15 half-life=fast -->\n")
    _write(tmp_path, "pyproject.toml", "[tool.agentic-os.freshness]\nfast_days = 5\n")
    assert _run(monkeypatch, tmp_path, "--check") == 1


def test_marker_inside_code_fence_is_ignored(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    # A marker shown as a documented example must not be graded as a live fact.
    body = "Example:\n```\n<!-- freshness: as-of=2020-01-01 half-life=fast -->\n```\n"
    _write(tmp_path, "docs/a.md", body)
    assert _run(monkeypatch, tmp_path, "--check") == 0


def test_strip_fenced_code_preserves_line_count() -> None:
    text = "a\n```\nb\n```\nc"
    assert freshness.strip_fenced_code(text).count("\n") == text.count("\n")


def test_marker_in_inline_code_is_ignored(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    # FEATURES.md shows the marker shape inline in backticks - not a live fact.
    _write(tmp_path, "docs/a.md", "Carried as `<!-- freshness: as-of= half-life= -->` markers.\n")
    assert _run(monkeypatch, tmp_path, "--check") == 0


def test_report_lists_markers(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture[str]
) -> None:
    _write(tmp_path, "docs/a.md", "<!-- freshness: as-of=2026-06-24 half-life=fast -->\n")
    assert _run(monkeypatch, tmp_path) == 0
    assert str(Path("docs") / "a.md") in capsys.readouterr().out
