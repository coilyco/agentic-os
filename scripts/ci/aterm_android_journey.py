#!/usr/bin/env python3
"""Drive the debug aterm app on the CI emulator through the image journeys (COI-3485).

Journey A: add a host, open the seat, tap Add image, see the system picker, pick the
seeded PNG, Send, then check the daemon wrote the same bytes (aterm_android_seat.py).
Journey C: stop the daemon, send again, keep the page text and screenshots as evidence.
The page is driven over DevTools (the debug WebView is debuggable), the picker through
uiautomator. A screenshot follows every step, and a failed step keeps its own.
Design: tooling-aterm-client references/android.md. Standard library only.
"""
from __future__ import annotations

import argparse
import base64
import json
import os
import re
import socket
import struct
import subprocess
import sys
import time
import urllib.request
import xml.etree.ElementTree as ET
import zlib
from pathlib import Path

APP = "dev.coilyco.treehouse"
HERE = Path(__file__).resolve().parent
PNG_NAME = "ci-seed.png"


class Fail(Exception):
    """A step did not hold. Its text names what was expected and what was seen."""


def adb(*args: str, check: bool = True, timeout: int = 60) -> str:
    done = subprocess.run(["adb", *args], capture_output=True, text=True, timeout=timeout, check=False)
    if check and done.returncode != 0:
        raise Fail(f"adb {' '.join(args)} failed: {done.stderr.strip() or done.stdout.strip()}")
    return done.stdout


