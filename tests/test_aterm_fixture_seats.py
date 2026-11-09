"""The fixture seat script against a throwaway aterm daemon, and its refusal of the real one."""

from __future__ import annotations

import json
import os
import shutil
import socket
import subprocess
import sys
import tempfile
import time
from pathlib import Path

import pytest

SCRIPT = Path(__file__).resolve().parent.parent / "scripts" / "aterm_fixture_seats.py"
needs_aterm = pytest.mark.skipif(shutil.which("aterm") is None, reason="needs the aterm binary on PATH")


def script(*args: str, env: dict[str, str] | None = None) -> subprocess.CompletedProcess[str]:
    return subprocess.run([sys.executable, str(SCRIPT), *args], env=env or os.environ, capture_output=True, text=True, check=False)


def test_the_default_daemon_socket_is_refused() -> None:
    env = {k: v for k, v in os.environ.items() if k != "ATERM_DAEMON_SOCKET"}
    done = script("up", "--socket", f"/tmp/aterm-{os.getuid()}/daemon.sock", env=env)
    assert done.returncode == 2 and "holds real seats" in done.stderr


def test_a_daemon_must_be_named() -> None:
    env = {k: v for k, v in os.environ.items() if k != "ATERM_DAEMON_SOCKET"}
    done = script("up", env=env)
    assert done.returncode == 2 and "--socket" in done.stderr


@pytest.fixture
def daemon():
    # A unix socket path is limited to about 100 bytes, so pytest's tmp_path is too long.
    root = Path(tempfile.mkdtemp(prefix="fx", dir="/tmp"))
    root.chmod(0o700)
    path = root / "d.sock"
    env = os.environ | {"ATERM_DAEMON_SOCKET": str(path), "ATERM_DAEMON_WS": "127.0.0.1:0"}
    proc = subprocess.Popen(["aterm", "daemon", "--tailnet-port", ""], env=env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    for _ in range(50):
        if path.exists():
            break
        time.sleep(0.2)
    yield str(path), env
    proc.terminate()
    proc.wait(timeout=10)
    shutil.rmtree(root, ignore_errors=True)


@needs_aterm
def test_up_raises_a_stamped_card_and_down_removes_the_seats(daemon) -> None:
    path, env = daemon
    assert script("up", "--socket", path, env=env).stdout.count("spawned") == 2
    assert script("up", "--socket", path, env=env).stdout.count("already up") == 2
    watcher = socket.socket(socket.AF_UNIX)
    watcher.settimeout(20)
    watcher.connect(path)
    wire = watcher.makefile("rw", buffering=1)
    wire.write(json.dumps({"type": "hello", "format": "aterm.daemon.v1", "version": "test"}) + "\n")
    wire.readline()
    wire.write(json.dumps({"type": "subscribe", "id": "s", "channel": "sessions"}) + "\n")
    ask = None
    while ask is None:
        frame = json.loads(wire.readline())
        ask = frame.get("ask") if frame.get("type") == "ask" else None
    assert ask["from"] == "fixture Cards" and [o["label"] for o in ask["options"]] == ["red", "green", "blue"]
    assert "closed" in script("down", "--socket", path, env=env).stdout
    assert script("status", "--socket", path, env=env).stdout.count("not up") == 2
