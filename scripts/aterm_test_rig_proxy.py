#!/usr/bin/env -S uv run
# /// script
# requires-python = ">=3.12"
# dependencies = []
# ///
"""Relay in front of a TEST aterm daemon, so the Android app reaches its loopback socket.

The daemon's loopback listener serves only loopback page origins, and the app's page is
https://tauri.localhost. This rewrites that one Origin and speaks websocket frames both
ways, refusing client frames that run commands (spawn, launch) and dropping a connection
it cannot parse. `--refuse-uploads-file` is the CI journey's refusal stub (COI-3526).
Loopback only. Reference: tooling-aterm-client android-emulator and android.
"""

from __future__ import annotations

import argparse
import asyncio
import ipaddress
import json
import os
import struct
import sys
from pathlib import Path

APP_ORIGIN = "https://tauri.localhost"
DENY_TYPES = ("spawn", "launch")
EXIT_USAGE = 2
MAX_HEAD = 65536
MAX_MESSAGE = 16 << 20
TEXT, BINARY, CLOSE, PING, PONG, CONT = 0x1, 0x2, 0x8, 0x9, 0xA, 0x0
# The refusals image_upload can carry, in the daemon's own words (aterm/daemon_image.go).
UPLOAD_REFUSALS = {
    "not_an_image": "the bytes are not a PNG, JPEG, GIF or WebP image",
    "too_large": "the image is over 3000000 bytes (3 MB), the most one upload carries",
    "too_many": "this seat already holds 16 images, which is the most a seat keeps",
    "session_unknown": "not attached to a seat named by this upload",
}


class Drop(Exception):
    """A connection the relay will not carry on, with the reason it logs."""


def rewrite_head(head: bytes, app_origin: str, as_origin: str) -> tuple[bytes, bool]:
    """The request head with the app's Origin replaced, and whether it was."""
    lines = head.split(b"\r\n")
    changed = False
    for i, line in enumerate(lines):
        name, _, value = line.partition(b":")
        if name.strip().lower() == b"origin" and value.strip().decode("latin-1").lower() == app_origin.lower():
            lines[i] = b"Origin: " + as_origin.encode("latin-1")
            changed = True
    return b"\r\n".join(lines), changed


def parse_head(head: bytes) -> tuple[str, str, list[tuple[str, str]]]:
    """Method, target and headers of a request or response head, or Drop."""
    try:
        lines = head.decode("latin-1").split("\r\n")
    except UnicodeDecodeError as err:
        raise Drop("unreadable head") from err
    parts = lines[0].split(" ", 2)
    if len(parts) < 3:
        raise Drop("bad start line")
    headers = []
    for line in lines[1:]:
        if not line:
            continue
        name, sep, value = line.partition(":")
        if not sep:
            raise Drop("bad header line")
        headers.append((name.strip().lower(), value.strip()))
    return parts[0], parts[1], headers


def encode_frame(opcode: int, payload: bytes, mask: bool) -> bytes:
    """One unfragmented frame. A client-to-server frame is masked with a fresh key."""
    first = bytes([0x80 | opcode])
    size = len(payload)
    flag = 0x80 if mask else 0
    if size < 126:
        head = first + bytes([flag | size])
    elif size < 65536:
        head = first + bytes([flag | 126]) + struct.pack(">H", size)
    else:
        head = first + bytes([flag | 127]) + struct.pack(">Q", size)
    if not mask:
        return head + payload
    key = os.urandom(4)
    return head + key + bytes(b ^ key[i % 4] for i, b in enumerate(payload))


async def read_frame(reader: asyncio.StreamReader, from_client: bool) -> tuple[bool, int, bytes]:
    """One frame as (fin, opcode, payload), or Drop. Client frames must be masked."""
    try:
        b0, b1 = await reader.readexactly(2)
        if b0 & 0x70:
            raise Drop("reserved bits set, so an extension is in use")
        opcode, masked, size = b0 & 0x0F, bool(b1 & 0x80), b1 & 0x7F
        if size == 126:
            (size,) = struct.unpack(">H", await reader.readexactly(2))
        elif size == 127:
            (size,) = struct.unpack(">Q", await reader.readexactly(8))
        if from_client != masked:
            raise Drop("a client frame must be masked and a server frame must not be")
        if size > MAX_MESSAGE:
            raise Drop("frame too large")
        key = await reader.readexactly(4) if masked else b""
        payload = await reader.readexactly(size)
    except asyncio.IncompleteReadError as err:
        raise Drop("connection closed") from err
    if masked:
        payload = bytes(b ^ key[i % 4] for i, b in enumerate(payload))
    fin = bool(b0 & 0x80)
    if opcode >= 0x8 and (not fin or size > 125):
        raise Drop("bad control frame")
    return fin, opcode, payload


