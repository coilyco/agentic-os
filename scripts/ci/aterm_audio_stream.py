#!/usr/bin/env python3
"""Turn a 16 kHz mono 16-bit WAV into a stream for `injectAudio` (COI-3591).

One JSON object per line for grpcurl: the first carries the format, each holds a chunk.
The emulator's gRPC `EmulatorController/injectAudio` feeds its virtual microphone.
Standard library only.
"""
from __future__ import annotations

import base64
import json
import sys
import wave

# 100 ms of 16 kHz 16-bit mono.
CHUNK_BYTES = 3200
RATE = 16000


def messages(pcm: bytes) -> list[dict]:
    out: list[dict] = []
    for start in range(0, len(pcm), CHUNK_BYTES):
        message: dict = {"audio": base64.b64encode(pcm[start : start + CHUNK_BYTES]).decode()}
        if start == 0:
            message["format"] = {"samplingRate": RATE, "channels": "Mono", "format": "AUD_FMT_S16", "mode": "MODE_REAL_TIME"}
        out.append(message)
    return out


def read_pcm(path: str) -> bytes:
    with wave.open(path) as wav:
        if (wav.getframerate(), wav.getnchannels(), wav.getsampwidth()) != (RATE, 1, 2):
            raise SystemExit(f"{path} must be 16 kHz mono 16-bit, got {wav.getframerate()} Hz, {wav.getnchannels()} ch, {wav.getsampwidth() * 8} bit")
        return wav.readframes(wav.getnframes())


def main() -> int:
    if len(sys.argv) != 3:
        raise SystemExit("usage: aterm_audio_stream.py IN.wav OUT.jsonl")
    lines = messages(read_pcm(sys.argv[1]))
    with open(sys.argv[2], "w") as out:
        out.write("".join(json.dumps(line) + "\n" for line in lines))
    print(f"{len(lines)} chunks of {CHUNK_BYTES // 32} ms")
    return 0


if __name__ == "__main__":
    sys.exit(main())
