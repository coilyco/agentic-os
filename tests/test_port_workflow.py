"""Tests for the Forgejo to GitHub workflow translation."""

from __future__ import annotations

from pathlib import Path

import pytest

from agentic_os import port_workflow as port
from agentic_os.pre_commit import check_github_workflow_contract as check
from agentic_os.workflow_contract import load_contract

CONTRACT = load_contract()
DOCKER = "docker"
REDESIGN = next(iter(CONTRACT.needs_redesign))
HOST = CONTRACT.forbidden_hosts[0]

SOURCE = f"""# Gate, kept comment.
name: ci

on:
  pull_request:

jobs:
  gate:
    runs-on: {DOCKER}
    container:
      image: {HOST}/o/agentic-os:release
    steps:
      - uses: actions/checkout@v6

      # Why this step exists.
      - name: Check
        run: just check
"""


def forgejo_file(tmp_path: Path, text: str = SOURCE) -> Path:
    path = tmp_path / ".forgejo/workflows/ci.yml"
    path.parent.mkdir(parents=True)
    path.write_text(text, encoding="utf-8")
    return path


def test_label_is_mapped_container_dropped_and_setup_ci_added(tmp_path: Path) -> None:
    text, todos = port.port_text(SOURCE, tmp_path, CONTRACT)
    assert f"runs-on: {CONTRACT.forgejo_to_github[DOCKER]}" in text
    assert "container" not in text
    lines = [line.strip() for line in text.splitlines()]
    assert lines[lines.index("steps:") + 1] == f"- uses: {CONTRACT.setup_ci}"
    assert todos == []


def test_comments_and_blank_lines_survive(tmp_path: Path) -> None:
    text, _ = port.port_text(SOURCE, tmp_path, CONTRACT)
    assert "# Gate, kept comment." in text
    assert "# Why this step exists." in text


def test_output_passes_the_checker(tmp_path: Path) -> None:
    source = forgejo_file(tmp_path)
    assert port.main([str(source)]) == 0
    target = tmp_path / ".github/workflows/ci.yml"
    assert check.check_file(target, tmp_path) == []


def test_job_with_no_container_gets_no_setup_ci(tmp_path: Path) -> None:
    source = SOURCE.replace(f"    container:\n      image: {HOST}/o/agentic-os:release\n", "")
    text, _ = port.port_text(source, tmp_path, CONTRACT)
    assert CONTRACT.setup_ci not in text


def test_redesign_label_is_a_todo(tmp_path: Path) -> None:
    text, todos = port.port_text(SOURCE.replace(DOCKER, REDESIGN, 1), tmp_path, CONTRACT)
    assert any(REDESIGN in todo for todo in todos)
    assert f"runs-on: {CONTRACT.forgejo_to_github[REDESIGN]}" in text


def test_unknown_label_is_left_and_reported(tmp_path: Path) -> None:
    _, todos = port.port_text(SOURCE.replace(DOCKER, "mystery", 1), tmp_path, CONTRACT)
    assert any("mystery" in todo for todo in todos)


def test_remaining_forgejo_host_is_a_todo(tmp_path: Path) -> None:
    source = SOURCE + f"      - run: curl https://{HOST}/api\n"
    _, todos = port.port_text(source, tmp_path, CONTRACT)
    assert any(HOST in todo for todo in todos)


def test_repo_signals_are_todos(tmp_path: Path) -> None:
    (tmp_path / "go.mod").write_text("module x\n", encoding="utf-8")
    (tmp_path / ".pre-commit-config.yaml").write_text(
        f"repos:\n  - repo: https://{HOST}/o/agentic-os\n", encoding="utf-8"
    )
    _, todos = port.port_text(SOURCE, tmp_path, CONTRACT)
    assert any("setup-go" in todo for todo in todos)
    assert any("pre-commit" in todo for todo in todos)


def test_existing_output_is_not_overwritten(
    tmp_path: Path, capsys: pytest.CaptureFixture[str]
) -> None:
    source = forgejo_file(tmp_path)
    target = tmp_path / ".github/workflows/ci.yml"
    target.parent.mkdir(parents=True)
    target.write_text("keep\n", encoding="utf-8")
    assert port.main([str(source)]) == 1
    assert target.read_text(encoding="utf-8") == "keep\n"
    assert port.main([str(source), "--force"]) == 0
    assert "runs-on" in target.read_text(encoding="utf-8")
