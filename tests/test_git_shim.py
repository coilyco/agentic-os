"""Tests for the git guard stub.

Each case runs the installed stub against a scratch repository with a local bare
remote, so a call that is not refused really does rewrite the remote. umbra's
metacharacter gate refused `<` and `(` in any argument of a named grant, which
failed every attributed commit (COI-2738), so the allowed cases carry them.
"""

from __future__ import annotations

import os
import shutil
import subprocess
from pathlib import Path

import pytest

ROOT = Path(__file__).resolve().parents[1]
INSTALL = ROOT / "scripts" / "fs-shims" / "install.sh"
HOST_WRAPPER = Path.home() / ".local" / "share" / "agent-git-attribution" / "agent_git_attribution.py"

# A bare PATH: the developer's own may already carry an attribution wrapper, and
# two wrappers that each skip only themselves loop.
SYSTEM_PATH = "/usr/bin:/bin"
BASE_ENV = {
    **{k: v for k, v in os.environ.items()
       if not k.startswith(("GIT_CONFIG_", "AGENT_GIT_ATTRIBUTION_"))},
    "GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@example.invalid",
    "GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@example.invalid",
    "GIT_CONFIG_GLOBAL": os.devnull, "GIT_CONFIG_SYSTEM": os.devnull,
}

REFUSED = [
    ["push", "--force", "origin", "HEAD:main"],
    ["push", "-f", "origin", "HEAD:main"],
    ["push", "-uf", "origin", "HEAD:main"],
    ["push", "--forc", "origin", "HEAD:main"],
    ["push", "--force-with-lease", "origin", "HEAD:main"],
    ["push", "--force-with-lease=main:abc", "origin", "HEAD:main"],
    ["push", "--force-if-includes", "origin", "HEAD:main"],
    ["push", "origin", "+HEAD:main"],
    ["push", "--no-verify", "origin", "HEAD:main"],
    ["push", "--no-verif", "origin", "HEAD:main"],
    ["commit", "--no-verify", "-m", "x"],
    ["commit", "--no-verif", "-m", "x"],
    ["commit", "-n", "-m", "x"],
    ["commit", "-an", "-m", "x"],
    ["commit", "-m", "x", "-n"],
    ["merge", "--no-verify", "main"],
    ["rebase", "--no-verify", "main"],
    ["-c", "core.hooksPath=/dev/null", "commit", "-m", "x"],
    ["--config-env=core.hooksPath=X", "status"],
    ["config", "core.hooksPath", "/dev/null"],
    ["config", "--global", "core.hookspath", "/dev/null"],
]

ALLOWED = [
    ["status", "-s"],
    ["log", "--oneline", "-1"],
    ["log", "-n", "1"],
    ["push", "origin", "HEAD:refs/heads/task"],
    ["push", "-u", "origin", "HEAD:refs/heads/task2"],
    ["merge", "--no-verify-signatures", "--no-edit", "HEAD"],
    ["-C", ".", "--version"],
    ["commit", "-a", "-m", "a message that mentions -n and --force"],
    ["commit", "-a", "-m", "feat(guard): a;b <c> $d `e` | f & g"],
    ["commit", "-a", "--author", "A <a@b.invalid>", "-m", "x"],
    ["commit", "-a", "-m", "x", "--trailer", "Co-authored-by: Claude <noreply@anthropic.com>"],
]


@pytest.fixture(scope="module")
def stubs(tmp_path_factory: pytest.TempPathFactory) -> Path:
    shims = tmp_path_factory.mktemp("guard-shims")
    subprocess.run(["sh", str(INSTALL), str(shims)], check=True, capture_output=True)
    return shims


def _run(argv: list[str], cwd: Path, path: str, extra: dict[str, str] | None = None):
    env = {**BASE_ENV, "PATH": path, **(extra or {})}
    return subprocess.run(argv, cwd=cwd, env=env, capture_output=True, text=True)