def seed_png(path: Path) -> None:
    """A small red PNG with a signature the daemon accepts, made here so no binary is tracked."""
    def chunk(kind: bytes, body: bytes) -> bytes:
        return struct.pack(">I", len(body)) + kind + body + struct.pack(">I", zlib.crc32(kind + body) & 0xFFFFFFFF)
    rows = b"".join(b"\x00" + bytes([220, 30, 30]) * 64 for _ in range(64))
    path.write_bytes(b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", 64, 64, 8, 2, 0, 0, 0)) + chunk(b"IDAT", zlib.compress(rows)) + chunk(b"IEND", b""))


class Page:
    """A DevTools session on the app's page: evaluate script and send trusted mouse events."""

    def __init__(self, url: str) -> None:
        host, port, path = re.match(r"ws://([^:/]+):(\d+)(/.*)", url).groups()  # type: ignore[union-attr]
        self.sock = socket.create_connection((host, int(port)), timeout=20)
        key = base64.b64encode(os.urandom(16)).decode()
        self.sock.sendall(f"GET {path} HTTP/1.1\r\nHost: {host}:{port}\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: {key}\r\nSec-WebSocket-Version: 13\r\n\r\n".encode())
        head = b""
        while b"\r\n\r\n" not in head:
            head += self.sock.recv(1)
        if b" 101 " not in head.split(b"\r\n")[0]:
            raise Fail(f"DevTools refused the upgrade: {head.decode(errors='replace')[:120]}")
        self.seq = 0

    def _read(self, n: int) -> bytes:
        buf = b""
        while len(buf) < n:
            piece = self.sock.recv(n - len(buf))
            if not piece:
                raise Fail("DevTools closed the socket")
            buf += piece
        return buf

    def _frame(self) -> str:
        data = b""
        while True:
            b0, b1 = self._read(2)
            size = b1 & 0x7F
            size = struct.unpack(">H", self._read(2))[0] if size == 126 else struct.unpack(">Q", self._read(8))[0] if size == 127 else size
            data += self._read(size)
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
                    raise Fail(f"{method}: {reply['error']}")
                return reply.get("result", {})

    def js(self, expression: str) -> object:
        out = self.call("Runtime.evaluate", expression=expression, returnByValue=True, userGesture=True)
        if "exceptionDetails" in out:
            raise Fail(f"script threw: {out['exceptionDetails'].get('text')} in {expression[:80]}")
        return out["result"].get("value")

    def text(self) -> str:
        return str(self.js("document.body.innerText") or "")

    def center(self, finder: str) -> tuple[float, float] | None:
        """Viewport centre of the element finder() returns, or None."""
        return self.js(f"(()=>{{const e=({finder})();if(!e)return null;const r=e.getBoundingClientRect();return [r.x+r.width/2,r.y+r.height/2]}})()")  # type: ignore[return-value]

    def tap(self, finder: str, wait: float = 20) -> None:
        deadline = time.monotonic() + wait
        while time.monotonic() < deadline:
            at = self.center(finder)
            if at:
                for kind in ("mousePressed", "mouseReleased"):
                    self.call("Input.dispatchMouseEvent", type=kind, x=at[0], y=at[1], button="left", clickCount=1)
                return
            time.sleep(0.5)
        raise Fail(f"nothing to tap for {finder}. page text: {self.text()[:300]!r}")

    def wait_text(self, needle: str, wait: float = 20) -> None:
        deadline = time.monotonic() + wait
        while time.monotonic() < deadline:
            if needle in self.text():
                return
            time.sleep(0.5)
        raise Fail(f"{needle!r} never appeared. page text: {self.text()[:300]!r}")


def by_text(text: str, selector: str = "button,[role=tab],[role=button],a,li") -> str:
    return f"()=>[...document.querySelectorAll({json.dumps(selector)})].find(e=>e.textContent.includes({json.dumps(text)}))"


class Journey:
    def __init__(self, args: argparse.Namespace) -> None:
        self.args = args
        self.out = Path(args.out)
        self.out.mkdir(parents=True, exist_ok=True)
        self.shots = 0
        self.page: Page | None = None

    def shot(self, name: str) -> None:
        self.shots += 1
        data = subprocess.run(["adb", "exec-out", "screencap", "-p"], capture_output=True, check=False, timeout=30).stdout
        (self.out / f"{self.shots:02d}-{name}.png").write_bytes(data)

    def step(self, name: str, fn) -> None:
        print(f"== {name}", flush=True)
        try:
            fn()
        except (Fail, OSError, subprocess.TimeoutExpired) as err:
            self.shot(f"FAILED-{name}")
            self.evidence()
            print(f"FAILED {name}: {err}", file=sys.stderr)
            raise SystemExit(1) from err
        self.shot(name)

    def evidence(self) -> None:
        (self.out / "window.txt").write_text(adb("shell", "dumpsys", "window", check=False))
        (self.out / "logcat.txt").write_text(adb("logcat", "-d", "-t", "4000", check=False))
        if self.page:
            try:
                (self.out / "page.txt").write_text(self.page.text())
            except (Fail, OSError):
                pass
        (self.out / "ui.xml").write_text(self.ui_xml())

    def ui_xml(self) -> str:
        adb("shell", "uiautomator", "dump", "/sdcard/ui.xml", check=False)
        return adb("exec-out", "cat", "/sdcard/ui.xml", check=False)

    def focus(self) -> str:
        return "\n".join(line for line in adb("shell", "dumpsys", "window").splitlines() if "mCurrentFocus" in line)

    def install_and_launch(self) -> None:
        adb("reverse", "tcp:7419", "tcp:7419")
        adb("install", "-r", "-g", self.args.apk, timeout=180)
        png = self.out / PNG_NAME
        seed_png(png)
        adb("push", str(png), f"/sdcard/Pictures/{PNG_NAME}")
        adb("shell", "am", "broadcast", "-a", "android.intent.action.MEDIA_SCANNER_SCAN_FILE", "-d", f"file:///sdcard/Pictures/{PNG_NAME}")
        adb("shell", "am", "start", "-n", f"{APP}/.MainActivity")

    def attach_page(self) -> None:
        deadline = time.monotonic() + 60
        while time.monotonic() < deadline:
            sockets = re.findall(r"@(webview_devtools_remote_\d+)", adb("shell", "cat", "/proc/net/unix", check=False))
            if sockets:
                adb("forward", "tcp:9222", f"localabstract:{sockets[0]}")
                try:
                    targets = json.load(urllib.request.urlopen("http://127.0.0.1:9222/json", timeout=5))
                except OSError:
                    targets = []
                pages = [t for t in targets if t.get("type") == "page" and "tauri.localhost" in t.get("url", "")]
                if pages:
                    self.page = Page(pages[0]["webSocketDebuggerUrl"].replace("localhost", "127.0.0.1"))
                    return
            time.sleep(1)
        raise Fail("no debuggable page at tauri.localhost: the WebView is not debuggable or the app did not start")

    def add_host_and_open_seat(self) -> None:
        page = self.page
        assert page
        page.wait_text("Add a host")
        page.js("(()=>{const i=document.querySelector('#add-host');i.value='ws://127.0.0.1:7419';i.dispatchEvent(new Event('input',{bubbles:true}))})()")
        page.tap(by_text("Add host", "button"))
        page.tap("()=>document.querySelector('[id^=tab-host-]')")
        page.tap(by_text(self.args.seat))

    def open_picker(self) -> None:
        page = self.page
        assert page
        page.tap("()=>document.querySelector('[aria-label=\"Add image\"]')")
        deadline = time.monotonic() + 20
        while time.monotonic() < deadline:
            if "documentsui" in self.focus():
                return
            time.sleep(0.5)
        raise Fail(f"no documentsui window in front. focus: {self.focus() or 'none'}")

    def pick_image(self) -> None:
        deadline = time.monotonic() + 30
        while time.monotonic() < deadline:
            # uiautomator's dump from the emulator this job started, not untrusted XML.
            tree = ET.fromstring(self.ui_xml() or "<hierarchy/>")
            for node in tree.iter("node"):
                if PNG_NAME in (node.get("text", "") + node.get("content-desc", "")) or node.get("resource-id", "").endswith(("icon_thumb", "item_root")):
                    x0, y0, x1, y1 = map(int, re.findall(r"\d+", node.get("bounds", "0,0,0,0")))
                    adb("shell", "input", "tap", str((x0 + x1) // 2), str((y0 + y1) // 2))
                    return
            time.sleep(1)
        raise Fail("the picker showed no image to tap, ui.xml has what it did show")

    def chip_then_send(self) -> None:
        page = self.page
        assert page
        page.wait_text(PNG_NAME.split(".")[0], 20)
        page.tap(by_text("Send", "button"))

    def daemon_got_the_bytes(self) -> None:
        png = self.out / PNG_NAME
        done = subprocess.run([sys.executable, "-I", str(HERE / "aterm_android_seat.py"), "expect-image", self.args.socket, self.args.seat, str(png), self.args.seat_log, "--timeout", "30"], capture_output=True, text=True, check=False)
        print(done.stdout + done.stderr, end="")
        if done.returncode != 0:
            raise Fail(f"the daemon did not receive the image: {done.stderr.strip()}")

    def journey_c_daemon_down(self) -> None:
        """Evidence only: the failure copy is the client's to specify (COI-3485)."""
        page = self.page
        assert page
        self.open_picker()
        self.pick_image()
        page.wait_text(PNG_NAME.split(".")[0], 20)
        subprocess.run(["pkill", "-f", f"aterm daemon --socket {self.args.socket}"], check=False)
        time.sleep(1)
        page.tap(by_text("Send", "button"))
        time.sleep(3)
        (self.out / "journey-c-page.txt").write_text(page.text())
        if PNG_NAME.split(".")[0] not in page.text():
            raise Fail("the image chip is gone after a failed send, the draft was not kept")
        words = os.environ.get("ATERM_JOURNEY_C_WORDS")
        if words and not re.search(words, page.text(), re.IGNORECASE):
            raise Fail(f"the page did not say {words!r}")
        print("journey C: chip kept; failure words " + ("asserted" if words else "captured in journey-c-page.txt, not asserted"))

    def run(self) -> None:
        self.step("install-and-launch", self.install_and_launch)
        self.step("attach-devtools", self.attach_page)
        self.step("add-host-and-open-seat", self.add_host_and_open_seat)
        self.step("tap-add-image-picker-opens", self.open_picker)
        self.step("pick-the-seeded-png", self.pick_image)
        self.step("chip-then-send", self.chip_then_send)
        self.step("daemon-received-the-bytes", self.daemon_got_the_bytes)
        print("journey A: passed")
        self.step("journey-c-daemon-down", self.journey_c_daemon_down)
        self.evidence()


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ("apk", "out", "socket", "seat", "seat-log"):
        parser.add_argument(f"--{name}", required=True)
    Journey(parser.parse_args()).run()
    return 0


if __name__ == "__main__":
    sys.exit(main())
