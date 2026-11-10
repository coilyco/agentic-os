"""A test photo for the Android emulator's media library, shared by its two drivers.

`scripts/aterm_android_emulator.py` (`photo`) and `scripts/ci/aterm_android_journey.py`
both seed a picture and ask MediaStore to list it, so the picker has something to show
(COI-3573). Standard library only, because the CI job runs `python3 -I`.
"""

from __future__ import annotations

import struct
import zlib


def png(red: int = 220, size: int = 64) -> bytes:
    """A small PNG with a signature the daemon accepts, made here so no binary is tracked.

    `red` makes each picture's hash differ from the others.
    """

    def chunk(kind: bytes, body: bytes) -> bytes:
        return struct.pack(">I", len(body)) + kind + body + struct.pack(">I", zlib.crc32(kind + body) & 0xFFFFFFFF)

    rows = b"".join(b"\x00" + bytes([red, 30, 30]) * size for _ in range(size))
    return b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", size, size, 8, 2, 0, 0, 0)) + chunk(b"IDAT", zlib.compress(rows)) + chunk(b"IEND", b"")


def scan_attempts(name: str) -> tuple[tuple[str, tuple[str, ...]], ...]:
    """The device-shell commands that ask MediaStore to list /sdcard/Pictures/<name>, in order of cost."""
    return (
        ("scan-file", ("shell", "am", "broadcast", "-a", "android.intent.action.MEDIA_SCANNER_SCAN_FILE", "-d", f"file:///sdcard/Pictures/{name}")),
        ("scan-volume", ("shell", "content", "call", "--uri", "content://media/", "--method", "scan_volume", "--arg", "external_primary")),
    )
