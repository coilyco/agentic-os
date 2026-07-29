"""Smoke coverage for the AOS checkout and identity propagation path."""
from __future__ import annotations

import os
import subprocess
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parent.parent


def _foreign_repo(tmp_path: Path) -> Path:
    repo = tmp_path / "foreign"
    repo.mkdir()
    subprocess.run(["git", "init"], cwd=repo, check=True, capture_output=True, text=True)
    env = os.environ.copy()
    env.update(
        {
            "GIT_AUTHOR_NAME": "Test User",
            "GIT_AUTHOR_EMAIL": "test@example.com",
            "GIT_COMMITTER_NAME": "Test User",
            "GIT_COMMITTER_EMAIL": "test@example.com",
        }
    )
    subprocess.run(
        ["git", "commit", "--allow-empty", "-m", "foreign"],
        cwd=repo,
        check=True,
        capture_output=True,
        text=True,
        env=env,
    )
    return repo


def test_common_shell_does_not_set_retired_ward_config_reference(tmp_path: Path) -> None:
    foreign = _foreign_repo(tmp_path)
    env = os.environ.copy()
    env.update(
        {
            "HOME": str(tmp_path / "home"),
            "PATH": "/usr/bin:/bin",
            "AOS_REPO_ROOT": "",
            "FORGEJO_WORKSPACE": str(REPO_ROOT),
        }
    )
    proc = subprocess.run(
        [
            "bash",
            "-lc",
            f'source "{REPO_ROOT / "shell" / "common.sh"}"; printf "%s" "${{WARD_CONFIG_REF-}}"',
        ],
        cwd=foreign,
        check=True,
        capture_output=True,
        text=True,
        env=env,
    )

    assert proc.stdout == ""


def test_container_entrypoint_seeds_the_read_only_surface_env(tmp_path: Path) -> None:
    foreign = _foreign_repo(tmp_path)
    env = os.environ.copy()
    env.update(
        {
            "HOME": str(tmp_path / "home"),
            "PATH": "/usr/bin:/bin",
            "AOS_REPO_ROOT": "",
            "FORGEJO_WORKSPACE": str(REPO_ROOT),
            "WARD_READONLY": "1",
            "AOS_GIT_NAME": "deployment-bot",
            "AOS_GIT_EMAIL": "deployment-bot@example.com",
            "WARD_GIT_NAME": "neutral-default",
            "WARD_GIT_EMAIL": "neutral-default@example.com",
        }
    )
    entrypoint = REPO_ROOT / "docker" / "dev-base" / "ward-shell-entrypoint.sh"
    proc = subprocess.run(
        [
            "bash",
            str(entrypoint),
            "bash",
            "-lc",
            'printf "%s\n%s\n%s" "$AOS_REPO_ROOT" "$WARD_GIT_NAME" "$WARD_GIT_EMAIL"',
        ],
        cwd=foreign,
        check=True,
        capture_output=True,
        text=True,
        env=env,
    )

    root, git_name, git_email = proc.stdout.splitlines()
    assert root == str(REPO_ROOT)
    assert git_name == env["AOS_GIT_NAME"]
    assert git_email == env["AOS_GIT_EMAIL"]
