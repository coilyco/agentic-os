"""Run the app-token key decoder the way a step does, with a generated PEM.

The mint itself is actions/create-github-app-token, which only a real run can
exercise. This covers what the composite adds: decode, mask, multi-line output.
"""
from __future__ import annotations

import base64
import os
import subprocess
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
DECODE = ROOT / "actions" / "app-token" / "decode-key.sh"
PEM = "-----BEGIN RSA PRIVATE KEY-----\nQUJDRA==\nRUZHSA==\n-----END RSA PRIVATE KEY-----\n"


def run_decode(tmp_path: Path, key: str) -> subprocess.CompletedProcess[str]:
    output = tmp_path / "github_output"
    output.write_text("", encoding="utf-8")
    env = {**os.environ, "APP_KEY_BASE64": key, "GITHUB_OUTPUT": str(output)}
    return subprocess.run(["bash", str(DECODE)], env=env, capture_output=True, text=True, check=False)


def test_decoded_pem_lands_in_the_step_output(tmp_path: Path) -> None:
    key = base64.b64encode(PEM.encode()).decode()
    result = run_decode(tmp_path, key)
    assert result.returncode == 0, result.stderr
    written = (tmp_path / "github_output").read_text(encoding="utf-8")
    header, _, rest = written.partition("\n")
    delimiter = header.removeprefix("pem<<")
    assert header.startswith("pem<<pem_")
    assert rest == PEM + delimiter + "\n"


def test_every_pem_line_is_masked_before_it_is_written(tmp_path: Path) -> None:
    result = run_decode(tmp_path, base64.b64encode(PEM.encode()).decode())
    masks = [line for line in result.stdout.splitlines() if line.startswith("::add-mask::")]
    assert masks == [f"::add-mask::{line}" for line in PEM.splitlines()]


def test_a_non_pem_value_fails_without_writing(tmp_path: Path) -> None:
    result = run_decode(tmp_path, base64.b64encode(b"not a key").decode())
    assert result.returncode == 1
    assert "does not decode to a PEM private key" in result.stderr
    assert (tmp_path / "github_output").read_text(encoding="utf-8") == ""
    assert "::add-mask::" not in result.stdout
