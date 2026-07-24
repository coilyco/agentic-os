from __future__ import annotations

from pathlib import Path

from agentic_os.pre_commit.check_composed_skills import layout_problems


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
