from __future__ import annotations

from pathlib import Path

from agentic_os.pre_commit.check_composed_skills import (
    catalogue_problems,
    layout_problems,
    role_selectors,
)


def write(path: Path, body: str = "# Source\n") -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(body, encoding="utf-8")


def test_composed_layout_accepts_distinct_composed_entrypoints(tmp_path: Path) -> None:
    write(tmp_path / ".agents" / "skills" / "categories.yaml")
    write(tmp_path / ".agents" / "skills" / "coding-go" / "SKILL.md")
    write(tmp_path / ".agents" / "composed" / "coding-shape-cli" / "COMPOSED.md")

    assert layout_problems(tmp_path) == []


def test_composed_layout_rejects_discoverable_and_colliding_sources(
    tmp_path: Path,
) -> None:
    write(tmp_path / ".agents" / "skills" / "categories.yaml")
    write(tmp_path / ".agents" / "skills" / "coding-go" / "SKILL.md")
    write(tmp_path / ".agents" / "composed" / "coding-go" / "COMPOSED.md")
    write(tmp_path / ".agents" / "composed" / "coding-go" / "nested" / "SKILL.md")
    (tmp_path / ".agents" / "composed" / "missing").mkdir(parents=True)

    problems = layout_problems(tmp_path)

    assert any("must use COMPOSED.md" in problem for problem in problems)
    assert any("collides with" in problem for problem in problems)
    assert any("missing COMPOSED.md" in problem for problem in problems)


def test_composed_layout_requires_the_ordinary_taxonomy(tmp_path: Path) -> None:
    write(tmp_path / ".agents" / "composed" / "design-system" / "COMPOSED.md")

    assert layout_problems(tmp_path) == [
        ".agents/composed: role-composed sources require "
        ".agents/skills/categories.yaml"
    ]


# Deleting the parked `role exec` block left the Executive Strategist composing
# zero methods for eleven days, unnoticed. agentic-os#1073


def _catalogue(tmp_path: Path, kdl: str | None, names: list[str]) -> Path:
    composed = tmp_path / ".agents" / "composed"
    for name in names:
        (composed / name).mkdir(parents=True, exist_ok=True)
        (composed / name / "COMPOSED.md").write_text("", encoding="utf-8")
    composed.mkdir(parents=True, exist_ok=True)
    if kdl is not None:
        (tmp_path / ".agents" / "roles.kdl").write_text(kdl, encoding="utf-8")
    return composed


def test_a_role_with_no_selector_fails(tmp_path: Path) -> None:
    kdl = "roles {\n    role exec {\n    }\n\n    role qa {\n        composed-skill a\n    }\n}\n"
    composed = _catalogue(tmp_path, kdl, ["a"])

    problems = catalogue_problems(tmp_path, composed)

    assert len(problems) == 1
    assert "role exec has no composed-skill selector" in problems[0]


def test_an_unselected_source_fails(tmp_path: Path) -> None:
    kdl = "roles {\n    role qa {\n        composed-skill a\n    }\n}\n"
    composed = _catalogue(tmp_path, kdl, ["a", "orphan"])

    problems = catalogue_problems(tmp_path, composed)

    assert len(problems) == 1
    assert "orphan: no role selects it" in problems[0]


def test_a_glob_selector_claims_its_sources(tmp_path: Path) -> None:
    # The control. Most selectors are globs, so a literal-only match would
    # report the whole catalogue as orphaned.
    kdl = 'roles {\n    role qa {\n        composed-skill "coding-*"\n    }\n}\n'
    composed = _catalogue(tmp_path, kdl, ["coding-go", "coding-rust"])

    assert catalogue_problems(tmp_path, composed) == []


def test_a_repo_with_no_role_graph_is_left_alone(tmp_path: Path) -> None:
    # The hook ships to consumers that carry composed sources and no roles.kdl.
    composed = _catalogue(tmp_path, None, ["a"])

    assert role_selectors(tmp_path) is None
    assert catalogue_problems(tmp_path, composed) == []


def test_this_repos_catalogue_and_role_graph_agree() -> None:
    # The live pairing, through the opt-out list rather than around it.
    root = Path(__file__).resolve().parent.parent

    assert catalogue_problems(root, root / ".agents" / "composed") == []


def _with_unselected(tmp_path: Path, patterns: list[str]) -> None:
    body = ", ".join(f'"{p}"' for p in patterns)
    (tmp_path / "pyproject.toml").write_text(
        f"[tool.agentic-os.check-composed-skills]\nunselected = [{body}]\n",
        encoding="utf-8",
    )


def test_a_live_exemption_still_passes(tmp_path: Path) -> None:
    # The control. A pattern covering a genuinely unselected source is the
    # whole point of the escape hatch and must stay silent.
    kdl = "roles {\n    role qa {\n        composed-skill a\n    }\n}\n"
    composed = _catalogue(tmp_path, kdl, ["a", "orphan"])
    _with_unselected(tmp_path, ["orphan"])

    assert catalogue_problems(tmp_path, composed) == []


def test_an_exemption_whose_source_is_gone_fails(tmp_path: Path) -> None:
    kdl = "roles {\n    role qa {\n        composed-skill a\n    }\n}\n"
    composed = _catalogue(tmp_path, kdl, ["a"])
    _with_unselected(tmp_path, ["deleted-long-ago"])

    problems = catalogue_problems(tmp_path, composed)

    assert len(problems) == 1
    assert "'deleted-long-ago' covers no unselected source" in problems[0]


def test_an_exemption_a_role_now_selects_fails(tmp_path: Path) -> None:
    # The dangerous drift: the exemption is wrong and still active, so removing
    # the selector later would leave the gate silent.
    kdl = "roles {\n    role qa {\n        composed-skill a\n        composed-skill b\n    }\n}\n"
    composed = _catalogue(tmp_path, kdl, ["a", "b"])
    _with_unselected(tmp_path, ["b"])

    problems = catalogue_problems(tmp_path, composed)

    assert len(problems) == 1
    assert "'b' covers no unselected source" in problems[0]
