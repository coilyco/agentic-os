from __future__ import annotations

import os
import shutil
import subprocess
from pathlib import Path

import pytest


REPO_ROOT = Path(__file__).resolve().parent.parent


def _fake_aws(tmp_path: Path) -> tuple[Path, Path]:
    fake_bin = tmp_path / "bin"
    fake_bin.mkdir(exist_ok=True)
    log = tmp_path / "aws-argv"
    script = fake_bin / "aws"
    script.write_text(
        """#!/bin/sh
printf '%s\\n' "$@" >> "$AWS_FIXTURE_LOG"
if [ "${AWS_FIXTURE_FAIL:-}" = 1 ]; then
    echo "fixture failure" >&2
    exit 23
fi
printf '%s\\n' '{"Version":1,"Tier":"Standard"}'
""",
        encoding="utf-8",
    )
    script.chmod(0o755)
    return fake_bin, log


def _run_ward(
    tmp_path: Path,
    *args: str,
    bundle: Path | None = None,
    fail_upstream: bool = False,
) -> subprocess.CompletedProcess[str]:
    ward = shutil.which("ward")
    if ward is None:
        pytest.skip("ward binary is required for bundle integration coverage")
    fake_bin, log = _fake_aws(tmp_path)
    env = os.environ.copy()
    env.update(
        {
            "AWS_FIXTURE_FAIL": "1" if fail_upstream else "0",
            "AWS_FIXTURE_LOG": str(log),
            "CLIGUARD_NO_SANDBOX": "1",
            "HOME": str(tmp_path / "home"),
            "PATH": f"{fake_bin}{os.pathsep}{env['PATH']}",
            "WARD_CONFIG_REF": f"file://{bundle or REPO_ROOT / '.ward'}",
        }
    )
    return subprocess.run(
        [ward, "ops", "aws", "ssm", *args],
        cwd=REPO_ROOT,
        env=env,
        capture_output=True,
        text=True,
        check=False,
    )


def test_put_parameter_creates_and_overwrites_without_value_disclosure(tmp_path: Path) -> None:
    value_file = tmp_path / "value"
    secret = "fixture-value-must-not-print"
    value_file.write_text(secret, encoding="utf-8")
    value_uri = value_file.as_uri()

    created = _run_ward(
        tmp_path,
        "put-parameter",
        "--name",
        "/fixture/example",
        "--value",
        value_uri,
        "--type",
        "String",
    )
    overwritten = _run_ward(
        tmp_path,
        "put-parameter",
        "--name",
        "/fixture/example",
        "--value",
        value_uri,
        "--type",
        "SecureString",
        "--overwrite",
    )

    assert created.returncode == 0, created.stderr
    assert overwritten.returncode == 0, overwritten.stderr
    assert secret not in created.stdout + created.stderr
    assert secret not in overwritten.stdout + overwritten.stderr
    assert "--overwrite" in (tmp_path / "aws-argv").read_text(encoding="utf-8")


@pytest.mark.parametrize(
    "args",
    [
        ("put-parameter", "--value", "file:///tmp/value", "--type", "String"),
        (
            "put-parameter",
            "--name",
            "/fixture/example",
            "--value",
            "inline-value",
            "--type",
            "String",
        ),
    ],
)
def test_put_parameter_requires_a_name_and_file_value_source(
    tmp_path: Path, args: tuple[str, ...]
) -> None:
    proc = _run_ward(tmp_path, *args)

    assert proc.returncode != 0
    assert "denied:" in proc.stderr
    assert "fail-closed" in proc.stderr
    assert "inline-value" not in proc.stderr
    assert not (tmp_path / "aws-argv").exists()


def test_engineer_role_guardfile_withholds_put_parameter(tmp_path: Path) -> None:
    proc = _run_ward(
        tmp_path,
        "put-parameter",
        "--help",
        bundle=REPO_ROOT / ".ward" / "guardfile.aws.role-engineer.kdl",
    )

    assert proc.returncode != 0
    assert "No help topic" in proc.stderr
    assert not (tmp_path / "aws-argv").exists()


def test_put_parameter_preserves_failure_without_disclosing_value(tmp_path: Path) -> None:
    value_file = tmp_path / "value"
    secret = "fixture-value-must-not-print"
    value_file.write_text(secret, encoding="utf-8")

    proc = _run_ward(
        tmp_path,
        "put-parameter",
        "--name",
        "/fixture/example",
        "--value",
        value_file.as_uri(),
        "--type",
        "SecureString",
        fail_upstream=True,
    )

    assert proc.returncode != 0
    assert "fixture failure" in proc.stderr
    assert secret not in proc.stdout + proc.stderr
