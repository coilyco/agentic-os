"""Tests for the emulator job's host-side helpers (COI-3485), with no emulator."""

from __future__ import annotations

import argparse
import hashlib
import importlib.util
import json
import socket
import threading
import types
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


def _daemon(sock_path: Path, features: list[str]) -> threading.Thread:
    """A stand-in daemon that answers one hello with a welcome listing these features."""
    server = socket.socket(socket.AF_UNIX)
    server.bind(str(sock_path))
    server.listen(1)

    def serve() -> None:
        conn, _ = server.accept()
        with conn, conn.makefile("rb") as reader:
            reader.readline()
            conn.sendall(json.dumps({"type": "welcome", "format": "aterm.daemon.v1", "features": features}).encode() + b"\n")
        server.close()

    thread = threading.Thread(target=serve, daemon=True)
    thread.start()
    return thread


def test_open_facts_say_what_the_daemon_offered_and_whether_an_open_was_asked(tmp_path: Path) -> None:
    journey = _load("aterm_android_journey")
    sock = tmp_path / "d.sock"
    thread = _daemon(sock, ["image-upload", "open-url"])
    (tmp_path / "daemon.log").write_text("x asked to open an address, taken by 1 clients\n")
    me = types.SimpleNamespace(args=argparse.Namespace(socket=str(sock), out=str(tmp_path / "screens")))
    facts = journey.Journey.open_facts(me)
    thread.join(5)
    assert 'welcome features: ["image-upload", "open-url"]' in facts
    assert "daemon open log: x asked to open an address, taken by 1 clients" in facts


def test_open_facts_name_an_open_that_never_reached_the_daemon(tmp_path: Path) -> None:
    journey = _load("aterm_android_journey")
    sock = tmp_path / "d.sock"
    thread = _daemon(sock, [])
    me = types.SimpleNamespace(args=argparse.Namespace(socket=str(sock), out=str(tmp_path)))
    facts = journey.Journey.open_facts(me)
    thread.join(5)
    assert "welcome features: []" in facts
    assert "no open_url was asked of the daemon" in facts


def test_the_stub_seat_is_told_which_socket_its_daemon_listens_on(tmp_path: Path) -> None:
    """The spawn frame carries ATERM_DAEMON_SOCKET, or the seat's `aterm open` misses this daemon (COI-3559)."""
    seat = _load("aterm_android_seat")
    sent: list[dict] = []

    def fake_talk(_sock: str):
        def send(frame: dict, reply: bool = True) -> dict:
            sent.append(frame)
            return {"type": "spawned", "session": frame["session"], "pid": 1}

        return send, {}

    seat.talk = fake_talk
    args = argparse.Namespace(socket=str(tmp_path / "d.sock"), seat="seat", log=str(tmp_path / "seat.log"))
    assert seat.start(args) == 0
    assert sent[0]["env"] == [f"ATERM_DAEMON_SOCKET={tmp_path / 'd.sock'}"]
