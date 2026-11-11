"""The shared DevTools client against a fake DevTools websocket, with no emulator (COI-3526)."""

from __future__ import annotations

import base64
import hashlib
import importlib.util
import json
import socket
import struct
import threading
from pathlib import Path

import pytest

SCRIPT = Path(__file__).resolve().parent.parent / "scripts" / "aterm_devtools.py"
GUID = b"258EAFA5-E914-47DA-95CA-C5AB0DC85B11"


def _load():
    spec = importlib.util.spec_from_file_location("aterm_devtools", SCRIPT)
    assert spec and spec.loader
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


devtools = _load()


def _frame(opcode: int, payload: bytes) -> bytes:
    size = len(payload)
    head = bytes([0x80 | opcode])
    head += bytes([size]) if size < 126 else b"\x7e" + struct.pack(">H", size) if size < 65536 else b"\x7f" + struct.pack(">Q", size)
    return head + payload


def _read_exactly(conn: socket.socket, n: int) -> bytes:
    buf = b""
    while len(buf) < n:
        piece = conn.recv(n - len(buf))
        if not piece:
            raise EOFError
        buf += piece
    return buf


def _read_message(conn: socket.socket) -> dict:
    _, b1 = _read_exactly(conn, 2)
    size = b1 & 0x7F
    size = struct.unpack(">H", _read_exactly(conn, 2))[0] if size == 126 else struct.unpack(">Q", _read_exactly(conn, 8))[0] if size == 127 else size
    key = _read_exactly(conn, 4)
    data = _read_exactly(conn, size)
    return json.loads(bytes(b ^ key[i % 4] for i, b in enumerate(data)))


@pytest.fixture
def server():
    """A DevTools stand-in that answers each call with `handler(message)`, a list of frames to send."""
    listener = socket.socket()
    listener.bind(("127.0.0.1", 0))
    listener.listen(1)
    state: dict = {"handler": lambda message: [{"id": message["id"], "result": {"result": {"value": None}}}], "seen": []}

    def run() -> None:
        conn, _ = listener.accept()
        with conn:
            head = b""
            while b"\r\n\r\n" not in head:
                head += conn.recv(1)
            key = next(line.split(b":")[1].strip() for line in head.split(b"\r\n") if line.lower().startswith(b"sec-websocket-key"))
            accept = base64.b64encode(hashlib.sha1(key + GUID).digest())
            conn.sendall(b"HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: " + accept + b"\r\n\r\n")
            try:
                while True:
                    message = _read_message(conn)
                    state["seen"].append(message)
                    for out in state["handler"](message):
                        conn.sendall(out if isinstance(out, bytes) else _frame(0x1, json.dumps(out).encode()))
            except (EOFError, OSError):
                return

    thread = threading.Thread(target=run, daemon=True)
    thread.start()
    state["url"] = f"ws://127.0.0.1:{listener.getsockname()[1]}/devtools/page/1"
    yield state
    listener.close()


def test_js_returns_the_value_and_asks_for_an_awaited_by_value_evaluate_with_a_gesture(server) -> None:
    server["handler"] = lambda m: [{"id": m["id"], "result": {"result": {"value": 42}}}]
    with devtools.Page(server["url"]) as page:
        assert page.js("6*7") == 42
    params = server["seen"][0]["params"]
    assert server["seen"][0]["method"] == "Runtime.evaluate"
    assert (params["returnByValue"], params["awaitPromise"], params["userGesture"]) == (True, True, True)


def test_a_reply_skips_events_and_pings_and_matches_only_its_own_id(server) -> None:
    def handler(m):
        return [{"method": "Page.loadEventFired", "params": {}}, _frame(0x9, b"hi"), {"id": m["id"] + 7, "result": {}}, {"id": m["id"], "result": {"result": {"value": "mine"}}}]

    server["handler"] = handler
    with devtools.Page(server["url"]) as page:
        assert page.js("x") == "mine"


def test_a_reply_larger_than_a_sixteen_bit_length_arrives_whole(server) -> None:
    big = "x" * 200_000
    server["handler"] = lambda m: [{"id": m["id"], "result": {"result": {"value": big}}}]
    with devtools.Page(server["url"]) as page:
        assert page.js("big") == big


