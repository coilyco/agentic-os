from __future__ import annotations

import pathlib


ROOT = pathlib.Path(__file__).resolve().parents[1]


def test_pull_request_ci_workflow_exposes_branch_protection_context() -> None:
    workflow = (ROOT / ".forgejo" / "workflows" / "ci.yml").read_text()
    assert "name: ci" in workflow
    assert "pull_request:" in workflow
    assert "ward exec test" in workflow
    assert "ward exec pre-commit-all" in workflow


def test_pull_request_ci_docs_name_the_required_context() -> None:
    docs = (ROOT / "docs" / "ci-in-dev-base.md").read_text()
    assert "ci / gate" in docs
    assert "pull-requests-and-merge" in docs
