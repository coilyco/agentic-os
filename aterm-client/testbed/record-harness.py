"""Records a real harness session in a pty, for the recorded-diff journey (COI-3552).

RECSTEPS='[["Try", "prompt\\r", 30]]' python3 record-harness.py OUT DIR ROWS COLS -- claude

A step is [regex, text to send, seconds to wait]. See the skill's testbed.md.
"""

import fcntl
import json
import os
import pty
import re
import select
import struct
import sys
import termios
import time

out, cwd, rows, cols = sys.argv[1], sys.argv[2], int(sys.argv[3]), int(sys.argv[4])
argv = sys.argv[sys.argv.index("--") + 1 :]
steps = json.loads(os.environ["RECSTEPS"])
ANSI = re.compile(
    rb"\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(\x07|\x1b\\)|\x1b[()][A-Z0-9]|\x1b."
)

pid, fd = pty.fork()
if pid == 0:
    os.chdir(cwd)
    os.execvpe(
        argv[0], argv, dict(os.environ, TERM="xterm-256color", COLORTERM="truecolor")
    )
fcntl.ioctl(fd, termios.TIOCSWINSZ, struct.pack("HHHH", rows, cols, 0, 0))
log = open(out, "wb")  # noqa: SIM115 - closed after the exit keys, before the trim
seen = b""


def pump(seconds: float) -> bool:
    global seen
    end = time.time() + seconds
    while time.time() < end:
        if select.select([fd], [], [], 0.2)[0]:
            try:
                data = os.read(fd, 65536)
            except OSError:
                return False
            if not data:
                return False
            log.write(data)
            log.flush()
            seen += data
    return True


def plain(data: bytes) -> str:
    return ANSI.sub(b"", data).decode("utf8", "replace")


for pattern, send, timeout in steps:
    mark, deadline, matched = len(seen), time.time() + timeout, False
    while time.time() < deadline and not matched:
        alive = pump(1)
        matched = (
            re.search(pattern, plain(seen[mark:]), re.DOTALL | re.IGNORECASE)
            is not None
        )
        if not alive:
            break
    print(f"step {pattern!r}: {'matched' if matched else 'TIMEOUT'}", flush=True)
    if not matched:
        print(plain(seen)[-1500:])
        break
    pump(1.5)
    raw = send.encode().decode("unicode_escape").encode("latin1")
    if raw.endswith(b"\r") and len(raw) > 1:
        os.write(fd, raw[:-1])
        pump(1.0)
        os.write(fd, b"\r")
    elif raw:
        os.write(fd, raw)
pump(4)
log.flush()
keep = os.path.getsize(out)
for _ in range(2):
    try:
        os.write(fd, b"\x03")
    except OSError:
        break
    pump(1)
try:
    os.kill(pid, 9)
except OSError:
    pass
log.close()
os.truncate(out, keep)
