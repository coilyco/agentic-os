"""Exercise the App key decoder behind actions/github-app-token."""

from __future__ import annotations

import base64
import os
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parent.parent
SCRIPT = ROOT / "actions" / "github-app-token" / "decode-key.sh"
PEM = "-----BEGIN RSA PRIVATE KEY-----\nQUJDREVGRw==\nSElKS0xNTg==\n-----END RSA PRIVATE KEY-----"


def _decode(tmp_path: Path, secret: str) -> tuple[subprocess.CompletedProcess[str], str]:
    output = tmp_path / "github_output"
    output.write_text("", encoding="utf-8")
    done = subprocess.run(
        ["bash", str(SCRIPT)],
        env=os.environ | {"APP_KEY_B64": secret, "GITHUB_OUTPUT": str(output)},
        capture_output=True,
        text=True,
        check=False,
    )
    return done, output.read_text(encoding="utf-8")


def test_the_key_is_decoded_into_a_multi_line_output(tmp_path: Path) -> None:
    done, output = _decode(tmp_path, base64.b64encode(PEM.encode()).decode())

    assert done.returncode == 0, done.stderr
    assert output == f"pem<<APP_KEY_PEM_EOF\n{PEM}\nAPP_KEY_PEM_EOF\n"


def test_every_decoded_line_is_masked_before_it_reaches_the_output(tmp_path: Path) -> None:
    done, _ = _decode(tmp_path, base64.b64encode(PEM.encode()).decode())

    for line in PEM.splitlines():
        assert f"::add-mask::{line}" in done.stdout


def test_an_empty_secret_fails_loudly(tmp_path: Path) -> None:
    done, output = _decode(tmp_path, "")

    assert done.returncode != 0
    assert "empty or not set" in done.stderr
    assert output == ""


def test_a_secret_that_is_not_a_pem_is_refused(tmp_path: Path) -> None:
    done, output = _decode(tmp_path, base64.b64encode(b"a plain token").decode())

    assert done.returncode != 0
    assert "not a PEM block" in done.stderr
    assert output == ""
