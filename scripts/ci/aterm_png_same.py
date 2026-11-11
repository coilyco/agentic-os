"""Exit 0 when two PNG files draw the same pixels, whatever their encoding (COI-3616).

A release build re-encodes the launcher icons (aapt2 crunches them), so the APK's bytes
differ from the repo's while the picture is the same. Standard library only, because the
CI steps run `python3 -I` with no packages. Interlaced files are refused, aapt2 has none.
"""

from __future__ import annotations

import struct
import sys
import zlib
from pathlib import Path

SIGNATURE = b"\x89PNG\r\n\x1a\n"


def _chunks(data: bytes):
    if not data.startswith(SIGNATURE):
        raise ValueError("not a PNG")
    at = len(SIGNATURE)
    while at + 8 <= len(data):
        size, kind = struct.unpack(">I4s", data[at : at + 8])
        yield kind, data[at + 8 : at + 8 + size]
        at += 12 + size


def _predict(left: int, up: int, corner: int) -> int:
    guess = left + up - corner
    nearest = min((abs(guess - v), i) for i, v in enumerate((left, up, corner)))
    return (left, up, corner)[nearest[1]]


def _unfilter(raw: bytes, height: int, row_bytes: int, step: int) -> list[bytearray]:
    rows: list[bytearray] = []
    at = 0
    for _ in range(height):
        kind, row = raw[at], bytearray(raw[at + 1 : at + 1 + row_bytes])
        at += 1 + row_bytes
        prev = rows[-1] if rows else bytearray(row_bytes)
        for i in range(row_bytes):
            left = row[i - step] if i >= step else 0
            corner = prev[i - step] if i >= step else 0
            if kind == 1:
                row[i] = (row[i] + left) & 255
            elif kind == 2:
                row[i] = (row[i] + prev[i]) & 255
            elif kind == 3:
                row[i] = (row[i] + (left + prev[i]) // 2) & 255
            elif kind == 4:
                row[i] = (row[i] + _predict(left, prev[i], corner)) & 255
            elif kind != 0:
                raise ValueError(f"unknown filter {kind}")
        rows.append(row)
    return rows


def pixels(path: Path) -> tuple[int, int, bytes]:
    """Width, height and the RGBA bytes of a PNG, with the colour of a fully clear pixel zeroed."""
    header = palette = trans = b""
    idat = b""
    for kind, body in _chunks(path.read_bytes()):
        if kind == b"IHDR":
            header = body
        elif kind == b"PLTE":
            palette = body
        elif kind == b"tRNS":
            trans = body
        elif kind == b"IDAT":
            idat += body
    width, height, depth, ctype, _comp, _filt, interlace = struct.unpack(">IIBBBBB", header)
    if interlace:
        raise ValueError("interlaced PNG")
    channels = {0: 1, 2: 3, 3: 1, 4: 2, 6: 4}[ctype]
    bits = channels * depth
    row_bytes = (width * bits + 7) // 8
    rows = _unfilter(zlib.decompress(idat), height, row_bytes, max(1, bits // 8))
    out = bytearray()
    for row in rows:
        for x in range(width):
            if depth == 8:
                px = list(row[x * channels : (x + 1) * channels])
            elif depth == 16:
                px = [row[(x * channels + c) * 2] for c in range(channels)]
            else:
                shift = 8 - depth - (x * depth) % 8
                value = (row[(x * depth) // 8] >> shift) & ((1 << depth) - 1)
                px = [value]
            if ctype == 3:
                i = px[0]
                red, green, blue = palette[3 * i : 3 * i + 3]
                alpha = trans[i] if i < len(trans) else 255
            elif ctype in (0, 4):
                scale = 255 // ((1 << min(depth, 8)) - 1)
                gray = px[0] * scale
                red = green = blue = gray
                alpha = px[1] if ctype == 4 else 255
            else:
                red, green, blue = px[0], px[1], px[2]
                alpha = px[3] if ctype == 6 else 255
            out += bytes((red, green, blue, alpha)) if alpha else b"\x00\x00\x00\x00"
    return width, height, bytes(out)


def main() -> int:
    if len(sys.argv) != 3:
        print("usage: aterm_png_same.py A.png B.png", file=sys.stderr)
        return 2
    try:
        same = pixels(Path(sys.argv[1])) == pixels(Path(sys.argv[2]))
    except (ValueError, KeyError, struct.error, zlib.error) as err:
        print(f"cannot compare: {err}", file=sys.stderr)
        return 2
    return 0 if same else 1


if __name__ == "__main__":
    sys.exit(main())
