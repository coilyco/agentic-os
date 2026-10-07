"""Integration tests for scripts/trufflehog-scan.sh, run against the real trufflehog binary.

The hook scans staged content at pre-commit, so those cases stage a change in a
throwaway git repo and run the script there (COI-2134: a clone-based scan passed staged
secrets). At pre-push it scans the pushed commits, so those cases commit instead and
set the variables pre-commit exports, PRE_COMMIT=1 included (COI-2430). A mock binary
cannot catch either class of bug, because the failure was in what trufflehog was
pointed at.
"""
from __future__ import annotations

import os
import shutil
import subprocess
from pathlib import Path

import pytest


SCRIPT = Path(__file__).resolve().parent.parent / "scripts" / "trufflehog-scan.sh"

# The public sha256 of the sentry_sdk-2.70.0.tar.gz sdist, which trufflehog's
# SentryToken detector reads as a secret (agentic-os#8407).
SENTRY_SDIST_SHA256 = "27e4b512f47be93136645dbe8f02d430473ee73a389ce430b2b7104baa66cf75"  # trufflehog:ignore (public sdist hash under test)

# A non-Sentry credential shape that must be caught anywhere, lockfiles included:
# a registry resolved URL carrying embedded basic-auth credentials.
BASIC_AUTH_RESOLVED = "https://alice:hunter2@registry.example.com/demo/-/demo-1.0.0.tgz"  # trufflehog:ignore (fake credential under test)

_SENTRY_SDIST_LINE = (
    'sdist = { url = "https://files.pythonhosted.org/packages/52/0a/'
    "e37553f3106d0f7f0d4b8ad2c7b92410d131929786ffd116833d3b8164df/"
    'sentry_sdk-2.70.0.tar.gz", hash = "sha256:'
    f"{SENTRY_SDIST_SHA256}"
    '", size = 1047234, upload-time = "2026-09-22T09:40:11.721Z" }'
)

_UV_LOCK_WITH_SENTRY = (
    "version = 1\n"
    "\n"
    "[[package]]\n"
    'name = "sentry-sdk"\n'
    'version = "2.70.0"\n'
    f"{_SENTRY_SDIST_LINE}\n"
)

_PACKAGE_LOCK_WITH_BASIC_AUTH = (
    "{\n"
    '  "name": "demo",\n'
    f'  "resolved": "{BASIC_AUTH_RESOLVED}"\n'
    "}\n"
)

_CREDENTIAL_NOTE = f"resolved {BASIC_AUTH_RESOLVED}\n"


@pytest.fixture(autouse=True)
def _require_trufflehog() -> None:
    # Absent binary is an environment defect, not a reason to skip: the hook needs it.
    assert shutil.which("trufflehog"), "trufflehog must be on PATH for the scan tests"


def _git_env() -> dict[str, str]:
    # Drop the GIT_* variables pre-commit exports (GIT_INDEX_FILE, GIT_DIR, ...) so
    # the fixture repo never touches the repo that is running these tests.
    return {k: v for k, v in os.environ.items() if not k.startswith("GIT_")}


def _git(root: Path, *args: str) -> None:
    subprocess.run(
        ["git", "-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false", *args],
        cwd=root,
        env=_git_env(),
        check=True,
        capture_output=True,
    )


def _repo(root: Path) -> Path:
    """A fresh repo with one base commit holding empty lockfiles."""
    _git(root, "init", "-q", ".")
    (root / "uv.lock").write_text("version = 1\n", encoding="utf-8")
    (root / "package-lock.json").write_text("{}\n", encoding="utf-8")
    _git(root, "add", "-A")
    _git(root, "commit", "-q", "-m", "base")
    return root


def _stage(root: Path, rel: str, content: str, *, force: bool = False) -> None:
    path = root / rel
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(content, encoding="utf-8")
    _git(root, "add", *(["-f"] if force else []), "--", rel)


def _scan(root: Path, extra_env: dict[str, str] | None = None) -> subprocess.CompletedProcess[str]:
    command = [str(SCRIPT)]
    if os.name == "nt":
        bash = shutil.which("bash")
        assert bash is not None
        command = [bash, str(SCRIPT)]
    return subprocess.run(
        command, cwd=root, env={**_git_env(), **(extra_env or {})}, text=True, capture_output=True, check=False
    )


