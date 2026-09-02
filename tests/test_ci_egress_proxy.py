"""Tests for the CI egress proxy path and the retired pre-commit cache.

Runner egress has no direct route out, so a cold hook install reached
github.com unproxied and reset. The `actions/cache` block kept the failure
intermittent rather than visible, which is how it stayed unnoticed. See
agentic-os#1031 and deploy#402.
"""

from __future__ import annotations

import os
import subprocess
from pathlib import Path

import pytest

ROOT = Path(__file__).resolve().parent.parent
WRAPPER = ROOT / "scripts" / "ci-command.sh"
WORKFLOWS = sorted((ROOT / ".forgejo" / "workflows").glob("*.yml"))
GATE = "scripts/ci/repo-test-gate.sh"


def _run(*args: str, env: dict | None = None) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        ["bash", str(WRAPPER), *args],
        check=False,
        capture_output=True,
        text=True,
        env=os.environ | (env or {}),
    )


def test_the_proxy_is_exported_to_the_wrapped_command() -> None:
    got = _run("printenv", "HTTPS_PROXY", env={"FORGEJO_EGRESS_PROXY": "http://p:3128"})

    assert got.returncode == 0, got.stderr
    assert got.stdout.strip() == "http://p:3128"


def test_forgejo_itself_stays_off_the_proxy() -> None:
    # Proxying the forge deadlocks the checkout against its own ingress.
    got = _run("printenv", "NO_PROXY", env={"FORGEJO_EGRESS_PROXY": "http://p:3128"})

    assert "forgejo.coilysiren.me" in got.stdout


def test_an_unset_proxy_is_a_no_op_not_a_failure() -> None:
    # Local runs and any runner without the var must behave exactly as before.
    got = _run("printenv", "HTTPS_PROXY", env={"FORGEJO_EGRESS_PROXY": ""})

    assert got.stdout.strip() == ""


def test_the_wrapper_execs_rather_than_swallowing_status() -> None:
    assert _run("false").returncode == 1
    assert _run("true").returncode == 0


def test_no_command_is_a_usage_error() -> None:
    assert _run().returncode == 2


@pytest.mark.parametrize("workflow", WORKFLOWS, ids=lambda p: p.name)
def test_no_workflow_caches_the_retired_pre_commit_home(workflow: Path) -> None:
    # agentic-os:release bakes PRE_COMMIT_HOME=/opt/pre-commit, so these blocks
    # saved and restored an empty directory while reading as protection.
    assert "~/.cache/pre-commit" not in workflow.read_text(encoding="utf-8")


def test_every_gate_invocation_crosses_the_proxy_wrapper() -> None:
    callers = [
        path
        # The justfile is here because `just repo-test-gate` is the spelling
        # an agent types, and it was the one unwrapped caller (#1212).
        for path in [
            *WORKFLOWS,
            *sorted((ROOT / "scripts" / "ci").glob("*.sh")),
            ROOT / "justfile",
        ]
        if GATE in path.read_text(encoding="utf-8") and path.name != "repo-test-gate.sh"
    ]

    assert callers, "no caller of the gate found, so this test proves nothing"
    for path in callers:
        for line in path.read_text(encoding="utf-8").splitlines():
            if GATE in line:
                assert "ci-command.sh" in line, f"{path.name}: {line.strip()}"
