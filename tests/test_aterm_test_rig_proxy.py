"""The test rig relay against a real throwaway aterm daemon: origin, frames, refusals."""

from __future__ import annotations

import importlib.util
import json
import os
import shutil
import socket
import struct
import subprocess
import sys
import tempfile
import threading
import time
from pathlib import Path

import pytest

SCRIPT = Path(__file__).resolve().parent.parent / "scripts" / "aterm_test_rig_proxy.py"
APP = "https://tauri.localhost"
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


def request(port: int, line: str = "GET / HTTP/1.1", origin: str = APP, upgrade: bool = True, extra: str = "") -> socket.socket:
    head = f"{line}\r\nHost: 127.0.0.1:{port}\r\n"
    if upgrade:
        head += "Upgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\nSec-WebSocket-Version: 13\r\n"
    head += (f"Origin: {origin}\r\n" if origin else "") + extra + "\r\n"
    conn = socket.create_connection(("127.0.0.1", port), timeout=10)
    conn.sendall(head.encode())
    return conn


def status(conn: socket.socket) -> str:
    return conn.recv(512).decode(errors="replace").split("\r\n", 1)[0]


class Wire:
    """A websocket client over a raw socket, so a test can send frames no library would."""

    def __init__(self, port: int):
        self.conn = request(port)
        data = b""
        while b"\r\n\r\n" not in data:
            data += self.conn.recv(4096)
        head, _, self.buffer = data.partition(b"\r\n\r\n")
        assert head.startswith(b"HTTP/1.1 101"), head

    def frame(self, opcode: int, payload: bytes, fin: bool = True, masked: bool = True) -> bytes:
        key = b"\x01\x02\x03\x04"
        size = len(payload)
        head = bytes([(0x80 if fin else 0) | opcode])
        head += bytes([(0x80 if masked else 0) | size]) if size < 126 else bytes([(0x80 if masked else 0) | 126]) + struct.pack(">H", size)
        body = bytes(b ^ key[i % 4] for i, b in enumerate(payload)) if masked else payload
        return head + (key if masked else b"") + body

    def send(self, obj: dict | str, **kw) -> None:
        text = obj if isinstance(obj, str) else json.dumps(obj)
        self.conn.sendall(self.frame(0x1, text.encode(), **kw))

    def read_exact(self, n: int) -> bytes:
        while len(self.buffer) < n:
            chunk = self.conn.recv(65536)
            if not chunk:
                raise EOFError
            self.buffer += chunk
        out, self.buffer = self.buffer[:n], self.buffer[n:]
        return out

    def receive(self, want: str | None = None, secs: float = 10) -> dict:
        """The next JSON text frame, or one of the wanted type. EOFError when the relay hangs up."""
        end = time.time() + secs
        while time.time() < end:
            b0, b1 = self.read_exact(2)
            size = b1 & 0x7F
            if size == 126:
                (size,) = struct.unpack(">H", self.read_exact(2))
            elif size == 127:
                (size,) = struct.unpack(">Q", self.read_exact(8))
            payload = self.read_exact(size)
            if b0 & 0x0F == 0x1:
                message = json.loads(payload)
                if want is None or message.get("type") == want:
                    return message
            elif b0 & 0x0F == 0x8:
                raise EOFError("closed")
        raise TimeoutError(want)

    def hangs_up(self) -> bool:
        self.conn.settimeout(5)
        try:
            while True:
                if not self.conn.recv(65536):
                    return True
        except (ConnectionError, TimeoutError, socket.timeout):
            return True


def test_only_the_app_origin_is_rewritten() -> None:
    proxy = load()
    head = b"GET / HTTP/1.1\r\nHost: x\r\nOrigin: https://TAURI.localhost\r\nUpgrade: websocket\r\n\r\n"
    out, changed = proxy.rewrite_head(head, APP, "http://127.0.0.1")
    assert changed and b"Origin: http://127.0.0.1\r\n" in out and b"tauri" not in out.lower()
    other = b"GET / HTTP/1.1\r\nOrigin: https://tauri.localhost.example\r\n\r\n"
    assert proxy.rewrite_head(other, APP, "http://127.0.0.1") == (other, False)


def test_a_frame_is_judged_the_way_the_daemon_reads_it() -> None:
    proxy = load()
    deny = ("spawn", "launch")
    assert proxy.judge(b'{"type":"list","id":"a"}', deny) == ("forward", "list")
    assert proxy.judge(b'{"type":"spawn","kind":"terminal"}', deny)[0] == "refuse"
    assert proxy.judge(b'{"Type":"Spawn"}', deny)[0] == "refuse"
    assert proxy.judge(b'{"TYPE":"launch","role":"x"}', deny)[0] == "refuse"
    for bad in (b'{"type":"list","Type":"spawn"}', b"[1,2]", b'{"id":"a"}', b'{"type":5}', b"not json", b"\xff\xfe", b'{"type":"list"} {"type":"spawn"}', b"[]"):
        with pytest.raises(proxy.Drop):
            proxy.judge(bad, deny)


