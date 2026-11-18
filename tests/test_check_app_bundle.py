"""Tests for the bundle check behind `just treehouse-mac-build` (COI-3700, COI-3703).

Mach-O headers and plists are built by hand, so the cases need no compiler, lipo or Mac.
"""

from __future__ import annotations

import importlib.util
import os
import plistlib
import struct
import subprocess
import sys
from pathlib import Path

import pytest

SCRIPT = Path(__file__).parents[1] / "scripts" / "check_app_bundle.py"
SPEC = importlib.util.spec_from_file_location("check_app_bundle", SCRIPT)
assert SPEC and SPEC.loader
CHECK = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(CHECK)

X86_64, ARM64, I386 = 0x01000007, 0x0100000C, 0x00000007


def thin(cpu: int) -> bytes:
    """A 64-bit little endian Mach-O header, which is all the check reads."""
    return struct.pack("<II", 0xFEEDFACF, cpu) + bytes(24)


def fat(*cpus: int, wide: bool = False) -> bytes:
    """A fat file whose slices sit past its table, as lipo writes them."""
    body = struct.pack(">II", 0xCAFEBABF if wide else 0xCAFEBABE, len(cpus))
    for index, cpu in enumerate(cpus):
        offset = 4096 * (index + 1)
        if wide:
            body += struct.pack(">IIQQII", cpu, 0, offset, 64, 12, 0)
        else:
            body += struct.pack(">IIIII", cpu, 0, offset, 64, 12)
    return body.ljust(4096 * (len(cpus) + 1), b"\0")


def archs(blob: bytes) -> list[str] | None:
    return CHECK.macho_archs(blob[:1024], len(blob))


def test_a_thin_file_reports_its_one_architecture() -> None:
    assert archs(thin(X86_64)) == ["x86_64"]
    assert archs(thin(ARM64)) == ["arm64"]
    assert archs(thin(I386)) == ["i386"]


def test_a_fat_file_reports_every_slice_in_both_widths() -> None:
    assert archs(fat(X86_64, ARM64)) == ["x86_64", "arm64"]
    assert archs(fat(X86_64, ARM64, wide=True)) == ["x86_64", "arm64"]
    assert archs(fat(X86_64)) == ["x86_64"]


@pytest.mark.parametrize(
    "blob",
    [
        b"",
        b"#!/bin/sh\nexit 0\n",
        b"\x7fELF" + bytes(60),
        # A Java class file starts 0xCAFEBABE too, its version where a fat count is.
        struct.pack(">IHH", 0xCAFEBABE, 0, 61) + bytes(200),
        # A fat table whose slice starts past the end of the file.
        struct.pack(">IIIIIII", 0xCAFEBABE, 1, X86_64, 0, 99999, 64, 12),
        # A fat table naming a CPU nobody ships.
        struct.pack(">IIIIIII", 0xCAFEBABE, 1, 0x12345678, 0, 4096, 64, 12).ljust(8192, b"\0"),
    ],
)
def test_anything_that_is_not_a_mach_o_is_ignored(blob: bytes) -> None:
    assert archs(blob) is None


@pytest.mark.parametrize(
    ("found", "intel_only"),
    [
        (["x86_64"], True),
        (["i386"], True),
        (["x86_64", "i386"], True),
        (["arm64"], False),
        (["x86_64", "arm64"], False),
        (["arm64", "x86_64"], False),
        (["arm"], False),
    ],
)
def test_only_an_intel_slice_with_no_arm64_slice_fails(found: list[str], intel_only: bool) -> None:
    assert CHECK.intel_only(found) is intel_only


def bundle(tmp_path: Path, files: dict[str, bytes]) -> Path:
    app = tmp_path / "Treehouse.app"
    for relative, content in files.items():
        path = app / relative
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(content)
    return app


def with_plist(app: Path, info: dict, binary: bool = False) -> None:
    fmt = plistlib.FMT_BINARY if binary else plistlib.FMT_XML
    path = app / "Contents" / "Info.plist"
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_bytes(plistlib.dumps(info, fmt=fmt))


def run(*args: object) -> subprocess.CompletedProcess:
    return subprocess.run(
        [sys.executable, "-I", str(SCRIPT), *map(str, args)], capture_output=True, text=True, timeout=60
    )


