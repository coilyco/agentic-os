"""Tests for the dev-base agent-name.sh `gitidentity` mode (agentic-os#244).

The status line and session banner still carry the agent self-name, but the git
committer identity now resolves to the coilyco-ops bot account so Forgejo links
the commit to the deployment identity instead of an example fallback. These
drive the baked container script via subprocess.
"""
from __future__ import annotations

import subprocess
from pathlib import Path

SCRIPT = Path(__file__).resolve().parent.parent / "docker" / "dev-base" / "agent-name.sh"
PAYLOAD = '{"session_id":"AbC123xyz"}'


def _run(
    mode: str,
    home: Path,
    container: str | None = None,
    extra_env: dict[str, str] | None = None,
) -> str:
    # A fresh HOME isolates --global writes/reads to this test's gitconfig, and
    # TMPDIR isolates the per-session name cache so runs never cross-talk.
    env = {
        "HOME": str(home),
        "TMPDIR": str(home),
        "PATH": "/usr/bin:/bin",
        "GIT_CONFIG_GLOBAL": str(home / ".gitconfig"),
        "GIT_CONFIG_SYSTEM": "/dev/null",
    }
    # WARD_CONTAINER_NAME drives the container-name suffix; leave it unset to
    # exercise the native-host / self-suppress path (agentic-os#296).
    if container is not None:
        env["WARD_CONTAINER_NAME"] = container
    if extra_env is not None:
        env.update(extra_env)
    proc = subprocess.run(
        [str(SCRIPT), mode],
        input=PAYLOAD,
        capture_output=True,
        text=True,
        check=True,
        env=env,
    )
    return proc.stdout.strip()


def _git_global(key: str, home: Path) -> str:
    proc = subprocess.run(
        ["git", "config", "--global", "--get", key],
        capture_output=True,
        text=True,
        env={
            "HOME": str(home),
            "PATH": "/usr/bin:/bin",
            "GIT_CONFIG_GLOBAL": str(home / ".gitconfig"),
            "GIT_CONFIG_SYSTEM": "/dev/null",
        },
    )
    return proc.stdout.strip()  # empty string when unset (exit 1 ignored)


def _git_effective(key: str, home: Path, system_config: Path | None = None) -> str:
    env = {
        "HOME": str(home),
        "PATH": "/usr/bin:/bin",
        "GIT_CONFIG_GLOBAL": str(home / ".gitconfig"),
        "GIT_CONFIG_SYSTEM": "/dev/null",
    }
    if system_config is not None:
        env["GIT_CONFIG_SYSTEM"] = str(system_config)
    proc = subprocess.run(
        ["git", "config", "--get", key],
        capture_output=True,
        text=True,
        env=env,
    )
    return proc.stdout.strip()


def test_gitidentity_sets_bot_committer_identity_when_unconfigured(tmp_path: Path) -> None:
    _run("gitidentity", tmp_path)
    assert _git_global("user.name", tmp_path) == "coilyco-ops"
    assert _git_global("user.email", tmp_path) == "coilyco-ops@coilysiren.me"


def test_gitidentity_honors_baked_git_identity(tmp_path: Path) -> None:
    system_config = tmp_path / "gitconfig-system"
    system_config.write_text(
        "[user]\n"
        "\tname = baked-bot\n"
        "\temail = baked-bot@example.com\n",
        encoding="utf-8",
    )
    _run("gitidentity", tmp_path, extra_env={"GIT_CONFIG_SYSTEM": str(system_config)})
    assert _git_global("user.name", tmp_path) == ""
    assert _git_global("user.email", tmp_path) == ""
    assert _git_effective("user.name", tmp_path, system_config) == "baked-bot"
    assert _git_effective("user.email", tmp_path, system_config) == "baked-bot@example.com"


def test_gitidentity_honors_committer_overrides(tmp_path: Path) -> None:
    _run(
        "gitidentity",
        tmp_path,
        extra_env={
            "WARD_GIT_NAME": "custom-bot",
            "WARD_GIT_EMAIL": "custom-bot@example.com",
        },
    )
    assert _git_global("user.name", tmp_path) == "custom-bot"
    assert _git_global("user.email", tmp_path) == "custom-bot@example.com"


def test_gitidentity_is_idempotent(tmp_path: Path) -> None:
    _run("gitidentity", tmp_path)
    first = _git_global("user.name", tmp_path)
    _run("gitidentity", tmp_path)
    assert _git_global("user.name", tmp_path) == first


def test_statusline_appends_container_name(tmp_path: Path) -> None:
    # In a warded container ward exports WARD_CONTAINER_NAME; the status line
    # shows it in brackets next to the agent name (agentic-os#296).
    out = _run("statusline", tmp_path, container="engineer-claude-ward-338")
    assert out.startswith("claude-")
    assert out.endswith("[engineer-claude-ward-338]")


def test_statusline_suppresses_container_name_when_unset(tmp_path: Path) -> None:
    # Native host / before ward ships the env var: no brackets, no error.
    out = _run("statusline", tmp_path)
    assert out.startswith("claude-")
    assert "[" not in out and "]" not in out
