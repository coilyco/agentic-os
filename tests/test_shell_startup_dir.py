"""Smoke coverage for the shared shell startup directory."""
from __future__ import annotations

import os
import shutil
import subprocess
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parent.parent
BASH = shutil.which("bash") or "bash"
if os.name == "nt":
    git_bash = Path(os.environ["ProgramFiles"]) / "Git" / "bin" / "bash.exe"
    if git_bash.exists():
        BASH = str(git_bash)


def _bash_path(path: Path) -> str:
    if os.name != "nt":
        return str(path)

    proc = subprocess.run(
        [BASH, "-lc", 'cygpath -u "$1"', "bash", str(path)],
        check=True,
        capture_output=True,
        text=True,
    )
    return proc.stdout.strip()


def _interactive_shell_pwd(home: Path, startup_dir: Path | None = None) -> str:
    env = os.environ.copy()
    env.pop("WARP_STARTUP_DIR", None)
    env.update(
        {
            "HOME": _bash_path(home),
            "PATH": "/usr/bin:/bin",
            "AOS_REPO_ROOT": _bash_path(REPO_ROOT),
        }
    )
    if startup_dir is not None:
        env["WARP_STARTUP_DIR"] = _bash_path(startup_dir)

    proc = subprocess.run(
        [
            BASH,
            "--noprofile",
            "--rcfile",
            _bash_path(REPO_ROOT / "shell" / "common.sh"),
            "-ic",
            'printf "%s" "$PWD"',
        ],
        cwd=home,
        check=True,
        capture_output=True,
        text=True,
        env=env,
    )

    return proc.stdout


def test_interactive_shell_lands_in_projects_from_home(tmp_path: Path) -> None:
    home = tmp_path / "home"
    projects = home / "projects"
    projects.mkdir(parents=True)

    assert _interactive_shell_pwd(home) == _bash_path(projects)


def test_interactive_shell_honors_startup_dir_override(tmp_path: Path) -> None:
    home = tmp_path / "home"
    startup_dir = tmp_path / "profile-repo"
    home.mkdir()
    startup_dir.mkdir()

    assert _interactive_shell_pwd(home, startup_dir) == _bash_path(startup_dir)
