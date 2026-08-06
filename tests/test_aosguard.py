"""Static contract for the independent aosguard specgen project."""

from __future__ import annotations

import gzip
import json
import os
import re
import subprocess
from pathlib import Path

import pytest


ROOT = Path(__file__).resolve().parents[1]
PROJECT = ROOT / ".specgen" / "guardfiles"
SOURCE = PROJECT / "aosguard"


@pytest.fixture(scope="module")
def aosguard_binary(tmp_path_factory: pytest.TempPathFactory) -> Path:
    suffix = ".exe" if os.name == "nt" else ""
    binary = tmp_path_factory.mktemp("aosguard") / f"aosguard{suffix}"
    subprocess.run(
        [
            "specgen",
            "--project-root",
            str(PROJECT),
            "build",
            "--out",
            str(binary),
        ],
        cwd=ROOT,
        check=True,
        capture_output=True,
        text=True,
    )
    return binary


def _fake_aws(tmp_path: Path) -> tuple[Path, Path]:
    fake_bin = tmp_path / "bin"
    fake_bin.mkdir(exist_ok=True)
    log = tmp_path / "aws-argv"
    if os.name == "nt":
        script = fake_bin / "aws.cmd"
        body = """@echo off
echo %*>>"%AWS_FIXTURE_LOG%"
if "%AWS_FIXTURE_FAIL%"=="1" (
    echo fixture failure 1>&2
    exit /b 23
)
echo {"Version":1,"Tier":"Standard"}
"""
    else:
        script = fake_bin / "aws"
        body = """#!/bin/sh
printf '%s\\n' "$@" >> "$AWS_FIXTURE_LOG"
if [ "${AWS_FIXTURE_FAIL:-}" = 1 ]; then
    echo "fixture failure" >&2
    exit 23
fi
printf '%s\\n' '{"Version":1,"Tier":"Standard"}'
"""
    script.write_text(body, encoding="utf-8")
    script.chmod(0o755)
    return fake_bin, log


def _run_aosguard_aws(
    aosguard_binary: Path,
    tmp_path: Path,
    *args: str,
    fail_upstream: bool = False,
) -> subprocess.CompletedProcess[str]:
    fake_bin, _ = _fake_aws(tmp_path)
    env = os.environ.copy()
    env.update(
        {
            "AWS_FIXTURE_FAIL": "1" if fail_upstream else "0",
            "AWS_FIXTURE_LOG": str(tmp_path / "aws-argv"),
            "CLIGUARD_NO_SANDBOX": "1",
            "HOME": str(tmp_path / "home"),
            "PATH": f"{fake_bin}{os.pathsep}{env['PATH']}",
        }
    )
    return subprocess.run(
        [str(aosguard_binary), "ops", "aws", "ssm", *args],
        cwd=ROOT,
        env=env,
        capture_output=True,
        text=True,
        check=False,
    )


def test_aosguard_has_one_static_binary_group() -> None:
    members = sorted(SOURCE.rglob("*.kdl"))
    assert members
    assert not list(PROJECT.glob("*.kdl"))

    wraps: set[str] = set()
    for member in members:
        text = member.read_text(encoding="utf-8")
        wraps.update(re.findall(r"(?m)^wrap\s+(\S+)", text))
        assert "wrap aos-agent " not in text
        assert "wrap aos-ward " not in text
        assert 'argv ".ward/' not in text
        assert "doc-link" not in text

    assert wraps == {"aosguard"}


def test_aosguard_actions_use_packaged_python_modules() -> None:
    text = (SOURCE / "actions.kdl").read_text(encoding="utf-8")

    assert "exec python3" in text
    assert ".specgen/guardfiles/aosguard/scripts/" not in text
    for module in (
        "forgejo_actions_list",
        "forgejo_actions_logs",
        "forgejo_actions_rerun",
    ):
        assert f'"agentic_os.{module}"' in text
        assert (ROOT / "agentic_os" / f"{module}.py").is_file()


def test_aosguard_forgejo_storage_measurement_is_sealed() -> None:
    text = (SOURCE / "forgejo-storage.kdl").read_text(encoding="utf-8")

    assert "wrap aosguard ops forgejo-storage" in text
    assert 'argv "-I"' in text
    assert 'embed "forgejo_storage_measure.py"' in text
    assert "agentic_os.forgejo_storage_measure" not in text
    assert "sealed" in text
    assert (SOURCE / "forgejo_storage_measure.py").is_file()


