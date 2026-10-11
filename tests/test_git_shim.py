"""Tests for the guarded `git` replacement.

Each case runs the built binary against a scratch repository with a local bare
remote, so a call that is not refused really does rewrite the remote. The
`-n`, `-f` and bundled-flag cases only hold on umbra v0.254.0 or later, so a
stale lock fails here instead of passing in silence. See COI-2724.
"""

from __future__ import annotations

import os
import shutil
import subprocess
from pathlib import Path

import pytest

ROOT = Path(__file__).resolve().parents[1]
PROJECT = ROOT / ".umbra" / "shims"
MEMBER = PROJECT / "git" / "git.kdl"
LOCK = ROOT / ".umbra" / "guardfiles" / "specverb.lock"

ENV = {
    **os.environ,
    "GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@example.invalid",
    "GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@example.invalid",
    "GIT_CONFIG_GLOBAL": os.devnull, "GIT_CONFIG_SYSTEM": os.devnull,
}

REFUSED = [
    ["push", "--force", "origin", "HEAD:main"],
    ["push", "-f", "origin", "HEAD:main"],
    ["push", "-uf", "origin", "HEAD:main"],
    ["push", "--force-with-lease", "origin", "HEAD:main"],
    ["push", "--force-with-lease=main:abc", "origin", "HEAD:main"],
    ["push", "--force-if-includes", "origin", "HEAD:main"],
    ["push", "origin", "+HEAD:main"],
    ["push", "--no-verify", "origin", "HEAD:main"],
    ["commit", "--no-verify", "-m", "x"],
    ["commit", "-n", "-m", "x"],
    ["commit", "-an", "-m", "x"],
    ["merge", "--no-verify", "main"],
    ["-c", "core.hooksPath=/dev/null", "commit", "-m", "x"],
    ["config", "core.hooksPath", "/dev/null"],
]

ALLOWED = [
    ["status", "-s"],
    ["log", "--oneline", "-1"],
    ["push", "origin", "HEAD:refs/heads/task"],
    ["push", "-u", "origin", "HEAD:refs/heads/task2"],
    ["commit", "-a", "-m", "a message that mentions -n and --force"],
]


@pytest.fixture(scope="module")
def shim(tmp_path_factory: pytest.TempPathFactory) -> Path:
    binary = tmp_path_factory.mktemp("git-shim") / "git"
    shutil.copyfile(LOCK, PROJECT / "specverb.lock")
    subprocess.run(
        ["umbra", "--project-root", str(PROJECT), "--guardfile", str(MEMBER),
         "build", "--out", str(binary)],
        cwd=ROOT, check=True, capture_output=True, text=True,
    )
    return binary


@pytest.fixture()
def repo(tmp_path: Path, shim: Path):
    """A clone whose main has diverged from its bare remote, so a force would land."""
    real = shutil.which("git")
    assert real is not None

    def git(*argv: str, cwd: Path = tmp_path, binary: str = real):
        return subprocess.run([binary, *argv], cwd=cwd, env=ENV,
                              capture_output=True, text=True)

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
    remote_main = git("rev-parse", "main", cwd=tmp_path / "remote.git").stdout
    (work / "f").write_text("d")
    return work, tmp_path / "remote.git", remote_main, str(shim)


def _run(binary: str, argv: list[str], cwd: Path):
    return subprocess.run([binary, *argv], cwd=cwd, env=ENV, capture_output=True, text=True)


@pytest.mark.parametrize("argv", REFUSED, ids=lambda a: " ".join(a))
def test_the_spelling_is_refused_and_the_remote_is_untouched(repo, argv) -> None:
    work, remote, before, binary = repo
    subprocess.run([shutil.which("git") or "git", "add", "f"], cwd=work, env=ENV)
    result = _run(binary, argv, work)
    assert result.returncode != 0
    assert "denied" in result.stderr or "never" in result.stderr
    after = subprocess.run([shutil.which("git") or "git", "rev-parse", "main"],
                           cwd=remote, env=ENV, capture_output=True, text=True).stdout
    assert after == before


@pytest.mark.parametrize("argv", ALLOWED, ids=lambda a: " ".join(a))
def test_ordinary_in_lane_work_still_passes(repo, argv) -> None:
    work, _, _, binary = repo
    result = _run(binary, argv, work)
    assert result.returncode == 0, result.stderr
