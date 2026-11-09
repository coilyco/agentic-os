#!/usr/bin/env -S uv run
# /// script
# requires-python = ">=3.12"
# dependencies = []
# ///
"""Spawn fixture seats on a test aterm daemon, for a seat to drive the Android app against.

A fixture seat is a spawn frame whose argv is a shell loop, with no agent harness. It
refuses the default daemon socket, which holds real seats. Reference:
tooling-aterm-client references/android-emulator.md.
"""

from __future__ import annotations

import argparse
import json
import os
import socket
import sys

# Spawned as `claude`, so the daemon waits for the prompt mark before the seat is ready.
PROMPT = r"""printf 'Try "\r\033[2K'; """
SEATS = {
    "fixture-cards": ("Cards", 'while :; do aterm ask --header Fixture "Pick one" "red::the first option" green blue || true; sleep 2; done'),
    "fixture-quiet": ("Quiet", 'n=0; while :; do n=$((n+1)); echo "fixture heartbeat $n"; sleep 5; done'),
}


class Refused(Exception):
    """A request the script declines, with the exit code to return."""

    def __init__(self, message: str, code: int = 1):
        super().__init__(message)
        self.code = code


def default_socket() -> str:
    return f"/tmp/aterm-{os.getuid()}/daemon.sock"


def choose_socket(arg: str | None, env: dict[str, str]) -> str:
    path = arg or env.get("ATERM_DAEMON_SOCKET", "")
    if not path:
        raise Refused("name the test daemon with --socket or ATERM_DAEMON_SOCKET", 2)
    if os.path.realpath(path) == os.path.realpath(default_socket()):
        raise Refused("that is the default daemon socket, which holds real seats, use a test daemon", 2)
    return path


class Daemon:
    """One connection speaking aterm.daemon.v1, one JSON object per line."""

    def __init__(self, path: str):
        self.sock = socket.socket(socket.AF_UNIX)
        self.sock.settimeout(15)
        try:
            self.sock.connect(path)
        except OSError as err:
            raise Refused(f"no daemon at {path}: {err}") from err
        self.file = self.sock.makefile("rw", buffering=1)
        reply = self.request({"type": "hello", "format": "aterm.daemon.v1", "version": "fixture-seats"}, wait_for=None)
        if reply.get("type") != "welcome":
            raise Refused(f"the daemon did not welcome the hello: {reply}")

    def request(self, frame: dict, wait_for: str | None = "") -> dict:
        self.file.write(json.dumps(frame) + "\n")
        while True:
            line = self.file.readline()
            if not line:
                raise Refused("the daemon closed the connection")
            reply = json.loads(line)
            if wait_for is None or reply.get("id") == frame.get("id") or reply.get("type") in ("welcome", "error"):
                return reply

    def sessions(self) -> dict[str, str]:
        reply = self.request({"type": "list", "id": "list"})
        return {s["name"]: s.get("state", "") for s in reply.get("sessions") or []}


def up(daemon: Daemon, socket_path: str) -> list[str]:
    live = daemon.sessions()
    lines = []
    for name, (identity, script) in SEATS.items():
        if name in live:
            lines.append(f"{name} already up, {live[name]}")
            continue
        reply = daemon.request({
            "type": "spawn", "id": name, "session": name, "role": "fixture", "identity": identity, "seat": "claude",
            "argv": ["/bin/sh", "-c", PROMPT + script], "cwd": "/", "rows": 24, "cols": 120,
            "env": [f"{k}={v}" for k, v in os.environ.items()] + [f"ATERM_DAEMON_SOCKET={socket_path}"],
        })
        if reply.get("type") != "spawned":
            raise Refused(f"spawning {name} failed: {reply}")
        lines.append(f"{name} spawned")
    return lines


def down(daemon: Daemon) -> list[str]:
    live = daemon.sessions()
    lines = []
    for name in SEATS:
        if name not in live:
            continue
        reply = daemon.request({"type": "close", "id": name, "target": name, "force": True})
        lines.append(f"{name} closed" if reply.get("type") != "error" else f"{name} close refused: {reply}")
    return lines or ["no fixture seats were up"]


def main(argv: list[str] | None = None, env: dict[str, str] | None = None) -> int:
    parser = argparse.ArgumentParser(prog="aterm-fixture-seats", description=(__doc__ or "").split("\n\n")[0])
    parser.add_argument("action", choices=["up", "down", "status"])
    parser.add_argument("--socket", help="the test daemon's socket, or ATERM_DAEMON_SOCKET")
    args = parser.parse_args(argv)
    try:
        path = choose_socket(args.socket, dict(os.environ if env is None else env))
        daemon = Daemon(path)
        if args.action == "up":
            lines = up(daemon, path)
        elif args.action == "down":
            lines = down(daemon)
        else:
            live = daemon.sessions()
            lines = [f"{name} {live.get(name, 'not up')}" for name in SEATS]
    except Refused as err:
        print(f"aterm-fixture-seats: {err}", file=sys.stderr)
        return err.code
    print("\n".join(lines))
    return 0


if __name__ == "__main__":
    sys.exit(main())
