"""Tests for the emulator job's host-side helpers (COI-3485), with no emulator."""

from __future__ import annotations

import argparse
import hashlib
import importlib.util
from pathlib import Path

CI = Path(__file__).resolve().parent.parent / "scripts" / "ci"


def _load(name: str):
    spec = importlib.util.spec_from_file_location(name, CI / f"{name}.py")
    assert spec and spec.loader
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def _rig(tmp_path: Path, written: bytes, typed_path: bool):
    """A socket dir holding one image the daemon 'wrote', and a seat log, as expect-image reads them."""
    folder = tmp_path / "images" / "seat-abc-1"
    folder.mkdir(parents=True)
    file = folder / "x.png"
    file.write_bytes(written)
    log = tmp_path / "seat.log"
    log.write_text(f"{file}\n" if typed_path else "")
    return tmp_path / "d.sock", file, log


def _args(sock: Path, png: Path, log: Path) -> argparse.Namespace:
    return argparse.Namespace(socket=str(sock), seat="seat", png=str(png), log=str(log), timeout=0.2)


def test_expect_image_passes_only_when_the_bytes_match_and_the_path_reached_the_seat(tmp_path: Path) -> None:
    seat = _load("aterm_android_seat")
    seed = tmp_path / "seed.png"
    seed.write_bytes(b"\x89PNG\r\n\x1a\nseed")
    sock, _file, log = _rig(tmp_path, seed.read_bytes(), typed_path=True)
    assert seat.expect_image(_args(sock, seed, log)) == 0


def test_expect_image_fails_for_different_bytes_and_for_a_path_never_typed(tmp_path: Path) -> None:
    seat = _load("aterm_android_seat")
    seed = tmp_path / "seed.png"
    seed.write_bytes(b"\x89PNG\r\n\x1a\nseed")
    sock, _file, log = _rig(tmp_path / "a", b"\x89PNG\r\n\x1a\nother", typed_path=True)
    assert seat.expect_image(_args(sock, seed, log)) == 1
    sock, _file, log = _rig(tmp_path / "b", seed.read_bytes(), typed_path=False)
    assert seat.expect_image(_args(sock, seed, log)) == 1


def test_landed_names_the_file_by_hash_inside_the_seats_own_folder(tmp_path: Path) -> None:
    seat = _load("aterm_android_seat")
    data = b"\x89PNG\r\n\x1a\nzz"
    _sock, file, _log = _rig(tmp_path, data, typed_path=False)
    assert seat.landed(tmp_path, "seat", hashlib.sha256(data).hexdigest()) == file
    assert seat.landed(tmp_path, "other-seat", hashlib.sha256(data).hexdigest()) is None


def test_the_seeded_png_has_the_signature_the_daemon_reads(tmp_path: Path) -> None:
    journey = _load("aterm_android_journey")
    png = tmp_path / "seed.png"
    journey.seed_png(png)
    assert png.read_bytes().startswith(b"\x89PNG\r\n\x1a\n")
