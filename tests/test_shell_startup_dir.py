"""Smoke coverage for the shared shell startup directory."""
from __future__ import annotations

import os
import shlex
import shutil
import subprocess
from pathlib import Path

import pytest


REPO_ROOT = Path(__file__).resolve().parent.parent
BASH = shutil.which("bash") or "bash"
ZSH = shutil.which("zsh")
if os.name == "nt":
    git_bash = Path(os.environ["ProgramFiles"]) / "Git" / "bin" / "bash.exe"
    if git_bash.exists():
        BASH = str(git_bash)
    msys_zsh = Path("C:/msys64/usr/bin/zsh.exe")
    if msys_zsh.exists():
        ZSH = str(msys_zsh)


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


def _native_path(path: Path) -> str:
    return path.as_posix() if os.name == "nt" else str(path)


def _zsh_path(path: Path) -> str:
    if os.name != "nt" or ZSH is None:
        return str(path)

    cygpath = Path(ZSH).parent / "cygpath.exe"
    proc = subprocess.run(
        [cygpath, "-u", str(path)],
        check=True,
        capture_output=True,
        text=True,
    )
    return proc.stdout.strip()


def _interactive_shell_pwd(
    home: Path,
    startup_dir: Path | None = None,
    *,
    projects_root: Path | None = None,
    repo_root: Path = REPO_ROOT,
    cwd: Path | None = None,
    rcfile: Path | None = None,
    default_shell: str | None = None,
    native_sessions_root: Path | None = None,
    existing_terminal_tree: bool = False,
) -> str:
    env = os.environ.copy()
    env.pop("AOS_STARTUP_DIR", None)
    env.pop("PROJECTS_ROOT", None)
    env.pop("AOS_NATIVE_SESSIONS_DIR", None)
    env.pop("_SIREN_SHELL_ENV", None)
    env.update(
        {
            "HOME": _bash_path(home),
            "PATH": "/usr/bin:/bin",
            # Git for Windows receives this value while MSYS_NO_PATHCONV may be
            # inherited, so keep the drive-qualified native form.
            "AOS_REPO_ROOT": _native_path(repo_root),
        }
    )
    if startup_dir is not None:
        env["AOS_STARTUP_DIR"] = _bash_path(startup_dir)
    if projects_root is not None:
        env["PROJECTS_ROOT"] = _bash_path(projects_root)
    if default_shell is not None:
        env["SHELL"] = default_shell
    if native_sessions_root is not None:
        env["AOS_NATIVE_SESSIONS_DIR"] = _bash_path(native_sessions_root)
    if existing_terminal_tree:
        env["_SIREN_SHELL_ENV"] = "1"

    proc = subprocess.run(
        [
            BASH,
            "--noprofile",
            "--rcfile",
            _bash_path(rcfile or REPO_ROOT / "shell" / "common.sh"),
            "-ic",
            'printf "%s" "$PWD"',
        ],
        cwd=cwd or home,
        check=True,
        capture_output=True,
        text=True,
        env=env,
    )

    return proc.stdout


def _interactive_zsh_pwd(
    home: Path,
    projects_root: Path,
    *,
    cwd: Path | None = None,
    native_sessions_root: Path | None = None,
) -> str:
    if ZSH is None:
        pytest.skip("zsh is unavailable")

    env = os.environ.copy()
    env.pop("AOS_STARTUP_DIR", None)
    env.pop("AOS_NATIVE_SESSIONS_DIR", None)
    env.pop("_SIREN_SHELL_ENV", None)
    env.update(
        {
            "HOME": _zsh_path(home),
            "PROJECTS_ROOT": _zsh_path(projects_root),
            "AOS_REPO_ROOT": _native_path(REPO_ROOT),
            # The account's default shell must not redirect the running Zsh
            # process into Bash startup syntax.
            "SHELL": "/bin/bash",
        }
    )
    if native_sessions_root is not None:
        env["AOS_NATIVE_SESSIONS_DIR"] = _zsh_path(native_sessions_root)
    zshrc = shlex.quote(_zsh_path(REPO_ROOT / "shell" / "zshrc"))
    proc = subprocess.run(
        [ZSH, "-dfi", "-c", f'source {zshrc}; printf "%s" "$PWD"'],
        cwd=cwd or home,
        check=True,
        capture_output=True,
        text=True,
        env=env,
    )

    assert "command not found" not in proc.stderr
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


