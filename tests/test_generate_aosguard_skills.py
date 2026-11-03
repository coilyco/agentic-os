"""Tests for per-area aosguard skill generation.

One `aosguard` skill only loads once an agent already suspects it needs
aosguard, which is the retrieval failure agentic-os#1028 records: an agent
concluded `release uninstall` was denied because the MCP lacked it, while
`aosguard ops helm release uninstall` existed. Per-area skills match the entity
the agent is demonstrably working with.
"""

from __future__ import annotations

from pathlib import Path

import pytest
import yaml

from agentic_os.generators import generate_aosguard_skills as gen

ROOT = Path(__file__).resolve().parent.parent
CONCEPT = ROOT / ".agents" / "skills" / "tooling-aosguard" / "SKILL.md"


def _index(root: Path, paths: list[list[str]]) -> Path:
    target = root / "aosguard" / "references"
    target.mkdir(parents=True, exist_ok=True)
    commands = [{"path": p, "summary": " ".join(p)} for p in paths]
    (target / "commands.yaml").write_text(
        yaml.safe_dump({"commands": commands}), encoding="utf-8"
    )
    return root


def test_each_wrapped_area_gets_its_own_skill(tmp_path: Path) -> None:
    _index(tmp_path, [
        ["aosguard", "ops", "helm", "release", "uninstall"],
        ["aosguard", "ops", "helm", "release", "list"],
        ["aosguard", "ops", "kubectl", "get"],
    ])

    written, _ = gen.generate(tmp_path)

    assert written == ["aosguard-helm", "aosguard-kubectl"]
    assert (tmp_path / "aosguard-helm" / "SKILL.md").is_file()
    assert (tmp_path / "aosguard-helm" / "references" / "commands.yaml").is_file()


def test_a_new_area_needs_no_hand_edit(tmp_path: Path) -> None:
    # The acceptance criterion: adding a wrapped area produces its skill.
    _index(tmp_path, [["aosguard", "ops", "brand-new", "verb"]])

    written, _ = gen.generate(tmp_path)

    assert written == ["aosguard-brand-new"]


def test_an_area_skill_carries_only_its_own_verbs(tmp_path: Path) -> None:
    _index(tmp_path, [
        ["aosguard", "ops", "helm", "release", "uninstall"],
        ["aosguard", "ops", "kubectl", "get"],
    ])
    gen.generate(tmp_path)

    index = yaml.safe_load(
        (tmp_path / "aosguard-helm" / "references" / "commands.yaml").read_text()
    )

    assert [c["path"] for c in index["commands"]] == [
        ["aosguard", "ops", "helm", "release", "uninstall"]
    ]


def test_a_retired_area_loses_its_skill(tmp_path: Path) -> None:
    # A skill for a verb the policy no longer wraps is worse than none.
    _index(tmp_path, [["aosguard", "ops", "gone", "verb"]])
    gen.generate(tmp_path)
    _index(tmp_path, [["aosguard", "ops", "kept", "verb"]])

    _, removed = gen.generate(tmp_path)

    assert removed == ["aosguard-gone"]
    assert not (tmp_path / "aosguard-gone").exists()


def test_a_missing_index_is_an_error_not_zero_skills(tmp_path: Path) -> None:
    # Writing no skills and reporting success is the failure this refuses.
    with pytest.raises(SystemExit):
        gen.generate(tmp_path)


def test_an_index_with_no_area_leaves_is_an_error(tmp_path: Path) -> None:
    _index(tmp_path, [["aosguard", "--version"]])

    with pytest.raises(SystemExit):
        gen.generate(tmp_path)


def test_the_concept_skill_carries_what_no_spec_can() -> None:
    # The two hand-written facts the issue names, neither derivable from a spec.
    body = CONCEPT.read_text(encoding="utf-8")

    assert "absent verb is not a denial" in body.lower()
    assert "aosguard is not ward" in body.lower()


def test_both_build_paths_generate_the_area_skills() -> None:
    # The image must route the same way a local build does, or a container
    # agent gets the retrieval failure this closes.
    module = "agentic_os.generators.generate_aosguard_skills"

    assert module in (ROOT / "justfile").read_text(encoding="utf-8")
    dockerfile = (ROOT / "docker" / "dev-base" / "full" / "Dockerfile").read_text()
    assert module in dockerfile
    assert "aosguard-python" in dockerfile
