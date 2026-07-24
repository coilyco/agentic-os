"""Tests for the Actions one-line run policy hook."""

from __future__ import annotations

from pathlib import Path

import pytest

from agentic_os.pre_commit import check_actions_run_one_line as check


def write(path: Path, content: str) -> Path:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(content, encoding="utf-8")
    return path


def workflow(run: str) -> str:
    return (
        "name: test\n"
        "on: push\n"
        "jobs:\n"
        "  test:\n"
        "    runs-on: ubuntu-latest\n"
        "    steps:\n"
        f"{run}"
    )


def test_plain_one_line_run_passes(tmp_path: Path) -> None:
    path = write(
        tmp_path / ".forgejo/workflows/test.yml",
        workflow("      - run: ward exec test\n"),
    )
    assert check.check_file(path, tmp_path) == []


def test_composite_action_one_line_run_passes(tmp_path: Path) -> None:
    path = write(
        tmp_path / "actions/example/action.yml",
        "name: example\n"
        "runs:\n"
        "  using: composite\n"
        "  steps:\n"
        '    - run: bash "$GITHUB_ACTION_PATH/run.sh"\n'
        "      shell: bash\n",
    )
    assert check.check_file(path, tmp_path) == []


@pytest.mark.parametrize("indicator", ["|", "|-", "|+", ">", ">-", ">+"])
def test_block_scalar_run_fails(tmp_path: Path, indicator: str) -> None:
    path = write(
        tmp_path / ".github/workflows/test.yaml",
        workflow(
            f"      - run: {indicator}\n"
            "          echo first\n"
            "          echo second\n"
        ),
    )
    violations = check.check_file(path, tmp_path)
    assert len(violations) == 1
    assert "cannot use a YAML block scalar" in violations[0].reason


def test_quoted_scalar_spanning_source_lines_fails(tmp_path: Path) -> None:
    path = write(
        tmp_path / ".forgejo/workflows/test.yml",
        workflow(
            '      - run: "echo first\n'
            '          echo second"\n'
        ),
    )
    violations = check.check_file(path, tmp_path)
    assert len(violations) == 1
    assert "cannot span physical YAML lines" in violations[0].reason


def test_escaped_newline_fails(tmp_path: Path) -> None:
    path = write(
        tmp_path / ".github/workflows/test.yml",
        workflow('      - run: "echo first\\necho second"\n'),
    )
    violations = check.check_file(path, tmp_path)
    assert len(violations) == 1
    assert "cannot decode to multiple lines" in violations[0].reason


def test_nested_input_named_run_is_not_a_step_command(tmp_path: Path) -> None:
    path = write(
        tmp_path / "action.yaml",
        "name: example\n"
        "runs:\n"
        "  using: composite\n"
        "  steps:\n"
        "    - uses: example/action@v1\n"
        "      with:\n"
        "        run: |\n"
        "          this value belongs to the called action\n",
    )
    assert check.check_file(path, tmp_path) == []


def test_invalid_yaml_reports_source_location(tmp_path: Path) -> None:
    path = write(
        tmp_path / ".forgejo/workflows/test.yml",
        "jobs:\n  test:\n    steps:\n      - run: [\n",
    )
    violations = check.check_file(path, tmp_path)
    assert len(violations) == 1
    assert violations[0].line > 0
    assert "invalid Actions YAML" in violations[0].reason


@pytest.mark.parametrize(
    ("relative_path", "expected"),
    [
        (".github/workflows/test.yml", True),
        (".forgejo/workflows/nested/test.yaml", True),
        ("actions/example/action.yml", True),
        ("action.yaml", True),
        (".ward/ward.yaml", False),
        ("fixtures/workflow.yml", False),
    ],
)
def test_actions_file_selection(relative_path: str, expected: bool) -> None:
    assert check.is_actions_yaml(Path(relative_path)) is expected
