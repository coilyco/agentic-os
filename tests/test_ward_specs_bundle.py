from __future__ import annotations

import os
import subprocess
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


def _run_ward_doctor() -> subprocess.CompletedProcess[str]:
    env = os.environ.copy()
    return subprocess.run(
        ["ward", "doctor"],
        cwd=ROOT,
        check=True,
        capture_output=True,
        text=True,
        env=env,
    )


def test_yaml_configuration_is_accepted_by_ward_doctor() -> None:
    result = _run_ward_doctor()
    assert "ward doctor: all checks passed" in result.stdout


def test_ward_directory_has_no_retired_kdl_configuration() -> None:
    assert not list((ROOT / ".ward").glob("*.kdl"))
