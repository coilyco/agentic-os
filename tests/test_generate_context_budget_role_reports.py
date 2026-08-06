"""Tests for role context-budget reports."""

from __future__ import annotations

import json
from pathlib import Path

import pytest

from agentic_os.context_budget_role_seat import FORMAT
from agentic_os.generators import generate_context_budget_role_reports as reports

TEST_LAYOUTS = {
    "cloud-a": "frontier",
    "cloud-b": "frontier",
    "oss-a": "low-context",
    "oss-b": "low-context",
}


def _snapshot(
    role: str,
    seat: str,
    model_class: str,
    *,
    eager: int,
    lazy: int,
    composed: int,
) -> dict[str, object]:
    return {
        "format": FORMAT,
        "subject": {"role": role, "seat": seat},
        "bundle": {"model_class": model_class},
        "totals": {
            "eager": {"components": 1, "bytes": eager * 4, "tokens": eager},
            "lazy": {"components": 1, "bytes": lazy * 4, "tokens": lazy},
        },
        "breakdown": {
            "eager": {
                "role-composed-frontmatter": {
                    "components": composed,
                    "bytes": 0,
                    "tokens": 0,
                }
            },
            "lazy": {},
        },
        "components": {"eager": {}, "lazy": {}},
        "skills": {},
    }


def _write_snapshots(
    docs_dir: Path,
    *,
    wrong_class_seat: str | None = None,
) -> dict[str, str]:
    frontier_index = 0
    for seat, model_class in TEST_LAYOUTS.items():
        if model_class == "frontier":
            eager = 100 + frontier_index * 10
            frontier_index += 1
        else:
            eager = 80
        declared_class = model_class
        if seat == wrong_class_seat:
            declared_class = (
                "low-context" if model_class == "frontier" else "frontier"
            )
        snapshot = _snapshot(
            "content",
            seat,
            declared_class,
            eager=eager,
            lazy=200 if model_class == "frontier" else 120,
            composed=5 if model_class == "frontier" else 3,
        )
        path = docs_dir / f"context-budget-content-{seat}-current.yaml"
        path.write_text(json.dumps(snapshot), encoding="utf-8")
    return TEST_LAYOUTS


def test_loads_canonical_roles_from_launch_profiles(tmp_path: Path) -> None:
    profiles = tmp_path / "harness-launch-profiles.yaml"
    profiles.write_text(
        "roles:\n  engineer:\n    agent: codex\n  ops:\n    agent: claude\n",
        encoding="utf-8",
    )

    assert reports.load_canonical_roles(profiles) == ("engineer", "ops")


def test_render_computes_class_envelope_diff(tmp_path: Path) -> None:
    layouts = _write_snapshots(tmp_path)
    built = reports.build_reports(tmp_path, roles=("content",), layouts=layouts)
    rendered = built[tmp_path / "context-budget-role-content-current.md"]

    assert "**Frontier Content Manager**" in rendered
    assert "**Low-context Content Manager**" in rendered
    assert "eager saves 20 to 30 tokens" in rendered
    assert "lazy saves 80 tokens" in rendered
    assert "composed sources 5 -> 3" in rendered
    assert "every checked-in current snapshot" in rendered


def test_generate_then_check_drift(tmp_path: Path) -> None:
    layouts = _write_snapshots(tmp_path)
    assert reports.generate(tmp_path, roles=("content",), layouts=layouts) == 0
    assert reports.check_drift(tmp_path, roles=("content",), layouts=layouts) == 0

    report = tmp_path / "context-budget-role-content-current.md"
    report.write_text("# stale\n", encoding="utf-8")
    assert reports.check_drift(tmp_path, roles=("content",), layouts=layouts) == 1


def test_rejects_snapshot_model_class_drift(tmp_path: Path) -> None:
    low_context_seat = next(
        seat
        for seat, model_class in TEST_LAYOUTS.items()
        if model_class == "low-context"
    )
    layouts = _write_snapshots(tmp_path, wrong_class_seat=low_context_seat)

    with pytest.raises(RuntimeError, match="expected model class"):
        reports.build_reports(tmp_path, roles=("content",), layouts=layouts)


def test_rejects_malformed_snapshot(tmp_path: Path) -> None:
    layouts = _write_snapshots(tmp_path)
    malformed = tmp_path / "context-budget-content-oss-a-current.yaml"
    malformed.write_text("{}\n", encoding="utf-8")

    with pytest.raises(RuntimeError, match="unsupported role-seat context snapshot"):
        reports.build_reports(tmp_path, roles=("content",), layouts=layouts)


def test_available_classes_are_derived_from_snapshots(tmp_path: Path) -> None:
    layouts = _write_snapshots(tmp_path)
    for seat, model_class in layouts.items():
        if model_class == "low-context":
            (tmp_path / f"context-budget-content-{seat}-current.yaml").unlink()

    built = reports.build_reports(tmp_path, roles=("content",), layouts=layouts)
    rendered = built[tmp_path / "context-budget-role-content-current.md"]
    inventory = built[tmp_path / "context-budget-role-seat-current.md"]

    assert "**Frontier Content Manager**" in rendered
    assert "**Low-context Content Manager**" not in rendered
    assert "Only frontier snapshots are available" in rendered
    assert "frontier only." in inventory


def test_committed_role_reports_are_current() -> None:
    assert reports.check_drift() == 0