def test_a_request_larger_than_a_sixteen_bit_length_is_sent_whole(server) -> None:
    server["handler"] = lambda m: [{"id": m["id"], "result": {"result": {"value": len(m["params"]["expression"])}}}]
    with devtools.Page(server["url"]) as page:
        assert page.js("y" * 100_000) == 100_000


def test_a_script_that_throws_raises_devtools_error_with_the_expression(server) -> None:
    server["handler"] = lambda m: [{"id": m["id"], "result": {"exceptionDetails": {"text": "Uncaught"}, "result": {}}}]
    with devtools.Page(server["url"]) as page:
        with pytest.raises(devtools.DevToolsError, match=r"script threw: Uncaught in boom\(\)"):
            page.js("boom()")


def test_a_thrown_error_says_what_it_was_when_the_page_describes_it(server) -> None:
    details = {"text": "Uncaught", "exception": {"description": "TypeError: x is not a function\n    at <anonymous>:1:1"}}
    server["handler"] = lambda m: [{"id": m["id"], "result": {"exceptionDetails": details, "result": {}}}]
    with devtools.Page(server["url"]) as page:
        with pytest.raises(devtools.DevToolsError, match=r"script threw: TypeError: x is not a function"):
            page.js("boom()")


def test_a_protocol_error_raises_devtools_error_naming_the_method(server) -> None:
    server["handler"] = lambda m: [{"id": m["id"], "error": {"code": -32601, "message": "no such method"}}]
    with devtools.Page(server["url"]) as page:
        with pytest.raises(devtools.DevToolsError, match="Input.dispatchMouseEvent: .*no such method"):
            page.call("Input.dispatchMouseEvent", type="mousePressed")


def test_a_close_frame_from_the_page_raises_instead_of_waiting(server) -> None:
    server["handler"] = lambda m: [_frame(0x8, b"")]
    with devtools.Page(server["url"]) as page:
        with pytest.raises(devtools.DevToolsError, match="closed the websocket"):
            page.js("x")


def test_a_refused_upgrade_says_what_the_page_answered() -> None:
    listener = socket.socket()
    listener.bind(("127.0.0.1", 0))
    listener.listen(1)

    def run() -> None:
        conn, _ = listener.accept()
        with conn:
            while b"\r\n\r\n" not in conn.recv(4096):
                pass
            conn.sendall(b"HTTP/1.1 403 Forbidden\r\n\r\n")

    threading.Thread(target=run, daemon=True).start()
    with pytest.raises(devtools.DevToolsError, match="refused the upgrade: HTTP/1.1 403"):
        devtools.Page(f"ws://127.0.0.1:{listener.getsockname()[1]}/x")
    listener.close()


def test_app_socket_is_named_by_the_apps_pid_and_never_another_apps() -> None:
    table = "00000000: x @webview_devtools_remote_1572\n00000000: y @webview_devtools_remote_4658\n00000000: z @chrome_devtools_remote"
    assert devtools.listed_sockets(table) == ["webview_devtools_remote_1572", "webview_devtools_remote_4658"]
    assert devtools.app_socket(["4658"], table) == "webview_devtools_remote_4658"
    assert devtools.app_socket(["9999", "4658"], table) == "webview_devtools_remote_4658"
    assert devtools.app_socket(["9999"], table) is None
    assert devtools.app_socket([], table) is None
    assert devtools.app_socket(["1572"], "") is None


def test_choose_page_rewrites_to_the_reachable_port_and_prefers_the_apps_origin() -> None:
    targets = [
        {"type": "page", "url": "about:blank", "webSocketDebuggerUrl": "ws://localhost:9222/devtools/page/A"},
        {"type": "service_worker", "url": "https://tauri.localhost/sw.js", "webSocketDebuggerUrl": "ws://localhost:9222/devtools/page/S"},
        {"type": "page", "url": "https://tauri.localhost/", "webSocketDebuggerUrl": "ws://localhost:9222/devtools/page/B"},
    ]
    assert devtools.choose_page(targets, 19222) == "ws://127.0.0.1:19222/devtools/page/B"
    assert devtools.choose_page(targets[:1], 19222) == "ws://127.0.0.1:19222/devtools/page/A"
    assert devtools.choose_page(targets[:1], 19222, app_only=True) is None
    assert devtools.choose_page([], 19222) is None
