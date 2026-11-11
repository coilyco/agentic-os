"""The Mac app shows the client's release version, never a stuck one (COI-3477)."""

from __future__ import annotations

from pathlib import Path
import os
import platform
import shutil
import subprocess

import pytest

ROOT = Path(__file__).resolve().parents[1]
pytestmark = pytest.mark.skipif(platform.system() != "Darwin", reason="the script runs on a Mac only")


def _tree(tmp_path: Path, tags: list[str]) -> Path:
    script = tmp_path / "scripts" / "treehouse-mac.sh"
    script.parent.mkdir(parents=True)
    shutil.copy(ROOT / "scripts" / "treehouse-mac.sh", script)
    git = ["git", "-C", str(tmp_path), "-c", "user.name=t", "-c", "user.email=t@example.invalid"]
    git += ["-c", "tag.gpgSign=false", "-c", "commit.gpgSign=false"]
    subprocess.run([*git, "init", "-q"], check=True)
    subprocess.run([*git, "commit", "-q", "--allow-empty", "-m", "x"], check=True)
    for tag in tags:
        subprocess.run([*git, "tag", tag], check=True)
    return script


def _version(script: Path, **env: str) -> subprocess.CompletedProcess[str]:
    clean = {key: value for key, value in os.environ.items() if key != "TREEHOUSE_VERSION"}
    return subprocess.run(["bash", str(script), "version"], capture_output=True, text=True, env={**clean, **env}, check=False)


def test_the_version_is_the_newest_android_release_tag_by_number_not_by_text(tmp_path: Path) -> None:
    script = _tree(tmp_path, ["aterm-android-v0.9.0", "aterm-android-v0.40.0", "aterm-android-v0.100.0", "aos-v9.9.9"])
    done = _version(script)
    assert (done.returncode, done.stdout) == (0, "0.100.0")


def test_a_tree_with_no_tags_takes_the_version_it_is_given_and_refuses_a_malformed_one(tmp_path: Path) -> None:
    script = _tree(tmp_path, [])
    assert _version(script, TREEHOUSE_VERSION="1.2.3").stdout == "1.2.3"
    for bad in ("", "latest", "1.2", "1.2.3-rc1"):
        done = _version(script, TREEHOUSE_VERSION=bad)
        assert (done.returncode, done.stdout) != (0, "1.2.3"), bad
        assert done.stdout == "", bad