def test_staged_lockfile_basic_auth_credential_fails(tmp_path: Path) -> None:
    root = _repo(tmp_path)
    _stage(root, "package-lock.json", _PACKAGE_LOCK_WITH_BASIC_AUTH)
    result = _scan(root)
    assert result.returncode != 0, result.stdout + result.stderr
    assert "package-lock.json" in result.stdout


def test_staged_source_file_credential_fails(tmp_path: Path) -> None:
    root = _repo(tmp_path)
    _stage(root, "notes.txt", _CREDENTIAL_NOTE)
    result = _scan(root)
    assert result.returncode != 0, result.stdout + result.stderr
    assert "notes.txt" in result.stdout


def test_staged_credential_under_dot_dir_and_spaced_name_fails(tmp_path: Path) -> None:
    root = _repo(tmp_path)
    _stage(root, ".github/ops notes.yml", _CREDENTIAL_NOTE)
    result = _scan(root)
    assert result.returncode != 0, result.stdout + result.stderr


def test_staged_lockfile_sentry_sdist_hash_passes(tmp_path: Path) -> None:
    root = _repo(tmp_path)
    _stage(root, "uv.lock", _UV_LOCK_WITH_SENTRY)
    result = _scan(root)
    assert result.returncode == 0, result.stdout + result.stderr


def test_staged_non_lockfile_sentry_sdist_hash_still_fails(tmp_path: Path) -> None:
    # Negative control for the test above: the detector is live outside lockfiles.
    root = _repo(tmp_path)
    _stage(root, "src/app.py", f'_SENTRY_DSN = "https://{SENTRY_SDIST_SHA256}@ingest.sentry.io/1"\n')
    result = _scan(root)
    assert result.returncode != 0, result.stdout + result.stderr


def test_force_added_build_cache_dir_is_not_scanned(tmp_path: Path) -> None:
    root = _repo(tmp_path)
    _stage(root, "node_modules/pkg/notes.txt", _CREDENTIAL_NOTE, force=True)
    result = _scan(root)
    assert result.returncode == 0, result.stdout + result.stderr


def test_scan_reads_the_index_not_the_working_tree(tmp_path: Path) -> None:
    root = _repo(tmp_path)
    _stage(root, "notes.txt", "nothing to see\n")
    (root / "notes.txt").write_text(_CREDENTIAL_NOTE, encoding="utf-8")  # unstaged edit
    result = _scan(root)
    assert result.returncode == 0, result.stdout + result.stderr


def test_nothing_staged_passes(tmp_path: Path) -> None:
    root = _repo(tmp_path)
    result = _scan(root)
    assert result.returncode == 0, result.stdout + result.stderr


def _commit(root: Path, rel: str, content: str) -> str:
    """Commit one file and return the new commit's sha."""
    path = root / rel
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(content, encoding="utf-8")
    _git(root, "add", "--", rel)
    _git(root, "commit", "-q", "-m", f"add {rel}")
    return _rev(root, "HEAD")


def _rev(root: Path, ref: str) -> str:
    return subprocess.run(
        ["git", "rev-parse", ref], cwd=root, env=_git_env(), check=True, capture_output=True, text=True
    ).stdout.strip()


def _push_range(root: Path, from_ref: str) -> subprocess.CompletedProcess[str]:
    """Scan as pre-push does for a branch whose remote tip is from_ref."""
    return _scan(root, {"PRE_COMMIT": "1", "PRE_COMMIT_FROM_REF": from_ref, "PRE_COMMIT_TO_REF": _rev(root, "HEAD")})


def test_push_committed_credential_in_range_fails(tmp_path: Path) -> None:
    root = _repo(tmp_path)
    base = _rev(root, "HEAD")
    _commit(root, "notes.txt", _CREDENTIAL_NOTE)
    result = _push_range(root, base)
    assert result.returncode != 0, result.stdout + result.stderr
    assert "notes.txt" in result.stdout


def test_push_credential_removed_within_range_still_fails(tmp_path: Path) -> None:
    root = _repo(tmp_path)
    base = _rev(root, "HEAD")
    _commit(root, "notes.txt", _CREDENTIAL_NOTE)
    _commit(root, "notes.txt", "clean\n")
    result = _push_range(root, base)
    assert result.returncode != 0, result.stdout + result.stderr


