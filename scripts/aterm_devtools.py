"""The one DevTools client for the Treehouse app's WebView, shared by its two drivers.

`scripts/aterm_android_emulator.py` (a seat's lock-aware verb) and
`scripts/ci/aterm_android_journey.py` (the unattended CI job) both reach the page
through this, so a DevTools quirk is fixed once (COI-3526). Reference:
tooling-aterm-client references/android-emulator.md. Standard library only, because the
CI job runs `python3 -I` with no packages.
"""

from __future__ import annotations

import base64
import json
import os
import re
import socket
import struct
import time


def listed_sockets(unix_table: str) -> list[str]:
    """Every WebView DevTools socket in /proc/net/unix, whichever app owns it."""
    return re.findall(r"@(webview_devtools_remote_\d+)", unix_table)


def app_socket(pids: list[str], unix_table: str) -> str | None:
    """The DevTools socket of one of the app's own processes, never another app's (COI-3600).

    A Google-APIs image runs other WebView hosts, such as the Google app, whose sockets can
    sort first, so the socket is named by the app's pid and not taken from the list.
    """
    listed = set(listed_sockets(unix_table))
    return next((f"webview_devtools_remote_{pid}" for pid in pids if f"webview_devtools_remote_{pid}" in listed), None)


class DevToolsError(Exception):
    """A DevTools step that did not hold. Its text names what was expected and what was seen."""


def choose_page(targets: list[dict], port: int, *, app_only: bool = False) -> str | None:
    """The debugger URL of the app's page, rewritten to loopback `port`, or None.

    DevTools reports the socket's own host (`localhost`), which is not the end the
    caller can reach when the port is forwarded or tunnelled. `app_only` waits for
    the app's own origin instead of taking the first page, which a cold start
    needs because the WebView can list a blank page first.
    """
    pages = [t for t in targets if t.get("type") == "page"]
    ours = [t for t in pages if "tauri.localhost" in t.get("url", "")]
    chosen = ours or ([] if app_only else pages)
    if not chosen:
        return None
    return re.sub(r"ws://[^/]+", f"ws://127.0.0.1:{port}", chosen[0]["webSocketDebuggerUrl"])


class Page:
    """A DevTools session on the app's page: evaluate script and send trusted mouse events."""

    def __init__(self, url: str, timeout: float = 20) -> None:
        found = re.match(r"ws://([^:/]+):(\d+)(/.*)", url)
        if not found:
            raise DevToolsError(f"not a DevTools websocket address: {url}")
        host, port, path = found.groups()
        self.sock = socket.create_connection((host, int(port)), timeout=timeout)
        key = base64.b64encode(os.urandom(16)).decode()
        self.sock.sendall(f"GET {path} HTTP/1.1\r\nHost: {host}:{port}\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: {key}\r\nSec-WebSocket-Version: 13\r\n\r\n".encode())
        head = b""
        while b"\r\n\r\n" not in head:
            piece = self.sock.recv(1)
            if not piece:
                raise DevToolsError("DevTools closed the socket during the upgrade")
            head += piece
        if b" 101 " not in head.split(b"\r\n")[0]:
            raise DevToolsError(f"DevTools refused the upgrade: {head.decode(errors='replace')[:120]}")
        self.seq = 0

    def close(self) -> None:
        self.sock.close()

    def __enter__(self) -> Page:
        return self

    def __exit__(self, *_: object) -> None:
        self.close()

    def _read(self, n: int) -> bytes:
        buf = b""
        while len(buf) < n:
            piece = self.sock.recv(n - len(buf))
            if not piece:
                raise DevToolsError("DevTools closed the socket")
            buf += piece
        return buf

    def _frame(self) -> str:
        data = b""
        while True:
            b0, b1 = self._read(2)
            size = b1 & 0x7F
            size = struct.unpack(">H", self._read(2))[0] if size == 126 else struct.unpack(">Q", self._read(8))[0] if size == 127 else size
            payload = self._read(size)
            if b0 & 0x0F == 0x8:
                raise DevToolsError("DevTools closed the websocket")
            if b0 & 0x0F in (0x9, 0xA):
                continue
            data += payload
            if b0 & 0x80:
                return data.decode()

    def call(self, method: str, **params: object) -> dict:
        self.seq += 1
        body = json.dumps({"id": self.seq, "method": method, "params": params}).encode()
        mask = os.urandom(4)
        head = bytes([0x81]) + (bytes([0x80 | len(body)]) if len(body) < 126 else bytes([0x80 | 126]) + struct.pack(">H", len(body)) if len(body) < 65536 else bytes([0x80 | 127]) + struct.pack(">Q", len(body)))
        self.sock.sendall(head + mask + bytes(b ^ mask[i % 4] for i, b in enumerate(body)))
        while True:
            reply = json.loads(self._frame())
            if reply.get("id") == self.seq:
                if "error" in reply:
                    raise DevToolsError(f"{method}: {reply['error']}")
                return reply.get("result", {})

    def js(self, expression: str) -> object:
        """Evaluate in the page and return the value. A promise is awaited."""
        out = self.call("Runtime.evaluate", expression=expression, returnByValue=True, awaitPromise=True, userGesture=True)
        if "exceptionDetails" in out:
            thrown = out["exceptionDetails"]
            why = (thrown.get("exception") or {}).get("description") or thrown.get("text")
            raise DevToolsError(f"script threw: {str(why)[:300]} in {expression[:80]}")
        return out["result"].get("value")

    def text(self) -> str:
        # Right after a cold start the page has no body yet, which reads as empty text.
        return str(self.js("document.body ? document.body.innerText : ''") or "")

    def labels(self) -> str:
        """The interactive elements' names, so a failed find says what was there to tap."""
        found = self.js("[...document.querySelectorAll('button,[role=tab],[role=button],a,[aria-label]')].slice(0,40).map(e=>e.tagName+'['+(e.getAttribute('aria-label')||e.textContent.trim().slice(0,30))+']').join(' ')")
        return str(found)

    def center(self, finder: str) -> tuple[float, float] | None:
        """Viewport centre of the element finder() returns, or None. One below the fold scrolls in first."""
        return self.js(f"(()=>{{const e=({finder})();if(!e)return null;e.scrollIntoView({{block:'nearest',inline:'nearest'}});const r=e.getBoundingClientRect();return [r.x+r.width/2,r.y+r.height/2]}})()")  # type: ignore[return-value]

    def tap(self, finder: str, wait: float = 20) -> None:
        """A trusted mouse press and release at the element's centre, waiting for it to exist."""
        deadline = time.monotonic() + wait
        while time.monotonic() < deadline:
            at = self.center(finder)
            if at:
                for kind in ("mousePressed", "mouseReleased"):
                    self.call("Input.dispatchMouseEvent", type=kind, x=at[0], y=at[1], button="left", clickCount=1)
                return
            time.sleep(0.5)
        raise DevToolsError(f"nothing to tap for {finder[-90:]}. elements: {self.labels()[:900]}")

    def wait_selector(self, css: str, wait: float = 20) -> None:
        deadline = time.monotonic() + wait
        while time.monotonic() < deadline:
            if self.js(f"!!document.querySelector({json.dumps(css)})"):
                return
            time.sleep(0.5)
        raise DevToolsError(f"{css} never appeared. elements: {self.labels()[:900]}")

    def wait_text(self, needle: str, wait: float = 20) -> None:
        deadline = time.monotonic() + wait
        while time.monotonic() < deadline:
            if needle in self.text():
                return
            time.sleep(0.5)
        raise DevToolsError(f"{needle!r} never appeared. page text: {self.text()[:300]!r}")
