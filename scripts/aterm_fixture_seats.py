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
import signal
import socket
import subprocess
import sys
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path

# Spawned as `claude`, so the daemon waits for the prompt mark before the seat is ready.
PROMPT = r"""printf 'Try "\r\033[2K'; """
# The daemon reads last_reply from a claude transcript found by the --session-id in argv.
CONVERSATION = "f1f1f1f1-0000-4000-8000-000000000001"
REPLY = "Fixture reply: the build is green.\n\nThe second paragraph is here so the pinned reply has more than its first line to open.\n\n- one\n- two\n- three"
WIDE_SEAT, WIDE_COLS = "fixture-wide", 160
RULER = "0123456789" * 15
# A full-screen program with the mouse on, as vim runs (COI-3596). A row per PTY row,
# and prints the bytes of the last wheel it was sent on row 1.
ALT = (
    r"""stty -echo -icanon min 0 time 5; printf '\033[?1049h\033[?1000h\033[?1006h'; w=none; last=; """
    r"""draw() { printf '\033[H\033[2J'; i=2; while [ $i -le ${last% *} ]; do printf '\033[%d;1Halt row %d' $i $i; i=$((i+1)); done; }; """
    r"""while :; do s=$(stty size); [ "$s" != "$last" ] && { last=$s; draw; }; """
    r"""b=$(dd bs=32 count=1 2>/dev/null | od -An -c | tr -s ' \n' ' '); [ -n "$b" ] && w=$b; printf '\033[1;1H\033[2Kwheel:%s' "$w"; done"""
)
# The normal screen, mouse on, long history: a swipe has to scroll that history.
MOUSE = r"""printf '\033[?1000h\033[?1006h'; i=1; while [ $i -le 200 ]; do echo "history line $i"; i=$((i+1)); done; n=0; while :; do n=$((n+1)); echo "mouse seat $n"; sleep 5; done"""


@dataclass(frozen=True)
class Fixture:
    """One seat: the name a person sees, its shell loop, and the reply its transcript carries."""

    identity: str
    script: str
    reply: str = ""


def flip(period: int) -> str:
    """Asks for `period` seconds, then ends the ask by killing the asker, then rests as long."""
    return f'while :; do aterm ask --header Fixture "Flip {period}" one two & asker=$!; sleep {period}; kill $asker 2>/dev/null; wait $asker 2>/dev/null; sleep {period}; done'


