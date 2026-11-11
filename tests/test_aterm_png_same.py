"""Two PNG files that draw the same pixels compare equal whatever their encoding (COI-3616)."""

from __future__ import annotations

import importlib.util
import struct
import zlib
from pathlib import Path

CI = Path(__file__).resolve().parent.parent / "scripts" / "ci"
ICONS = Path(__file__).resolve().parent.parent / "aterm-android/src-tauri/icons/android"


def _same_module():
    spec = importlib.util.spec_from_file_location("aterm_png_same", CI / "aterm_png_same.py")
    assert spec and spec.loader
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def _chunk(kind: bytes, body: bytes) -> bytes:
    return struct.pack(">I", len(body)) + kind + body + struct.pack(">I", zlib.crc32(kind + body))


def _png(width: int, rows: list[bytes], ctype: int, depth: int = 8, filt: int = 0, palette: bytes = b"", trans: bytes = b"") -> bytes:
    """Encode raw packed rows as a PNG, every row with one filter type (0 none, 1 sub, 2 up, 4 the nearest-neighbour predictor)."""
    channels = {0: 1, 2: 3, 3: 1, 4: 2, 6: 4}[ctype]
    step = max(1, channels * depth // 8)
    module = _same_module()
    out, prev = b"", bytes(len(rows[0]))
    for row in rows:
        enc = bytearray()
        for i, value in enumerate(row):
            left = row[i - step] if i >= step else 0
            corner = prev[i - step] if i >= step else 0
            base = {0: 0, 1: left, 2: prev[i], 4: module._predict(left, prev[i], corner)}[filt]
            enc.append((value - base) & 255)
        out += bytes([filt]) + bytes(enc)
        prev = row
    extra = (_chunk(b"PLTE", palette) if palette else b"") + (_chunk(b"tRNS", trans) if trans else b"")
    return (
        module.SIGNATURE
        + _chunk(b"IHDR", struct.pack(">IIBBBBB", width, len(rows), depth, ctype, 0, 0, 0))
        + extra
        + _chunk(b"IDAT", zlib.compress(out))
        + _chunk(b"IEND", b"")
    )


def _write(tmp_path: Path, name: str, data: bytes) -> Path:
    path = tmp_path / name
    path.write_bytes(data)
    return path


def _same(a: Path, b: Path) -> bool:
    module = _same_module()
    return module.pixels(a) == module.pixels(b)


RGBA_ROWS = [bytes([200, 10, 10, 255, 10, 200, 10, 255]), bytes([10, 10, 200, 255, 0, 0, 0, 0])]


def test_the_same_picture_in_a_palette_and_with_every_filter_compares_equal(tmp_path: Path) -> None:
    plain = _write(tmp_path, "a.png", _png(2, RGBA_ROWS, 6))
    palette = bytes([200, 10, 10, 10, 200, 10, 10, 10, 200, 7, 7, 7])
    indexed = _write(tmp_path, "b.png", _png(2, [bytes([0b00_01_0000]), bytes([0b10_11_0000])], 3, depth=2, palette=palette, trans=bytes([255, 255, 255, 0])))
    assert _same(plain, indexed)
    for filt in (1, 2, 4):
        assert _same(plain, _write(tmp_path, f"f{filt}.png", _png(2, RGBA_ROWS, 6, filt=filt))), filt


def test_a_changed_pixel_or_size_compares_different(tmp_path: Path) -> None:
    plain = _write(tmp_path, "a.png", _png(2, RGBA_ROWS, 6))
    changed = [RGBA_ROWS[0], bytes([10, 10, 201, 255, 0, 0, 0, 0])]
    assert not _same(plain, _write(tmp_path, "b.png", _png(2, changed, 6)))
    assert not _same(plain, _write(tmp_path, "c.png", _png(1, [r[:4] for r in RGBA_ROWS], 6)))


def test_the_colour_under_a_fully_clear_pixel_does_not_matter(tmp_path: Path) -> None:
    plain = _write(tmp_path, "a.png", _png(2, RGBA_ROWS, 6))
    clear = [RGBA_ROWS[0], bytes([10, 10, 200, 255, 9, 9, 9, 0])]
    assert _same(plain, _write(tmp_path, "b.png", _png(2, clear, 6)))


def test_every_launcher_icon_in_the_repo_decodes_and_equals_itself() -> None:
    module = _same_module()
    icons = sorted(ICONS.glob("mipmap-*/*.png"))
    assert icons
    for icon in icons:
        width, height, data = module.pixels(icon)
        assert len(data) == width * height * 4
        assert module.pixels(icon) == (width, height, data)


def test_the_command_line_says_equal_different_and_cannot_compare(tmp_path: Path) -> None:
    import subprocess
    import sys

    a = _write(tmp_path, "a.png", _png(2, RGBA_ROWS, 6))
    b = _write(tmp_path, "b.png", _png(2, RGBA_ROWS, 6, filt=1))
    other = _write(tmp_path, "c.png", _png(2, [RGBA_ROWS[0], bytes(8)], 6))
    junk = _write(tmp_path, "d.png", b"not a png")
    run = lambda x, y: subprocess.run([sys.executable, "-I", str(CI / "aterm_png_same.py"), str(x), str(y)], capture_output=True, check=False).returncode  # noqa: E731
    assert (run(a, b), run(a, other), run(a, junk)) == (0, 1, 2)
