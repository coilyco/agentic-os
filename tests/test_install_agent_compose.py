"""The runner installer for agent-compose must fail closed on a bad download."""

from __future__ import annotations

import os
from pathlib import Path
import platform
import subprocess

ROOT = Path(__file__).resolve().parent.parent
SCRIPT = ROOT / "scripts" / "ci" / "install-agent-compose.sh"


def test_a_download_that_misses_its_pinned_checksum_installs_nothing(tmp_path: Path) -> None:
    served = tmp_path / "served"
    served.mkdir()
    for name in ("agent-compose-linux-amd64", "agent-compose-roster.tar.gz"):
        (served / name).write_bytes(b"not the pinned release")
    dest = tmp_path / "dest"
    done = subprocess.run(
        ["bash", str(SCRIPT)],
        env=os.environ
        | {
            "AGENT_COMPOSE_BASE": f"file://{served}",
            "AGENT_COMPOSE_DEST": str(dest),
            "GITHUB_PATH": str(tmp_path / "github_path"),
        },
        capture_output=True,
        text=True,
        check=False,
    )

    # The checksum is only reached on the runner's architecture, so the gate job
    # exercises it and any other host proves the refusal comes first.
    amd64 = platform.machine() in ("x86_64", "AMD64")
    assert done.returncode != 0
    assert ("does not match the pinned checksum" if amd64 else "linux-amd64 only") in done.stderr
    assert not (dest / "bin" / "agent-compose").exists()