def test_a_non_loopback_address_and_a_deny_list_without_spawn_are_refused() -> None:
    cases = (
        ["--listen", "0.0.0.0:7419", "--upstream", "127.0.0.1:7420"],
        ["--listen", "127.0.0.1:7419", "--upstream", "10.0.0.5:7420"],
        ["--listen", "127.0.0.1:7419", "--upstream", "127.0.0.1:7420", "--deny", "launch"],
    )
    for flag in cases:
        done = subprocess.run([sys.executable, str(SCRIPT), *flag], capture_output=True, text=True, check=False, timeout=20)
        assert done.returncode == 2 and ("loopback" in done.stderr or "spawn" in done.stderr)


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


def attach(port: int) -> Wire:
    wire = Wire(port)
    wire.send({"type": "hello", "format": "aterm.daemon.v1"})
    assert wire.receive("welcome")["format"] == "aterm.daemon.v1"
    return wire


def terminals(wire: Wire) -> int:
    wire.send({"type": "list", "id": "probe"})
    reply = wire.receive("sessions")
    return len(reply.get("terminals") or []) + len(reply.get("sessions") or [])


@needs_aterm
def test_the_app_origin_opens_a_socket_through_the_relay_and_not_without_it(rig) -> None:
    daemon_port, relay_port = rig
    assert "403" in status(request(daemon_port))
    assert "101" in status(request(relay_port))


@needs_aterm
def test_an_attached_client_lists_and_a_fragmented_frame_still_works(rig) -> None:
    _, relay_port = rig
    wire = attach(relay_port)
    assert terminals(wire) == 0
    wire.conn.sendall(wire.frame(0x1, b'{"type":"list",', fin=False) + wire.frame(0x0, b'"id":"split"}'))
    assert wire.receive("sessions")["id"] == "split"


@needs_aterm
def test_a_spawn_frame_is_refused_in_every_form_and_nothing_is_spawned(rig) -> None:
    _, relay_port = rig
    wire = attach(relay_port)
    attempts = [
        {"type": "spawn", "id": "t", "kind": "terminal"},
        {"type": "spawn", "id": "s", "session": "x-y", "role": "x", "identity": "Y", "seat": "claude", "argv": ["/bin/sh", "-c", "touch /tmp/rig-pwned"], "cwd": "/"},
        {"Type": "SPAWN", "id": "case", "kind": "terminal"},
        {"type": "launch", "id": "l", "role": "eng-platform"},
    ]
    for attempt in attempts:
        wire.send(attempt)
        refused = wire.receive("error")
        assert refused["id"] == attempt["id"] and refused["reason"] == "rig_refused", attempt
    fragments = json.dumps({"type": "spawn", "id": "frag", "kind": "terminal"}).encode()
    wire.conn.sendall(wire.frame(0x1, fragments[:9], fin=False) + wire.frame(0x0, fragments[9:]))
    assert wire.receive("error")["id"] == "frag"
    assert terminals(wire) == 0
    assert not Path("/tmp/rig-pwned").exists()


@needs_aterm
def test_a_connection_is_dropped_on_anything_the_relay_cannot_parse(rig) -> None:
    _, relay_port = rig
    cases = {
        "ambiguous type": lambda w: w.send('{"type":"list","Type":"spawn"}'),
        "not json": lambda w: w.send("hello there"),
        "binary": lambda w: w.conn.sendall(w.frame(0x2, b"\x00\x01")),
        "unmasked": lambda w: w.send({"type": "list"}, masked=False),
        "reserved bit": lambda w: w.conn.sendall(bytes([0xC1, 0x80, 1, 2, 3, 4])),
        "lone continuation": lambda w: w.conn.sendall(w.frame(0x0, b"x")),
    }
    for name, act in cases.items():
        wire = attach(relay_port)
        act(wire)
        assert wire.hangs_up(), name


@needs_aterm
def test_only_the_session_socket_and_reach_are_served(rig) -> None:
    _, relay_port = rig
    for line in ("GET /_aterm/cdp/x HTTP/1.1", "GET /_aterm/gateway/x HTTP/1.1", "GET /anything HTTP/1.1"):
        assert "403" in status(request(relay_port, line, upgrade=True)), line
    assert "403" in status(request(relay_port, "GET / HTTP/1.1", upgrade=False))
    assert "403" in status(request(relay_port, "POST / HTTP/1.1", upgrade=True))
    assert "200" in status(request(relay_port, "GET /_aterm/reach HTTP/1.1", upgrade=False))
    assert "403" in status(request(relay_port, "GET / HTTP/1.1", origin="https://evil.example"))


