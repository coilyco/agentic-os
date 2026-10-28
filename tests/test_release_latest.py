"""Exercise scripts/ci/release-latest.sh against a stub gh."""

from __future__ import annotations

import os
from pathlib import Path
import stat
import subprocess

ROOT = Path(__file__).resolve().parent.parent
SCRIPT = ROOT / "scripts" / "ci" / "release-latest.sh"


def _run(tmp_path: Path, verb: str, *, release_exists: bool, latest: str) -> subprocess.CompletedProcess[str]:
    bindir = tmp_path / "bin"
    bindir.mkdir()
    stub = bindir / "gh"
    stub.write_text(
        "#!/bin/sh\n"
        'case "$2" in\n'
        f'  */releases/tags/*) [ "{int(release_exists)}" = 1 ] && echo \'{{"id":1}}\' || exit 1 ;;\n'
        f"  */releases/latest) echo {latest} ;;\n"
        "esac\n",
        encoding="utf-8",
    )
    stub.chmod(stub.stat().st_mode | stat.S_IXUSR)
    return subprocess.run(
        ["bash", str(SCRIPT), verb],
        env=os.environ
        | {"PATH": f"{bindir}{os.pathsep}{os.environ['PATH']}", "TAG": "aos-v1.2.3", "GITHUB_REPOSITORY": "o/r"},
        capture_output=True,
        text=True,
        check=False,
    )


def test_a_tag_with_a_release_passes_the_check(tmp_path: Path) -> None:
    assert _run(tmp_path, "check", release_exists=True, latest="x").returncode == 0


def test_a_tag_without_a_release_is_refused(tmp_path: Path) -> None:
    done = _run(tmp_path, "check", release_exists=False, latest="x")

    assert done.returncode == 1
    assert "no release exists for aos-v1.2.3" in done.stderr


def test_verify_passes_when_latest_is_the_tag(tmp_path: Path) -> None:
    done = _run(tmp_path, "verify", release_exists=True, latest="aos-v1.2.3")

    assert done.returncode == 0
    assert "releases/latest reads aos-v1.2.3" in done.stdout


def test_verify_fails_when_latest_is_another_tag(tmp_path: Path) -> None:
    done = _run(tmp_path, "verify", release_exists=True, latest="aos-v0.481.0")

    assert done.returncode == 1
    assert "expected aos-v1.2.3" in done.stderr
