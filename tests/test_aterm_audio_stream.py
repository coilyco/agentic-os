"""Tests for the emulator lane's phrase stream (COI-3591), with no emulator."""

from __future__ import annotations

import base64
import importlib.util
import json
import wave
from pathlib import Path

import pytest

CI = Path(__file__).resolve().parent.parent / "scripts" / "ci"


def _load():
    spec = importlib.util.spec_from_file_location("aterm_audio_stream", CI / "aterm_audio_stream.py")
    assert spec and spec.loader
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def _wav(path: Path, pcm: bytes, rate: int = 16000, channels: int = 1) -> None:
    with wave.open(str(path), "wb") as out:
        out.setnchannels(channels)
        out.setsampwidth(2)
        out.setframerate(rate)
        out.writeframes(pcm)


def test_the_first_message_carries_the_format_and_every_one_carries_samples() -> None:
    stream = _load()
    pcm = bytes(range(256)) * 50
    lines = stream.messages(pcm)
    assert lines[0]["format"] == {"samplingRate": 16000, "channels": "Mono", "format": "AUD_FMT_S16", "mode": "MODE_REAL_TIME"}
    assert all("format" not in line for line in lines[1:])
    assert b"".join(base64.b64decode(line["audio"]) for line in lines) == pcm


def test_a_chunk_is_a_tenth_of_a_second() -> None:
    stream = _load()
    lines = stream.messages(bytes(stream.CHUNK_BYTES * 2 + 10))
    assert [len(base64.b64decode(line["audio"])) for line in lines] == [stream.CHUNK_BYTES, stream.CHUNK_BYTES, 10]
    assert stream.CHUNK_BYTES == 16000 * 2 // 10


def test_main_writes_one_json_object_per_line(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    stream = _load()
    source, out = tmp_path / "in.wav", tmp_path / "out.jsonl"
    _wav(source, bytes(stream.CHUNK_BYTES * 3))
    monkeypatch.setattr("sys.argv", ["x", str(source), str(out)])
    assert stream.main() == 0
    parsed = [json.loads(line) for line in out.read_text().splitlines()]
    assert len(parsed) == 3
    assert "format" in parsed[0]


@pytest.mark.parametrize(("rate", "channels"), [(44100, 1), (16000, 2)])
def test_a_wav_in_another_format_is_refused_not_played_wrong(tmp_path: Path, rate: int, channels: int) -> None:
    stream = _load()
    bad = tmp_path / "bad.wav"
    _wav(bad, bytes(64), rate=rate, channels=channels)
    with pytest.raises(SystemExit, match="must be 16 kHz mono 16-bit"):
        stream.read_pcm(str(bad))
