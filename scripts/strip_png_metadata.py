#!/usr/bin/env python3
"""
Strip metadata-like PNG chunks from a file (in-place).

We keep only the chunks required for decoding:
  - IHDR, PLTE (if present), tRNS (if present), IDAT, IEND

This removes common metadata carriers like:
  - eXIf, iTXt/tEXt/zTXt (XMP/text), tIME, iCCP, pHYs, etc.
"""

from __future__ import annotations

import os
import struct
import sys
from pathlib import Path

PNG_SIG = b"\x89PNG\r\n\x1a\n"
KEEP = {b"IHDR", b"PLTE", b"tRNS", b"IDAT", b"IEND"}


def strip_png_metadata(path: Path) -> None:
    data = path.read_bytes()
    if len(data) < 8 or data[:8] != PNG_SIG:
        raise ValueError(f"not a PNG file: {path}")

    out = bytearray()
    out += PNG_SIG

    i = 8
    while i + 8 <= len(data):
        length = struct.unpack(">I", data[i : i + 4])[0]
        ctype = data[i + 4 : i + 8]
        start = i
        i += 8
        end = i + length + 4  # + CRC
        if end > len(data):
            raise ValueError(f"truncated PNG chunk {ctype!r} in {path}")
        chunk_bytes = data[start:end]
        i = end

        if ctype in KEEP:
            out += chunk_bytes

        if ctype == b"IEND":
            break

    tmp = path.with_suffix(path.suffix + ".tmp")
    mode = path.stat().st_mode
    tmp.write_bytes(out)
    os.chmod(tmp, mode)
    os.replace(tmp, path)


def main(argv: list[str]) -> int:
    if len(argv) != 2:
        print(f"Usage: {Path(argv[0]).name} <file.png>", file=sys.stderr)
        return 2

    p = Path(argv[1])
    strip_png_metadata(p)
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv))