def test_a_bundle_with_an_intel_only_helper_fails_and_names_it(tmp_path: Path) -> None:
    app = bundle(
        tmp_path,
        {
            "Contents/MacOS/Treehouse": thin(ARM64),
            "Contents/Frameworks/helper.dylib": thin(X86_64),
            "Contents/Resources/run.sh": b"#!/bin/sh\n",
        },
    )

    result = run(app)

    assert result.returncode == 1
    assert "INTEL-ONLY x86_64" in result.stdout and "helper.dylib" in result.stdout
    assert "MacOS/Treehouse" not in result.stdout
    assert "2 Mach-O file(s) scanned, 1 problem(s)" in result.stderr


def test_an_intel_only_main_executable_fails(tmp_path: Path) -> None:
    app = bundle(tmp_path, {"Contents/MacOS/Treehouse": thin(X86_64)})

    assert run(app).returncode == 1


def test_a_bundle_of_arm64_and_universal_files_passes(tmp_path: Path) -> None:
    app = bundle(
        tmp_path,
        {
            "Contents/MacOS/Treehouse": thin(ARM64),
            "Contents/Frameworks/universal.dylib": fat(X86_64, ARM64),
            "Contents/Resources/run.sh": b"#!/bin/sh\n",
        },
    )

    result = run(app)

    assert result.returncode == 0
    assert result.stdout == ""
    assert "2 Mach-O file(s) scanned, 0 problem(s)" in result.stderr


def test_all_lists_every_mach_o_and_still_passes(tmp_path: Path) -> None:
    app = bundle(tmp_path, {"Contents/MacOS/Treehouse": thin(ARM64)})

    result = run("--all", app)

    assert result.returncode == 0 and "arm64" in result.stdout


def test_a_symlink_is_not_followed_out_of_the_bundle(tmp_path: Path) -> None:
    outside = tmp_path / "elsewhere"
    outside.write_bytes(thin(X86_64))
    app = bundle(tmp_path, {"Contents/MacOS/Treehouse": thin(ARM64)})
    os.symlink(outside, app / "Contents" / "linked")

    assert run(app).returncode == 0


def test_an_empty_or_missing_directory_scans_nothing_and_passes(tmp_path: Path) -> None:
    assert run(tmp_path / "absent").returncode == 0
    assert "0 Mach-O file(s) scanned" in run(tmp_path).stderr


@pytest.mark.parametrize("binary", [False, True])
def test_a_bundle_whose_plist_declares_lsrequirescarbon_fails_in_either_plist_format(
    tmp_path: Path, binary: bool
) -> None:
    app = bundle(tmp_path, {"Contents/MacOS/Treehouse": thin(ARM64)})
    with_plist(app, {"CFBundleExecutable": "Treehouse", "LSRequiresCarbon": True}, binary)

    result = run(app)

    assert result.returncode == 1
    assert "LEGACY-KEY LSRequiresCarbon" in result.stdout and "Info.plist" in result.stdout


def test_the_key_fails_even_when_it_is_false(tmp_path: Path) -> None:
    app = bundle(tmp_path, {"Contents/MacOS/Treehouse": thin(ARM64)})
    with_plist(app, {"LSRequiresCarbon": False})

    assert run(app).returncode == 1


def test_a_plist_without_the_key_passes(tmp_path: Path) -> None:
    app = bundle(tmp_path, {"Contents/MacOS/Treehouse": thin(ARM64)})
    with_plist(app, {"CFBundleExecutable": "Treehouse", "NSHighResolutionCapable": True})

    result = run(app)

    assert result.returncode == 0 and result.stdout == ""


def test_the_key_and_an_intel_slice_are_both_reported(tmp_path: Path) -> None:
    app = bundle(tmp_path, {"Contents/MacOS/Treehouse": thin(X86_64)})
    with_plist(app, {"LSRequiresCarbon": True})

    result = run(app)

    assert result.returncode == 1
    assert "LEGACY-KEY" in result.stdout and "INTEL-ONLY" in result.stdout
    assert "1 Mach-O file(s) scanned, 2 problem(s)" in result.stderr


@pytest.mark.parametrize("content", [b"", b"not a plist", b"<plist><dict>"])
def test_a_plist_that_cannot_be_read_is_a_problem_not_a_pass(tmp_path: Path, content: bytes) -> None:
    app = bundle(tmp_path, {"Contents/MacOS/Treehouse": thin(ARM64), "Contents/Info.plist": content})

    result = run(app)

    assert result.returncode == 1
    assert "UNREADABLE-PLIST" in result.stdout and "Traceback" not in result.stderr


def test_a_plain_directory_with_a_nested_plist_is_not_read_as_a_bundle(tmp_path: Path) -> None:
    folder = tmp_path / "tools"
    nested = folder / "Some.app"
    with_plist(nested, {"LSRequiresCarbon": True})

    assert run(folder).returncode == 0
