#!/usr/bin/env python3
"""The daemon half of the Android emulator journey: a stub seat and the host-side assertions.

The emulated app sends an image to a real `aterm daemon` on the runner. This
script starts the stub seat that daemon serves, and afterwards reads what the
daemon did: the file it wrote beside its socket and the path the app typed into
the seat. `upload` plays the client, so the same assertion has a positive
control that needs no emulator. Design: tooling-aterm-client references/android.md.
Standard library only.
"""
from __future__ import annotations

import argparse
import base64
import hashlib
import json
import socket
import sys
import time
from pathlib import Path

ROLE = "frontend-eng"


def talk(sock_path: str):
    """Return (send, recv) over the daemon's unix socket, one JSON object per line."""
    sock = socket.socket(socket.AF_UNIX)
    sock.settimeout(10)
    sock.connect(sock_path)
    reader = sock.makefile("rb")

    def recv() -> dict:
        line = reader.readline()
        if not line:
            raise SystemExit("the daemon closed the socket")
        return json.loads(line)

    def send(frame: dict, reply: bool = True) -> dict:
        """Send a frame and return the reply that echoes its id, skipping pushed frames."""
        frame = {**frame, "id": frame.get("id", "1")}
        sock.sendall(json.dumps(frame).encode() + b"\n")
        if not reply:
            return {}
        while True:
            reply = recv()
            if reply.get("id") == frame["id"]:
                return reply

    sock.sendall(json.dumps({"type": "hello", "format": "aterm.daemon.v1", "version": "ci"}).encode() + b"\n")
    welcome = recv()
    if welcome.get("type") != "welcome" or welcome.get("error"):
        raise SystemExit(f"expected a welcome frame, got {welcome.get('type')!r}")
    return send, welcome


def start(args: argparse.Namespace) -> int:
    """Spawn a seat that appends each line typed into it to args.log."""
    send, _ = talk(args.socket)
    script = f'while IFS= read -r line; do printf "%s\\n" "$line" >> "{args.log}"; done'
    reply = send({"type": "spawn", "session": args.seat, "role": ROLE, "argv": ["/bin/sh", "-c", script], "rows": 24, "cols": 80})
    if reply.get("type") != "spawned":
        raise SystemExit(f"spawn refused: {reply}")
    print(f"seat {reply.get('session')} running as pid {reply.get('pid')}")
    return 0


def upload(args: argparse.Namespace) -> int:
    """Act as a client: attach to the seat, send the image, and type the returned path in."""
    send, welcome = talk(args.socket)
    if "image-upload" not in (welcome.get("features") or []):
        raise SystemExit("this daemon does not offer image-upload")
    attached = send({"type": "attach", "session": args.seat, "rows": 24, "cols": 80})
    if attached.get("type") == "error":
        raise SystemExit(f"attach refused: {attached}")
    data = base64.b64encode(Path(args.png).read_bytes()).decode()
    reply = send({"type": "image_upload", "session": args.seat, "data": data})
    print(json.dumps({k: v for k, v in reply.items() if k != "data"}))
    if reply.get("type") != "image_uploaded":
        return 1
    send({"type": "input", "session": args.seat, "data": base64.b64encode((reply["path"] + "\n").encode()).decode()}, reply=False)
    return 0


def landed(socket_dir: Path, seat: str, digest: str) -> Path | None:
    """The file the daemon wrote for this seat whose bytes hash to digest, if any."""
    for path in sorted(socket_dir.glob(f"images/{seat}-*/*")):
        if hashlib.sha256(path.read_bytes()).hexdigest() == digest:
            return path
    return None


def expect_image(args: argparse.Namespace) -> int:
    """Wait for the image to land and its path to reach the seat. Exit 1 names what is missing."""
    digest = hashlib.sha256(Path(args.png).read_bytes()).hexdigest()
    socket_dir = Path(args.socket).parent
    log = Path(args.log)
    deadline = time.monotonic() + args.timeout
    file = None
    typed = ""
    while time.monotonic() < deadline:
        file = file or landed(socket_dir, args.seat, digest)
        typed = log.read_text(errors="replace") if log.exists() else ""
        if file and str(file) in typed:
            print(f"ok: {file} hashes to {digest[:12]} and its path reached the seat")
            return 0
        time.sleep(0.5)
    print(f"missing: file landed={bool(file)}, path typed into the seat={bool(file) and str(file) in typed}", file=sys.stderr)
    return 1


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="verb", required=True)
    for name, fn, extra in (("start", start, ["log"]), ("upload", upload, ["png"]), ("expect-image", expect_image, ["png", "log"])):
        verb = sub.add_parser(name)
        verb.add_argument("socket")
        verb.add_argument("seat")
        for arg in extra:
            verb.add_argument(arg)
        if name == "expect-image":
            verb.add_argument("--timeout", type=float, default=30)
        verb.set_defaults(fn=fn)
    args = parser.parse_args()
    return args.fn(args)


if __name__ == "__main__":
    sys.exit(main())
