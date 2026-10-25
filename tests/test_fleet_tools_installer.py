"""Behavior coverage for the fleet tools installer shared by CI and the dev container."""
from __future__ import annotations

import hashlib
import io
import os
import re
import subprocess
import tarfile
from pathlib import Path

import pytest


REPO_ROOT = Path(__file__).resolve().parent.parent
INSTALLER = REPO_ROOT / ".devcontainer" / "features" / "agentic-os-tools" / "install.sh"
ARCHES = ("amd64", "arm64")
HEX64 = re.compile(r"^[0-9a-f]{64}$")


def _bash(script: str, *, env: dict[str, str] | None = None) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        ["bash", "-c", script],
        check=False,
        capture_output=True,
        text=True,
        env=os.environ | (env or {}),
    )


def _plan(arch: str, tools: str | None = None) -> list[list[str]]:
    env = {"PLAN": "1", "ARCH": arch}
    if tools is not None:
        env["TOOLS"] = tools
    done = subprocess.run(
        ["bash", str(INSTALLER)], check=True, capture_output=True, text=True, env=os.environ | env
    )
    return [line.split("\t") for line in done.stdout.splitlines()]


@pytest.mark.parametrize("arch", ARCHES)
def test_every_default_tool_has_a_verifiable_pin(arch: str) -> None:
    rows = _plan(arch)

    assert rows
    for tool, version, row_arch, url, sha in rows:
        assert row_arch == arch
        assert version, tool
        if tool == "pre-commit":
            assert url == "pypi"
            continue
        assert url.startswith("https://"), tool
        assert HEX64.match(sha), f"{tool} has no SHA-256 pin"


def test_both_architectures_pin_the_same_tool_versions() -> None:
    by_arch = {arch: {row[0]: row[1] for row in _plan(arch)} for arch in ARCHES}

    assert by_arch["amd64"] == by_arch["arm64"]


def test_an_unsupported_architecture_is_refused() -> None:
    done = _bash(f'PLAN=1 ARCH=riscv64 bash "{INSTALLER}"')

    assert done.returncode != 0
    assert "unsupported architecture" in done.stderr


def test_a_tool_without_a_pin_is_refused() -> None:
    done = _bash(f'PLAN=1 ARCH=arm64 TOOLS=not-a-tool bash "{INSTALLER}"')

    assert done.returncode != 0
    assert "no pin for not-a-tool" in done.stderr


def test_install_is_refused_off_linux(tmp_path: Path) -> None:
    stub = tmp_path / "bin"
    stub.mkdir()
    (stub / "uname").write_text('#!/bin/sh\necho Darwin\n', encoding="utf-8")
    (stub / "uname").chmod(0o755)

    done = _bash(f'bash "{INSTALLER}"', env={"PATH": f"{stub}:{os.environ['PATH']}"})

    assert done.returncode != 0
    assert "only Linux installs are supported" in done.stderr


def test_fetch_verified_keeps_a_matching_download(tmp_path: Path) -> None:
    source = tmp_path / "asset"
    source.write_bytes(b"payload")
    want = hashlib.sha256(b"payload").hexdigest()
    dest = tmp_path / "out"

    done = _bash(f'source "{INSTALLER}"; fetch_verified "file://{source}" {want} "{dest}"')

    assert done.returncode == 0, done.stderr
    assert dest.read_bytes() == b"payload"


def test_fetch_verified_rejects_and_removes_a_mismatched_download(tmp_path: Path) -> None:
    source = tmp_path / "asset"
    source.write_bytes(b"payload")
    dest = tmp_path / "out"

    done = _bash(f'source "{INSTALLER}"; fetch_verified "file://{source}" {"0" * 64} "{dest}"')

    assert done.returncode != 0
    assert "checksum mismatch" in done.stderr
    assert not dest.exists()


def _archive(members: dict[str, bytes]) -> bytes:
    buffer = io.BytesIO()
    with tarfile.open(fileobj=buffer, mode="w:gz") as tar:
        for name, data in members.items():
            info = tarfile.TarInfo(name)
            info.size = len(data)
            tar.addfile(info, io.BytesIO(data))
    return buffer.getvalue()


def test_install_extracts_members_and_honors_the_rename_syntax(tmp_path: Path) -> None:
    payload = _archive({"nested-1.0/real": b"#!/bin/sh\necho 1.0\n", "nested-1.0/other": b"x"})
    asset = tmp_path / "fake.tar.gz"
    asset.write_bytes(payload)
    pin = f"fake|1.0|arm64|file://{asset}|{hashlib.sha256(payload).hexdigest()}|nested-1.0/real>renamed nested-1.0/other"
    install_dir = tmp_path / "bin"

    script = f"""source "{INSTALLER}"
pins() {{ echo '{pin}'; }}
INSTALL_DIR="{install_dir}" ARCH=arm64 install_pinned fake"""
    done = _bash(script, env={"TMPDIR": str(tmp_path)})

    assert done.returncode == 0, done.stderr
    assert (install_dir / "renamed").read_text(encoding="utf-8").startswith("#!/bin/sh")
    assert (install_dir / "other").read_bytes() == b"x"
    assert os.access(install_dir / "renamed", os.X_OK)


def test_install_skips_a_tool_already_at_the_pinned_version(tmp_path: Path) -> None:
    install_dir = tmp_path / "bin"
    install_dir.mkdir()
    (install_dir / "fake").write_text("#!/bin/sh\necho fake 1.0\n", encoding="utf-8")
    (install_dir / "fake").chmod(0o755)
    pin = "fake|1.0|arm64|file:///does/not/exist|" + "0" * 64 + "|fake"

    script = f"""source "{INSTALLER}"
pins() {{ echo '{pin}'; }}
INSTALL_DIR="{install_dir}" ARCH=arm64 install_pinned fake"""
    done = _bash(script, env={"TMPDIR": str(tmp_path)})

    assert done.returncode == 0, done.stderr
    assert "already installed" in done.stdout
