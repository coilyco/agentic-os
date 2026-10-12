"""Tests for the dev-base git-identity hook (agentic-os#244).

Split out of the retired agent-name.sh, which carried this beside naming for no
reason beyond both running at SessionStart.
"""
from __future__ import annotations

import subprocess
from pathlib import Path

SCRIPT = Path(__file__).resolve().parent.parent / "docker" / "dev-base" / "git-identity.sh"
DEPLOYMENT_NAME = "deployment-bot"
DEPLOYMENT_EMAIL = "deployment-bot@example.com"


def _run(home: Path, extra_env: dict[str, str] | None = None) -> None:
    # A fresh HOME isolates --global writes/reads to this test's gitconfig.
    env = {
        "HOME": str(home),
        "TMPDIR": str(home),
        "PATH": "/usr/bin:/bin",
        "GIT_CONFIG_GLOBAL": str(home / ".gitconfig"),
        "GIT_CONFIG_SYSTEM": "/dev/null",
        "AOS_GIT_NAME": DEPLOYMENT_NAME,
        "AOS_GIT_EMAIL": DEPLOYMENT_EMAIL,
    }
    if extra_env is not None:
        env.update(extra_env)
    # cwd=home keeps this repo's .git/config out of every git resolution below.
    # Any repo-local identity outranks the isolated global one these tests stage.
    subprocess.run(
        [str(SCRIPT)],
        capture_output=True,
        text=True,
        check=True,
        env=env,
        cwd=home,
    )


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
        cwd=home,
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
        cwd=home,
    )
    return proc.stdout.strip()


def test_sets_bot_committer_identity_when_unconfigured(tmp_path: Path) -> None:
    _run(tmp_path)
    assert _git_global("user.name", tmp_path) == DEPLOYMENT_NAME
    assert _git_global("user.email", tmp_path) == DEPLOYMENT_EMAIL


def test_honors_baked_git_identity(tmp_path: Path) -> None:
    system_config = tmp_path / "gitconfig-system"
    system_config.write_text(
        "[user]\n"
        "\tname = baked-bot\n"
        "\temail = baked-bot@example.com\n",
        encoding="utf-8",
    )
    _run(tmp_path, extra_env={"GIT_CONFIG_SYSTEM": str(system_config)})
    assert _git_global("user.name", tmp_path) == ""
    assert _git_global("user.email", tmp_path) == ""
    assert _git_effective("user.name", tmp_path, system_config) == "baked-bot"
    assert _git_effective("user.email", tmp_path, system_config) == "baked-bot@example.com"


def test_honors_committer_overrides(tmp_path: Path) -> None:
    _run(
        tmp_path,
        extra_env={
            "AOS_GIT_NAME": "custom-bot",
            "AOS_GIT_EMAIL": "custom-bot@example.com",
        },
    )
    assert _git_global("user.name", tmp_path) == "custom-bot"
    assert _git_global("user.email", tmp_path) == "custom-bot@example.com"


def test_is_idempotent(tmp_path: Path) -> None:
    _run(tmp_path)
    first = _git_global("user.name", tmp_path)
    _run(tmp_path)
    assert _git_global("user.name", tmp_path) == first
