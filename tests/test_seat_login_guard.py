"""Tests for the seat login guard (COI-3682).

Every case feeds a command string to the hook and reads its verdict. Nothing here
runs `claude`, so no case can open a browser or reach the Keychain.
"""

from __future__ import annotations

import importlib.util
import json
import subprocess
import sys
from pathlib import Path

import pytest

SCRIPT = Path(__file__).parents[1] / "scripts" / "seat_login_guard.py"
SPEC = importlib.util.spec_from_file_location("seat_login_guard", SCRIPT)
assert SPEC and SPEC.loader
GUARD = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(GUARD)

SEATS_DIR = "/Users/seat/.claude-seats"


@pytest.fixture(autouse=True)
def seat_environment(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("HOME", "/Users/seat")
    monkeypatch.setenv("CLAUDE_CONFIG_DIR", SEATS_DIR)
    monkeypatch.delenv("XDG_CONFIG_HOME", raising=False)


REFUSED = [
    # The incident: a clean environment and a scratch HOME.
    "env -i HOME=/tmp/scratch claude",
    "env -i HOME=/tmp/scratch PATH=/usr/bin:/bin /Users/seat/.local/bin/claude",
    "env - HOME=/tmp/scratch claude",
    "env --ignore-environment HOME=/tmp/x claude",
    "env -i claude",
    # Under tmux, the way the seats ran it.
    "tmux new-session -d -s login 'env -i HOME=/tmp/scratch claude'",
    "tmux -L cap new-session -d -x 100 -y 40 \"env -i HOME=/tmp/s TERM=xterm claude --model sonnet\"",
    # Inline and exported variables.
    "HOME=/tmp/scratch claude",
    "CLAUDE_CONFIG_DIR=/tmp/scratch claude -p hi",
    "XDG_CONFIG_HOME=/tmp/scratch claude",
    "export HOME=/tmp/scratch; claude",
    "HOME=/tmp/scratch\nclaude",
    "cd /tmp/scratch && env HOME=$PWD claude",
    "env CLAUDE_CONFIG_DIR=/tmp/x claude",
    # Wrapped in another command or shell.
    "sh -c 'env -i HOME=/tmp/x claude'",
    "bash -lc \"HOME=/tmp/x claude\"",
    "eval 'env -i HOME=/tmp/x claude'",
    "env -S 'HOME=/tmp/x claude'",
    "nohup env -i HOME=/tmp/x claude &",
    "(env -i HOME=/tmp/x claude)",
    "echo ok; exec env -i HOME=/tmp/x claude",
    "env -i HOME=/tmp/x claude-code",
]

ALLOWED = [
    "claude",
    "claude -p 'say hi'",
    "claude --version",
    "env -i claude --version",
    "HOME=/tmp/scratch claude --help",
    # The seat's own values are not a fresh login.
    f"CLAUDE_CONFIG_DIR={SEATS_DIR} claude -p hi",
    "HOME=$HOME claude -p hi",
    "env HOME=/Users/seat claude -p hi",
    # `claude` as an argument or a name is not a launch.
    "echo claude",
    "grep -r claude docs",
    "git commit -m 'env -i HOME=/tmp/x claude'",
    "ls ~/.claude",
    "HOME=/tmp/scratch git status",
    "env -i HOME=/tmp/scratch git status",
    "tmux new-session -d -s build 'make build'",
    "bash -c 'echo hi'",
    "export HOME=/tmp/scratch; git status",
    "",
]


@pytest.mark.parametrize("command", REFUSED)
def test_a_launch_with_no_login_is_refused(command: str) -> None:
    assert GUARD.refusal(command), command


@pytest.mark.parametrize("command", ALLOWED)
def test_ordinary_commands_and_the_seats_own_launch_pass(command: str) -> None:
    assert GUARD.refusal(command) == "", command


def test_the_refusal_names_the_incident_and_the_safe_alternatives() -> None:
    reason = GUARD.refusal("env -i HOME=/tmp/s claude")

    assert "COI-3682" in reason
    assert "captured screens" in reason and "emulator lane" in reason
    assert "env -i" in reason


def test_an_unbalanced_quote_still_reads_the_words() -> None:
    assert GUARD.refusal("env -i HOME=/tmp/x claude 'unterminated")


def run_hook(payload: object, **kwargs) -> subprocess.CompletedProcess:
    return subprocess.run(
        [sys.executable, "-I", str(SCRIPT)],
        input=payload if isinstance(payload, str) else json.dumps(payload),
        capture_output=True,
        text=True,
        timeout=30,
        **kwargs,
    )


def bash(command: str) -> dict:
    return {"hook_event_name": "PreToolUse", "tool_name": "Bash", "tool_input": {"command": command}}


def test_the_hook_exits_2_with_the_reason_on_stderr_for_a_refused_call() -> None:
    result = run_hook(bash("env -i HOME=/tmp/s claude"))

    assert result.returncode == 2
    assert "COI-3682" in result.stderr
    assert result.stdout == ""


def test_the_hook_passes_an_allowed_call_silently() -> None:
    result = run_hook(bash("git status"))

    assert (result.returncode, result.stdout, result.stderr) == (0, "", "")


def test_the_hook_ignores_a_tool_that_is_not_bash() -> None:
    payload = {"tool_name": "Write", "tool_input": {"command": "env -i HOME=/x claude"}}

    assert run_hook(payload).returncode == 0


@pytest.mark.parametrize("payload", ["", "not json", "[]", '{"tool_name": "Bash", "tool_input": 3}'])
def test_the_hook_fails_open_on_input_it_cannot_read(payload: str) -> None:
    assert run_hook(payload).returncode == 0