def test_push_credential_before_the_range_is_not_rescanned(tmp_path: Path) -> None:
    root = _repo(tmp_path)
    _commit(root, "notes.txt", _CREDENTIAL_NOTE)
    base = _rev(root, "HEAD")
    _commit(root, "other.txt", "nothing to see\n")
    result = _push_range(root, base)
    assert result.returncode == 0, result.stdout + result.stderr


def test_push_empty_range_passes(tmp_path: Path) -> None:
    root = _repo(tmp_path)
    _commit(root, "notes.txt", _CREDENTIAL_NOTE)
    result = _push_range(root, _rev(root, "HEAD"))
    assert result.returncode == 0, result.stdout + result.stderr


def test_first_push_scans_the_whole_history_including_the_root_commit(tmp_path: Path) -> None:
    # pre-commit sets no refs when the push reaches the root commit, only the remote name.
    root = tmp_path
    _git(root, "init", "-q", ".")
    _commit(root, "notes.txt", _CREDENTIAL_NOTE)
    _commit(root, "other.txt", "nothing to see\n")
    result = _scan(root, {"PRE_COMMIT": "1", "PRE_COMMIT_REMOTE_NAME": "origin"})
    assert result.returncode != 0, result.stdout + result.stderr


def test_push_lockfile_sentry_sdist_hash_passes(tmp_path: Path) -> None:
    root = _repo(tmp_path)
    base = _rev(root, "HEAD")
    _commit(root, "uv.lock", _UV_LOCK_WITH_SENTRY)
    result = _push_range(root, base)
    assert result.returncode == 0, result.stdout + result.stderr


def test_push_lockfile_basic_auth_credential_fails(tmp_path: Path) -> None:
    root = _repo(tmp_path)
    base = _rev(root, "HEAD")
    _commit(root, "package-lock.json", _PACKAGE_LOCK_WITH_BASIC_AUTH)
    result = _push_range(root, base)
    assert result.returncode != 0, result.stdout + result.stderr
    assert "package-lock.json" in result.stdout


def test_push_non_lockfile_sentry_sdist_hash_still_fails(tmp_path: Path) -> None:
    root = _repo(tmp_path)
    base = _rev(root, "HEAD")
    _commit(root, "src/app.py", f'_SENTRY_DSN = "https://{SENTRY_SDIST_SHA256}@ingest.sentry.io/1"\n')
    result = _push_range(root, base)
    assert result.returncode != 0, result.stdout + result.stderr


def test_push_committed_build_cache_dir_is_not_scanned(tmp_path: Path) -> None:
    root = _repo(tmp_path)
    base = _rev(root, "HEAD")
    path = root / "node_modules/pkg/notes.txt"
    path.parent.mkdir(parents=True)
    path.write_text(_CREDENTIAL_NOTE, encoding="utf-8")
    _git(root, "add", "-f", "--", "node_modules/pkg/notes.txt")
    _git(root, "commit", "-q", "-m", "vendored")
    result = _push_range(root, base)
    assert result.returncode == 0, result.stdout + result.stderr


@pytest.mark.skipif(os.name == "nt", reason="the fake git is a POSIX shell script")
def test_push_scan_that_reads_nothing_fails_instead_of_passing(tmp_path: Path) -> None:
    # trufflehog exits 0 with `chunks: 0` when its own `git log` fails. Not a clean scan.
    # The fake wraps a fixed-path git: re-exec by name loops with an attribution shim.
    real_git = next(
        (c for c in ("/usr/bin/git", "/opt/homebrew/bin/git", "/usr/local/bin/git") if os.access(c, os.X_OK)), None
    )
    if real_git is None:
        pytest.skip("no git at a fixed system path to wrap")
    shim_dir = tmp_path / "shim"
    shim_dir.mkdir()
    fake = shim_dir / "git"
    fake.write_text(f'#!/bin/sh\ncase " $* " in *" log "*) exit 2;; esac\nexec {real_git} "$@"\n', encoding="utf-8")
    fake.chmod(0o755)
    root = tmp_path / "repo"
    root.mkdir()
    _repo(root)
    base = _rev(root, "HEAD")
    _commit(root, "notes.txt", _CREDENTIAL_NOTE)
    result = _scan(
        root,
        {
            "PATH": f"{shim_dir}{os.pathsep}{os.environ['PATH']}",
            "PRE_COMMIT_FROM_REF": base,
            "PRE_COMMIT_TO_REF": _rev(root, "HEAD"),
        },
    )
    assert result.returncode != 0, result.stdout + result.stderr
    assert "read no chunks" in result.stderr
