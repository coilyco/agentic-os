#!/usr/bin/env python3
"""Wait until no seat would lose anything to an aterm daemon restart (COI-3615). Stdlib only.

A restart keeps every seat alive, so what it can cost is a question a seat is waiting
on (the daemon holds the ask) and a draft Kai is typing. `pending` is not a block: on a
busy or idle seat it is messages in flight, and it stayed above 0 for minutes on a live
fleet. Each blocker prints with its state and age so whoever owns the seat is told, and
the guard exits 0 after several clear reads in a row. Usage: docs/aterm-daemon.md.
"""

from __future__ import annotations

import argparse
import json
import subprocess
import sys
import time
from collections.abc import Callable
from typing import Any

PROMPT = "prompt"


def blockers(sessions: list[dict[str, Any]]) -> list[str]:
    """One line per seat a restart would hurt: its name, state, how long, and what is waiting."""
    lines = []
    for seat in sessions:
        drafting = bool(seat.get("kai_drafting"))
        if seat.get("state") != PROMPT and not drafting:
            continue
        age = int(seat.get("state_seconds") or 0)
        note = " kai_drafting" if drafting else ""
        lines.append(f"{seat.get('name', '?')} state={seat.get('state', '?')} for {age}s pending={seat.get('pending', 0)}{note}")
    return lines


def read_sessions(aterm: str = "aterm") -> list[dict[str, Any]]:
    out = subprocess.run([aterm, "agents", "--json"], capture_output=True, text=True, check=False)
    if out.returncode != 0:
        raise RuntimeError(f"{aterm} agents failed with exit {out.returncode}: {out.stderr.strip()[:200]}")
    return json.loads(out.stdout).get("sessions", [])


def wait_until_clear(
    read: Callable[[], list[dict[str, Any]]],
    reads: int,
    interval: float,
    timeout: float,
    say: Callable[[str], None] = print,
    sleep: Callable[[float], None] = time.sleep,
    now: Callable[[], float] = time.monotonic,
) -> int:
    """0 once `reads` clear reads run in a row, 1 when the timeout passes with a blocker."""
    deadline = now() + timeout
    clear = 0
    last: list[str] = []
    while True:
        found = blockers(read())
        if found:
            clear = 0
            last = found
            say("blocking: " + "; ".join(found))
        else:
            clear += 1
            if clear >= reads:
                say(f"clear: {reads} reads in a row with no seat at a prompt or a draft")
                return 0
        if now() >= deadline:
            say("timeout, still blocking: " + ("; ".join(last) or "none seen"))
            return 1
        sleep(interval)


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=(__doc__ or "").splitlines()[0])
    parser.add_argument("--reads", type=int, default=3, help="clear reads in a row (default 3)")
    parser.add_argument("--interval", type=float, default=10.0, help="seconds between reads (default 10)")
    parser.add_argument("--timeout", type=float, default=900.0, help="seconds before giving up (default 900)")
    parser.add_argument("--aterm", default="aterm", help="the aterm binary")
    args = parser.parse_args(argv)
    try:
        return wait_until_clear(lambda: read_sessions(args.aterm), args.reads, args.interval, args.timeout)
    except (RuntimeError, ValueError) as err:
        print(f"error: {err}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    sys.exit(main())