@pytest.fixture()
def repo(tmp_path: Path, stubs: Path):
    """A clone whose main has diverged from its bare remote, so a force would land."""
    real = shutil.which("git", path=SYSTEM_PATH)
    assert real is not None

    def git(*argv: str, cwd: Path = tmp_path):
        return subprocess.run([real, *argv], cwd=cwd, env=BASE_ENV, capture_output=True, text=True)

    git("init", "-q", "--bare", "remote.git")
    git("clone", "-q", "remote.git", "work")
    work = tmp_path / "work"
    for name, text in (("one", "a"), ("two", "b")):
        (work / "f").write_text(text)
        git("add", "f", cwd=work)
        git("commit", "-q", "-m", name, cwd=work)
        git("push", "-q", "origin", "HEAD:main", cwd=work)
    git("reset", "-q", "--hard", "HEAD~1", cwd=work)
    (work / "f").write_text("c")
    git("commit", "-q", "-am", "diverge", cwd=work)
    before = git("rev-parse", "main", cwd=tmp_path / "remote.git").stdout
    (work / "f").write_text("d")
    path = f"{stubs}{os.pathsep}{SYSTEM_PATH}"
    return work, tmp_path / "remote.git", before, path, git


@pytest.mark.parametrize("argv", REFUSED, ids=lambda a: " ".join(a))
def test_the_spelling_is_refused_and_the_remote_is_untouched(repo, argv) -> None:
    work, remote, before, path, git = repo
    git("add", "f", cwd=work)
    result = _run(["git", *argv], work, path)
    assert result.returncode == 2, result.stderr
    assert "denied" in result.stderr
    assert git("rev-parse", "main", cwd=remote).stdout == before


@pytest.mark.parametrize("argv", ALLOWED, ids=lambda a: " ".join(a))
def test_ordinary_in_lane_work_still_passes(repo, argv) -> None:
    work, _, _, path, _ = repo
    result = _run(["git", *argv], work, path)
    assert result.returncode == 0, result.stderr


# These exit non-zero for git's own reasons, so only a refusal is checked.
PASSED_THROUGH = [
    ["push", "-o", "-f", "origin", "HEAD:refs/heads/opt"],
    ["config", "core.hooksPath"],
    ["config", "--get", "core.hooksPath"],
    ["config", "--unset", "core.hooksPath"],
]


@pytest.mark.parametrize("argv", PASSED_THROUGH, ids=lambda a: " ".join(a))
def test_a_read_or_a_flag_shaped_value_is_not_refused(repo, argv) -> None:
    work, _, _, path, _ = repo
    assert "denied" not in _run(["git", *argv], work, path).stderr


def test_a_hooks_path_in_the_environment_is_refused(repo) -> None:
    work, _, _, path, _ = repo
    result = _run(["git", "status"], work, path, {"GIT_CONFIG_PARAMETERS": "'core.hookspath=/dev/null'"})
    assert result.returncode == 2 and "denied" in result.stderr


def test_an_attribution_wrapper_in_front_of_the_guard_still_commits(repo, tmp_path: Path) -> None:
    """The wrapper adds --author and --trailer with angle brackets, then runs the next git."""
    work, _, _, path, git = repo
    front = tmp_path / "front"
    front.mkdir()
    wrapper = front / "git"
    wrapper.write_text(
        "#!/bin/sh\n"
        'dir=$(dirname "$0"); PATH=$(printf %s "$PATH" | tr : "\\n" | grep -vx "$dir" | paste -sd: -)\n'
        'case "$1" in commit) shift; exec git commit --author "Claude <noreply@anthropic.com>" '
        '--trailer "Co-authored-by: Claude <noreply@anthropic.com>" "$@";; esac\n'
        'exec git "$@"\n'
    )
    wrapper.chmod(0o755)
    git("add", "f", cwd=work)
    result = _run(["git", "commit", "-q", "-m", "feat(x): y"], work, f"{front}{os.pathsep}{path}")
    assert result.returncode == 0, result.stderr
    body = git("log", "-1", "--format=%an <%ae>%n%B", cwd=work).stdout
    assert "Claude <noreply@anthropic.com>" in body


@pytest.mark.skipif(not HOST_WRAPPER.is_file(), reason="the host attribution wrapper is not installed")
def test_the_real_attribution_wrapper_in_front_of_the_guard_still_commits(repo, tmp_path: Path) -> None:
    work, _, _, path, git = repo
    front = tmp_path / "libexec"
    front.mkdir()
    (front / "git").symlink_to(HOST_WRAPPER)
    git("add", "f", cwd=work)
    result = _run(
        ["git", "commit", "-q", "-m", "feat(x): y"], work, f"{front}{os.pathsep}{path}",
        {"AGENT_GIT_ATTRIBUTION_COAUTHOR": "Claude <noreply@anthropic.com>"},
    )
    assert result.returncode == 0, result.stderr
    assert "Claude <noreply@anthropic.com>" in git("log", "-1", "--format=%B", cwd=work).stdout
