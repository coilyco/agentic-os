#!/usr/bin/env python3
"""Probe what an aterm daemon listener lets an unauthenticated peer do.

Dials the listener, upgrades to a websocket, sends `hello`, then a `spawn` of a harmless
seat that writes MARKER, and reports the reply and whether the marker exists. A
refusal (`passkey_required`) is the safe answer, a `spawned` frame means any process that
can reach the listener can run any argv as the daemon's user (COI-3498, COI-3494).
Exit 0 refused, 1 a spawn ran, 2 no verdict. Standard library only.
Design: tooling-aterm-client references/linux-seat.md.
"""
from __future__ import annotations

import argparse
import base64
import json
import os
import socket
import ssl
import struct
import sys
import time
from pathlib import Path


class Unprobed(Exception):
    """The probe could not reach a verdict, which is not a refusal."""


def connect(host: str, port: int, use_tls: bool, name: str, origin: str, insecure: bool) -> socket.socket:
    raw = socket.create_connection((host, port), timeout=10)
    context = ssl.create_default_context()
    if insecure:
        # The probe judges what the daemon allows, not its certificate chain.
        context.check_hostname, context.verify_mode = False, ssl.CERT_NONE
    sock = context.wrap_socket(raw, server_hostname=name) if use_tls else raw
    key = base64.b64encode(os.urandom(16)).decode()
    sock.sendall(
        f"GET / HTTP/1.1\r\nHost: {name}:{port}\r\nOrigin: {origin}\r\nUpgrade: websocket\r\n"
        f"Connection: Upgrade\r\nSec-WebSocket-Key: {key}\r\nSec-WebSocket-Version: 13\r\n\r\n".encode()
    )
    head = b""
    while b"\r\n\r\n" not in head:
        piece = sock.recv(1)
        if not piece:
            raise Unprobed(f"closed during the upgrade: {head[:200]!r}")
        head += piece
    status = head.split(b"\r\n")[0].decode(errors="replace")
    if " 101 " not in status:
        body = head.split(b"\r\n\r\n", 1)[1][:200]
        raise Unprobed(f"upgrade refused: {status} {body!r}")
    return sock


def send(sock: socket.socket, obj: dict) -> None:
    body = json.dumps(obj).encode()
    mask = os.urandom(4)
    size = len(body)
    head = bytes([0x81]) + (bytes([0x80 | size]) if size < 126 else bytes([0x80 | 126]) + struct.pack(">H", size))
    sock.sendall(head + mask + bytes(b ^ mask[i % 4] for i, b in enumerate(body)))


def read(sock: socket.socket) -> dict:
    def take(n: int) -> bytes:
        buf = b""
        while len(buf) < n:
            piece = sock.recv(n - len(buf))
            if not piece:
                raise Unprobed("the daemon closed the socket")
            buf += piece
        return buf

    data = b""
    while True:
        b0, b1 = take(2)
        size = b1 & 0x7F
        size = struct.unpack(">H", take(2))[0] if size == 126 else struct.unpack(">Q", take(8))[0] if size == 127 else size
        data += take(size)
        if b0 & 0x80:
            return json.loads(data)


def reply_to(sock: socket.socket, wanted_id: str) -> dict:
    while True:
        frame = read(sock)
        if frame.get("id") == wanted_id:
            return frame


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("host")
    parser.add_argument("port", type=int)
    parser.add_argument("--tls", action="store_true")
    parser.add_argument("--insecure", action="store_true", help="skip certificate verification")
    parser.add_argument("--name", help="the Host and SNI name, default the host")
    parser.add_argument("--origin", default="https://coilyco.dev")
    parser.add_argument("--marker", required=True, help="a path the probe seat would create")
    args = parser.parse_args()
    marker = Path(args.marker)
    marker.unlink(missing_ok=True)
    try:
        return probe(args, marker)
    except (Unprobed, OSError, ssl.SSLError) as err:
        print(f"unprobed: {err}", file=sys.stderr)
        return 2


def probe(args: argparse.Namespace, marker: Path) -> int:
    sock = connect(args.host, args.port, args.tls, args.name or args.host, args.origin, args.insecure)
    send(sock, {"type": "hello", "format": "aterm.daemon.v1", "version": "probe", "id": "h"})
    welcome = read(sock)
    print(f"welcome: format={welcome.get('format')} error={welcome.get('error')!r}")
    send(sock, {"type": "spawn", "id": "s", "session": "probe-seat", "role": "probe", "argv": ["/bin/sh", "-c", f"touch {marker}; sleep 5"], "rows": 24, "cols": 80})
    answer = reply_to(sock, "s")
    time.sleep(1.5)
    print(f"spawn answer: type={answer.get('type')} reason={answer.get('reason')!r} error={answer.get('error')!r}")
    print(f"marker created: {marker.exists()}")
    return 1 if answer.get("type") == "spawned" or marker.exists() else 0


if __name__ == "__main__":
    sys.exit(main())