# Three togglers at unrelated periods, so the roster reorders on its own (COI-3314).
SEATS = {
    "fixture-cards": Fixture("Cards", 'while :; do aterm ask --header Fixture "Pick one" "red::the first option" green blue || true; sleep 2; done'),
    "fixture-quiet": Fixture("Quiet", 'n=0; while :; do n=$((n+1)); echo "fixture heartbeat $n"; sleep 5; done'),
    "fixture-reply": Fixture("Reply", 'n=0; while :; do n=$((n+1)); echo "fixture reply seat $n"; sleep 5; done', REPLY),
    "fixture-free": Fixture("Free text", '(n=0; while :; do n=$((n+1)); echo "output while the card is open $n"; sleep 1; done) & while :; do aterm ask --other --header Fixture "Say it in your own words" "yes::the first" no || true; sleep 2; done'),
    "fixture-flip-a": Fixture("Flip A", flip(7)),
    "fixture-flip-b": Fixture("Flip B", flip(11)),
    "fixture-flip-c": Fixture("Flip C", flip(13)),
    "fixture-alt": Fixture("Alt screen", ALT),
    "fixture-mouse": Fixture("Mouse", MOUSE),
    WIDE_SEAT: Fixture("Wide", f'n=0; while :; do n=$((n+1)); echo "wide line $n {RULER}"; sleep 5; done'),
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


def holder_pid_file(socket_path: str) -> Path:
    return Path(socket_path).parent / f"{WIDE_SEAT}.pid"


def holder_alive(socket_path: str) -> bool:
    """Whether the wide client's recorded process still runs."""
    try:
        os.kill(int(holder_pid_file(socket_path).read_text()), 0)
    except (OSError, ValueError):
        return False
    return True


def start_holder(socket_path: str) -> str:
    """A second client, 160 columns wide and silent, so the daemon sizes the wide seat to it (COI-3317)."""
    if holder_alive(socket_path):
        return f"{WIDE_SEAT} wide client still running"
    proc = subprocess.Popen(
        [sys.executable, os.path.abspath(__file__), "hold", "--socket", socket_path],
        stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, start_new_session=True,
    )
    holder_pid_file(socket_path).write_text(str(proc.pid))
    return f"{WIDE_SEAT} wide client started"


def stop_holder(socket_path: str) -> str:
    if holder_alive(socket_path):
        os.kill(int(holder_pid_file(socket_path).read_text()), signal.SIGTERM)
        holder_pid_file(socket_path).unlink(missing_ok=True)
        return f"{WIDE_SEAT} wide client stopped"
    holder_pid_file(socket_path).unlink(missing_ok=True)
    return f"{WIDE_SEAT} wide client was not up"


def hold(socket_path: str) -> None:
    """Attach to the wide seat at WIDE_COLS columns and read until the daemon hangs up."""
    daemon = Daemon(socket_path)
    reply = daemon.request({"type": "attach", "id": "wide", "session": WIDE_SEAT, "rows": 24, "cols": WIDE_COLS})
    if reply.get("type") == "error":
        raise Refused(f"attaching to {WIDE_SEAT} failed: {reply}")
    daemon.sock.settimeout(None)
    while daemon.file.readline():
        pass


def claude_dir(socket_path: str) -> Path:
    """Where the reply seat's claude transcript lives, beside the socket and gone with it."""
    return Path(socket_path).parent / "fixture-claude"


def write_transcript(socket_path: str, reply: str) -> None:
    """A finished claude turn, in the shape the daemon reads a seat's last_reply from."""
    now = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")
    lines = [
        {"type": "user", "timestamp": now, "message": {"role": "user", "content": "Is the build green?"}},
        {"type": "assistant", "timestamp": now, "message": {
            "role": "assistant", "model": "claude-fixture", "stop_reason": "end_turn",
            "content": [{"type": "text", "text": reply}],
            "usage": {"input_tokens": 52000, "output_tokens": 300, "cache_creation_input_tokens": 0, "cache_read_input_tokens": 0},
        }},
    ]
    path = claude_dir(socket_path) / "projects" / "fixture" / f"{CONVERSATION}.jsonl"
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text("".join(json.dumps(line) + "\n" for line in lines))


def up(daemon: Daemon, socket_path: str) -> list[str]:
    live = daemon.sessions()
    lines = []
    for name, fixture in SEATS.items():
        if name in live:
            lines.append(f"{name} already up, {live[name]}")
            continue
        argv = ["/bin/sh", "-c", PROMPT + fixture.script]
        env = [f"{k}={v}" for k, v in os.environ.items()] + [f"ATERM_DAEMON_SOCKET={socket_path}"]
        if fixture.reply:
            write_transcript(socket_path, fixture.reply)
            argv += [name, "--session-id", CONVERSATION]
            env.append(f"CLAUDE_CONFIG_DIR={claude_dir(socket_path)}")
        reply = daemon.request({
            "type": "spawn", "id": name, "session": name, "role": "fixture", "identity": fixture.identity, "seat": "claude",
            "argv": argv, "cwd": "/", "rows": 24, "cols": 120, "env": env,
        })
        if reply.get("type") != "spawned":
            raise Refused(f"spawning {name} failed: {reply}")
        lines.append(f"{name} spawned")
    lines.append(start_holder(socket_path))
    return lines


def down(daemon: Daemon, socket_path: str) -> list[str]:
    live = daemon.sessions()
    lines = [stop_holder(socket_path)]
    for name in SEATS:
        if name not in live:
            continue
        reply = daemon.request({"type": "close", "id": name, "target": name, "force": True})
        lines.append(f"{name} closed" if reply.get("type") != "error" else f"{name} close refused: {reply}")
    return lines


def main(argv: list[str] | None = None, env: dict[str, str] | None = None) -> int:
    parser = argparse.ArgumentParser(prog="aterm-fixture-seats", description=(__doc__ or "").split("\n\n")[0])
    parser.add_argument("action", choices=["up", "down", "status", "hold"])
    parser.add_argument("--socket", help="the test daemon's socket, or ATERM_DAEMON_SOCKET")
    args = parser.parse_args(argv)
    try:
        path = choose_socket(args.socket, dict(os.environ if env is None else env))
        if args.action == "hold":
            hold(path)
            return 0
        daemon = Daemon(path)
        if args.action == "up":
            lines = up(daemon, path)
        elif args.action == "down":
            lines = down(daemon, path)
        else:
            live = daemon.sessions()
            lines = [f"{name} {live.get(name, 'not up')}" for name in SEATS]
            lines.append(f"{WIDE_SEAT} wide client {'up' if holder_alive(path) else 'not up'}")
    except Refused as err:
        print(f"aterm-fixture-seats: {err}", file=sys.stderr)
        return err.code
    print("\n".join(lines))
    return 0


if __name__ == "__main__":
    sys.exit(main())