def judge(message: bytes, deny: tuple[str, ...]) -> tuple[str, str]:
    """('forward' or 'refuse', the frame type), or Drop. Reads every key as Go's decoder does."""
    try:
        pairs = json.loads(message.decode("utf-8"), object_pairs_hook=list)
    except (UnicodeDecodeError, ValueError) as err:
        raise Drop("a message that is not JSON") from err
    if not (isinstance(pairs, list) and all(isinstance(p, tuple) and len(p) == 2 and isinstance(p[0], str) for p in pairs)):
        raise Drop("a message that is not a JSON object")
    kinds = [value for key, value in pairs if key.casefold() == "type"]
    if len(kinds) != 1 or not isinstance(kinds[0], str):
        raise Drop("a frame with no single string type")
    return ("refuse" if kinds[0].casefold() in deny else "forward"), kinds[0]


def refusal(message: bytes, kind: str, reason: str = "rig_refused", words: str = "") -> bytes:
    """A daemon-shaped error for the refused frame, echoing its id when it has one."""
    ident = next((v for k, v in json.loads(message, object_pairs_hook=list) if k.casefold() == "id"), "")
    body = {"type": "error", "id": ident if isinstance(ident, str) else "", "error": words or f"the test rig refuses {kind} frames", "code": 1, "reason": reason}
    return encode_frame(TEXT, json.dumps(body).encode(), mask=False)


def upload_refusal(path: str | None) -> str | None:
    """The image refusal the control file names right now, or None. Read per frame so a journey can change it."""
    if not path:
        return None
    try:
        reason = Path(path).read_text().strip()
    except OSError:
        return None
    return reason if reason in UPLOAD_REFUSALS else None


async def read_head(reader: asyncio.StreamReader) -> bytes:
    try:
        head = await reader.readuntil(b"\r\n\r\n")
    except (asyncio.IncompleteReadError, asyncio.LimitOverrunError) as err:
        raise Drop("no complete head") from err
    if len(head) > MAX_HEAD:
        raise Drop("head too large")
    return head


async def serve_frames(client_r, client_w, up_r, up_w, deny: tuple[str, ...], refuse_file: str | None = None) -> None:
    lock = asyncio.Lock()

    async def to_client(data: bytes) -> None:
        async with lock:
            client_w.write(data)
            await client_w.drain()

    async def server_to_client() -> None:
        while True:
            fin, opcode, payload = await read_frame(up_r, from_client=False)
            if not fin or opcode == CONT:
                raise Drop("a fragmented frame from the daemon")
            await to_client(encode_frame(opcode, payload, mask=False))

    async def client_to_server() -> None:
        pieces: list[bytes] = []
        first_opcode = 0
        while True:
            fin, opcode, payload = await read_frame(client_r, from_client=True)
            if opcode in (CLOSE, PING, PONG):
                up_w.write(encode_frame(opcode, payload, mask=True))
                await up_w.drain()
                continue
            if opcode == CONT:
                if not pieces:
                    raise Drop("a continuation with nothing to continue")
            elif opcode in (TEXT, BINARY):
                if pieces:
                    raise Drop("a new message inside a fragmented one")
                first_opcode = opcode
            else:
                raise Drop(f"unknown opcode {opcode}")
            pieces.append(payload)
            if sum(map(len, pieces)) > MAX_MESSAGE:
                raise Drop("message too large")
            if not fin:
                continue
            message, pieces = b"".join(pieces), []
            if first_opcode != TEXT:
                raise Drop("a binary message")
            verdict, kind = judge(message, deny)
            if verdict == "refuse":
                print(f"relay: refused a {kind} frame", flush=True)
                await to_client(refusal(message, kind))
                continue
            stubbed = upload_refusal(refuse_file) if kind.casefold() == "image_upload" else None
            if stubbed:
                print(f"relay: answered an image_upload with {stubbed}", flush=True)
                await to_client(refusal(message, kind, stubbed, UPLOAD_REFUSALS[stubbed]))
                continue
            up_w.write(encode_frame(TEXT, message, mask=True))
            await up_w.drain()

    tasks = [asyncio.ensure_future(server_to_client()), asyncio.ensure_future(client_to_server())]
    try:
        done, _ = await asyncio.wait(tasks, return_when=asyncio.FIRST_COMPLETED)
        for task in done:
            task.result()
    finally:
        for task in tasks:
            task.cancel()


def reply(status: str, text: str) -> bytes:
    body = text.encode()
    return f"HTTP/1.1 {status}\r\nX-Aterm-Refusal: rig\r\nContent-Type: text/plain\r\nContent-Length: {len(body)}\r\nConnection: close\r\n\r\n".encode() + body


