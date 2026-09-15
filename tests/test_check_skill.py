from __future__ import annotations

from pathlib import Path

import pytest

from agentic_os.pre_commit import check_skill

CATEGORIES = """\
categories:
  - id: lore
    kind: prefix
    prefix: lore-
    enforce_status: false
forbidden_body_strings: []
max_skill_md_lines: 500
max_skill_md_bytes: 4000
max_description_bytes: 0
"""

ARCHIVE = CATEGORIES + "archive_path_components: ['{component}']\n"


def write_skill(root: Path, name: str, body_bytes: int) -> None:
    path = root / ".agents" / "skills" / name / "SKILL.md"
    path.parent.mkdir(parents=True, exist_ok=True)
    head = f"---\nname: {name}\ndescription: An entry.\n---\n\n# Entry\n\n"
    path.write_text(head + "x" * max(0, body_bytes - len(head)), encoding="utf-8")


@pytest.fixture
def repo(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> Path:
    monkeypatch.setattr(check_skill, "REPO_ROOT", tmp_path, raising=True)
    return tmp_path


# Silently, this reads as enforcement while every cap goes unevaluated, which is
# how lore's declared 4000-character cap had no observer (lore#7753).
def test_missing_categories_yaml_says_it_is_not_enforcing(
    repo: Path, capsys: pytest.CaptureFixture[str]
) -> None:
    write_skill(repo, "lore-oversized", 9000)

    assert check_skill.main(["check-skills"]) == 0

    out = capsys.readouterr().out
    assert "not enforcing" in out
    assert ".agents/skills/categories.yaml" in out


# The contrast that makes a green run mean something: the same over-cap entry
# fails once the spec carries the number.
def test_categories_yaml_arms_the_size_cap(repo: Path) -> None:
    write_skill(repo, "lore-oversized", 9000)
    (repo / ".agents" / "skills" / "categories.yaml").write_text(
        CATEGORIES, encoding="utf-8"
    )

    assert check_skill.main(["check-skills"]) == 1


def test_entry_under_the_cap_passes(repo: Path) -> None:
    write_skill(repo, "lore-small", 500)
    (repo / ".agents" / "skills" / "categories.yaml").write_text(
        CATEGORIES, encoding="utf-8"
    )

    assert check_skill.main(["check-skills"]) == 0


def write_spec(repo: Path, body: str) -> None:
    (repo / ".agents" / "skills" / "categories.yaml").write_text(body, encoding="utf-8")


def test_archive_exemption_matches_a_repo_relative_component(repo: Path) -> None:
    write_skill(repo, "lore-frozen", 9000)
    write_spec(repo, ARCHIVE.format(component="lore-frozen"))

    assert check_skill.main(["check-skills"]) == 0


# The exemption reads an absolute path, so matching every component let any
# ancestor directory name switch off the caps for the whole repo, silently.
def test_archive_exemption_ignores_an_ancestor_directory_name(repo: Path) -> None:
    write_skill(repo, "lore-oversized", 9000)
    write_spec(repo, ARCHIVE.format(component=repo.parent.name))

    assert check_skill.main(["check-skills"]) == 1
