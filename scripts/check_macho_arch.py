#!/usr/bin/env python3
"""Fail when a Mach-O under a path carries an Intel slice and no arm64 slice (COI-3700).

macOS warns "Support Ending for Intel-based Apps" for an app that holds such a binary.
Headers are read directly, so the check needs no lipo or macOS runner. Why and what it
misses: .agents/skills/tooling-aterm-client/references/desktop.md.
"""
from __future__ import annotations

import argparse
import os
import struct
import sys
from collections.abc import Iterator

CPU_ARCHS = {
    0x00000007: "i386",
    0x01000007: "x86_64",
    0x0000000C: "arm",
    0x0100000C: "arm64",
    0x0200000C: "arm64_32",
}
INTEL = {"i386", "x86_64"}
THIN_LITTLE = {0xFEEDFACE, 0xFEEDFACF}  # the first four file bytes read little endian
FAT = {0xCAFEBABE: 20, 0xCAFEBABF: 32}  # magic, then bytes per arch entry
# A Java class file also starts 0xCAFEBABE, with a version where a fat count would be.
MAX_FAT_ARCHS = 16
HEADER_BYTES = 1024


def macho_archs(header: bytes, size: int) -> list[str] | None:
    """Architectures a Mach-O file holds, or None when it is not one."""
    if len(header) < 8:
        return None
    (little,) = struct.unpack("<I", header[:4])
    if little in THIN_LITTLE:
        (cpu,) = struct.unpack("<i", header[4:8])
        return [CPU_ARCHS.get(cpu & 0xFFFFFFFF, f"cpu{cpu & 0xFFFFFFFF:#x}")]
    (big,) = struct.unpack(">I", header[:4])
    entry = FAT.get(big)
    if entry is None:
        return None
    (count,) = struct.unpack(">I", header[4:8])
    if not 1 <= count <= MAX_FAT_ARCHS or 8 + count * entry > len(header):
        return None
    archs = []
    for index in range(count):
        base = 8 + index * entry
        (cpu,) = struct.unpack(">I", header[base : base + 4])
        offset = struct.unpack(">Q", header[base + 8 : base + 16])[0] if entry == 32 else struct.unpack(">I", header[base + 8 : base + 12])[0]
        if cpu not in CPU_ARCHS or offset >= size:
            return None
        archs.append(CPU_ARCHS[cpu])
    return archs


def scan(root: str) -> Iterator[tuple[str, list[str]]]:
    """Every Mach-O under root, links not followed, with its architectures."""
    for folder, _, files in os.walk(root, followlinks=False):
        for name in files:
            path = os.path.join(folder, name)
            try:
                if os.path.islink(path):
                    continue
                with open(path, "rb") as handle:
                    header = handle.read(HEADER_BYTES)
                archs = macho_archs(header, os.path.getsize(path))
            except OSError:
                continue
            if archs is not None:
                yield path, archs


def intel_only(archs: list[str]) -> bool:
    return bool(INTEL & set(archs)) and "arm64" not in archs


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("roots", nargs="+", help="a bundle or directory to scan")
    parser.add_argument("--all", action="store_true", help="list every Mach-O, not only the Intel-only ones")
    args = parser.parse_args()
    found = bad = 0
    for root in args.roots:
        for path, archs in scan(root):
            found += 1
            if intel_only(archs):
                bad += 1
                print(f"INTEL-ONLY {'+'.join(archs)} {path}")
            elif args.all:
                print(f"ok         {'+'.join(archs)} {path}")
    print(f"{found} Mach-O file(s) scanned, {bad} Intel-only", file=sys.stderr)
    return 1 if bad else 0


if __name__ == "__main__":
    sys.exit(main())