def test_interactive_shell_honors_projects_root_override(tmp_path: Path) -> None:
    home = tmp_path / "home"
    projects = tmp_path / "workspace"
    home.mkdir()
    projects.mkdir()

    assert _interactive_shell_pwd(home, projects_root=projects) == _bash_path(projects)


def test_interactive_shell_derives_projects_root_from_aos_checkout(tmp_path: Path) -> None:
    home = tmp_path / "home"
    projects = tmp_path / "projects"
    repo = projects / "coilyco-flight-deck" / "agentic-os"
    home.mkdir()
    repo.mkdir(parents=True)
    subprocess.run(
        ["git", "init", str(repo)],
        check=True,
        capture_output=True,
        text=True,
    )

    assert _interactive_shell_pwd(home, repo_root=repo) == _bash_path(projects)


def test_interactive_shell_preserves_explicit_working_directory(tmp_path: Path) -> None:
    home = tmp_path / "home"
    projects = home / "projects"
    checkout = projects / "coilyco-flight-deck" / "agentic-os"
    checkout.mkdir(parents=True)

    assert _interactive_shell_pwd(home, cwd=checkout) == _bash_path(checkout)


def test_fresh_shell_leaves_native_session_root(tmp_path: Path) -> None:
    home = tmp_path / "home"
    projects = home / "projects"
    native_sessions = tmp_path / "aos" / "native"
    native_projects = native_sessions / "session" / "projects"
    projects.mkdir(parents=True)
    native_projects.mkdir(parents=True)

    assert _interactive_shell_pwd(
        home,
        projects_root=projects,
        cwd=native_projects,
        native_sessions_root=native_sessions,
    ) == _bash_path(projects)


def test_nested_shell_preserves_native_session_root(tmp_path: Path) -> None:
    home = tmp_path / "home"
    projects = home / "projects"
    native_sessions = tmp_path / "aos" / "native"
    native_projects = native_sessions / "session" / "projects"
    projects.mkdir(parents=True)
    native_projects.mkdir(parents=True)

    assert _interactive_shell_pwd(
        home,
        projects_root=projects,
        cwd=native_projects,
        native_sessions_root=native_sessions,
        existing_terminal_tree=True,
    ) == _bash_path(native_projects)


def test_bash_entry_ignores_account_default_shell(tmp_path: Path) -> None:
    home = tmp_path / "home"
    projects = home / "projects"
    projects.mkdir(parents=True)

    assert _interactive_shell_pwd(
        home,
        projects_root=projects,
        rcfile=REPO_ROOT / "shell" / "bashrc",
        default_shell="/bin/zsh",
    ) == _bash_path(projects)


def test_zsh_entry_ignores_account_default_shell(tmp_path: Path) -> None:
    home = tmp_path / "home"
    projects = home / "projects"
    projects.mkdir(parents=True)

    assert _interactive_zsh_pwd(home, projects) == _zsh_path(projects)


def test_zsh_fresh_shell_leaves_native_session_root(tmp_path: Path) -> None:
    home = tmp_path / "home"
    projects = home / "projects"
    native_sessions = tmp_path / "aos" / "native"
    native_projects = native_sessions / "session" / "projects"
    projects.mkdir(parents=True)
    native_projects.mkdir(parents=True)

    assert _interactive_zsh_pwd(
        home,
        projects,
        cwd=native_projects,
        native_sessions_root=native_sessions,
    ) == _zsh_path(projects)
