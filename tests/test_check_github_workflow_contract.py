"""Tests for the GitHub workflow contract hook."""

from __future__ import annotations

from pathlib import Path

import pytest

from agentic_os.pre_commit import check_github_workflow_contract as check
from agentic_os.workflow_contract import load_contract

CONTRACT = load_contract()
HOSTED = CONTRACT.hosted[0]
FORGEJO_LABEL = next(iter(CONTRACT.forgejo_to_github))
FORGEJO_HOST = CONTRACT.forbidden_hosts[0]


def write(path: Path, content: str) -> Path:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(content, encoding="utf-8")
    return path


def workflow(runs_on: str = f"runs-on: {HOSTED}", step: str = "run: just check") -> str:
    return (
        "name: t\non: push\njobs:\n  gate:\n"
        f"    {runs_on}\n    steps:\n      - {step}\n"
    )


def violations(tmp_path: Path, content: str) -> list[check.Violation]:
    path = write(tmp_path / ".github/workflows/t.yml", content)
    return check.check_file(path, tmp_path)


def test_hosted_label_passes(tmp_path: Path) -> None:
    assert violations(tmp_path, workflow()) == []


def test_forgejo_label_fails_and_names_the_replacement(tmp_path: Path) -> None:
    found = violations(tmp_path, workflow(f"runs-on: {FORGEJO_LABEL}"))
    assert len(found) == 1
    assert CONTRACT.forgejo_to_github[FORGEJO_LABEL] in found[0].reason
    assert found[0].line == 5


def test_self_hosted_fails(tmp_path: Path) -> None:
    found = violations(tmp_path, workflow("runs-on: [self-hosted, linux]"))
    assert [v.reason.split("`")[1] for v in found] == ["self-hosted", "linux"]


def test_runner_group_fails(tmp_path: Path) -> None:
    content = workflow("runs-on:\n      group: fleet\n      labels: " + HOSTED)
    found = violations(tmp_path, content)
    assert len(found) == 1
    assert "runner group" in found[0].reason


def test_label_expression_fails(tmp_path: Path) -> None:
    found = violations(tmp_path, workflow("runs-on: ${{ matrix.os }}"))
    assert len(found) == 1
    assert "expression" in found[0].reason


def test_forgejo_host_in_a_step_fails(tmp_path: Path) -> None:
    found = violations(tmp_path, workflow(step=f"run: curl https://{FORGEJO_HOST}/x"))
    assert len(found) == 1
    assert FORGEJO_HOST in found[0].reason


def test_forgejo_host_in_an_image_fails(tmp_path: Path) -> None:
    content = workflow().replace(
        "    steps:", f"    container: {FORGEJO_HOST}/o/img:release\n    steps:"
    )
    assert len(violations(tmp_path, content)) == 1


def test_forgejo_host_in_a_comment_passes(tmp_path: Path) -> None:
    content = f"# ported from {FORGEJO_HOST}\n" + workflow()
    assert violations(tmp_path, content) == []


def test_reusable_workflow_job_without_runs_on_passes(tmp_path: Path) -> None:
    content = "name: t\non: push\njobs:\n  call:\n    uses: ./.github/workflows/x.yml\n"
    assert violations(tmp_path, content) == []


def test_forgejo_tree_is_not_checked(tmp_path: Path) -> None:
    write(tmp_path / ".forgejo/workflows/t.yml", workflow(f"runs-on: {FORGEJO_LABEL}"))
    assert check.main([str(tmp_path / ".forgejo/workflows/t.yml")]) == 0


def test_main_exit_codes(tmp_path: Path, capsys: pytest.CaptureFixture[str]) -> None:
    good = write(tmp_path / ".github/workflows/good.yml", workflow())
    bad = write(
        tmp_path / ".github/workflows/bad.yml", workflow(f"runs-on: {FORGEJO_LABEL}")
    )
    assert check.main([str(good)]) == 0
    assert check.main([str(bad)]) == 1
    assert "bad.yml:5:14" in capsys.readouterr().err


def test_invalid_yaml_is_a_violation(tmp_path: Path) -> None:
    assert len(violations(tmp_path, "jobs: [unclosed\n")) == 1