async def handle(client_r, client_w, upstream, app_origin, as_origin, deny, refuse_file: str | None = None) -> None:
    up_w = None
    try:
        head = await read_head(client_r)
        method, target, headers = parse_head(head)
        names = dict(headers)
        if len(names) != len(headers):
            raise Drop("a request with a repeated header")
        if names.get("content-length", "0") != "0" or "transfer-encoding" in names:
            raise Drop("a request with a body")
        upgrade = names.get("upgrade", "").lower() == "websocket"
        if method != "GET" or not ((upgrade and target == "/") or (not upgrade and target == "/_aterm/reach")):
            client_w.write(reply("403 Forbidden", "test rig relay: only the session socket and /_aterm/reach are served"))
            await client_w.drain()
            print(f"relay: refused a request for {method} {target}", flush=True)
            return
        head, changed = rewrite_head(head, app_origin, as_origin)
        drop = (b"sec-websocket-extensions:",) if upgrade else (b"connection:",)
        kept = [line for line in head.split(b"\r\n") if line and not line.lower().startswith(drop)]
        head = b"\r\n".join(kept + ([] if upgrade else [b"Connection: close"])) + b"\r\n\r\n"
        print(f"relay: {'upgrade' if upgrade else 'reach'}, origin rewritten: {changed}", flush=True)
        up_r, up_w = await asyncio.open_connection(*upstream, limit=MAX_HEAD)
        up_w.write(head)
        await up_w.drain()
        response = await read_head(up_r)
        client_w.write(response)
        await client_w.drain()
        if upgrade and response.startswith(b"HTTP/1.1 101"):
            if any(name == "sec-websocket-extensions" for name, _ in parse_head(response)[2]):
                raise Drop("the daemon negotiated an extension")
            await serve_frames(client_r, client_w, up_r, up_w, deny, refuse_file)
        else:
            while chunk := await up_r.read(65536):
                client_w.write(chunk)
                await client_w.drain()
    except Drop as err:
        print(f"relay: dropped a connection: {err}", flush=True)
    except (OSError, ConnectionError):
        print("relay: dropped a connection: network error", flush=True)
    finally:
        for writer in (client_w, up_w):
            if writer is not None:
                writer.close()


def parse_addr(text: str) -> tuple[str, int]:
    host, _, port = text.rpartition(":")
    if not host or not port.isdigit():
        raise argparse.ArgumentTypeError(f"{text} is not host:port")
    return host, int(port)


async def serve(listen, upstream, app_origin: str, as_origin: str, deny: tuple[str, ...], refuse_file: str | None = None) -> None:
    server = await asyncio.start_server(lambda r, w: handle(r, w, upstream, app_origin, as_origin, deny, refuse_file), *listen, limit=MAX_HEAD)
    print(f"relay: {listen[0]}:{listen[1]} to {upstream[0]}:{upstream[1]}, refusing {','.join(deny)} frames", flush=True)
    async with server:
        await server.serve_forever()


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(prog="aterm-test-rig-proxy", description=(__doc__ or "").split("\n\n")[0])
    parser.add_argument("--listen", type=parse_addr, required=True, help="loopback host:port the app dials")
    parser.add_argument("--upstream", type=parse_addr, required=True, help="the test daemon's loopback host:port")
    parser.add_argument("--app-origin", default=APP_ORIGIN)
    parser.add_argument("--as-origin", default="http://127.0.0.1", help="the loopback origin sent in its place")
    parser.add_argument("--refuse-uploads-file", help="a file whose content, an image refusal reason, makes image_upload frames refused (CI journey stub)")
    parser.add_argument("--deny", default=",".join(DENY_TYPES), help="client frame types refused, comma separated")
    args = parser.parse_args(argv)
    for name, (host, _) in (("--listen", args.listen), ("--upstream", args.upstream)):
        try:
            loopback = ipaddress.ip_address(host).is_loopback
        except ValueError:
            loopback = host == "localhost"
        if not loopback:
            print(f"aterm-test-rig-proxy: {name} must be a loopback address, got {host}", file=sys.stderr)
            return EXIT_USAGE
    deny = tuple(t.strip().casefold() for t in args.deny.split(",") if t.strip())
    if "spawn" not in deny:
        print("aterm-test-rig-proxy: --deny must include spawn", file=sys.stderr)
        return EXIT_USAGE
    try:
        asyncio.run(serve(args.listen, args.upstream, args.app_origin, args.as_origin, deny, args.refuse_uploads_file))
    except KeyboardInterrupt:
        pass
    return 0


if __name__ == "__main__":
    sys.exit(main())
