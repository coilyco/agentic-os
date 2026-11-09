#!/usr/bin/env -S uv run
# /// script
# requires-python = ">=3.12"
# dependencies = []
# ///
"""Relay in front of a TEST aterm daemon, so the Android app reaches its loopback socket.

The daemon's loopback listener serves only loopback page origins, and the app's page is
https://tauri.localhost. This rewrites that one Origin header on the first request of a
connection and relays bytes both ways, so the shipped daemon stays unchanged. Any other
origin passes through untouched and meets the daemon's own policy. Loopback listen only.
Reference: tooling-aterm-client references/android-emulator.md.
"""

from __future__ import annotations

import argparse
import asyncio
import ipaddress
import sys

APP_ORIGIN = "https://tauri.localhost"
EXIT_USAGE = 2
MAX_HEAD = 65536


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


async def pipe(reader: asyncio.StreamReader, writer: asyncio.StreamWriter) -> None:
    try:
        while chunk := await reader.read(65536):
            writer.write(chunk)
            await writer.drain()
    except (ConnectionError, asyncio.IncompleteReadError):
        pass
    finally:
        writer.close()


async def handle(client_r, client_w, upstream: tuple[str, int], app_origin: str, as_origin: str) -> None:
    try:
        head = await client_r.readuntil(b"\r\n\r\n")
        if len(head) > MAX_HEAD:
            raise ValueError("request head too large")
        head, changed = rewrite_head(head, app_origin, as_origin)
        up_r, up_w = await asyncio.open_connection(*upstream)
    except (OSError, asyncio.IncompleteReadError, asyncio.LimitOverrunError, ValueError) as err:
        print(f"relay: dropped a connection: {type(err).__name__}", flush=True)
        client_w.close()
        return
    print(f"relay: connection, origin rewritten: {changed}", flush=True)
    up_w.write(head)
    await up_w.drain()
    await asyncio.gather(pipe(client_r, up_w), pipe(up_r, client_w))


def parse_addr(text: str) -> tuple[str, int]:
    host, _, port = text.rpartition(":")
    if not host or not port.isdigit():
        raise argparse.ArgumentTypeError(f"{text} is not host:port")
    return host, int(port)


async def serve(listen: tuple[str, int], upstream: tuple[str, int], app_origin: str, as_origin: str) -> None:
    server = await asyncio.start_server(lambda r, w: handle(r, w, upstream, app_origin, as_origin), *listen, limit=MAX_HEAD)
    print(f"relay: {listen[0]}:{listen[1]} to {upstream[0]}:{upstream[1]}, rewriting {app_origin} to {as_origin}", flush=True)
    async with server:
        await server.serve_forever()


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(prog="aterm-test-rig-proxy", description=(__doc__ or "").split("\n\n")[0])
    parser.add_argument("--listen", type=parse_addr, required=True, help="loopback host:port the app dials")
    parser.add_argument("--upstream", type=parse_addr, required=True, help="the test daemon's loopback host:port")
    parser.add_argument("--app-origin", default=APP_ORIGIN)
    parser.add_argument("--as-origin", default="http://127.0.0.1", help="the loopback origin sent in its place")
    args = parser.parse_args(argv)
    for name, (host, _) in (("--listen", args.listen), ("--upstream", args.upstream)):
        try:
            loopback = ipaddress.ip_address(host).is_loopback
        except ValueError:
            loopback = host == "localhost"
        if not loopback:
            print(f"aterm-test-rig-proxy: {name} must be a loopback address, got {host}", file=sys.stderr)
            return EXIT_USAGE
    try:
        asyncio.run(serve(args.listen, args.upstream, args.app_origin, args.as_origin))
    except KeyboardInterrupt:
        pass
    return 0


if __name__ == "__main__":
    sys.exit(main())