def test_aosguard_vendored_forgejo_contract_is_encoded_json() -> None:
    vendored = sorted(SOURCE.glob("*.swagger.v1.json.gz"))
    assert vendored
    assert all(json.loads(gzip.decompress(path.read_bytes())) for path in vendored)

    contract = json.loads(
        gzip.decompress((SOURCE / "forgejo.swagger.v1.json.gz").read_bytes())
    )
    assert contract["info"]["version"].startswith("16.")
    assert "/repos/{owner}/{repo}/actions/jobs/{job_id}/logs" in contract["paths"]
    assert "/repos/{owner}/{repo}/actions/runs/{run_id}/logs" in contract["paths"]


def test_aosguard_forgejo_lock_is_encoded_json() -> None:
    encoded = sorted(SOURCE.glob("*.swagger.lock.json.gz"))
    assert encoded
    assert not list(SOURCE.glob("*.swagger.lock.json"))
    assert all(json.loads(gzip.decompress(path.read_bytes())) for path in encoded)


def test_aosguard_dependency_lock_is_committed() -> None:
    lock = json.loads((PROJECT / "specverb.lock").read_text(encoding="utf-8"))
    cli_guard = lock["cliGuard"]
    assert re.fullmatch(r"v\d+\.\d+\.\d+", cli_guard)
    assert f"cli-guard {cli_guard}" in "\n".join(lock["goMod"])
    assert any(f"cli-guard {cli_guard} " in line for line in lock["goSum"])
    assert not list(SOURCE.glob("*.md"))


def test_aosguard_builds_a_native_skill_out_of_band() -> None:
    lock_script = (ROOT / "scripts" / "aosguard-lock.sh").read_text(
        encoding="utf-8"
    )
    ward = (ROOT / ".ward" / "ward.yaml").read_text(encoding="utf-8")
    dockerfile = (
        ROOT / "docker" / "dev-base" / "full" / "Dockerfile"
    ).read_text(encoding="utf-8")

    assert "--skills-out dist/skills" in lock_script
    assert "--skills-out dist/skills" in ward
    assert "--skills-out /opt/agentic-os/aosguard-skill" in dockerfile


def test_native_release_wrapper_embeds_the_actions_bridge() -> None:
    wrapper = ROOT / "aosguard-release" / "main.go"
    text = wrapper.read_text(encoding="utf-8")

    assert "//go:embed payload/aosguard payload/agentic_os/*" in text
    assert "PYTHONPATH=" in text
    build = (ROOT / "scripts" / "aos-release-build.sh").read_text(encoding="utf-8")
    generate = build.index('"$specgen" --project-root "$project" gen')
    decode = build.index("find \"$project\" -type f -name '*.lock.json.gz'")
    compile_binary = build.index('go build -trimpath -ldflags "-s -w -X main.Version=')
    assert generate < decode < compile_binary
    for module in (
        "forgejo_actions_list.py",
        "forgejo_actions_logs.py",
        "forgejo_actions_rerun.py",
        "forgejo_actions_web.py",
    ):
        assert f'"$repo_root/agentic_os/{module}"' in build
    assert '"$repo_root/agentic_os/forgejo_storage_measure.py"' not in build


def test_aosguard_forgejo_storage_measurement_mounts_exec_group(
    aosguard_binary: Path,
) -> None:
    result = subprocess.run(
        [str(aosguard_binary), "ops", "forgejo-storage", "--help"],
        check=True,
        capture_output=True,
        text=True,
    )

    assert "measure" in result.stdout
    assert "bounded read-only Forgejo" in result.stdout


def test_repo_topic_replace_all_dry_run_builds_put_body(
    aosguard_binary: Path,
) -> None:
    result = subprocess.run(
        [
            str(aosguard_binary),
            "ops",
            "forgejo-admin",
            "repo-topic",
            "replace-all",
            "coilyco-example",
            "sample",
            "--topics",
            "release-ready",
            "--topics",
            "documentation",
            "--dry-run",
        ],
        check=True,
        capture_output=True,
        text=True,
    )

    assert "method: PUT" in result.stdout
    assert "/repos/coilyco-example/sample/topics" in result.stdout
    assert "release-ready" in result.stdout
    assert "documentation" in result.stdout
    assert "Authorization: token <redacted>" in result.stdout


