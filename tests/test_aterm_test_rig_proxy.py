"""The test rig relay against a real throwaway aterm daemon, and its loopback-only rule."""

from __future__ import annotations

import importlib.util
import os
import shutil
import socket
import subprocess
import sys
import tempfile
import time
from pathlib import Path

import pytest

SCRIPT = Path(__file__).resolve().parent.parent / "scripts" / "aterm_test_rig_proxy.py"
needs_aterm = pytest.mark.skipif(shutil.which("aterm") is None, reason="needs the aterm binary on PATH")


def load():
    spec = importlib.util.spec_from_file_location("rig_proxy", SCRIPT)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def free_port() -> int:
    with socket.socket() as probe:
        probe.bind(("127.0.0.1", 0))
        return probe.getsockname()[1]


def upgrade_status(port: int, origin: str) -> str:
    """The status line of a websocket upgrade sent with the given Origin."""
    head = (
        f"GET / HTTP/1.1\r\nHost: 127.0.0.1:{port}\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n"
        "Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\nSec-WebSocket-Version: 13\r\n"
        f"Origin: {origin}\r\n\r\n"
    )
    with socket.create_connection(("127.0.0.1", port), timeout=10) as conn:
        conn.sendall(head.encode())
        return conn.recv(512).decode(errors="replace").split("\r\n", 1)[0]


def test_only_the_app_origin_is_rewritten() -> None:
    proxy = load()
    head = b"GET / HTTP/1.1\r\nHost: x\r\nOrigin: https://TAURI.localhost\r\nUpgrade: websocket\r\n\r\n"
    out, changed = proxy.rewrite_head(head, "https://tauri.localhost", "http://127.0.0.1")
    assert changed and b"Origin: http://127.0.0.1\r\n" in out and b"tauri" not in out.lower()
    other = b"GET / HTTP/1.1\r\nOrigin: https://tauri.localhost.example\r\n\r\n"
    assert proxy.rewrite_head(other, "https://tauri.localhost", "http://127.0.0.1") == (other, False)


def test_a_non_loopback_address_is_refused() -> None:
    for flag in (["--listen", "0.0.0.0:7419", "--upstream", "127.0.0.1:7420"], ["--listen", "127.0.0.1:7419", "--upstream", "10.0.0.5:7420"]):
        done = subprocess.run([sys.executable, str(SCRIPT), *flag], capture_output=True, text=True, check=False, timeout=20)
        assert done.returncode == 2 and "loopback" in done.stderr


@pytest.fixture
def rig():
    # A unix socket path is limited to about 100 bytes, so pytest's tmp_path is too long.
    root = Path(tempfile.mkdtemp(prefix="rp", dir="/tmp"))
    root.chmod(0o700)
    daemon_port, relay_port = free_port(), free_port()
    env = os.environ | {"ATERM_DAEMON_SOCKET": str(root / "d.sock"), "ATERM_DAEMON_WS": f"127.0.0.1:{daemon_port}"}
    daemon = subprocess.Popen(["aterm", "daemon", "--tailnet-port", ""], env=env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    relay = subprocess.Popen(
        [sys.executable, str(SCRIPT), "--listen", f"127.0.0.1:{relay_port}", "--upstream", f"127.0.0.1:{daemon_port}"],
        stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
    )
    for _ in range(50):
        try:
            socket.create_connection(("127.0.0.1", daemon_port), timeout=1).close()
            socket.create_connection(("127.0.0.1", relay_port), timeout=1).close()
            break
        except OSError:
            time.sleep(0.2)
    yield daemon_port, relay_port
    for proc in (relay, daemon):
        proc.terminate()
        proc.wait(timeout=10)
    shutil.rmtree(root, ignore_errors=True)


@needs_aterm
def test_the_app_origin_opens_a_socket_through_the_relay_and_not_without_it(rig) -> None:
    daemon_port, relay_port = rig
    app = "https://tauri.localhost"
    assert "403" in upgrade_status(daemon_port, app)
    assert "101" in upgrade_status(relay_port, app)


@needs_aterm
def test_the_relay_does_not_widen_what_the_daemon_serves(rig) -> None:
    _, relay_port = rig
    for origin in ("https://evil.example", "https://tauri.localhost.example", "http://tauri.localhost"):
        assert "403" in upgrade_status(relay_port, origin), origin
