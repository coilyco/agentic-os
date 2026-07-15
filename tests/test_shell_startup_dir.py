"""Smoke coverage for the shared shell startup directory."""
from __future__ import annotations

import os
import subprocess
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parent.parent


def test_interactive_shell_lands_in_projects_from_home(tmp_path: Path) -> None:
    home = tmp_path / "home"
    projects = home / "projects"
    projects.mkdir(parents=True)

    env = os.environ.copy()
    env.update(
        {
            "HOME": str(home),
            "PATH": "/usr/bin:/bin",
            "AOS_REPO_ROOT": str(REPO_ROOT),
        }
    )
    proc = subprocess.run(
        [
            "bash",
            "--noprofile",
            "--rcfile",
            str(REPO_ROOT / "shell" / "common.sh"),
            "-ic",
            'printf "%s" "$PWD"',
        ],
        cwd=home,
        check=True,
        capture_output=True,
        text=True,
        env=env,
    )

    assert proc.stdout == str(projects)