def test_repo_description_edit_dry_run_builds_bounded_patch(
    aosguard_binary: Path,
) -> None:
    result = subprocess.run(
        [
            str(aosguard_binary),
            "ops",
            "forgejo-admin",
            "repo",
            "edit",
            "coilyco-example",
            "sample",
            "--description",
            "Bounded repository description.",
            "--dry-run",
        ],
        check=True,
        capture_output=True,
        text=True,
    )

    assert "method: PATCH" in result.stdout
    assert "/repos/coilyco-example/sample" in result.stdout
    assert "description: Bounded repository description." in result.stdout
    assert "private:" not in result.stdout
    assert "Authorization: token <redacted>" in result.stdout


def test_repo_description_edit_rejects_out_of_scope_owner(
    aosguard_binary: Path,
) -> None:
    result = subprocess.run(
        [
            str(aosguard_binary),
            "ops",
            "forgejo-admin",
            "repo",
            "edit",
            "outside-example",
            "sample",
            "--description",
            "Blocked repository description.",
            "--dry-run",
        ],
        check=False,
        capture_output=True,
        text=True,
    )

    assert result.returncode != 0
    assert 'owner="outside-example" is outside the allowed scope' in result.stderr


def test_repo_topic_replace_all_rejects_out_of_scope_owner(
    aosguard_binary: Path,
) -> None:
    result = subprocess.run(
        [
            str(aosguard_binary),
            "ops",
            "forgejo-admin",
            "repo-topic",
            "replace-all",
            "outside-example",
            "sample",
            "--topics",
            "documentation",
            "--dry-run",
        ],
        check=False,
        capture_output=True,
        text=True,
    )

    assert result.returncode != 0
    assert 'owner="outside-example" is outside the allowed scope' in result.stderr


@pytest.mark.parametrize(
    ("resource", "identifier", "extra", "expected_path"),
    [
        (
            "action-job",
            "9134",
            ("--attempt", "2"),
            "/repos/coilyco-example/sample/actions/jobs/9134/logs?attempt=2",
        ),
        (
            "action-run",
            "6281",
            (),
            "/repos/coilyco-example/sample/actions/runs/6281/logs",
        ),
    ],
)
def test_forgejo_v16_log_leaves_resolve_official_routes(
    aosguard_binary: Path,
    resource: str,
    identifier: str,
    extra: tuple[str, ...],
    expected_path: str,
) -> None:
    result = subprocess.run(
        [
            str(aosguard_binary),
            "ops",
            "forgejo",
            resource,
            "logs",
            "coilyco-example",
            "sample",
            identifier,
            *extra,
            "--dry-run",
        ],
        check=True,
        capture_output=True,
        text=True,
    )

    assert "method: GET" in result.stdout
    assert expected_path in result.stdout
    assert "Authorization: token <redacted>" in result.stdout


def test_aws_put_parameter_creates_and_overwrites_without_value_disclosure(
    aosguard_binary: Path,
    tmp_path: Path,
) -> None:
    value_file = tmp_path / "value"
    secret = "fixture-value-must-not-print"
    value_file.write_text(secret, encoding="utf-8")
    value_uri = value_file.as_uri()

    created = _run_aosguard_aws(
        aosguard_binary,
        tmp_path,
        "put-parameter",
        "--name",
        "/fixture/example",
        "--value",
        value_uri,
        "--type",
        "String",
    )
    overwritten = _run_aosguard_aws(
        aosguard_binary,
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
def test_aws_put_parameter_requires_a_name_and_file_value_source(
    aosguard_binary: Path,
    tmp_path: Path,
    args: tuple[str, ...],
) -> None:
    proc = _run_aosguard_aws(aosguard_binary, tmp_path, *args)

    assert proc.returncode != 0
    assert "denied:" in proc.stderr
    assert "fail-closed" in proc.stderr
    assert "inline-value" not in proc.stderr
    assert not (tmp_path / "aws-argv").exists()


def test_aws_put_parameter_preserves_failure_without_disclosing_value(
    aosguard_binary: Path,
    tmp_path: Path,
) -> None:
    value_file = tmp_path / "value"
    secret = "fixture-value-must-not-print"
    value_file.write_text(secret, encoding="utf-8")

    proc = _run_aosguard_aws(
        aosguard_binary,
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
