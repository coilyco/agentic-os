from __future__ import annotations

import pathlib

import yaml


ROOT = pathlib.Path(__file__).resolve().parents[1]
WORKFLOWS = ROOT / ".github" / "workflows"


def test_promote_gates_through_ci_and_promotes_only_after_it() -> None:
    # promote reuses ci.yml as its gate, so it can never run a narrower suite
    # than a pull request does and silently promote a red main.
    promote = yaml.safe_load((WORKFLOWS / "promote.yml").read_text())
    jobs = promote["jobs"]

    assert jobs["gate"]["uses"] == "./.github/workflows/ci.yml"
    assert jobs["promote-release"]["needs"] == ["gate"]


def test_ci_workflow_runs_the_repo_gate_script() -> None:
    ci = (WORKFLOWS / "ci.yml").read_text()
    gate = (ROOT / "scripts" / "ci" / "repo-test-gate.sh").read_text()

    assert "workflow_call:" in ci, "promote calls ci.yml, so it must be callable"
    assert "scripts/ci/repo-test-gate.sh" in ci
    assert "uv run pytest" in gate
    assert "pre-commit run --all-files" in gate
