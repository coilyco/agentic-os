"""Tests for the disk-pressure guard script and its installer.

df is real, docker is a recorder. A threshold above the volume's free space
forces the pressure path, a threshold of 1 KiB forces the quiet one. See COI-2766.
"""

from __future__ import annotations

import os
import plistlib
import re
import subprocess
from pathlib import Path

import pytest

ROOT = Path(__file__).resolve().parent.parent
GUARD = ROOT / "scripts" / "disk-guard" / "disk-guard.sh"
INSTALL = ROOT / "scripts" / "disk-guard" / "install.sh"
HUGE = str(2**50)


@pytest.fixture()
def run(tmp_path: Path):
    """Run the guard against a fake docker that records its argv."""
    calls = tmp_path / "docker-calls"
    fake = tmp_path / "docker"
    fake.write_text(f'#!/bin/sh\necho "$@" >> "{calls}"\n[ -z "${{FAKE_DOCKER_FAIL:-}}" ]\n')
    fake.chmod(0o755)

    def go(min_kib: str, **extra: str):
        env = {**os.environ, "DISK_GUARD_DOCKER": str(fake), "DISK_GUARD_VOLUME": str(tmp_path),
               "DISK_GUARD_MIN_KIB": min_kib, **extra}
        result = subprocess.run(["sh", str(GUARD)], env=env, capture_output=True, text=True)
        lines = calls.read_text().splitlines() if calls.exists() else []
        return result, lines

    return go


def test_below_the_floor_prunes_build_cache_only_and_logs_both_numbers(run) -> None:
    result, calls = run(HUGE)
    assert result.returncode == 0
    assert calls == ["builder prune -f"]
    assert re.search(r"free before=\d+ KiB after=\d+ KiB", result.stdout)
    assert "below" in result.stdout


def test_above_the_floor_does_nothing(run) -> None:
    result, calls = run("1")
    assert result.returncode == 0
    assert calls == []
    assert result.stdout == "" and result.stderr == ""


def test_a_failed_prune_exits_nonzero_and_says_so(run) -> None:
    result, calls = run(HUGE, FAKE_DOCKER_FAIL="1")
    assert result.returncode == 1
    assert calls == ["builder prune -f"]
    assert "prune failed" in result.stderr


def test_install_writes_a_valid_job_pointing_at_the_copied_script(tmp_path: Path) -> None:
    bin_dir, agents, log = tmp_path / "bin", tmp_path / "agents", tmp_path / "logs" / "dg.log"
    subprocess.run(["sh", str(INSTALL), str(bin_dir), str(agents), str(log)], check=True,
                   capture_output=True)
    script = bin_dir / "disk-guard.sh"
    assert os.access(script, os.X_OK)
    job = plistlib.loads((agents / "dev.coilyco.disk-guard.plist").read_bytes())
    assert job["Label"] == "dev.coilyco.disk-guard"
    assert job["ProgramArguments"] == ["/bin/sh", str(script)]
    assert job["StartInterval"] == 900 and job["RunAtLoad"] is True
    assert job["StandardOutPath"] == str(log)
