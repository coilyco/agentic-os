"""Tests for the GitHub pull-request PreToolUse guard."""

from __future__ import annotations

import json
import os
import subprocess
from pathlib import Path

import pytest


SCRIPT = Path(__file__).parents[1] / "scripts" / "github-pr-guard.sh"


def run_guard(payload: dict, env: dict | None = None) -> dict | None:
    """Run the hook on one payload, returning its decision or None for a pass."""
    result = subprocess.run(
        ["bash", str(SCRIPT)],
        input=json.dumps(payload),
        capture_output=True,
        text=True,
        env=env,
    )
    assert result.returncode == 0, result.stderr
    if not result.stdout.strip():
        return None
    return json.loads(result.stdout)


def bash(command: str) -> dict:
    return {"tool_name": "Bash", "tool_input": {"command": command}}


def reason_of(decision: dict | None) -> str:
    assert decision is not None, "expected a refusal, got a pass"
    specific = decision["hookSpecificOutput"]
    assert specific["hookEventName"] == "PreToolUse"
    assert specific["permissionDecision"] == "deny"
    return specific["permissionDecisionReason"]


REFUSED_COMMANDS = [
    "gh pr create",
    "gh pr create --title x --body y",
    "cd /tmp && gh pr create --fill",
    "export FOO=1; gh pr create --web",
    "gh pr merge 12 --squash",
    "gh pr review 12 --approve",
    "gh pr comment 12 --body hi",
    "gh pr edit 12 --title x",
    "gh pr close 12",
    "gh pr reopen 12",
    "gh pr ready 12",
    "gh api --method POST repos/coilysiren/x/pulls -f title=x",
    "gh api -X POST /repos/coilysiren/x/pulls",
    "gh api graphql -f query='mutation { createPullRequest }'",
]

ALLOWED_COMMANDS = [
    "gh pr view 12",
    "gh pr list --state open",
    "gh pr diff 12",
    "gh pr checks 12",
    "gh repo list coilysiren --limit 300",
    "gh api /repos/coilysiren/x/pulls",
    "gh api repos/coilysiren/x/readme --cache 1h",
    "gh issue create --title x",
    "gh auth status",
    "git push origin HEAD",
    # The Forgejo surface this guard steers work onto is never the thing refused.
    "aosguard ops forgejo pr create --repo coilysiren/x --head b --base main",
]


@pytest.mark.parametrize("command", REFUSED_COMMANDS)
def test_github_pr_writes_are_refused(command: str) -> None:
    assert "Forgejo" in reason_of(run_guard(bash(command)))


@pytest.mark.parametrize("command", ALLOWED_COMMANDS)
def test_reads_and_forgejo_work_pass_through(command: str) -> None:
    assert run_guard(bash(command)) is None


def test_the_refusal_names_the_guarded_verb_that_replaces_it() -> None:
    reason = reason_of(run_guard(bash("gh pr create")))
    assert "aosguard ops forgejo pr create" in reason
    assert "create_pull-request" in reason


def test_mcp_pull_request_creation_is_refused_on_any_github_server() -> None:
    payload = {
        "tool_name": "mcp__claude_ai_GitHub__create_pull_request",
        "tool_input": {"owner": "coilysiren", "repo": "x", "head": "b", "base": "main"},
    }
    assert "Forgejo" in reason_of(run_guard(payload))


def test_mcp_pull_request_merge_and_review_are_refused() -> None:
    for leaf in ("merge_pull_request", "pull_request_review_write"):
        payload = {"tool_name": f"mcp__claude_ai_GitHub__{leaf}", "tool_input": {}}
        assert "Forgejo" in reason_of(run_guard(payload))


def test_a_github_read_tool_is_left_alone() -> None:
    payload = {"tool_name": "mcp__claude_ai_GitHub__pull_request_read", "tool_input": {}}
    assert run_guard(payload) is None


def test_the_forgejo_mcp_is_not_this_guards_business() -> None:
    payload = {
        "tool_name": "mcp__tailnet_coilyco_forgejo__create_pull-request",
        "tool_input": {"repo": "coilyco-flight-deck/agentic-os"},
    }
    assert run_guard(payload) is None


def test_the_off_switch_stands_the_guard_down() -> None:
    env = os.environ.copy()
    env["AOS_GITHUB_PR_GUARD_MODE"] = "off"
    assert run_guard(bash("gh pr create"), env=env) is None
