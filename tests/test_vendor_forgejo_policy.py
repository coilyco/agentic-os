"""Tests for the deploy vendoring push. See docs/vendor-forgejo-policy.md.

The push carries a credential and writes another repository, so the properties
worth holding are the ones whose loss is silent: that an absent token skips
rather than fails, and that the job stops at a pull request.
"""
from __future__ import annotations

from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent
SCRIPT = REPO_ROOT / "scripts" / "ci" / "vendor-forgejo-policy.sh"
WORKFLOW = REPO_ROOT / ".forgejo" / "workflows" / "vendor-forgejo-policy.yml"
POLICY = REPO_ROOT / ".umbra" / "guardfiles" / "aosguard" / "forgejo.kdl"
SPEC = REPO_ROOT / ".umbra" / "guardfiles" / "aosguard" / "forgejo.swagger.v1.json.gz"


def test_an_absent_token_skips_rather_than_fails() -> None:
    # aos-cli-release.sh uses the same guard for its tap and scoop pushes. A
    # vendoring job that failed every release would get disabled instead.
    text = SCRIPT.read_text(encoding="utf-8")

    assert 'if [ -z "${DEPLOY_WRITE_TOKEN:-}" ]; then' in text
    guard = text.split('if [ -z "${DEPLOY_WRITE_TOKEN:-}" ]; then', 1)[1].split("fi", 1)[0]
    assert "exit 0" in guard, guard


def test_the_push_stops_at_a_pull_request() -> None:
    text = SCRIPT.read_text(encoding="utf-8")

    assert '"HEAD:${branch}"' in text, "the push must target a branch, never main"
    assert "/pulls" in text, "a pushed branch owes its pull request"
    assert "/merge" not in text, "nothing here may land deploy's main"


def test_the_vendored_pair_names_its_source_commit() -> None:
    text = SCRIPT.read_text(encoding="utf-8")

    assert "forgejo.kdl" in text
    assert "forgejo.swagger.v1.json.gz" in text
    assert "$target_dir/SOURCE" in text
    assert "commit: %s" in text


def test_the_workflow_watches_exactly_the_files_it_ships() -> None:
    text = WORKFLOW.read_text(encoding="utf-8")

    for path in (
        ".umbra/guardfiles/aosguard/forgejo.kdl",
        ".umbra/guardfiles/aosguard/forgejo.swagger.v1.json.gz",
    ):
        assert path in text, f"{path} ships but does not trigger the push"
    assert "ci-command.sh" in text, "the push crosses the runner's egress proxy"


def test_both_vendored_files_exist_to_be_copied() -> None:
    assert POLICY.is_file()
    assert SPEC.is_file()
