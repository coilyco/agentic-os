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
            "AOS_GIT_NAME": "deployment-bot",
            "AOS_GIT_EMAIL": "deployment-bot@example.com",
        }
    )
    entrypoint = REPO_ROOT / "docker" / "dev-base" / "shell-entrypoint.sh"
    proc = subprocess.run(
        [
            "bash",
            str(entrypoint),
            "bash",
            "-lc",
            'printf "%s" "$AOS_REPO_ROOT"',
        ],
        cwd=foreign,
        check=True,
        capture_output=True,
        text=True,
        env=env,
    )

    assert proc.stdout == str(REPO_ROOT)


def test_container_entrypoint_refuses_a_container_without_the_image_identity(tmp_path: Path) -> None:
    env = {"HOME": str(tmp_path), "PATH": "/usr/bin:/bin"}
    entrypoint = REPO_ROOT / "docker" / "dev-base" / "shell-entrypoint.sh"
    proc = subprocess.run(
        ["bash", str(entrypoint), "true"], capture_output=True, text=True, env=env
    )

    assert proc.returncode != 0
    assert "AOS_GIT_NAME is required" in proc.stderr