@needs_aterm
def test_other_origins_stay_refused_and_a_request_with_a_body_or_repeated_header_is_dropped(rig) -> None:
    _, relay_port = rig
    for origin in ("https://evil.example", "https://tauri.localhost.example", "http://tauri.localhost"):
        assert "403" in status(request(relay_port, origin=origin)), origin
    body = request(relay_port, extra="Content-Length: 5\r\n")
    assert body.recv(512) == b""
    twice = request(relay_port, extra="Upgrade: websocket\r\n")
    assert twice.recv(512) == b""


@pytest.fixture
def stub_rig(tmp_path):
    """The relay in front of a fake upstream that only records the frame types it is sent, with a stub control file."""
    upstream = socket.socket()
    upstream.bind(("127.0.0.1", 0))
    upstream.listen(4)
    seen: list[str] = []

    def serve() -> None:
        while True:
            try:
                conn, _ = upstream.accept()
            except OSError:
                return
            threading.Thread(target=_record, args=(conn, seen), daemon=True).start()

    threading.Thread(target=serve, daemon=True).start()
    control = tmp_path / "refuse"
    relay_port = free_port()
    relay = subprocess.Popen(
        [sys.executable, str(SCRIPT), "--listen", f"127.0.0.1:{relay_port}", "--upstream", f"127.0.0.1:{upstream.getsockname()[1]}", "--refuse-uploads-file", str(control)],
        stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
    )
    for _ in range(50):
        try:
            socket.create_connection(("127.0.0.1", relay_port), timeout=1).close()
            break
        except OSError:
            time.sleep(0.1)
    yield relay_port, control, seen
    relay.terminate()
    relay.wait(timeout=10)
    upstream.close()


def _record(conn: socket.socket, seen: list[str]) -> None:
    with conn:
        head = b""
        while b"\r\n\r\n" not in head:
            head += conn.recv(1)
        conn.sendall(b"HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n")
        try:
            while True:
                _, b1 = conn.recv(2)
                size = b1 & 0x7F
                size = struct.unpack(">H", conn.recv(2))[0] if size == 126 else size
                key = conn.recv(4)
                data = b""
                while len(data) < size:
                    data += conn.recv(size - len(data))
                frame = json.loads(bytes(b ^ key[i % 4] for i, b in enumerate(data)))
                seen.append(next(v for k, v in frame.items() if k.casefold() == "type"))
        except (ValueError, OSError):
            return


def _wait_for(seen: list[str], expected: list[str]) -> None:
    for _ in range(50):
        if seen == expected:
            return
        time.sleep(0.1)
    assert seen == expected


def test_an_upload_is_answered_with_the_daemons_refusal_and_never_forwarded_while_the_file_names_one(stub_rig) -> None:
    relay_port, control, seen = stub_rig
    wire = Wire(relay_port)
    for reason, words in (("too_large", "3 MB"), ("not_an_image", "not a PNG")):
        control.write_text(reason + "\n")
        wire.send({"type": "image_upload", "id": f"u-{reason}", "session": "s", "data": "AAAA"})
        refused = wire.receive("error")
        assert (refused["id"], refused["reason"], refused["code"]) == (f"u-{reason}", reason, 1)
        assert words in refused["error"]
    wire.send({"type": "list", "id": "after"})
    _wait_for(seen, ["list"])


def test_an_upload_is_forwarded_when_the_file_is_absent_empty_or_names_nothing_known(stub_rig) -> None:
    relay_port, control, seen = stub_rig
    wire = Wire(relay_port)
    wire.send({"type": "image_upload", "id": "a", "session": "s", "data": "AAAA"})
    control.write_text("")
    wire.send({"type": "image_upload", "id": "b", "session": "s", "data": "AAAA"})
    control.write_text("rig_refused")
    wire.send({"Type": "IMAGE_UPLOAD", "id": "c", "session": "s", "data": "AAAA"})
    _wait_for(seen, ["image_upload", "image_upload", "IMAGE_UPLOAD"])


def test_the_stub_changes_nothing_for_other_frames_and_spawn_is_still_refused(stub_rig) -> None:
    relay_port, control, seen = stub_rig
    control.write_text("too_large")
    wire = Wire(relay_port)
    wire.send({"type": "spawn", "id": "x", "kind": "terminal"})
    assert wire.receive("error")["reason"] == "rig_refused"
    wire.send({"type": "attach", "id": "y", "session": "s"})
    _wait_for(seen, ["attach"])


def test_upload_refusal_reads_only_a_known_reason() -> None:
    proxy = load()
    assert proxy.upload_refusal(None) is None
    assert proxy.upload_refusal("/nonexistent/control") is None
    assert set(proxy.UPLOAD_REFUSALS) == {"not_an_image", "too_large", "too_many", "session_unknown"}
